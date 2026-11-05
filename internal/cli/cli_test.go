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

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"

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

func TestLabelsAndBroadcast(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hub := newHub(t)
	admin := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(home, "admin.yaml")}}
	_, out, _ := admin.run("enrollment", "create", "--name-prefix", "mac-", "--label", "machine=mac", "--format", "json")
	var created struct{ Token string }
	json.Unmarshal([]byte(out), &created)
	sess := func(id string) *harnessT {
		return &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ENROLL_TOKEN": created.Token, "CLAUDE_CODE_SESSION_ID": id}}
	}
	a, b, c := sess("s-a"), sess("s-b"), sess("s-c")
	a.run("agent", "enroll", "--name", "mac-a", "--label", "group=loops")
	b.run("agent", "enroll", "--name", "mac-b", "--label", "group=loops")
	c.run("agent", "enroll", "--name", "mac-c")
	boss := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_CONFIG": filepath.Join(home, "boss.yaml"), "MYCO_ADMIN_TOKEN": "admin-token-123456"}}
	boss.run("agent", "register", "--name", "boss")
	// mac-b only accepts tasks from mac-a: the broadcast skips it.
	admin.run("agent", "register", "--name", "mac-b", "--rotate-token", "--no-save", "--allow-from", "mac-a")

	// Self-service label + selector listing.
	if code, _, e := c.run("agent", "label", "add", "--label", "group=loops,gpu"); code != 0 {
		t.Fatalf("label add: %s", e)
	}
	code, out, _ := boss.run("agent", "list", "--selector", "group=loops,machine=mac", "--format", "json")
	if code != 0 || strings.Count(out, `"name": "mac-`) != 3 {
		t.Fatalf("selector list: %s", out)
	}

	if code, _, _ := boss.run("broadcast", "send", "--selector", "group=loops", "--body", "x", "--dry-run"); code != ExitDryRun {
		t.Fatalf("dry-run exit %d", code)
	}
	code, out, e := boss.run("broadcast", "send", "--selector", "group=loops", "--body", "status?", "--idempotency-key", "k1", "--format", "json")
	if code != 0 || !strings.Contains(out, `"agent": "mac-b"`) || strings.Count(out, `"to": "mac-`) != 2 {
		t.Fatalf("broadcast: %d %s %s", code, out, e)
	}
	var bc struct {
		BroadcastID string `json:"broadcast_id"`
		Messages    []struct{ ID, To string }
	}
	json.Unmarshal([]byte(out), &bc)
	_, again, _ := boss.run("broadcast", "send", "--selector", "group=loops", "--body", "status?", "--idempotency-key", "k1", "--format", "json")
	if !strings.Contains(again, `"duplicate": true`) || !strings.Contains(again, bc.BroadcastID) {
		t.Fatalf("idempotent broadcast: %s", again)
	}
	// Recipients answer; the sender sees both replies; others cannot read it.
	for _, m := range bc.Messages {
		h := map[string]*harnessT{"mac-a": a, "mac-c": c}[m.To]
		h.run("inbox", "pull")
		if code, _, e := h.run("message", "reply", "--id", m.ID, "--body", "ok from "+m.To); code != 0 {
			t.Fatalf("reply: %s", e)
		}
	}
	code, out, _ = boss.run("broadcast", "get", "--id", bc.BroadcastID, "--format", "json")
	if code != 0 || !strings.Contains(out, `"answered": 2`) || !strings.Contains(out, "ok from mac-c") {
		t.Fatalf("broadcast get: %s", out)
	}
	// Plain get is read-only: the replies are still in boss's inbox.
	if _, pending, _ := boss.run("inbox", "wait", "--timeout", "1s", "--format", "json"); strings.Count(pending, `"type": "reply"`) != 2 {
		t.Fatalf("get must not consume: %s", pending)
	}
	// --consume (what send --wait uses) clears them.
	boss.run("broadcast", "get", "--id", bc.BroadcastID, "--consume")
	if code, _, _ := boss.run("inbox", "wait", "--timeout", "1s"); code != ExitTimeout {
		t.Fatalf("consumed replies must leave the inbox, inbox wait exit=%d", code)
	}
	if code, _, _ := a.run("broadcast", "get", "--id", bc.BroadcastID); code != ExitNotFound {
		t.Fatalf("recipient must not read the broadcast: %d", code)
	}
}

func TestAttachments(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hub := newHub(t)
	mk := func(name string) *harnessT {
		h := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(home, name+".yaml")}}
		if code, _, e := h.run("agent", "register", "--name", name); code != 0 {
			t.Fatalf("register %s: %s", name, e)
		}
		return h
	}
	alice, bob, eve := mk("alice"), mk("bob"), mk("eve")
	doc := filepath.Join(home, "design.md")
	os.WriteFile(doc, []byte("# Design\nhello attachments\n"), 0o600)

	code, out, e := alice.run("message", "send", "--to", "bob", "--body", "see attached", "--attach", doc, "--format", "json")
	if code != 0 || !strings.Contains(out, `"name": "design.md"`) {
		t.Fatalf("send with attach: %d %s %s", code, out, e)
	}
	var sent struct {
		Message struct {
			ID          string
			Attachments []struct {
				FileID string `json:"file_id"`
			}
		}
	}
	json.Unmarshal([]byte(out), &sent)
	fid := sent.Message.Attachments[0].FileID

	// bob received it and can download it (checksum verified).
	if _, out, _ := bob.run("inbox", "pull", "--format", "json"); !strings.Contains(out, fid) {
		t.Fatalf("pull should list the attachment: %s", out)
	}
	dl := filepath.Join(home, "dl")
	os.MkdirAll(dl, 0o700)
	if code, _, e := bob.run("file", "get", "--id", fid, "--output", dl); code != 0 {
		t.Fatalf("bob download: %s", e)
	}
	if b, _ := os.ReadFile(filepath.Join(dl, "design.md")); string(b) != "# Design\nhello attachments\n" {
		t.Fatalf("content: %q", b)
	}
	// eve was not a recipient: not found (existence not revealed).
	if code, _, _ := eve.run("file", "get", "--id", fid, "--output", dl, "--force"); code != ExitNotFound {
		t.Fatalf("eve must not download: exit %d", code)
	}
	// bob forwards the same file id to eve, then eve can read it.
	if code, _, e := bob.run("message", "send", "--to", "eve", "--body", "fyi", "--type", "note", "--attach", fid); code != 0 {
		t.Fatalf("forward: %s", e)
	}
	if code, _, e := eve.run("file", "get", "--id", fid, "--output", filepath.Join(home, "eve.md")); code != 0 {
		t.Fatalf("eve after forward: %s", e)
	}
	// bob replies with a file; alice gets it via wait.
	res := filepath.Join(home, "result.txt")
	os.WriteFile(res, []byte("done"), 0o600)
	if code, _, e := bob.run("message", "reply", "--id", sent.Message.ID, "--body", "ok", "--attach", res); code != 0 {
		t.Fatalf("reply attach: %s", e)
	}
	if _, out, _ := alice.run("message", "wait", "--id", sent.Message.ID, "--timeout", "2s", "--format", "json"); !strings.Contains(out, `"name": "result.txt"`) {
		t.Fatalf("reply attachments: %s", out)
	}
	// Credential directories are never attached.
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	os.WriteFile(filepath.Join(home, ".ssh", "id_rsa"), []byte("secret"), 0o600)
	if code, _, _ := alice.run("message", "send", "--to", "bob", "--body", "x", "--attach", filepath.Join(home, ".ssh", "id_rsa")); code != ExitPermission {
		t.Fatalf("ssh key attach must be refused: %d", code)
	}
}

func TestOverviewAndUIServer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hub := newHub(t)
	mk := func(name string) *harnessT {
		h := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(home, name+".yaml")}}
		h.run("agent", "register", "--name", name)
		return h
	}
	alice, bob, carol := mk("alice"), mk("bob"), mk("carol")
	_, out, _ := alice.run("message", "send", "--to", "bob", "--body", "secret plan", "--format", "json")
	var sent struct{ Message struct{ ID string } }
	json.Unmarshal([]byte(out), &sent)
	bob.run("inbox", "pull")
	bob.run("message", "ack", "--id", sent.Message.ID)
	carol.run("message", "send", "--to", "bob", "--body", "queued one")

	cfg, _ := config.Load(filepath.Join(home, "alice.yaml"))
	ac, _ := client.New(hub, cfg.Token)
	ov, err := ac.Overview(context.Background(), 50)
	if err != nil {
		t.Fatal(err)
	}
	var bobSt *api.AgentStatus
	for i := range ov.Agents {
		if ov.Agents[i].Agent.Name == "bob" {
			bobSt = &ov.Agents[i]
		}
	}
	if bobSt == nil || bobSt.State != "working" || bobSt.InProgress != 1 || bobSt.Queued != 1 || len(bobSt.Current) != 0 {
		t.Fatalf("alice's view of bob: %+v", bobSt)
	}
	for _, m := range ov.Recent {
		if m.From != "alice" && m.To != "alice" {
			t.Fatalf("agent view leaked a foreign message: %+v", m)
		}
	}
	admin, _ := client.New(hub, "admin-token-123456")
	aov, err := admin.Overview(context.Background(), 50)
	if err != nil || aov.Viewer.Kind != "admin" || len(aov.Recent) != 2 {
		t.Fatalf("admin overview: %+v %v", aov, err)
	}
	if th, err := admin.Thread(context.Background(), sent.Message.ID); err != nil || len(th) != 1 {
		t.Fatalf("admin thread: %v %v", th, err)
	}

	// UI server guards.
	ui := &uiServer{hub: hub, agentName: "alice", agent: ac, admin: admin, key: "k123", port: 7790}
	h := ui.routes()
	do := func(method, target, host string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, nil)
		r.Host = host
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := do("GET", "/api/overview", "evil.com:7790", nil); w.Code != http.StatusForbidden {
		t.Fatalf("foreign host: %d", w.Code)
	}
	if w := do("GET", "/api/overview", "127.0.0.1:7790", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("no key: %d", w.Code)
	}
	if w := do("GET", "/?key=wrong", "127.0.0.1:7790", nil); w.Code != http.StatusForbidden {
		t.Fatalf("wrong key: %d", w.Code)
	}
	w := do("GET", "/?key=k123", "localhost:7790", nil)
	if w.Code != http.StatusFound || len(w.Result().Cookies()) != 1 || !w.Result().Cookies()[0].HttpOnly {
		t.Fatalf("key exchange: %d %v", w.Code, w.Result().Cookies())
	}
	ck := w.Result().Cookies()[0]
	if w := do("GET", "/api/overview?as=admin", "127.0.0.1:7790", ck); w.Code != 200 || !strings.Contains(w.Body.String(), `"kind":"admin"`) {
		t.Fatalf("admin overview via ui: %d %s", w.Code, w.Body.String())
	}
	if w := do("GET", "/", "127.0.0.1:7790", ck); w.Code != 200 || !strings.Contains(w.Body.String(), `<div id="root">`) {
		t.Fatalf("index: %d", w.Code)
	}
}

func TestAliasesAndDisplayNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hub := newHub(t)
	mk := func(name string) *harnessT {
		return &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(home, name+".yaml")}}
	}
	boss, w1, w2 := mk("boss"), mk("w1"), mk("w2")
	boss.run("agent", "register", "--name", "boss")
	if code, _, e := w1.run("agent", "register", "--name", "worker-1", "--display-name", "Worker One"); code != 0 {
		t.Fatalf("register: %s", e)
	}
	w2.run("agent", "register", "--name", "worker-2")

	// Alias management: dry-run, set, collisions, no chains.
	if code, _, _ := boss.run("agent", "alias", "set", "--alias", "reviewer", "--target", "worker-1", "--dry-run"); code != ExitDryRun {
		t.Fatalf("alias dry-run exit %d", code)
	}
	if code, _, e := boss.run("agent", "alias", "set", "--alias", "reviewer", "--target", "worker-1"); code != 0 {
		t.Fatalf("alias set: %s", e)
	}
	if code, _, _ := boss.run("agent", "alias", "set", "--alias", "worker-2", "--target", "worker-1"); code != ExitConflict {
		t.Fatalf("alias over an agent name must conflict, exit %d", code)
	}
	if code, _, _ := boss.run("agent", "alias", "set", "--alias", "lead", "--target", "reviewer"); code == 0 {
		t.Fatal("alias chains must be refused")
	}
	if code, _, _ := boss.run("agent", "alias", "set", "--alias", "lead", "--target", "nobody"); code != ExitNotFound {
		t.Fatalf("alias to a missing agent: exit %d", code)
	}
	if code, _, _ := mk("x").run("agent", "register", "--name", "reviewer"); code != ExitConflict {
		t.Fatalf("registering an alias name must conflict, exit %d", code)
	}

	// Only the admin manages aliases.
	_, out, _ := boss.run("agent", "register", "--name", "worker-1", "--rotate-token", "--no-save", "--format", "json")
	var reg struct{ Token string }
	json.Unmarshal([]byte(out), &reg)
	ac, _ := client.New(hub, reg.Token)
	if _, err := ac.SetAlias(context.Background(), "mine", "worker-1", false); err == nil {
		t.Fatal("agent token must not set aliases")
	}
	w1.env["MYCO_CONFIG"] = filepath.Join(home, "w1b.yaml")
	w1.run("agent", "login", "--token-file", writeFile(t, home, "w1.token", reg.Token))

	// Sending to the alias lands on the target and is stored under the real name.
	code, out, e := boss.run("message", "send", "--to", "reviewer", "--body", "please review", "--format", "json")
	if code != 0 || !strings.Contains(out, `"via_alias": "reviewer"`) || !strings.Contains(out, `"to": "worker-1"`) {
		t.Fatalf("send via alias: %d %s %s", code, out, e)
	}
	if _, out, _ := boss.run("message", "send", "--to", "reviewer", "--type", "note", "--body", "x", "--format", "text"); !strings.Contains(out, "via alias reviewer") {
		t.Fatalf("text output should mention the alias: %s", out)
	}
	if _, out, _ := w1.run("inbox", "pull", "--format", "json"); !strings.Contains(out, "please review") {
		t.Fatalf("worker-1 inbox: %s", out)
	}
	// get resolves aliases; list shows aliases and display names.
	if _, out, _ := boss.run("agent", "get", "--name", "reviewer", "--format", "json"); !strings.Contains(out, `"name": "worker-1"`) || !strings.Contains(out, `"display_name": "Worker One"`) {
		t.Fatalf("get via alias: %s", out)
	}
	if _, out, _ := boss.run("agent", "list", "--format", "text"); !strings.Contains(out, "Worker One") || !strings.Contains(out, "reviewer") {
		t.Fatalf("list: %s", out)
	}

	// Re-point: the role moves to worker-2.
	boss.run("agent", "alias", "set", "--alias", "reviewer", "--target", "worker-2")
	if _, out, _ := boss.run("message", "send", "--to", "reviewer", "--type", "note", "--body", "y", "--format", "json"); !strings.Contains(out, `"to": "worker-2"`) {
		t.Fatalf("re-pointed alias: %s", out)
	}
	if _, out, _ := boss.run("agent", "alias", "list", "--format", "json"); !strings.Contains(out, `"target": "worker-2"`) {
		t.Fatalf("alias list: %s", out)
	}

	// Profile updates: self-service and admin; empty clears.
	if code, _, e := w1.run("agent", "update", "--display-name", "Reviewer Bot"); code != 0 {
		t.Fatalf("update self: %s", e)
	}
	if _, out, _ := boss.run("agent", "get", "--name", "worker-1", "--format", "json"); !strings.Contains(out, "Reviewer Bot") {
		t.Fatalf("self update: %s", out)
	}
	boss.run("agent", "update", "--name", "worker-1", "--display-name", "", "--description", "reviews PRs")
	if _, out, _ := boss.run("agent", "get", "--name", "worker-1", "--format", "json"); strings.Contains(out, "display_name") || !strings.Contains(out, "reviews PRs") {
		t.Fatalf("admin update: %s", out)
	}
	if code, _, _ := w1.run("agent", "update"); code != ExitUsage {
		t.Fatalf("update without flags: exit %d", code)
	}
	if code, _, _ := w1.run("agent", "update", "--display-name", "bad\x07name"); code == 0 {
		t.Fatal("control characters must be refused")
	}

	// Removing the target drops its aliases; remove needs --force.
	if code, _, _ := boss.run("agent", "alias", "remove", "--alias", "reviewer"); code != ExitUsage {
		t.Fatalf("remove without --force: exit %d", code)
	}
	boss.run("agent", "remove", "--name", "worker-2", "--force")
	if _, out, _ := boss.run("agent", "alias", "list", "--format", "json"); strings.Contains(out, "reviewer") {
		t.Fatalf("aliases of a removed agent must go: %s", out)
	}
	if code, _, _ := boss.run("message", "send", "--to", "reviewer", "--body", "z"); code != ExitNotFound {
		t.Fatalf("send to dropped alias: exit %d", code)
	}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTableAlignsWideCharacters(t *testing.T) {
	var b bytes.Buffer
	tw := newTable(&b)
	io.WriteString(tw, "NAME\tDISPLAY\tX\n")
	io.WriteString(tw, "a\t云端开发会话\t1\n")
	io.WriteString(tw, "bb\tMac Claude（loops）\t2\n")
	tw.Flush()
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	col := func(s string) int { return displayWidth(s[:strings.LastIndex(s, " ")+1]) }
	if col(lines[0]) != col(lines[1]) || col(lines[1]) != col(lines[2]) {
		t.Fatalf("misaligned:\n%s", b.String())
	}
}

func TestExecNode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hub := newHub(t)
	admin := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456"}}
	boss := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(home, "boss.yaml")}}
	eve := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(home, "eve.yaml")}}
	boss.run("agent", "register", "--name", "boss")
	eve.run("agent", "register", "--name", "eve")

	if code, _, _ := admin.run("agent", "register", "--name", "box", "--harness", "exec", "--workdir", home); code != ExitUsage {
		t.Fatalf("exec node without --allow-from must be refused, exit %d", code)
	}
	if code, _, e := admin.run("agent", "register", "--name", "box", "--harness", "exec", "--allow-from", "boss", "--workdir", home); code != 0 {
		t.Fatalf("register exec node: %s", e)
	}
	if b, err := os.ReadFile(filepath.Join(home, ".myco", "agents", "box.yaml")); err != nil || !strings.Contains(string(b), "harness: exec") {
		t.Fatalf("exec node needs a profile for runner --all: %v %s", err, b)
	}

	// exec waits for the result while a runner serves the node.
	execute := func(h *harnessT, args ...string) (int, string, string) {
		type res struct {
			code     int
			out, err string
		}
		done := make(chan res, 1)
		go func() {
			c, o, e := h.run(append([]string{"exec", "--to", "box", "--timeout", "20s"}, args...)...)
			done <- res{c, o, e}
		}()
		runner := &harnessT{t, map[string]string{"MYCO_HUB": hub}}
		for {
			select {
			case r := <-done:
				return r.code, r.out, r.err
			default:
				runner.run("runner", "start", "--all", "--once", "--log-level", "error")
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
	code, out, e := execute(boss, "--cmd", "echo hi; echo oops >&2; exit 3", "--format", "text")
	if code != 3 || out != "hi\n" || !strings.Contains(e, "oops") {
		t.Fatalf("exec: code=%d out=%q err=%q", code, out, e)
	}
	code, out, _ = execute(boss, "--cmd", "pwd", "--format", "json")
	if code != 0 || !strings.Contains(out, `"exit_code": 0`) || !strings.Contains(out, home) {
		t.Fatalf("exec json: %d %s", code, out)
	}
	// Senders outside allow_from are stopped by the hub.
	if code, _, _ := eve.run("exec", "--to", "box", "--cmd", "id", "--timeout", "2s"); code != ExitPermission {
		t.Fatalf("eve must not run commands, exit %d", code)
	}
	// Not an exec node, and dry-run.
	if code, _, _ := boss.run("exec", "--to", "eve", "--cmd", "id"); code != ExitUsage {
		t.Fatalf("exec to a non-exec agent: exit %d", code)
	}
	if code, _, _ := boss.run("exec", "--to", "box", "--cmd", "id", "--dry-run"); code != ExitDryRun {
		t.Fatalf("dry-run: exit %d", code)
	}
	// Notes are never executed.
	boss.run("message", "send", "--to", "box", "--type", "note", "--body", "touch "+filepath.Join(home, "pwned"))
	(&harnessT{t, map[string]string{"MYCO_HUB": hub}}).run("runner", "start", "--all", "--once", "--log-level", "error")
	if _, err := os.Stat(filepath.Join(home, "pwned")); err == nil {
		t.Fatal("a note must not be executed")
	}
	// Every command is audited.
	b, _ := os.ReadFile(filepath.Join(home, ".myco", "logs", "box.exec.jsonl"))
	if strings.Count(string(b), "\n") != 2 || !strings.Contains(string(b), `"from":"boss"`) {
		t.Fatalf("audit log:\n%s", b)
	}
}
