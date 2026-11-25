package cli

import (
	"path/filepath"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/config"
)

// A Codex session that joins gets workspace access (Codex's own default
// sandbox has no network, so myco would fail in it); a level chosen before
// stays, and Claude Code sessions keep their CLI's default.
func TestJoinedCodexSessionGetsWorkspaceAccess(t *testing.T) {
	dir := t.TempDir()
	e := &env{defaultPath: filepath.Join(dir, "config.yaml")}
	if err := e.saveIdentity("http://hub", "cx", "tok", api.Binding{Harness: "codex", SessionID: "s1", Workdir: dir}); err != nil {
		t.Fatal(err)
	}
	if e.cfg.Runner.Access != "workspace" {
		t.Fatalf("codex access = %q, want workspace", e.cfg.Runner.Access)
	}

	path := profilePathFor(e.defaultPath, "http://hub", "cx")
	cfg, _ := config.Load(path)
	cfg.Runner.Access = "full"
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	e = &env{defaultPath: filepath.Join(dir, "config.yaml")}
	if err := e.saveIdentity("http://hub", "cx", "tok", api.Binding{Harness: "codex", SessionID: "s1", Workdir: dir}); err != nil {
		t.Fatal(err)
	}
	if e.cfg.Runner.Access != "full" {
		t.Fatalf("a chosen level was replaced: %q", e.cfg.Runner.Access)
	}

	e = &env{defaultPath: filepath.Join(dir, "config.yaml")}
	if err := e.saveIdentity("http://hub", "cc", "tok", api.Binding{Harness: "claude", SessionID: "s2", Workdir: dir}); err != nil {
		t.Fatal(err)
	}
	if e.cfg.Runner.Access != "" {
		t.Fatalf("claude access = %q, want the CLI default", e.cfg.Runner.Access)
	}
}
