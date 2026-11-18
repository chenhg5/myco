package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-q")
	for name, body := range map[string]string{"a.txt": "one\ntwo\n", "b.txt": "b\n", "c.txt": "c\n"} {
		os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
	}
	run("add", ".")
	run("commit", "-q", "-m", "start")
	return dir
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Only what the run changed counts: edits made before it (uncommitted, or
// untracked files already there) are part of the record it is compared with.
func TestChangesOfARun(t *testing.T) {
	dir := gitRepo(t)
	ctx := context.Background()
	write(t, dir, "b.txt", "b\nbefore the run\n") // uncommitted before the run
	write(t, dir, "notes.txt", "mine\n")          // untracked before the run

	snap := snapshotChanges(ctx, dir)
	if snap == nil {
		t.Fatal("no snapshot of a git work tree")
	}
	// The run:
	write(t, dir, "a.txt", "one\nTWO\nthree\n")
	write(t, dir, "b.txt", "b\nbefore the run\nby the run\n")
	os.Remove(filepath.Join(dir, "c.txt"))
	write(t, dir, "new.txt", "hello\nworld\n")

	got := map[string]struct {
		st, diff string
		a, r     int
	}{}
	for _, ev := range snap.changes(ctx) {
		if ev.Kind != "change" {
			t.Fatalf("kind: %+v", ev)
		}
		got[ev.Tool] = struct {
			st, diff string
			a, r     int
		}{ev.Input, ev.Output, ev.Added, ev.Removed}
	}
	if len(got) != 4 {
		t.Fatalf("changed files: %v", got)
	}
	if a := got["a.txt"]; a.st != "M" || a.a != 2 || a.r != 1 || !strings.Contains(a.diff, "+TWO") {
		t.Fatalf("a.txt: %+v", a)
	}
	// Only the run's own line, not the one there before it.
	if b := got["b.txt"]; b.st != "M" || b.a != 1 || b.r != 0 || strings.Contains(b.diff, "+before the run") || !strings.Contains(b.diff, "+by the run") {
		t.Fatalf("b.txt: %+v", b)
	}
	if c := got["c.txt"]; c.st != "D" || c.r != 1 {
		t.Fatalf("c.txt: %+v", c)
	}
	if n := got["new.txt"]; n.st != "A" || n.a != 2 || !strings.Contains(n.diff, "+world") {
		t.Fatalf("new.txt: %+v", n)
	}
	if _, ok := got["notes.txt"]; ok {
		t.Fatal("an untracked file from before the run is not the run's")
	}
	// Nothing was written to the work tree, the index or the refs.
	if out, _ := exec.Command("git", "-C", dir, "stash", "list").Output(); len(out) != 0 {
		t.Fatalf("stash list: %s", out)
	}
}

func TestChangesNotAGitDir(t *testing.T) {
	if snapshotChanges(context.Background(), t.TempDir()) != nil {
		t.Fatal("a plain folder has no snapshot")
	}
	var none *changeSnapshot
	if none.changes(context.Background()) != nil {
		t.Fatal("no snapshot: no changes")
	}
}

func TestChangesCutAndCapped(t *testing.T) {
	dir := gitRepo(t)
	ctx := context.Background()
	snap := snapshotChanges(ctx, dir)
	write(t, dir, "a.txt", strings.Repeat("a long line of text that keeps going\n", 2000))
	for i := 0; i < changesMaxFiles+3; i++ {
		write(t, dir, "f"+strings.Repeat("x", i%5)+string(rune('a'+i%26))+string(rune('a'+i/26))+".txt", "x\n")
	}
	evs := snap.changes(ctx)
	if len(evs) != changesMaxFiles+1 || evs[len(evs)-1].Text != "+4" || evs[len(evs)-1].Tool != "" {
		t.Fatalf("capped list: %d steps, last %+v", len(evs), evs[len(evs)-1])
	}
	for _, ev := range evs {
		if len(ev.Output) > changesMaxDiff+4 {
			t.Fatalf("%s diff is %d bytes", ev.Tool, len(ev.Output))
		}
	}
}

// A task run in a git folder ends its trace with what it changed, before
// the reply.
func TestRunTraceEndsWithChanges(t *testing.T) {
	dir := gitRepo(t)
	hub := &traceHub{fakeHub: newFakeHub(api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th"})}
	h := fakeHarness{fn: func(r harness.Request) (harness.Result, error) {
		r.OnEvent(harness.Event{Kind: "text", Text: "editing"})
		os.WriteFile(filepath.Join(r.Workdir, "a.txt"), []byte("one\nchanged\n"), 0o644)
		return harness.Result{Output: "done", SessionID: "s"}, nil
	}}
	New(hub, h, NewHubSessions(&fakeBinder{}, ""), nil, Options{Once: true, Concurrency: 1, Workdir: dir}, quiet()).Run(context.Background())
	if len(hub.events) != 2 || hub.atReply != 2 {
		t.Fatalf("steps: %d (at reply %d)", len(hub.events), hub.atReply)
	}
	var ev harness.Event
	json.Unmarshal(hub.events[1].Data, &ev)
	if hub.events[1].Kind != "change" || ev.Tool != "a.txt" || ev.Input != "M" || ev.Added != 1 || ev.Removed != 1 {
		t.Fatalf("last step: %s %+v", hub.events[1].Kind, ev)
	}
}
