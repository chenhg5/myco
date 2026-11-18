package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/testutil"
)

// One Claude Code session and two workspaces: joining the second joins a
// copy (again: the same copy); with --same-session the session answers for
// both, and inside it inbox wait/pull serve both and a command on a message
// acts as the agent that got it.
func TestOneSessionTwoWorkspaces(t *testing.T) {
	testutil.NeedsPOSIXShell(t)
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	// A claude that copies sessions: it prints a new session id and counts calls.
	bin := t.TempDir()
	calls := filepath.Join(bin, "calls")
	script := "#!/bin/sh\necho x >> '" + calls + "'\n" +
		`echo '{"type":"result","subtype":"success","session_id":"copy-2222","result":"ready","is_error":false}'` + "\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	callCount := func() int {
		b, _ := os.ReadFile(calls)
		return strings.Count(string(b), "x")
	}

	type ws struct {
		hub, tok string
		boss     *harnessT
	}
	mk := func(name string) ws {
		hub := newHub(t)
		admin := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(home, name+"-admin.yaml")}}
		_, out, _ := admin.run("enrollment", "create", "--name-prefix", name+"-", "--format", "json")
		var created struct{ Token string }
		json.Unmarshal([]byte(out), &created)
		boss := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(home, name+"-boss.yaml")}}
		if code, _, e := boss.run("agent", "register", "--name", name+"-boss"); code != 0 {
			t.Fatalf("register boss: %s", e)
		}
		return ws{hub, created.Token, boss}
	}
	a, b := mk("a"), mk("b")
	session := func(w *ws, extra ...string) *harnessT {
		env := map[string]string{"CLAUDE_CODE_SESSION_ID": "sess-1111"}
		if w != nil {
			env["MYCO_HUB"], env["MYCO_ENROLL_TOKEN"] = w.hub, w.tok
		}
		for i := 0; i+1 < len(extra); i += 2 {
			env[extra[i]] = extra[i+1]
		}
		return &harnessT{t, env}
	}
	type joined struct {
		Agent struct {
			Name    string
			Binding struct {
				SessionID string `json:"session_id"`
			}
		}
		Created bool
		Session string
		AlsoIn  []struct{ Agent, Hub string } `json:"also_in"`
	}
	join := func(h *harnessT, args ...string) joined {
		t.Helper()
		code, out, e := h.run(append([]string{"join", "--format", "json"}, args...)...)
		if code != 0 {
			t.Fatalf("join %v: %d %s %s", args, code, out, e)
		}
		var j joined
		if err := json.Unmarshal([]byte(out), &j); err != nil {
			t.Fatalf("join output: %s", out)
		}
		return j
	}

	ja := join(session(&a))
	if ja.Session != "" || len(ja.AlsoIn) != 0 || ja.Agent.Binding.SessionID != "sess-1111" {
		t.Fatalf("first workspace: %+v", ja)
	}
	// The second workspace: a copy joins, and the output says why.
	jb := join(session(&b))
	if jb.Session != "copy" || len(jb.AlsoIn) != 1 || jb.AlsoIn[0].Agent != ja.Agent.Name || jb.Agent.Binding.SessionID != "copy-2222" || callCount() != 1 {
		t.Fatalf("second workspace: %+v (claude calls %d)", jb, callCount())
	}
	if code, out, _ := session(&b).run("join", "--format", "text"); code != 0 || !strings.Contains(out, "a copy of it joined this workspace") {
		t.Fatalf("join text: %d %s", code, out)
	}
	// Again: the same copy, no new one.
	if again := join(session(&b)); again.Agent.Name != jb.Agent.Name || again.Created || callCount() != 1 {
		t.Fatalf("join again: %+v (claude calls %d)", again, callCount())
	}

	// Shared on purpose: the session itself answers for workspace B too.
	jb2 := join(session(&b), "--same-session", "--name", "b-shared")
	if jb2.Session != "shared" || jb2.Agent.Binding.SessionID != "sess-1111" {
		t.Fatalf("same session: %+v", jb2)
	}
	send := func(w ws, to, body string) string {
		code, out, e := w.boss.run("message", "send", "--to", to, "--body", body, "--format", "json")
		if code != 0 {
			t.Fatalf("send: %s", e)
		}
		var s struct{ Message struct{ ID string } }
		json.Unmarshal([]byte(out), &s)
		return s.Message.ID
	}
	inSession := session(nil)
	mA := send(a, ja.Agent.Name, "from A")
	code, out, e := inSession.run("inbox", "wait", "--timeout", "5s", "--format", "json")
	if code != 0 || !strings.Contains(out, mA) || !strings.Contains(out, `"for"`) {
		t.Fatalf("inbox wait: %d %s %s", code, out, e)
	}
	mB := send(b, "b-shared", "from B")
	code, out, e = inSession.run("inbox", "pull", "--format", "json")
	var pulled struct {
		Messages []struct {
			ID string
			As struct{ Agent, Hub string }
		}
	}
	json.Unmarshal([]byte(out), &pulled)
	if code != 0 || len(pulled.Messages) != 2 {
		t.Fatalf("inbox pull: %d %s %s", code, out, e)
	}
	for _, m := range pulled.Messages {
		want := map[string]string{mA: ja.Agent.Name, mB: "b-shared"}[m.ID]
		if m.As.Agent != want {
			t.Fatalf("message %s came to %s, want %s", m.ID, m.As.Agent, want)
		}
	}
	// Commands on a message act as the agent that got it (remembered by pull).
	for _, id := range []string{mA, mB} {
		if code, _, e := inSession.run("message", "reply", "--id", id, "--body", "ok"); code != 0 {
			t.Fatalf("reply %s: %s", id, e)
		}
	}
	// Not remembered: found by asking each workspace.
	mB2 := send(b, "b-shared", "again")
	os.Remove(messageOwnersPath(filepath.Join(home, ".myco", "config.yaml")))
	if code, _, e := inSession.run("message", "done", "--id", mB2); code != 0 {
		t.Fatalf("done unremembered: %s", e)
	}
	// Other commands act as one of them and say so; MYCO_AGENT picks.
	if code, out, e := inSession.run("agent", "get", "--format", "json"); code != 0 || !strings.Contains(e, "this session is also") || !strings.Contains(out, `"name": "`+ja.Agent.Name+`"`) {
		t.Fatalf("agent get: %d %s %s", code, out, e)
	}
	if code, out, e := session(nil, EnvAgent, "b-shared").run("agent", "get", "--format", "json"); code != 0 || strings.Contains(e, "this session is also") || !strings.Contains(out, `"name": "b-shared"`) {
		t.Fatalf("MYCO_AGENT: %d %s %s", code, out, e)
	}
}
