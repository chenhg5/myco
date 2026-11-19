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
	os.Chtimes(old, time.Now().AddDate(0, -7, 0), time.Now().AddDate(0, -7, 0))
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

// A session that joined two workspaces says both.
func TestLocalSessionsJoinedTwoWorkspaces(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	work := t.TempDir()
	id := "dddddddd-0000-0000-0000-000000000005"
	writeLines(t, filepath.Join(home, ".claude", "projects", "-work", id+".jsonl"),
		map[string]any{"type": "user", "cwd": work, "timestamp": "2026-11-10T08:00:00.000Z", "message": map[string]any{"content": "两个工作区"}})
	dir := filepath.Join(home, ".myco", "agents")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("hub: https://hub.example.com\nagent: mac-claude-a\ntoken: t\nharness: claude\nsession_id: "+id+"\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "b.yaml"), []byte("hub: https://hub.example.com/w/team\nagent: mac-claude-b\ntoken: t\nharness: claude\nsession_id: "+id+"\n"), 0o600)
	got := (&console.Local{Home: home}).LocalSessions(time.Now())
	if len(got) != 1 || len(got[0].JoinedAll) != 2 || got[0].Joined == nil || got[0].Joined.Agent != "mac-claude-a" {
		t.Fatalf("sessions: %+v", got)
	}
	if got[0].JoinedAll[1].Agent != "mac-claude-b" || got[0].JoinedAll[1].Hub != "https://hub.example.com/w/team" {
		t.Fatalf("joined_all: %+v", got[0].JoinedAll)
	}
}

// Sessions a script ran (claude -p, the Agent SDK, codex exec) are left out
// unless they joined, and one request asked again in the same folder shows
// once (the newest), unless a session of them joined.
func TestLocalSessionsScriptsAndRepeats(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	work := t.TempDir()
	n := 0
	claude := func(entry, text string, ago time.Duration) string {
		n++
		id := "eeeeeeee-0000-0000-0000-00000000000" + string(rune('0'+n))
		p := filepath.Join(home, ".claude", "projects", "-work", id+".jsonl")
		writeLines(t, p, map[string]any{"type": "user", "cwd": work, "entrypoint": entry,
			"timestamp": time.Now().Add(-ago).UTC().Format(time.RFC3339), "message": map[string]any{"content": text}})
		os.Chtimes(p, time.Now().Add(-ago), time.Now().Add(-ago))
		return id
	}
	newest := claude("claude-desktop", "say hello briefly", time.Hour)
	claude("claude-desktop", "say  hello briefly", 2*time.Hour)
	claude("cli", "say hello briefly", 3*time.Hour)
	claude("sdk-cli", "a script's prompt", 30*time.Minute)
	claude("sdk-ts", "an SDK app's prompt", 40*time.Minute)
	joined := claude("sdk-cli", "say hello briefly", 4*time.Hour)
	mine := claude("cli", "改一下首页", 5*time.Hour)
	exec := filepath.Join(home, ".codex", "sessions", "2026", "11", "08", "rollout-exec.jsonl")
	writeLines(t, exec,
		map[string]any{"type": "session_meta", "payload": map[string]any{"id": "0199-exec", "cwd": work, "originator": "codex_exec", "source": "exec"}},
		map[string]any{"type": "event_msg", "timestamp": "2026-11-08T10:00:00Z", "payload": map[string]any{"type": "user_message", "message": "run the checks"}})
	tui := filepath.Join(home, ".codex", "sessions", "2026", "11", "08", "rollout-tui.jsonl")
	writeLines(t, tui,
		map[string]any{"type": "session_meta", "payload": map[string]any{"id": "0199-tui", "cwd": work, "originator": "codex_cli_rs", "source": "cli"}},
		map[string]any{"type": "event_msg", "timestamp": "2026-11-08T09:00:00Z", "payload": map[string]any{"type": "user_message", "message": "run the checks"}})
	os.Chtimes(tui, time.Now().Add(-6*time.Hour), time.Now().Add(-6*time.Hour))
	os.MkdirAll(filepath.Join(home, ".myco", "agents"), 0o700)
	os.WriteFile(filepath.Join(home, ".myco", "agents", "a.yaml"), []byte("hub: https://hub.example.com\nagent: mac-claude-a\ntoken: t\nharness: claude\nsession_id: "+joined+"\n"), 0o600)

	// What people started is listed; what scripts ran is listed too, marked
	// (the App folds them away), each repeated prompt once.
	var ids, autos []string
	for _, s := range (&console.Local{Home: home}).LocalSessions(time.Now()) {
		if s.Auto && s.Joined == nil {
			autos = append(autos, s.SessionID)
		} else {
			ids = append(ids, s.SessionID)
		}
	}
	if want := []string{newest, joined, mine, "0199-tui"}; !slices.Equal(ids, want) {
		t.Fatalf("sessions: %v, want %v", ids, want)
	}
	if want := []string{"0199-exec", "eeeeeeee-0000-0000-0000-000000000004", "eeeeeeee-0000-0000-0000-000000000005"}; !slices.Equal(autos, want) {
		t.Fatalf("sessions programs ran: %v, want %v", autos, want)
	}
}

// Newer Codex versions write the request only as a message item, after the
// AGENTS.md and context items they add themselves.
func TestLocalSessionsCodexMessageItems(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	work := t.TempDir()
	user := func(text string) map[string]any {
		return map[string]any{"type": "response_item", "timestamp": "2026-11-17T05:00:00.000Z", "payload": map[string]any{
			"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": text}}}}
	}
	writeLines(t, filepath.Join(home, ".codex", "sessions", "2026", "11", "17", "rollout-new.jsonl"),
		map[string]any{"type": "session_meta", "payload": map[string]any{"id": "019a-new", "cwd": work, "originator": "Codex Desktop", "source": "vscode"}},
		map[string]any{"type": "response_item", "payload": map[string]any{"type": "message", "role": "developer", "content": []any{map[string]any{"type": "input_text", "text": "rules"}}}},
		user("# AGENTS.md instructions for "+work+"\n\n<INSTRUCTIONS>…</INSTRUCTIONS>"),
		user("<environment_context>…</environment_context>"),
		user("我的 iPhone 老是连不上工作区，帮我 debug 一下\n"),
	)
	got := (&console.Local{Home: home}).LocalSessions(time.Now())
	if len(got) != 1 || got[0].Title != "我的 iPhone 老是连不上工作区，帮我 debug 一下" {
		t.Fatalf("sessions: %+v", got)
	}
}

// A session's own title wins over its first request: Codex's thread name
// (its last line in session_index.jsonl), Claude Code's /rename title over
// the one it made.
func TestLocalSessionsOwnTitles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	work := t.TempDir()
	ask := func(text string) map[string]any {
		return map[string]any{"type": "user", "cwd": work, "message": map[string]any{"content": text}}
	}
	claudeDir := filepath.Join(home, ".claude", "projects", "-work")
	writeLines(t, filepath.Join(claudeDir, "11111111-2222-3333-4444-000000000001.jsonl"),
		ask("帮我看看为什么登录失败"),
		map[string]any{"type": "ai-title", "aiTitle": "登录失败排查"},
		map[string]any{"type": "custom-title", "customTitle": "旧名字"},
		map[string]any{"type": "custom-title", "customTitle": "登录 Bug"},
		map[string]any{"type": "ai-title", "aiTitle": "后来生成的标题"},
	)
	writeLines(t, filepath.Join(claudeDir, "11111111-2222-3333-4444-000000000002.jsonl"),
		ask("整理一下周报"), map[string]any{"type": "ai-title", "aiTitle": "周报整理"})
	writeLines(t, filepath.Join(claudeDir, "11111111-2222-3333-4444-000000000003.jsonl"), ask("只有第一句"))
	writeLines(t, filepath.Join(home, ".codex", "sessions", "2026", "11", "18", "rollout-a.jsonl"),
		map[string]any{"type": "session_meta", "payload": map[string]any{"id": "019a-a", "cwd": work}},
		map[string]any{"type": "event_msg", "payload": map[string]any{"type": "user_message", "message": "看看 WSL 为什么这么慢"}})
	writeLines(t, filepath.Join(home, ".codex", "session_index.jsonl"),
		map[string]any{"id": "019a-a", "thread_name": "WSL 网络"},
		map[string]any{"id": "019a-other", "thread_name": "别的"},
		map[string]any{"id": "019a-a", "thread_name": "排查 WSL 网络慢"})
	titles := map[string]string{}
	for _, s := range (&console.Local{Home: home}).LocalSessions(time.Now()) {
		titles[s.SessionID] = s.Title
	}
	want := map[string]string{
		"11111111-2222-3333-4444-000000000001": "登录 Bug",
		"11111111-2222-3333-4444-000000000002": "周报整理",
		"11111111-2222-3333-4444-000000000003": "只有第一句",
		"019a-a":                               "排查 WSL 网络慢",
	}
	for id, w := range want {
		if titles[id] != w {
			t.Errorf("%s: title %q, want %q", id, titles[id], w)
		}
	}
}
