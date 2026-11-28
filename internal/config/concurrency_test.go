package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEffectiveConcurrency(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		flag   int
		flagAt time.Time
		set    RunnerPolicy
		want   int
		src    string
	}{
		{"nothing", 0, time.Time{}, RunnerPolicy{}, DefaultMaxConcurrent, ConcurrencyFromDefault},
		{"flag only", 3, t0, RunnerPolicy{}, 3, ConcurrencyFromFlag},
		{"setting only", 0, time.Time{}, RunnerPolicy{MaxConcurrent: 12, SetAt: t0}, 12, ConcurrencyFromSetting},
		{"setting changed after the flag", 3, t0, RunnerPolicy{MaxConcurrent: 12, SetAt: t0.Add(time.Minute)}, 12, ConcurrencyFromSetting},
		{"flag chosen after the setting", 8, t0.Add(time.Hour), RunnerPolicy{MaxConcurrent: 12, SetAt: t0}, 8, ConcurrencyFromFlag},
		{"set in the same clock tick as the flag", 3, t0, RunnerPolicy{MaxConcurrent: 12, SetAt: t0}, 12, ConcurrencyFromSetting},
		{"setting above the ceiling", 0, time.Time{}, RunnerPolicy{MaxConcurrent: 500, SetAt: t0}, MaxConcurrentCeiling, ConcurrencyFromSetting},
	}
	for _, c := range cases {
		n, src := EffectiveConcurrency(c.flag, c.flagAt, c.set)
		if n != c.want || src != c.src {
			t.Errorf("%s: got %d (%s), want %d (%s)", c.name, n, src, c.want, c.src)
		}
	}
}

func TestSetRunnerConcurrencyKeepsTheRest(t *testing.T) {
	dir := t.TempDir()
	def := filepath.Join(dir, "config.yaml")
	if err := SetRunnerConcurrency(def, 5, time.Now()); err != ErrNotJoined {
		t.Fatalf("not joined: %v", err)
	}
	orig := "hub: https://hub.example\nmachine: wsl\nspawn:\n  max_access: full\nfuture_key: kept\n"
	if err := os.WriteFile(MachinePath(def), []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 2, 3, 4, 5, 0, time.UTC)
	if err := SetRunnerConcurrency(def, 12, at); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{0, MaxConcurrentCeiling + 1} {
		if err := SetRunnerConcurrency(def, n, at); err == nil {
			t.Fatalf("%d accepted", n)
		}
	}
	b, _ := os.ReadFile(MachinePath(def))
	if !strings.Contains(string(b), "future_key: kept") || !strings.Contains(string(b), "max_access: full") {
		t.Fatalf("other keys lost:\n%s", b)
	}
	p, err := LoadRunnerPolicy(def)
	if err != nil || p.MaxConcurrent != 12 || !p.SetAt.Equal(at) {
		t.Fatalf("policy %+v, %v", p, err)
	}
	// Rejoining (SaveMachine) keeps it.
	m, _ := LoadMachine(def)
	if err := SaveMachine(def, m); err != nil {
		t.Fatal(err)
	}
	if p, _ := LoadRunnerPolicy(def); p.MaxConcurrent != 12 {
		t.Fatalf("lost on save: %+v", p)
	}
}

func TestFlagSinceAndSlotsState(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	prev := &RunnerSlotsState{Flag: 3, FlagAt: t0}
	if got := FlagSince(prev, 3, t0.Add(time.Hour)); !got.Equal(t0) {
		t.Fatalf("same flag after a restart: %v", got)
	}
	if got := FlagSince(prev, 8, t0.Add(time.Hour)); !got.Equal(t0.Add(time.Hour)) {
		t.Fatalf("new flag: %v", got)
	}
	if got := FlagSince(prev, 0, t0); !got.IsZero() {
		t.Fatalf("no flag: %v", got)
	}
	def := filepath.Join(t.TempDir(), "config.yaml")
	if s, err := LoadRunnerSlots(def); s != nil || err != nil {
		t.Fatalf("none yet: %v %v", s, err)
	}
	now := time.Now()
	if err := SaveRunnerSlots(def, &RunnerSlotsState{PID: 1, Limit: 4, Source: ConcurrencyFromSetting, At: now}); err != nil {
		t.Fatal(err)
	}
	s, err := LoadRunnerSlots(def)
	if err != nil || s.Limit != 4 || !s.Fresh(now) || s.Fresh(now.Add(2*RunnerSlotsFresh)) {
		t.Fatalf("%+v %v", s, err)
	}
	var none *RunnerSlotsState
	if none.Fresh(now) {
		t.Fatal("nil is fresh")
	}
}
