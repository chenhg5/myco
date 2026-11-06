package execnode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

func run(t *testing.T, spec harness.ExecSpec, from, body string, env ...string) api.ExecResult {
	t.Helper()
	h, err := New(harness.Options{Exec: &spec})
	if err != nil {
		t.Fatal(err)
	}
	out, err := h.Run(context.Background(), harness.Request{Body: body, From: from, Type: "task", Workdir: t.TempDir(), Env: env})
	if err != nil {
		t.Fatal(err)
	}
	var res api.ExecResult
	if err := json.Unmarshal([]byte(out.Output), &res); err != nil {
		t.Fatalf("output is not an ExecResult: %q", out.Output)
	}
	return res
}

func TestExecRunsAndReportsExitCode(t *testing.T) {
	spec := harness.ExecSpec{Senders: []string{"boss"}}
	res := run(t, spec, "boss", `echo out; echo err >&2; exit 3`)
	if res.ExitCode != 3 || res.Stdout != "out\n" || res.Stderr != "err\n" || res.Denied {
		t.Fatalf("%+v", res)
	}
	if res := run(t, spec, "boss", "pwd"); res.ExitCode != 0 || !strings.Contains(res.Stdout, os.TempDir()) && res.Stdout == "" {
		t.Fatalf("pwd: %+v", res)
	}
}

func TestExecSendersAreRechecked(t *testing.T) {
	if res := run(t, harness.ExecSpec{Senders: []string{"boss"}}, "mallory", "id"); !res.Denied || res.ExitCode != ExitDenied {
		t.Fatalf("unknown sender must be refused: %+v", res)
	}
	if res := run(t, harness.ExecSpec{}, "boss", "id"); !res.Denied {
		t.Fatalf("no senders configured must refuse everything: %+v", res)
	}
}

func TestExecAllowlist(t *testing.T) {
	spec := harness.ExecSpec{Senders: []string{"boss"}, Allow: []string{"echo hello", "uname *"}}
	cases := map[string]bool{
		"echo hello":            true,
		"echo  hello":           true, // whitespace is normalized
		"echo hello world":      false,
		"uname":                 true,
		"uname -a":              true,
		"unamex":                false,
		"uname -a; id":          false,
		"uname -a && id":        false,
		"uname $(id)":           false,
		"uname `id`":            false,
		"uname -a > /tmp/x":     false,
		"uname -a | sh":         false,
		"uname -a\nid":          false,
		"cat /etc/passwd":       false,
		"uname -a \\\n; id":     false,
		"uname -a # comment ok": true,
	}
	for cmd, ok := range cases {
		res := run(t, spec, "boss", cmd)
		if ok == res.Denied {
			t.Errorf("%q: want allowed=%v, got %+v", cmd, ok, res)
		}
	}
	if _, err := New(harness.Options{Exec: &harness.ExecSpec{Allow: []string{"*"}}}); err == nil {
		t.Error(`allow "*" must be rejected`)
	}
}

func TestExecEnvironmentIsClean(t *testing.T) {
	t.Setenv("MYCO_TOKEN", "secret-from-runner-env")
	t.Setenv("SOME_RUNNER_SECRET", "x")
	res := run(t, harness.ExecSpec{Senders: []string{"boss"}}, "boss", "env",
		"MYCO_TOKEN=secret-passed-in", "MYCO_CONFIG=/x", "MYCO_FROM=boss", "FROM_ENV_FILE=1")
	if strings.Contains(res.Stdout, "secret") || strings.Contains(res.Stdout, "SOME_RUNNER_SECRET") || strings.Contains(res.Stdout, "MYCO_CONFIG") {
		t.Fatalf("credentials leaked into the command env:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "MYCO_FROM=boss") || !strings.Contains(res.Stdout, "FROM_ENV_FILE=1") || !strings.Contains(res.Stdout, "PATH=") {
		t.Fatalf("expected context/env_file/PATH variables:\n%s", res.Stdout)
	}
}

func TestExecTimeoutTruncationAndAudit(t *testing.T) {
	audit := filepath.Join(t.TempDir(), "logs", "node.exec.jsonl")
	spec := harness.ExecSpec{Senders: []string{"boss"}, Timeout: "200ms", MaxOutput: 10, AuditLog: audit}
	if res := run(t, spec, "boss", "sleep 5"); !res.TimedOut || res.ExitCode != ExitTimedOut {
		t.Fatalf("timeout: %+v", res)
	}
	if res := run(t, spec, "boss", "printf 0123456789abcdef"); !res.Truncated || res.Stdout != "0123456789" {
		t.Fatalf("truncation: %+v", res)
	}
	run(t, spec, "mallory", "id")
	b, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 3 || !strings.Contains(lines[0], `"command":"sleep 5"`) || !strings.Contains(lines[2], `"from":"mallory"`) || !strings.Contains(lines[2], `"denied":true`) {
		t.Fatalf("audit log:\n%s", b)
	}
	if st, _ := os.Stat(audit); st.Mode().Perm() != 0o600 {
		t.Fatalf("audit log mode %v", st.Mode().Perm())
	}
}

func TestExecAnySenderWhenExplicit(t *testing.T) {
	if res := run(t, harness.ExecSpec{Senders: []string{"*"}}, "anyone-at-all", "echo ok"); res.Denied || res.Stdout != "ok\n" {
		t.Fatalf("explicit * must admit every sender: %+v", res)
	}
}
