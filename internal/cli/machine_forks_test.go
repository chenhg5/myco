package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/harness"
	"github.com/myco-ai/myco/internal/harness/machinenode"
	"github.com/myco-ai/myco/internal/testutil"
)

func forkFixtureRepo(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	parent := t.TempDir()
	repo := filepath.Join(parent, "source")
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %v %s", err, out)
		}
	}
	run("init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("-c", "user.name=Example", "-c", "user.email=example@example.com", "commit", "-qm", "fixture")
	real, err := canonicalForkDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(real), real
}
func TestManagedWorktreePreservesDirtyAndRejectsReplacement(t *testing.T) {
	parent, repo := forkFixtureRepo(t)
	ctx := context.Background()
	m := &forkManifest{ID: "fork_demo", Name: "copy", SourceDir: repo, Workdir: filepath.Join(parent, "copy"), Repository: repo, Branch: "fork/copy", Worktree: true}
	if _, err := forkGit(ctx, repo, "worktree", "add", "-b", m.Branch, "--", m.Workdir, "HEAD"); err != nil {
		t.Fatal(err)
	}
	if err := verifyForkTree(ctx, m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.Workdir, "unsaved"), []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := forkGit(ctx, repo, "worktree", "remove", "--", m.Workdir); err == nil {
		t.Fatal("uncommitted work discarded")
	}
	if b, err := os.ReadFile(filepath.Join(m.Workdir, "unsaved")); err != nil || string(b) != "keep me" {
		t.Fatal("dirty work was lost")
	}
	if _, err := forkGit(ctx, m.Workdir, "checkout", "-b", "another-branch"); err != nil {
		t.Fatal(err)
	}
	if err := verifyForkTree(ctx, m); err == nil {
		t.Fatal("changed branch accepted for deletion")
	}
	m.Workdir = repo
	if err := verifyForkTree(ctx, m); err == nil {
		t.Fatal("source accepted as disposable worktree")
	}
}
func TestManagedWorktreeDoesNotRunCheckoutHooks(t *testing.T) {
	testutil.NeedsPOSIXShell(t)
	parent, repo := forkFixtureRepo(t)
	marker := filepath.Join(repo, "hook-ran")
	script := "#!/bin/sh\nprintf bad > '" + marker + "'\n"
	if err := os.WriteFile(filepath.Join(repo, ".git", "hooks", "post-checkout"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := forkGit(context.Background(), repo, "worktree", "add", "-b", "fork/hooks", "--", filepath.Join(parent, "copy"), "HEAD"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("provisioning ran repository code")
	}
}
func TestManagedWorktreeCannotEscapeAllowedSpawnRoot(t *testing.T) {
	parent, repo := forkFixtureRepo(t)
	ctx := context.Background()
	f := api.AgentFork{ID: "fork_restricted", Name: "demo-copy", ForkOf: "demo-source", SourceID: "source-id", CLI: "codex", SourceSession: "source-session", SourceDir: repo, State: "planned", Worktree: true, Force: true, ForkedBy: "coordinator", Access: "workspace"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(f)
			return
		}
		w.Write([]byte("{}"))
	}))
	defer srv.Close()
	e, _ := startupEnv(t, ctx)
	source := &config.Config{Hub: srv.URL, Agent: f.ForkOf, Harness: "codex", SessionID: f.SourceSession, Token: "fixture", Runner: config.Runner{Harness: "codex", Workdir: repo}}
	if err := config.Save(profilePathFor(e.defaultPath, srv.URL, f.ForkOf), source); err != nil {
		t.Fatal(err)
	}
	c, _ := client.New(srv.URL, "node-fixture")
	node := &machinenode.Node{Prefix: "demo-", Policy: machinenode.Policy{Home: parent, Dirs: []string{repo}, MaxAccess: harness.AccessWorkspace}}
	_, err := e.managedMachineFork(ctx, node, c, srv.URL, runnerShared{}, harness.Request{From: "coordinator"}, f.ID, false)
	if err == nil || !strings.Contains(err.Error(), "outside allowed spawn.dirs") {
		t.Fatalf("worktree escaped: %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, "source-fork-"+f.ID)); !os.IsNotExist(err) {
		t.Fatal("created a forbidden sibling")
	}
	for _, id := range []string{"../source", ".", "fork/other", "fork\\other", ""} {
		if _, err := e.forkManifestPath(id); err == nil {
			t.Fatalf("unsafe manifest ID accepted: %q", id)
		}
	}
}
