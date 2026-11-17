package console_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/console"
)

func writeLines(t *testing.T, path string, lines ...any) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o700)
	var b strings.Builder
	for _, l := range lines {
		j, _ := json.Marshal(l)
		b.Write(j)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLocalSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	work := t.TempDir()
	claudeID := "11111111-2222-3333-4444-555555555555"
	writeLines(t, filepath.Join(home, ".claude", "projects", "-work", claudeID+".jsonl"),
		map[string]any{"type": "user", "cwd": work, "message": map[string]any{"content": "<command-name>/clear</command-name>"}},
		map[string]any{"type": "user", "cwd": work, "isSidechain": true, "message": map[string]any{"content": "a subagent's prompt"}},
		map[string]any{"type": "user", "cwd": work, "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "  帮我把   README 改好看一点  "}}}},
	)
	// Not a session file (subagent transcripts and the like).
	writeLines(t, filepath.Join(home, ".claude", "projects", "-work", "agent-1234.jsonl"), map[string]any{"type": "user", "cwd": work})
	codexPath := filepath.Join(home, ".codex", "sessions", "2026", "11", "08", "rollout-2026-11-08T10-00-00-0199aaaa.jsonl")
	writeLines(t, codexPath,
		map[string]any{"type": "session_meta", "payload": map[string]any{"id": "0199aaaa-bbbb", "cwd": work}},
		map[string]any{"type": "event_msg", "payload": map[string]any{"type": "user_message", "message": strings.Repeat("修复登录页的样式问题", 20)}},
	)
	old := filepath.Join(home, ".codex", "sessions", "2025", "01", "01", "rollout-old.jsonl")
	writeLines(t, old, map[string]any{"type": "session_meta", "payload": map[string]any{"id": "old", "cwd": work}})
	os.Chtimes(old, time.Now().AddDate(0, -3, 0), time.Now().AddDate(0, -3, 0))
	os.Chtimes(codexPath, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	// The Claude session already joined a hub (a local agent profile).
	os.MkdirAll(filepath.Join(home, ".myco", "agents"), 0o700)
	os.WriteFile(filepath.Join(home, ".myco", "agents", "mac-claude-5555.yaml"), []byte("hub: https://hub.example.com\nagent: mac-claude-5555\ntoken: t\nharness: claude\nsession_id: "+claudeID+"\n"), 0o600)

	got := (&console.Local{Home: home}).LocalSessions(time.Now())
	if len(got) != 2 {
		t.Fatalf("sessions: %+v", got)
	}
	c, x := got[0], got[1] // newest first
	if c.CLI != "claude" || c.SessionID != claudeID || c.Workdir != work || c.Title != "帮我把 README 改好看一点" ||
		c.Joined == nil || c.Joined.Agent != "mac-claude-5555" {
		t.Fatalf("claude session: %+v", c)
	}
	if x.CLI != "codex" || x.SessionID != "0199aaaa-bbbb" || !strings.HasSuffix(x.Title, "…") || len([]rune(x.Title)) != 81 || x.Joined != nil {
		t.Fatalf("codex session: %+v", x)
	}
}

func TestLocalJoin(t *testing.T) {
	r := newRig(t)
	home := t.TempDir()
	work := t.TempDir()
	var calls [][]string
	var runnerRunning bool
	var restarts int
	run := func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
		calls = append(calls, args)
		get := func(k string) string {
			for _, kv := range env {
				if v, ok := strings.CutPrefix(kv, k+"="); ok {
					return v
				}
			}
			return ""
		}
		switch args[0] {
		case "join":
			if get("MYCO_APP_WORKDIR") != work || get("MYCO_HUB") != r.hubURL || !slices.Contains(args, "--session-id") {
				t.Errorf("myco join must run in the session dir against the hub: %v %v", args, env)
			}
			// What myco join does: enroll with the one-time token.
			name := args[slices.Index(args, "--name")+1]
			ec, _ := client.New(get("MYCO_HUB"), get("MYCO_ENROLL_TOKEN"))
			res, err := ec.Enroll(ctx, api.EnrollRequest{Name: name, Binding: api.Binding{Harness: "claude", SessionID: "s-1", Workdir: work}}, false)
			if err != nil {
				return []byte(`{"error":"x","message":"` + err.Error() + `"}`), errors.New("exit 4")
			}
			return json.Marshal(map[string]any{"agent": res.Agent, "created": res.Created})
		case "runner":
			if args[2] == "status" {
				return json.Marshal(map[string]any{"running": runnerRunning})
			}
			if args[2] == "restart" {
				restarts++
			}
			runnerRunning = true
			return []byte(`{}`), nil
		}
		return nil, errors.New("unexpected")
	}
	l := &console.Local{Home: home, Device: "test-mac", Run: run, Dirs: []string{filepath.Join(home, ".local", "bin")},
		LookPath: func(string) (string, error) { return "/x/myco", nil }}
	b := &console.Backend{Store: console.NewStore(r.store), Secrets: r.secrets, Local: l}
	r.app.Config.Handler = b.Handler()
	code, m, _ := r.do("POST", "/app/hubs", map[string]string{"name": "Main", "url": r.hubURL, "token": r.consoleToken("t")})
	if code != 201 {
		t.Fatalf("add hub: %d %v", code, m)
	}
	id := m["id"].(string)

	if code, _, _ := r.do("POST", "/app/hubs/"+id+"/local-join", map[string]string{"cli": "claude", "session_id": "s-1", "workdir": "/nope"}); code != 400 {
		t.Fatalf("missing dir: %d", code)
	}
	code, m, _ = r.do("POST", "/app/hubs/"+id+"/local-join", map[string]string{"cli": "claude", "session_id": "0199-abcd-ef12", "workdir": work})
	if code != 200 || m["agent"] != "test-mac-claude-ef12" || m["runner"] != "running" {
		t.Fatalf("local join: %d %v", code, m)
	}
	if !runnerRunning {
		t.Fatal("the runner service must be started when it is not running")
	}
	a, err := r.admin.GetAgent(context.Background(), "test-mac-claude-ef12")
	if err != nil || !slices.Contains(a.Labels, "machine=test-mac") {
		t.Fatalf("joined agent: %v %+v", err, a)
	}
	// "Bring it back online": the runner here restarts (it is running now).
	if code, m, _ := r.do("POST", "/app/local/runner", nil); code != 200 || m["runner"] != "running" || restarts != 1 {
		t.Fatalf("restart runner: %d %v (restarts %d)", code, m, restarts)
	}
	// The one-time enrollment is used up.
	enrs, _ := r.admin.ListEnrollments(context.Background())
	for _, e := range enrs {
		if e.Note == "myco app: local session" && e.Uses != 1 {
			t.Fatalf("enrollment: %+v", e)
		}
	}
}

// One conversation, several transcripts: a copy made to join (a fork keeps
// the first request's time and text), the same session under two project
// folders, and sessions kept in ~/.config/claude. Each shows once, the
// newest, joined when any copy joined.
func TestLocalSessionsCopiesShowOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	work := t.TempDir()
	first := func(text string) map[string]any {
		return map[string]any{"type": "user", "cwd": work, "timestamp": "2026-11-10T08:00:00.000Z", "message": map[string]any{"content": text}}
	}
	orig := "aaaaaaaa-0000-0000-0000-000000000001"
	fork := "aaaaaaaa-0000-0000-0000-000000000002"
	other := "bbbbbbbb-0000-0000-0000-000000000003"
	xdg := "cccccccc-0000-0000-0000-000000000004"
	at := func(path string, ago time.Duration) {
		os.Chtimes(path, time.Now().Add(-ago), time.Now().Add(-ago))
	}
	p := filepath.Join(home, ".claude", "projects", "-work", orig+".jsonl")
	writeLines(t, p, first("整理发布流程"))
	at(p, 3*time.Hour)
	p = filepath.Join(home, ".claude", "projects", "-work-sub", orig+".jsonl") // the same session, another folder
	writeLines(t, p, first("整理发布流程"))
	at(p, 2*time.Hour)
	p = filepath.Join(home, ".claude", "projects", "-work", fork+".jsonl") // the copy that joined
	writeLines(t, p, first("整理发布流程"), map[string]any{"type": "assistant", "cwd": work})
	at(p, time.Hour)
	p = filepath.Join(home, ".claude", "projects", "-work", other+".jsonl") // same text, another time: another conversation
	writeLines(t, p, map[string]any{"type": "user", "cwd": work, "timestamp": "2026-11-12T08:00:00.000Z", "message": map[string]any{"content": "整理发布流程"}})
	at(p, 4*time.Hour)
	p = filepath.Join(home, ".config", "claude", "projects", "-work", xdg+".jsonl")
	writeLines(t, p, first("在新位置的会话"))
	at(p, 5*time.Hour)
	os.MkdirAll(filepath.Join(home, ".myco", "agents"), 0o700)
	os.WriteFile(filepath.Join(home, ".myco", "agents", "mac-claude-0002.yaml"), []byte("hub: https://hub.example.com\nagent: mac-claude-0002\ntoken: t\nharness: claude\nsession_id: "+fork+"\n"), 0o600)

	got := (&console.Local{Home: home}).LocalSessions(time.Now())
	var ids []string
	for _, s := range got {
		ids = append(ids, s.SessionID)
	}
	if !slices.Equal(ids, []string{fork, other, xdg}) {
		t.Fatalf("sessions: %v", ids)
	}
	if got[0].Joined == nil || got[0].Joined.Agent != "mac-claude-0002" || got[1].Joined != nil {
		t.Fatalf("joined: %+v %+v", got[0].Joined, got[1].Joined)
	}
}
