package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/config"
)

func TestInboxWaitAutomaticTimeoutsAndHint(t *testing.T) {
	hub := newHub(t)
	dir := t.TempDir()
	alicePath := filepath.Join(dir, "alice.yaml")
	bobPath := filepath.Join(dir, "bob.yaml")
	alice := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": alicePath}}
	bob := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": bobPath}}
	if code, _, e := alice.run("agent", "register", "--name", "alice", "--harness", "claude", "--session-id", "sess-alice"); code != 0 {
		t.Fatalf("register alice: %s", e)
	}
	if code, _, e := bob.run("agent", "register", "--name", "bob"); code != 0 {
		t.Fatalf("register bob: %s", e)
	}
	if code, _, e := bob.run("message", "send", "--to", "alice", "--body", "work"); code != 0 {
		t.Fatalf("send: %s", e)
	}
	cfg, err := config.Load(alicePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, harness, env, flag, want string }{
		{"claude", "claude", "", "", "55m"},
		{"claude-cloud", "claude-cloud", "", "", "55m"},
		{"codex", "codex", "", "", "10m"},
		{"cursor", "cursor", "", "", "10m"},
		{"inbox", "inbox", "", "", "10m"},
		{"shell", "shell", "", "", "10m"},
		{"unknown", "mystery", "", "", "10m"},
		{"environment", "claude", "31s", "", "31s"},
		{"flag wins", "claude", "31s", "42s", "42s"},
		{"hub binding fallback", "", "", "", "55m"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg.Harness = tc.harness
			if err := config.Save(alicePath, cfg); err != nil {
				t.Fatal(err)
			}
			alice.env["MYCO_WAIT_TIMEOUT"] = tc.env
			args := []string{"inbox", "wait", "--format", "json"}
			if tc.flag != "" {
				args = append(args, "--timeout", tc.flag)
			}
			code, out, stderr := alice.run(args...)
			if code != 0 {
				t.Fatalf("wait: %d %s %s", code, out, stderr)
			}
			var result struct{ Timeout, Hint string }
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(err)
			}
			if result.Timeout != tc.want || !strings.HasSuffix(result.Hint, waitAgainHint) || strings.Contains(stderr, "waiting up to") {
				t.Fatalf("timeout/hint: %+v stderr=%q", result, stderr)
			}
		})
	}
	delete(alice.env, "MYCO_WAIT_TIMEOUT")
	cfg.Harness = "claude"
	if err := config.Save(alicePath, cfg); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := alice.run("inbox", "wait", "--format", "text")
	if code != 0 || !strings.Contains(stderr, "waiting up to 55m (Claude Code)") || !strings.HasSuffix(strings.TrimSpace(out), waitAgainHint) {
		t.Fatalf("text wait: %d out=%q stderr=%q", code, out, stderr)
	}
	if code, out, stderr := alice.run("agent", "get", "--format", "text"); code != 0 || strings.Contains(out+stderr, waitAgainHint) {
		t.Fatalf("other command got wait hint: %d %q %q", code, out, stderr)
	}
	if code, out, stderr := alice.run("schema", "inbox", "wait"); code != 0 || !strings.Contains(out, "MYCO_WAIT_TIMEOUT") || strings.Contains(stderr, waitAgainHint) {
		t.Fatalf("wait schema: %d %s %s", code, out, stderr)
	}
}

func TestInboxWaitTimeoutAndInterruptRemindOnlyWait(t *testing.T) {
	hub := newHub(t)
	cli := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(t.TempDir(), "agent.yaml")}}
	if code, _, e := cli.run("agent", "register", "--name", "idle"); code != 0 {
		t.Fatalf("register: %s", e)
	}
	code, _, stderr := cli.run("inbox", "wait", "--timeout", "1ms", "--format", "json")
	if code != ExitTimeout {
		t.Fatalf("timeout exit: %d %s", code, stderr)
	}
	var failure Error
	if err := json.Unmarshal([]byte(stderr), &failure); err != nil || !strings.HasSuffix(failure.Suggestion, waitAgainHint) {
		t.Fatalf("timeout error: %+v %v", failure, err)
	}
	code, _, stderr = cli.run("inbox", "wait", "--timeout", "1ms", "--format", "text")
	if code != ExitTimeout || !strings.HasSuffix(strings.TrimSpace(stderr), waitAgainHint) {
		t.Fatalf("timeout text: %d %q", code, stderr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var out, errb bytes.Buffer
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	code = Run(ctx, []string{"inbox", "wait", "--timeout", "10s", "--format", "json"}, strings.NewReader(""), &out, &errb,
		func(k string) string { return cli.env[k] }, false)
	if code != ExitTimeout {
		t.Fatalf("interrupted exit: %d %q", code, errb.String())
	}
	if err := json.Unmarshal(errb.Bytes(), &failure); err != nil || !strings.HasSuffix(failure.Suggestion, waitAgainHint) {
		t.Fatalf("interrupted error: %+v %v", failure, err)
	}
}

func TestInboxWaitDetectsHarnessWithoutConfig(t *testing.T) {
	e := &env{cfg: &config.Config{}, in: &cmdkit.Invocation{Getenv: func(k string) string {
		if k == "CLAUDE_CODE_SESSION_ID" {
			return "session-1"
		}
		return ""
	}}}
	d, raw, label, err := inboxWaitTimeout(e, nil)
	if err != nil || d != 55*time.Minute || raw != "55m" || label != "Claude Code" {
		t.Fatalf("detected timeout: %v %q %q %v", d, raw, label, err)
	}
}
