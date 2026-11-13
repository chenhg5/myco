package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/skill"
)

func TestEmbeddedSkill(t *testing.T) {
	if !strings.HasPrefix(skill.Myco, "---\nname: myco\n") || !strings.Contains(skill.Myco, "myco message ask") {
		t.Fatal("the skill is not embedded, or lost its front matter")
	}
	got := installedSkill()
	if !strings.HasPrefix(got, "---\nname: myco\n") || !strings.Contains(got, "---\n"+skillMarker+"\n") {
		t.Fatalf("the marker must follow the front matter:\n%.300s", got)
	}
}

func TestSkillTargetsAndInstall(t *testing.T) {
	home := t.TempDir()
	env := map[string]string{"HOME": home}
	getenv := func(k string) string { return env[k] }
	if got := skillTargets(getenv); len(got) != 0 {
		t.Fatalf("no CLI folders, no targets: %v", got)
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	codex := t.TempDir()
	env["CODEX_HOME"] = codex
	targets := skillTargets(getenv)
	want := []string{filepath.Join(home, ".claude", "skills", "myco", "SKILL.md"), filepath.Join(codex, "skills", "myco", "SKILL.md")}
	if strings.Join(targets, "|") != strings.Join(want, "|") {
		t.Fatalf("targets: %v", targets)
	}

	// --refresh creates nothing.
	if w, _, err := installSkill(targets, true); err != nil || len(w) != 0 {
		t.Fatalf("refresh without copies: %v %v", w, err)
	}
	// A file someone wrote is left alone.
	own := targets[1]
	_ = os.MkdirAll(filepath.Dir(own), 0o755)
	_ = os.WriteFile(own, []byte("my own notes"), 0o644)
	w, skipped, err := installSkill(targets, false)
	if err != nil || len(w) != 1 || w[0] != targets[0] || len(skipped) != 1 || skipped[0] != own {
		t.Fatalf("install: %v %v %v", w, skipped, err)
	}
	if b, _ := os.ReadFile(own); string(b) != "my own notes" {
		t.Fatal("overwrote someone's own file")
	}
	// Current: nothing to write. An older copy myco wrote: refreshed.
	if w, _, _ := installSkill(targets[:1], true); len(w) != 0 {
		t.Fatalf("already current: %v", w)
	}
	_ = os.WriteFile(targets[0], []byte(skillMarker+"\nold version"), 0o644)
	if w, _, _ := installSkill(targets[:1], true); len(w) != 1 {
		t.Fatalf("refresh: %v", w)
	}
	if b, _ := os.ReadFile(targets[0]); string(b) != installedSkill() {
		t.Fatal("not refreshed to this version")
	}
}
