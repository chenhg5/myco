package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/config"
)

func concurrencyHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg := filepath.Join(home, ".myco", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if config.DefaultPath() != cfg {
		t.Fatalf("default path %s is not in the test's home", config.DefaultPath())
	}
	return cfg
}

// The runner follows machine.yaml's setting while it runs: raising it starts
// waiting turns, lowering it never stops running ones.
func TestSlotLimitFollowsTheSettingLive(t *testing.T) {
	cfg := concurrencyHome(t)
	if err := os.WriteFile(config.MachinePath(cfg), []byte("hub: http://x\nmachine: m1\nenroll_token: t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l := newSlotLimit(cfg, 1, nil) // started with --max-concurrent 1
	if v := l.View(); v.Limit != 1 || v.Source != config.ConcurrencyFromFlag {
		t.Fatalf("start: %+v", v)
	}
	a := l.pool.Request("http://x", "a", "m1")
	b := l.pool.Request("http://x", "a", "m2")
	<-a.Ready

	// Raised from the App (machine.yaml, chosen after the flag).
	if err := config.SetRunnerConcurrency(cfg, 4, time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if !l.apply() {
		t.Fatal("raise not applied")
	}
	select {
	case <-b.Ready:
	case <-time.After(time.Second):
		t.Fatal("a waiting turn did not start after raising")
	}
	if v := l.View(); v.Limit != 4 || v.Source != config.ConcurrencyFromSetting || v.InUse != 2 {
		t.Fatalf("raised: %+v", v)
	}

	// Lowered through the machine node: both keep running, a new one waits.
	// (On a coarse clock the setting gets the very time the flag was seen:
	// set after start, it still wins.)
	l.now = func() time.Time { return l.flagAt }
	v, err := l.Set(1)
	if err != nil || v.Limit != 1 || v.InUse != 2 {
		t.Fatalf("lowered: %+v %v", v, err)
	}
	c := l.pool.Request("http://x", "a", "m3")
	a.Done()
	select {
	case <-c.Ready:
		t.Fatal("started above the lowered limit")
	default:
	}
	b.Done()
	<-c.Ready
	c.Done()

	if v := l.View(); v.Source != config.ConcurrencyFromSetting || v.InUse != 0 {
		t.Fatalf("after the lowered tasks ended: %+v", v)
	}

	// What the App on this computer reads, once written (Run writes it on
	// every change; here it is written directly).
	l.now = time.Now
	l.save()
	st, err := config.LoadRunnerSlots(cfg)
	if err != nil || st == nil || st.Limit != 1 || st.Source != config.ConcurrencyFromSetting || st.Flag != 1 || st.InUse != 0 || !st.Fresh(time.Now()) {
		t.Fatalf("runner-slots.json: %+v %v", st, err)
	}

	// A restart with the same flag keeps when it was chosen: the later
	// setting still wins. A new flag value wins over it.
	if l2 := newSlotLimit(cfg, 1, nil); l2.View().Limit != 1 || l2.View().Source != config.ConcurrencyFromSetting {
		t.Fatalf("restart, same flag: %+v", l2.View())
	}
	if l3 := newSlotLimit(cfg, 6, nil); l3.View().Limit != 6 || l3.View().Source != config.ConcurrencyFromFlag {
		t.Fatalf("restart, new flag: %+v", l3.View())
	}
	if l4 := newSlotLimit(cfg, 0, nil); l4.View().Limit != 1 || l4.View().Source != config.ConcurrencyFromSetting {
		t.Fatalf("no flag: %+v", l4.View())
	}
}

// Run writes runner-slots.json at once and stops with its context.
func TestSlotLimitRunWritesState(t *testing.T) {
	cfg := concurrencyHome(t)
	l := newSlotLimit(cfg, 0, nil)
	if v := l.View(); v.Limit != config.DefaultMaxConcurrent || v.Source != config.ConcurrencyFromDefault {
		t.Fatalf("default: %+v", v)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { l.Run(ctx); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if st, _ := config.LoadRunnerSlots(cfg); st != nil && st.Limit == config.DefaultMaxConcurrent {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("runner-slots.json not written")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
}

func TestMachineConcurrencyCommand(t *testing.T) {
	cfg := concurrencyHome(t)
	h := &harnessT{t, map[string]string{}}
	if code, _, e := h.run("machine", "concurrency", "--set", "12"); code != ExitUsage || !strings.Contains(e, "not_configured") {
		t.Fatalf("never joined: %d %s", code, e)
	}
	if err := os.WriteFile(config.MachinePath(cfg), []byte("hub: http://x\nmachine: m1\nenroll_token: t\nfuture_key: kept\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"0", "65"} {
		if code, _, e := h.run("machine", "concurrency", "--set", n); code != ExitUsage || !strings.Contains(e, "invalid_argument") {
			t.Fatalf("--set %s: %d %s", n, code, e)
		}
	}
	if code, _, _ := h.run("machine", "concurrency", "--set", "12", "--dry-run"); code != ExitDryRun {
		t.Fatalf("dry run: %d", code)
	}
	if p, _ := config.LoadRunnerPolicy(cfg); p.MaxConcurrent != 0 {
		t.Fatalf("dry run wrote %+v", p)
	}
	var out struct {
		Setting int             `json:"setting"`
		Default int             `json:"default"`
		Changed bool            `json:"changed"`
		Runner  json.RawMessage `json:"runner"`
	}
	code, o, e := h.run("machine", "concurrency", "--set", "12")
	if code != 0 || json.Unmarshal([]byte(o), &out) != nil || out.Setting != 12 || !out.Changed || string(out.Runner) != "null" {
		t.Fatalf("set: %d %s %s", code, o, e)
	}
	if p, _ := config.LoadRunnerPolicy(cfg); p.MaxConcurrent != 12 || p.SetAt.IsZero() {
		t.Fatalf("not written: %+v", p)
	}
	if b, _ := os.ReadFile(config.MachinePath(cfg)); !strings.Contains(string(b), "future_key: kept") {
		t.Fatalf("other keys lost: %s", b)
	}
	// With a runner running here, it says what the runner does.
	if err := config.SaveRunnerSlots(cfg, &config.RunnerSlotsState{PID: 1, Limit: 12, Source: config.ConcurrencyFromSetting, InUse: 3, Waiting: 2, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	var shown struct {
		Runner struct {
			Limit   int `json:"limit"`
			InUse   int `json:"in_use"`
			Waiting int `json:"waiting"`
		} `json:"runner"`
	}
	if code, o, _ = h.run("machine", "concurrency"); code != 0 || json.Unmarshal([]byte(o), &shown) != nil || shown.Runner.Limit != 12 || shown.Runner.InUse != 3 || shown.Runner.Waiting != 2 {
		t.Fatalf("show: %d %s", code, o)
	}
	if _, o, _ = h.run("schema", "--all"); !strings.Contains(o, "concurrency") {
		t.Fatal("schema lacks machine concurrency")
	}
}
