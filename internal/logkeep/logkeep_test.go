package logkeep

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Yesterday's log is copied aside under its day and the file emptied; a
// writer that holds it open for appending goes on at its new start.
func TestKeepsADayAndEmpties(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hub.log")
	w, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	w.WriteString("yesterday\n")
	yesterday := time.Date(2026, 11, 25, 23, 0, 0, 0, time.Local)
	os.Chtimes(path, yesterday, yesterday)
	now := time.Date(2026, 11, 26, 9, 0, 0, 0, time.Local)
	if err := Once(path, Policy{}, now); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path + ".20261125"); string(b) != "yesterday\n" {
		t.Fatalf("copy: %q", b)
	}
	w.WriteString("today\n")
	if b, _ := os.ReadFile(path); string(b) != "today\n" {
		t.Fatalf("the writer should go on at the start of the emptied file: %q", b)
	}
	// Today's, small: left as it is.
	os.Chtimes(path, now, now)
	if err := Once(path, Policy{}, now); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "today\n" {
		t.Fatalf("today's log is kept in place: %q", b)
	}
}

// A file bigger than allowed is copied aside at once, a second time on the
// same day under its time.
func TestBigFileTheSameDay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runner.log")
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.Local)
	for i := 0; i < 2; i++ {
		os.WriteFile(path, []byte(strings.Repeat("x", 100)), 0o600)
		at := now.Add(time.Duration(i) * time.Minute)
		os.Chtimes(path, at, at)
		if err := Once(path, Policy{MaxBytes: 50}, at); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := filepath.Glob(path + ".*")
	if len(got) != 2 {
		t.Fatalf("two copies the same day: %v", got)
	}
	if st, _ := os.Stat(path); st.Size() != 0 {
		t.Fatalf("emptied: %d", st.Size())
	}
}

// Copies older than the days kept go; other files stay.
func TestPrunesOldCopies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hub.log")
	os.WriteFile(path, nil, 0o600)
	for _, n := range []string{"hub.log.20261101", "hub.log.20261111-101010", "hub.log.20261113", "hub.log.20261125", "hub.log.bak", "other.log.20200101"} {
		os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600)
	}
	now := time.Date(2026, 11, 26, 9, 0, 0, 0, time.Local)
	if err := Once(path, Policy{Days: 14}, now); err != nil {
		t.Fatal(err)
	}
	var left []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		left = append(left, e.Name())
	}
	want := "hub.log hub.log.20261113 hub.log.20261125 hub.log.bak other.log.20200101"
	if strings.Join(left, " ") != want {
		t.Fatalf("left %q, want %q", strings.Join(left, " "), want)
	}
}

// Only the file this process's stderr writes to is ever trimmed.
func TestIsStderrOnlyForItsOwnFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hub.log")
	os.WriteFile(path, nil, 0o600)
	if IsStderr(path) {
		t.Fatal("a test's stderr is not this file")
	}
}
