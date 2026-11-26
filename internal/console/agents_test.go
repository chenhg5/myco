package console_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/console"
)

// fakeAgents makes a home with the given agent CLIs installed in ~/bin and a
// Local that finds them there and runs them through run.
func fakeAgents(t *testing.T, installed []string, run func(name string, args ...string) ([]byte, error)) (*console.Local, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake CLIs are shell-style executables")
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	os.MkdirAll(bin, 0o755)
	for _, n := range installed {
		os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\n"), 0o755)
	}
	return &console.Local{Home: home, Dirs: []string{bin}, BundledAgentPaths: func(string) []string { return nil },
		LookPath: func(string) (string, error) { return "", errors.New("not on PATH") },
		Run: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			return run(filepath.Base(name), args...)
		}}, home
}

func byCLI(as []console.LocalAgent) map[string]console.LocalAgent {
	m := map[string]console.LocalAgent{}
	for _, a := range as {
		m[a.CLI] = a
	}
	return m
}

func TestLocalAgents(t *testing.T) {
	codexSignedIn := false
	l, home := fakeAgents(t, []string{"claude", "codex", "opencode"}, func(name string, args ...string) ([]byte, error) {
		switch {
		case len(args) == 1 && args[0] == "--version":
			return []byte(name + " 1.2.3\n"), nil
		case name == "codex" && len(args) == 2 && args[0] == "login":
			if codexSignedIn {
				return []byte("Logged in using ChatGPT\n"), nil
			}
			return []byte("Not logged in\n"), errors.New("exit status 1")
		case name == "security":
			return nil, errors.New("exit status 44") // no keychain item
		}
		t.Errorf("unexpected run: %s %v", name, args)
		return nil, errors.New("unexpected")
	})

	got := byCLI(l.LocalAgents(context.Background()))
	if c := got["claude"]; c.State != "signed_out" || c.Version != "claude 1.2.3" || c.Path == "" || c.Login != "claude" {
		t.Fatalf("claude: %+v", c)
	}
	if c := got["codex"]; c.State != "signed_out" || c.Install != npmCmd("@openai/codex") || c.Needs != "npm" ||
		c.InstallMirror != npmCmd("@openai/codex")+" --registry=https://registry.npmmirror.com" {
		t.Fatalf("codex: %+v", c)
	}
	if g := got["gemini"]; g.State != "missing" || g.Path != "" {
		t.Fatalf("gemini: %+v", g)
	}
	if o := got["opencode"]; o.State != "unknown" { // installed, sign-in not visible: never "signed_out"
		t.Fatalf("opencode: %+v", o)
	}

	// Signed in: Claude Code's account in .claude.json, codex by its status command.
	os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"x"}}`), 0o600)
	codexSignedIn = true
	got = byCLI(l.LocalAgents(context.Background()))
	if got["claude"].State != "ready" || got["codex"].State != "ready" {
		t.Fatalf("signed in: %+v %+v", got["claude"], got["codex"])
	}

	// A provider in Claude Code's settings works without a sign-in; so does
	// a provider in codex's config.
	os.Remove(filepath.Join(home, ".claude.json"))
	os.MkdirAll(filepath.Join(home, ".claude"), 0o700)
	os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(`{"env":{"ANTHROPIC_AUTH_TOKEN":"k"}}`), 0o600)
	codexSignedIn = false
	os.MkdirAll(filepath.Join(home, ".codex"), 0o700)
	os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("model_provider = \"x\"\n"), 0o600)
	got = byCLI(l.LocalAgents(context.Background()))
	if got["claude"].State != "ready" || got["codex"].State != "unknown" {
		t.Fatalf("providers: %+v %+v", got["claude"], got["codex"])
	}
}

func TestAgentTerminal(t *testing.T) {
	l, _ := fakeAgents(t, nil, func(string, ...string) ([]byte, error) { return nil, errors.New("not installed") })
	var opened []string
	l.OpenTerminal = func(command string, env []string) error {
		opened = append(opened, command)
		if command == "codex login" {
			return errors.New("no terminal")
		}
		return nil
	}
	app := httptest.NewServer((&console.Backend{Local: l}).Handler())
	defer app.Close()
	post := func(path string, body any) (int, map[string]any) {
		b, _ := json.Marshal(body)
		resp, err := http.Post(app.URL+path, "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		return resp.StatusCode, m
	}
	if code, m := post("/app/local/agents/codex/terminal", map[string]string{"action": "install_mirror"}); code != 200 || m["opened"] != true ||
		m["command"] != npmCmd("@openai/codex")+" --registry=https://registry.npmmirror.com" {
		t.Fatalf("install: %d %v", code, m)
	}
	// No terminal could be opened: the app gets the command to show.
	if code, m := post("/app/local/agents/codex/terminal", map[string]string{"action": "login"}); code != 200 || m["opened"] != false || m["command"] != "codex login" {
		t.Fatalf("login: %d %v", code, m)
	}
	if code, _ := post("/app/local/agents/codex/terminal", map[string]string{"action": "rm -rf /"}); code != 400 {
		t.Fatalf("unknown action: %d", code)
	}
	if code, _ := post("/app/local/agents/nope/terminal", map[string]string{"action": "install"}); code != 404 {
		t.Fatalf("unknown cli: %d", code)
	}
	if len(opened) != 2 {
		t.Fatalf("opened: %v", opened)
	}
	resp, err := http.Get(app.URL + "/app/local/agents")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list struct{ Agents []console.LocalAgent }
	json.NewDecoder(resp.Body).Decode(&list)
	if len(list.Agents) < 4 || list.Agents[0].CLI != "claude" || list.Agents[0].State != "missing" {
		t.Fatalf("list: %+v", list.Agents)
	}
}

// npmCmd is how the App installs an npm package: into ~/.local off Windows
// (npm's global folder may belong to another user of the computer).
func npmCmd(pkg string) string {
	if runtime.GOOS == "windows" {
		return "npm install -g " + pkg
	}
	return `npm install -g --prefix "$HOME/.local" ` + pkg
}

func TestLocalAgentOpaqueAuthUsesRecentSuccess(t *testing.T) {
	t.Setenv("QODER_CONFIG_DIR", "")
	t.Setenv("QODER_PERSONAL_ACCESS_TOKEN", "")
	l, home := fakeAgents(t, []string{"qodercli"}, func(_ string, args ...string) ([]byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("1.0.0"), nil
		}
		t.Fatal("readiness must not invoke a model or sign-in command")
		return nil, errors.New("unexpected command")
	})
	dir := filepath.Join(home, ".myco")
	if err := os.MkdirAll(filepath.Join(dir, "agents"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "state"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agents", "demo.yaml"), []byte("agent: demo\nrunner:\n  harness: auto\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := byCLI(l.LocalAgents(context.Background()))["qoder"].State; got != "unknown" {
		t.Fatalf("no proof: %s", got)
	}
	b, _ := json.Marshal(map[string]any{"success": map[string]any{"cli": "qoder", "at": time.Now().Add(-time.Minute)}})
	if err := os.WriteFile(filepath.Join(dir, "state", "demo.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if got := byCLI(l.LocalAgents(context.Background()))["qoder"].State; got != "ready" {
		t.Fatalf("success proof: %s", got)
	}
	if err := os.Remove(filepath.Join(home, "bin", "qodercli")); err != nil {
		t.Fatal(err)
	}
	if got := byCLI(l.LocalAgents(context.Background()))["qoder"].State; got != "missing" {
		t.Fatalf("success cannot resurrect a missing CLI: %s", got)
	}
}

func TestWorkBuddyBundleFallback(t *testing.T) {
	l, home := fakeAgents(t, nil, func(name string, args ...string) ([]byte, error) { return []byte("2.106.4"), nil })
	bundled := filepath.Join(home, "WorkBuddy bundle", "codebuddy")
	if err := os.MkdirAll(filepath.Dir(bundled), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundled, []byte("#!/usr/bin/env node\n"), 0755); err != nil {
		t.Fatal(err)
	}
	l.BundledAgentPaths = func(binary string) []string {
		if binary != "codebuddy" {
			t.Fatalf("unexpected bundle lookup %s", binary)
		}
		return []string{bundled}
	}
	got := byCLI(l.LocalAgents(context.Background()))["codebuddy"]
	if got.Path != bundled || got.State == "missing" || got.Version != "2.106.4" {
		t.Fatalf("bundle not discovered: %+v", got)
	}
	// Explicit PATH entries still take priority over the desktop bundle.
	l.LookPath = func(binary string) (string, error) {
		if binary == "codebuddy" {
			return filepath.Join(home, "path-cli"), nil
		}
		return "", errors.New("missing")
	}
	if got := byCLI(l.LocalAgents(context.Background()))["codebuddy"]; got.Path == bundled {
		t.Fatal("bundle shadowed PATH")
	}
}
