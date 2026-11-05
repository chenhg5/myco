package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/hubserver"
)

type harnessT struct {
	t   *testing.T
	env map[string]string
}

func (h *harnessT) run(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := Run(context.Background(), args, strings.NewReader(""), &out, &errb, func(k string) string { return h.env[k] }, false)
	return code, out.String(), errb.String()
}

func newHub(t *testing.T) string {
	t.Helper()
	hub, err := hubserver.Build(hubserver.Options{DBPath: ":memory:", AdminToken: "admin-token-123456",
		Policy: app.DefaultMessagePolicy(), OnlineWindow: time.Minute, Log: newLogger("error")})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(hub.Handler)
	t.Cleanup(func() { srv.Close(); hub.Close() })
	return srv.URL
}

func TestCLIEndToEnd(t *testing.T) {
	hub := newHub(t)
	dir := t.TempDir()
	alice := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "a.yaml")}}
	bob := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "b.yaml")}}

	if code, _, e := alice.run("agent", "register", "--name", "alice"); code != 0 {
		t.Fatalf("register alice: %d %s", code, e)
	}
	if code, _, e := bob.run("agent", "register", "--name", "bob", "--capability", "code"); code != 0 {
		t.Fatalf("register bob: %d %s", code, e)
	}
	if code, _, _ := bob.run("agent", "register", "--name", "bob"); code != ExitConflict {
		t.Fatalf("duplicate register: want %d, got %d", ExitConflict, code)
	}

	code, out, _ := alice.run("message", "send", "--to", "bob", "--body", "hi")
	if code != 0 {
		t.Fatalf("send: %d", code)
	}
	var sent struct{ Message struct{ ID string } }
	if err := json.Unmarshal([]byte(out), &sent); err != nil || sent.Message.ID == "" {
		t.Fatalf("send output not JSON: %q", out)
	}

	code, out, _ = bob.run("inbox", "pull")
	if code != 0 || !strings.Contains(out, sent.Message.ID) {
		t.Fatalf("pull: %d %s", code, out)
	}
	if code, _, e := bob.run("message", "reply", "--id", sent.Message.ID, "--body", "pong"); code != 0 {
		t.Fatalf("reply: %d %s", code, e)
	}
	// Release + re-pull works and is not a failed attempt.
	code, out, _ = alice.run("message", "send", "--to", "bob", "--body", "later")
	var later struct{ Message struct{ ID string } }
	json.Unmarshal([]byte(out), &later)
	bob.run("inbox", "pull")
	if code, _, e := bob.run("message", "release", "--id", later.Message.ID, "--delay", "0s"); code != 0 {
		t.Fatalf("release: %d %s", code, e)
	}
	if code, out, _ := bob.run("inbox", "pull"); code != 0 || !strings.Contains(out, later.Message.ID) || !strings.Contains(out, `"attempts": 1`) {
		t.Fatalf("re-pull after release: %d %s", code, out)
	}

	code, out, _ = alice.run("message", "wait", "--id", sent.Message.ID, "--timeout", "1s")
	if code != 0 || !strings.Contains(out, "pong") {
		t.Fatalf("wait: %d %s", code, out)
	}

	// Agent-friendliness contract.
	if code, _, _ := alice.run("message", "send", "--to", "bob", "--body", "x", "--dry-run"); code != ExitDryRun {
		t.Errorf("dry-run exit: %d", code)
	}
	code, _, e := alice.run("message", "send", "--too", "bob")
	if code != ExitUsage || !strings.Contains(e, `did you mean \"to\"`) {
		t.Errorf("unknown flag: %d %s", code, e)
	}
	if code, _, e := alice.run("message", "get", "--id", "msg_nope"); code != ExitNotFound || !strings.Contains(e, `"error":"not_found"`) {
		t.Errorf("not found: %d %s", code, e)
	}
	if code, _, _ := alice.run("message", "send", "--to", "bob", "--body", "x", "--type", "reply"); code != ExitUsage {
		t.Errorf("enum validation: %d", code)
	}
	if code, out, _ := alice.run("schema", "message", "send"); code != 0 || !strings.Contains(out, `"idempotency-key"`) {
		t.Errorf("schema: %d %s", code, out)
	}
	if code, out, _ := alice.run("--help"); code != 0 || !strings.Contains(out, "Exit codes") {
		t.Errorf("help: %d %s", code, out)
	}
}

func TestCLIAutoEnroll(t *testing.T) {
	hub := newHub(t)
	dir := t.TempDir()
	admin := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "admin.yaml")}}
	code, out, e := admin.run("enrollment", "create", "--name-prefix", "cloud-", "--harness", "claude-cloud", "--format", "json")
	if code != 0 {
		t.Fatalf("create enrollment: %d %s", code, e)
	}
	var created struct{ Token string }
	json.Unmarshal([]byte(out), &created)

	session := &harnessT{t, map[string]string{
		"MYCO_HUB": hub, "MYCO_ENROLL_TOKEN": created.Token, "MYCO_CONFIG": filepath.Join(dir, "s.yaml"),
		"CLAUDE_CODE_REMOTE_SESSION_ID": "cse_01AbCdEfGh12345678", "CLAUDE_CODE_REMOTE_ENVIRONMENT_TYPE": "cloud_default",
	}}
	// Any agent command auto-enrolls on first use.
	code, out, e = session.run("agent", "get", "--format", "json")
	if code != 0 || !strings.Contains(e, "enrolled as cloud-") || !strings.Contains(out, `"session_id": "session_01AbCdEfGh12345678"`) {
		t.Fatalf("auto-enroll: %d out=%s err=%s", code, out, e)
	}
	// Container restart: config lost, same session -> same agent.
	os.Remove(filepath.Join(dir, "s.yaml"))
	code, out2, _ := session.run("agent", "enroll", "--format", "json")
	if code != 0 || !strings.Contains(out2, `"created": false`) {
		t.Fatalf("recover: %d %s", code, out2)
	}
	// Enrollment token cannot administer the hub.
	session.env["MYCO_ADMIN_TOKEN"] = created.Token
	if code, _, _ := session.run("agent", "register", "--name", "evil"); code != ExitPermission {
		t.Fatalf("enroll token used as admin: exit %d", code)
	}
}

func TestGatewayErrorsAreRetryable(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"pending":[{"id":"msg_x","from":"a","to":"b","thread_id":"msg_x","type":"task","body":"hi","status":"queued"}],"timed_out":false}`)
	}))
	defer srv.Close()
	h := &harnessT{t, map[string]string{"MYCO_HUB": srv.URL, "MYCO_TOKEN": "myco_x", "MYCO_CONFIG": filepath.Join(t.TempDir(), "c.yaml")}}
	if code, out, e := h.run("inbox", "wait", "--timeout", "30s"); code != 0 || !strings.Contains(out, "msg_x") {
		t.Fatalf("wait should survive a 502: %d %s %s", code, out, e)
	}
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) })
	code, _, e := h.run("message", "get", "--id", "msg_x")
	if code != ExitUnavailable || !strings.Contains(e, "HTTP 502") || !strings.Contains(e, `"retryable":true`) {
		t.Fatalf("502 should map to unavailable: %d %s", code, e)
	}
}

func TestDetectBinding(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	b := detectBinding(env(map[string]string{"CLAUDE_CODE_REMOTE_SESSION_ID": "cse_01Ab", "CLAUDE_CODE_SESSION_ID": "local-uuid"}))
	if b.Harness != "claude-cloud" || b.SessionID != "session_01Ab" {
		t.Fatalf("cloud: %+v", b)
	}
	b = detectBinding(env(map[string]string{"CLAUDE_CODE_SESSION_ID": "7f3c2a"}))
	if b.Harness != "claude" || b.SessionID != "7f3c2a" || b.Environment == "" {
		t.Fatalf("local claude: %+v", b)
	}
	if b = detectBinding(env(nil)); b.SessionID != "" || b.Harness != "" {
		t.Fatalf("nothing detected: %+v", b)
	}
}

func TestProfilesAndRunnerAll(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hub := newHub(t)
	admin := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(home, "admin.yaml")}}
	_, out, _ := admin.run("enrollment", "create", "--name-prefix", "mac-", "--format", "json")
	var created struct{ Token string }
	json.Unmarshal([]byte(out), &created)
	admin.run("agent", "register", "--name", "boss")

	// Two Claude Code sessions on the same machine enroll independently.
	sess := func(id string) *harnessT {
		return &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ENROLL_TOKEN": created.Token, "CLAUDE_CODE_SESSION_ID": id}}
	}
	a, b := sess("aaaa1111"), sess("bbbb2222")
	if code, _, e := a.run("agent", "enroll", "--name", "mac-a", "--workdir", home); code != 0 {
		t.Fatalf("enroll a: %s", e)
	}
	if code, _, e := b.run("agent", "enroll", "--name", "mac-b", "--workdir", home); code != 0 {
		t.Fatalf("enroll b: %s", e)
	}
	for name, h := range map[string]*harnessT{"mac-a": a, "mac-b": b} {
		code, out, _ := h.run("agent", "get", "--format", "json")
		if code != 0 || !strings.Contains(out, `"name": "`+name+`"`) {
			t.Fatalf("session should resolve to its own profile %s: %s", name, out)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".myco", "agents", "mac-a.yaml")); err != nil {
		t.Fatal("profile file missing")
	}

	// Use a shell harness in both profiles so the test needs no real CLI.
	for _, n := range []string{"mac-a", "mac-b"} {
		p := filepath.Join(home, ".myco", "agents", n+".yaml")
		data, _ := os.ReadFile(p)
		data = []byte(strings.Replace(string(data), "harness: claude\n    workdir", "harness: shell\n    command: 'echo handled-by-$MYCO_FROM'\n    workdir", 1))
		os.WriteFile(p, data, 0o600)
	}
	admin.env["MYCO_CONFIG"] = filepath.Join(home, ".myco", "config.yaml")
	boss := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_CONFIG": filepath.Join(home, "admin.yaml")}}
	for _, to := range []string{"mac-a", "mac-b"} {
		if code, _, e := boss.run("message", "send", "--to", to, "--body", "hi"); code != 0 {
			t.Fatalf("send: %s", e)
		}
	}
	plain := &harnessT{t, map[string]string{"MYCO_HUB": hub}}
	if code, _, e := plain.run("runner", "start", "--all", "--once", "--log-level", "error", "--rebind"); code != 0 {
		t.Fatalf("runner --all: %d %s", code, e)
	}
	code, out, _ := boss.run("inbox", "pull", "--format", "json")
	if code != 0 || strings.Count(out, "handled-by-boss") != 2 {
		t.Fatalf("both agents should have replied: %s", out)
	}
}

func TestHarnessListAndDoctor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hub := newHub(t)
	h := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(home, "c.yaml")}}
	code, out, _ := h.run("harness", "list", "--format", "json")
	for _, n := range []string{`"claude"`, `"codex"`, `"pi"`, `"kiro"`, `"template"`} {
		if code != 0 || !strings.Contains(out, n) {
			t.Fatalf("harness list missing %s: %s", n, out)
		}
	}
	h.run("agent", "register", "--name", "doc")
	// Healthy identity with a shell runner.
	cfg := filepath.Join(home, "c.yaml")
	data, _ := os.ReadFile(cfg)
	os.WriteFile(cfg, append(data, []byte("runner:\n    harness: shell\n    command: \"true\"\n    workdir: "+home+"\n")...), 0o600)
	if code, out, e := h.run("doctor", "--format", "json"); code != 0 || !strings.Contains(out, `"ok": true`) {
		t.Fatalf("doctor healthy: %d %s %s", code, out, e)
	}
	// Broken workdir and invalid access -> exit 1 with suggestions.
	os.WriteFile(cfg, append(data, []byte("runner:\n    harness: shell\n    command: \"true\"\n    workdir: /nope\n    access: superuser\n")...), 0o600)
	code, out, _ = h.run("doctor", "--format", "json")
	if code != ExitError || !strings.Contains(out, `"check": "workdir"`) || !strings.Contains(out, `"check": "runner.access"`) {
		t.Fatalf("doctor broken: %d %s", code, out)
	}
}

func TestRotateKeepsBindingAndLoginWritesProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hub := newHub(t)
	admin := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(home, "admin.yaml")}}
	if code, _, e := admin.run("agent", "register", "--name", "cx", "--harness", "codex", "--session-id", "sess-9", "--environment", "mac", "--description", "keep me", "--no-save"); code != 0 {
		t.Fatalf("register: %s", e)
	}
	code, out, e := admin.run("agent", "register", "--name", "cx", "--rotate-token", "--no-save", "--format", "json")
	if code != 0 || !strings.Contains(out, `"session_id": "sess-9"`) || !strings.Contains(out, `"description": "keep me"`) {
		t.Fatalf("rotate must keep binding/description: %d %s %s", code, out, e)
	}
	var res struct{ Token string }
	json.Unmarshal([]byte(out), &res)
	tokFile := filepath.Join(home, "cx.token")
	os.WriteFile(tokFile, []byte(res.Token+"\n"), 0o600)
	mac := &harnessT{t, map[string]string{"MYCO_HUB": hub}}
	if code, _, e := mac.run("agent", "login", "--token-file", tokFile, "--workdir", home); code != 0 {
		t.Fatalf("login: %s", e)
	}
	b, err := os.ReadFile(filepath.Join(home, ".myco", "agents", "cx.yaml"))
	if err != nil || !strings.Contains(string(b), "session_id: sess-9") || !strings.Contains(string(b), "harness: codex") {
		t.Fatalf("profile: %s %v", b, err)
	}
}

func TestReadEnvFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "e.env")
	os.WriteFile(f, []byte("# claude\nexport CLAUDE_CODE_OAUTH_TOKEN=\"sk-ant-oat01-x\"\nFOO = bar\n\n"), 0o600)
	got, err := config.ReadEnvFile(f)
	if err != nil || len(got) != 2 || got[0] != "CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-x" || got[1] != "FOO=bar" {
		t.Fatalf("%v %v", got, err)
	}
	os.WriteFile(f, []byte("not a pair\n"), 0o600)
	if _, err := config.ReadEnvFile(f); err == nil {
		t.Fatal("malformed line must fail")
	}
}

func TestRunnerEnvCapture(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ANTHROPIC_BASE_URL", "https://llm.example.com/anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "sk-secret value")
	t.Setenv("UNRELATED", "x")
	prof := filepath.Join(home, ".myco", "agents", "mac-claude.yaml")
	os.MkdirAll(filepath.Dir(prof), 0o700)
	os.WriteFile(prof, []byte("hub: http://h\nagent: mac-claude\ntoken: t\nruntime: x\n"), 0o600)
	os.WriteFile(prof, []byte("hub: http://h\nagent: mac-claude\ntoken: t\n"), 0o600)
	h := &harnessT{t, map[string]string{}}
	code, out, e := h.run("runner", "env", "capture", "--agent", "mac-claude", "--format", "json")
	if code != 0 || strings.Contains(out, "sk-secret") || !strings.Contains(out, "ANTHROPIC_API_KEY") {
		t.Fatalf("capture: %d %s %s", code, out, e)
	}
	envFile := filepath.Join(home, ".myco", "env", "mac-claude.env")
	st, err := os.Stat(envFile)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("env file perms: %v %v", st, err)
	}
	vars, err := config.ReadEnvFile(envFile)
	if err != nil || len(vars) != 2 || vars[0] != "ANTHROPIC_API_KEY=sk-secret value" {
		t.Fatalf("round trip: %v %v", vars, err)
	}
	cfg, _ := config.Load(prof)
	if cfg.Runner.EnvFile != envFile {
		t.Fatalf("profile env_file not set: %+v", cfg.Runner)
	}
}
