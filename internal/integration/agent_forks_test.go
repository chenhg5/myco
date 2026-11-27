package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/cli"
	"github.com/myco-ai/myco/internal/testutil"
)

// The model is a tiny CLI fixture; the Hub, SQLite, commands, machine node,
// profiles, fork markers, runner processes and git worktrees are real.
const forkCodexFixture = `#!/bin/sh
for arg do
 if [ "$arg" = "--version" ]; then echo 'codex 0.0.0-test'; exit 0; fi
 if [ "$arg" = "app-server" ]; then exit 0; fi
done
last=''; want=''; mode='started'; source=''; root=''
for arg do
 if [ "$want" = last ]; then last="$arg"; want=''; continue; fi
 if [ "$want" = source ]; then source="$arg"; want=''; continue; fi
 if [ "$want" = root ]; then root="$arg"; want=''; continue; fi
 case "$arg" in
  --output-last-message) want=last ;;
  --cd) want=root ;;
  fork) mode=forked; want=source ;;
  resume) mode=resumed; want=source ;;
 esac
done
cat >/dev/null
if [ "$mode" = forked ] && [ "$root" != "$(pwd)" ]; then echo 'missing explicit fork working root' >&2; exit 2; fi
text="$mode $source in $(pwd)"
[ -z "$last" ] || printf '%s' "$text" > "$last"
printf '{"type":"thread.started","thread_id":"fixture-session-%s"}\n' "$$"
printf '{"type":"item.completed","item":{"type":"agent_message","text":"%s"}}\n' "$text"
`

func TestAgentForkRetireWorktreeEndToEnd(t *testing.T) {
	testutil.NeedsPOSIXShell(t)
	home := isolatedHome(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(forkCodexFixture), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture git: %v %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("demo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "README.md")
	git("-c", "user.name=Example", "-c", "user.email=example@example.com", "commit", "-qm", "fixture")
	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	lead := newCLI(t, h, filepath.Join(home, "lead.yaml"))
	lead.must(c, "agent", "register", "--name", "lead")
	delete(lead.env, "MYCO_ADMIN_TOKEN")
	other := newCLI(t, h, filepath.Join(home, "other.yaml"))
	other.must(c, "agent", "register", "--name", "other")
	delete(other.env, "MYCO_ADMIN_TOKEN")
	link, err := h.admin().CreateEnrollment(c, api.CreateEnrollmentRequest{Kind: "link", AllowFrom: []string{"lead"}, MaxUses: 1, TTLSeconds: 3600}, false)
	if err != nil {
		t.Fatal(err)
	}
	box := newCLI(t, h, filepath.Join(home, ".myco", "config.yaml"))
	delete(box.env, "MYCO_ADMIN_TOKEN")
	delete(box.env, "MYCO_HUB")
	box.env["PATH"] = os.Getenv("PATH")
	box.must(c, "machine", "join", "--link", link.JoinURL, "--machine", "box", "--no-service")
	rctx, stop := context.WithCancel(c)
	done := make(chan struct{})
	go func() { defer close(done); box.run(rctx, "runner", "start", "--all", "--log-level", "error") }()
	t.Cleanup(func() { stop(); <-done })
	raw := lead.must(c, "agent", "spawn", "--on", "box", "--dir", repo, "--cli", "codex", "--name", "source", "--timeout", "15s")
	var spawned struct{ Result api.SpawnResult }
	if err = json.Unmarshal([]byte(raw), &spawned); err != nil {
		t.Fatal(err)
	}
	var source *api.Agent
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		source, err = h.admin().GetAgent(c, spawned.Result.Agent)
		node, e := h.admin().GetAgent(c, "box-machine")
		if err == nil && e == nil && source.MachineID != "" && slices.Contains(node.Runtime.Features, app.ForkFeature) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if source == nil || source.MachineID == "" {
		t.Fatal("source did not report its machine")
	}
	originalSession := source.Binding.SessionID
	// Only resource checks are bypassed: the real machine feature and ACL stay required.
	if code, _, _ := other.run(c, "agent", "fork", "--from", source.Name, "--force", "--worktree", "--timeout", "5s"); code != cli.ExitPermission {
		t.Fatalf("unlisted sender fork exit=%d", code)
	}
	dryCode, _, dryErr := lead.run(c, "agent", "fork", "--from", source.Name, "--force", "--worktree", "--dry-run")
	if dryCode != cli.ExitDryRun {
		t.Fatalf("dry run: %d %s", dryCode, dryErr)
	}
	raw = lead.must(c, "agent", "fork", "--from", source.Name, "--count", "2", "--force", "--worktree", "--idempotency-key", "e2e-batch", "--timeout", "20s")
	var batch struct {
		Forks []*api.AgentFork `json:"forks"`
	}
	if err = json.Unmarshal([]byte(raw), &batch); err != nil || len(batch.Forks) != 2 {
		t.Fatalf("fork output: %v %s", err, raw)
	}
	if batch.Forks[0].Workdir == repo || batch.Forks[0].Workdir == batch.Forks[1].Workdir {
		t.Fatal("copies share a checkout")
	}
	for _, f := range batch.Forks {
		if _, err = os.Stat(filepath.Join(f.Workdir, "README.md")); err != nil {
			t.Fatal(err)
		}
		reply := lead.must(c, "message", "send", "--to", f.Name, "--body", "report context", "--wait", "10s")
		if !strings.Contains(reply, "forked "+originalSession+" in "+f.Workdir) {
			t.Fatalf("copy did not fork in its own worktree: %s", reply)
		}
		if code, _, _ := other.run(c, "agent", "retire", "--name", f.Name, "--now", "--timeout", "5s"); code != cli.ExitPermission {
			t.Fatalf("unrelated retire exit=%d", code)
		}
		if code, _, _ := lead.run(c, "agent", "fork", "--from", f.Name, "--force", "--timeout", "5s"); code != cli.ExitPermission {
			t.Fatalf("copy used as fork source exit=%d", code)
		}
	}
	sourceAfter, err := h.admin().GetAgent(c, source.Name)
	if err != nil || sourceAfter.Binding.SessionID != originalSession {
		t.Fatal("fork changed original binding")
	}
	// An editable label cannot make an ordinary agent disposable.
	h.admin().SetLabels(c, "other", api.LabelsRequest{Add: []string{"fork_of=" + source.Name, "forked_by=lead"}})
	if code, _, _ := lead.run(c, "agent", "retire", "--name", "other", "--now"); code != cli.ExitPermission {
		t.Fatalf("forged labels permit retirement: %d", code)
	}
	// Failed cleanup preserves both the worktree and Hub identity. After the
	// user resolves changes, an explicit retry dispatches a new cleanup job.
	dirty := filepath.Join(batch.Forks[0].Workdir, "keep.txt")
	if err = os.WriteFile(dirty, []byte("uncommitted work\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, _, retireErr := lead.run(c, "agent", "retire", "--name", batch.Forks[0].Name, "--timeout", "10s")
	if code == 0 || !strings.Contains(retireErr, "retire_failed") {
		t.Fatalf("dirty cleanup did not fail clearly: %d %s", code, retireErr)
	}
	if _, err = os.Stat(dirty); err != nil {
		t.Fatal("dirty work was removed", err)
	}
	if _, err = h.admin().GetAgent(c, batch.Forks[0].Name); err != nil {
		t.Fatal("failed cleanup deleted identity", err)
	}
	if err = os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	lead.must(c, "agent", "retire", "--name", batch.Forks[0].Name, "--timeout", "20s")
	lead.must(c, "agent", "retire", "--fork-of", source.Name, "--timeout", "20s")
	for _, f := range batch.Forks {
		if _, err = os.Stat(f.Workdir); !os.IsNotExist(err) {
			t.Fatalf("worktree remains: %s %v", f.Workdir, err)
		}
		if _, err = h.admin().GetAgent(c, f.Name); err == nil {
			t.Fatal("copy remains on Hub")
		}
		if !strings.Contains(git("branch", "--list", f.Branch), f.Branch) {
			t.Fatal("commits/branch were discarded")
		}
	}
	if _, err = h.admin().GetAgent(c, source.Name); err != nil {
		t.Fatal("source was retired")
	}
	if got := git("status", "--porcelain"); got != "" {
		t.Fatalf("source modified: %s", got)
	}
}
