package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/testutil"
)

// An invited person joins with one command, no app: the invite link's script
// runs `myco member accept`, which makes this machine and the sessions
// joined on it theirs; their credential then manages their own nodes and
// pairs the app later.
func TestInviteOnTheCommandLine(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	hub := newHub(t)
	admin := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(t.TempDir(), "a.yaml")}}
	admin.run("agent", "register", "--name", "glenn-bot")

	code, out, e := admin.run("member", "invite", "--name", "laohu", "--display-name", "老胡", "--format", "json")
	if code != 0 {
		t.Fatalf("invite: %s", e)
	}
	var inv struct{ Link, Command string }
	json.Unmarshal([]byte(out), &inv)
	if !strings.HasPrefix(inv.Link, hub+"/c/myco_pair_") || inv.Command != "curl -fsSL "+inv.Link+" | bash" {
		t.Fatalf("invite: %s", out)
	}
	code0 := strings.TrimPrefix(inv.Link, hub+"/c/")
	if _, text, _ := admin.run("member", "invite", "--name", "matthew", "--format", "text"); !strings.Contains(text, "curl -fsSL ") || !strings.Contains(text, "myco app") {
		t.Fatalf("invite text: %s", text)
	}

	// One link, three faces; none of them uses the code up.
	get := func(hdr map[string]string) string {
		req, _ := http.NewRequest("GET", inv.Link, nil)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	if body := get(map[string]string{"User-Agent": "curl/8.5.0"}); !strings.Contains(body, "export MYCO_INVITE_CODE='"+code0+"'") ||
		!strings.Contains(body, "member accept") || !strings.Contains(body, `"$MYCO" join`) {
		t.Fatalf("script:\n%s", body)
	}
	if body := get(map[string]string{"Accept": "text/html"}); !strings.Contains(body, inv.Command) || !strings.Contains(body, "myco://add-hub?link=") {
		t.Fatalf("html:\n%s", body)
	}
	if body := get(map[string]string{"Accept": "text/markdown"}); !strings.Contains(body, inv.Command) {
		t.Fatalf("guide:\n%s", body)
	}

	// What the script runs, in a Claude Code session on laohu's machine.
	lh := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_INVITE_CODE": code0, "CLAUDE_CODE_SESSION_ID": "abcd-1234-ef56"}}
	code, out, e = lh.run("member", "accept", "--machine", "laohu-mbp", "--no-service", "--format", "text")
	if code != 0 || !strings.Contains(out, "you joined as laohu (老胡)") || !strings.Contains(out, `machine "laohu-mbp" joined`) {
		t.Fatalf("accept: %d %s %s", code, out, e)
	}
	b, _ := os.ReadFile(filepath.Join(home, ".myco", "machine.yaml"))
	if !strings.Contains(string(b), "member_token: myco_console_") {
		t.Fatalf("machine.yaml lacks the member credential:\n%s", b)
	}
	// Running it again is harmless (the code is spent, the credential kept).
	if code, out, e := lh.run("member", "accept", "--machine", "laohu-mbp", "--no-service", "--format", "text"); code != 0 || !strings.Contains(out, "laohu") {
		t.Fatalf("accept again: %d %s %s", code, out, e)
	}

	sess := &harnessT{t, map[string]string{"CLAUDE_CODE_SESSION_ID": "abcd-1234-ef56"}}
	code, out, e = sess.run("join", "--format", "json")
	if code != 0 || !strings.Contains(out, `"name": "laohu-mbp-claude-ef56"`) {
		t.Fatalf("join: %d %s %s", code, out, e)
	}
	// (The admin token alone: glenn-bot, saved in admin's config, no longer
	// sees laohu's node, which is private to laohu by default.)
	root := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(t.TempDir(), "root.yaml")}}
	_, out, e = root.run("agent", "get", "--name", "laohu-mbp-claude-ef56", "--format", "json")
	if !strings.Contains(out, `"owner": "laohu"`) || !strings.Contains(out, `"visibility": "private"`) {
		t.Fatalf("the session must be laohu's, private to him: %s %s", out, e)
	}

	// His credential manages his own nodes, not others', and pairs the app.
	me := &harnessT{t, map[string]string{"MYCO_HUB": hub}}
	if code, _, e := me.run("agent", "label", "add", "--name", "laohu-mbp-claude-ef56", "--label", "group=web"); code != 0 {
		t.Fatalf("label own node: %s", e)
	}
	if code, _, _ := me.run("agent", "label", "add", "--name", "glenn-bot", "--label", "x"); code != ExitPermission {
		t.Fatalf("labelling someone else's node: exit %d", code)
	}
	if code, _, e := me.run("agent", "alias", "set", "--alias", "laohu-reviewer", "--target", "laohu-mbp-claude-ef56"); code != 0 {
		t.Fatalf("owner sets own node alias: %d %s", code, e)
	}
	if code, _, _ := me.run("agent", "alias", "set", "--alias", "stolen", "--target", "glenn-bot"); code != ExitPermission {
		t.Fatalf("non-owner sets alias: exit %d", code)
	}
	// A role address that points at someone else's node is not his to take.
	if code, _, e := root.run("agent", "alias", "set", "--alias", "team-reviewer", "--target", "glenn-bot"); code != 0 {
		t.Fatalf("admin sets alias: %d %s", code, e)
	}
	if code, _, _ := me.run("agent", "alias", "set", "--alias", "team-reviewer", "--target", "laohu-mbp-claude-ef56"); code != ExitConflict {
		t.Fatalf("member re-points someone else's alias to his node: exit %d", code)
	}
	if code, _, e := me.run("agent", "alias", "remove", "--alias", "laohu-reviewer", "--force"); code != 0 {
		t.Fatalf("owner removes own node alias: %d %s", code, e)
	}
	if code, out, e := me.run("member", "pair", "--format", "text"); code != 0 || !strings.Contains(out, hub+"/c/myco_pair_") {
		t.Fatalf("pair: %d %s %s", code, out, e)
	}

	// A spent or bogus invite: a clear error.
	other := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_INVITE_CODE": code0, "MYCO_CONFIG": filepath.Join(t.TempDir(), "x.yaml")}}
	testutil.SetHome(t, t.TempDir())
	if code, _, e := other.run("member", "accept", "--no-service"); code != ExitPermission || !strings.Contains(e, "already used") {
		t.Fatalf("spent invite: %d %s", code, e)
	}
}
