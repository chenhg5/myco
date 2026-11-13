package cli

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/config"
)

type followHub struct {
	mu     sync.Mutex
	events []api.TraceEvent
	open   bool
	last   int
}

func (h *followHub) PostTrace(_ context.Context, _ string, events []api.TraceEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, events...)
	return nil
}

func (h *followHub) TaskOpen(context.Context, string) (bool, int, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.open, h.last, false, nil
}

func (h *followHub) kinds() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, ev := range h.events {
		out = append(out, ev.Kind)
	}
	return out
}

// Lines as Claude Code writes them to its session file.
const (
	sessionOld      = `{"type":"assistant","message":{"content":[{"type":"text","text":"before the task"}]}}`
	sessionThinking = `{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"read the handler first"}]}}`
	sessionTool     = `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./..."}}]}}`
	sessionResult   = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`
)

func TestFollowTraceUploadsNewSessionLines(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(file, []byte(sessionOld+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := &env{cfg: &config.Config{Agent: "me"}, defaultPath: filepath.Join(dir, "config.yaml")}
	hub := &followHub{open: true, last: 4}
	done := make(chan int, 1)
	go func() {
		n, err := e.followTrace(context.Background(), hub, "t1", file, time.Minute, false, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err != nil {
			t.Error(err)
		}
		done <- n
	}()
	// Written after it started, the second line in two pieces.
	time.Sleep(200 * time.Millisecond)
	f, _ := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0o600)
	f.WriteString(sessionThinking + "\n" + sessionTool[:40])
	f.Sync()
	time.Sleep(1500 * time.Millisecond)
	f.WriteString(sessionTool[40:] + "\n" + sessionResult + "\n")
	f.Close()
	deadline := time.Now().Add(5 * time.Second)
	for len(hub.kinds()) < 3 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if got := strings.Join(hub.kinds(), ","); got != "thinking,tool,tool_result" {
		t.Fatalf("steps uploaded: %s (the line from before the task must not be)", got)
	}
	hub.mu.Lock()
	first := hub.events[0].Seq
	hub.mu.Unlock()
	if first != 5 {
		t.Fatalf("steps continue after the hub's last one (4), got seq %d", first)
	}
	// Another task takes over this session: the follower stops.
	if err := os.WriteFile(e.followMarker(), []byte("t2"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-done:
		if n != 3 {
			t.Fatalf("steps: %d", n)
		}
	case <-time.After(followCheckEvery + 5*time.Second):
		t.Fatal("the follower did not stop when another task took over")
	}
}

func TestStartFollowerOnlyForSessionsWithoutARunner(t *testing.T) {
	e := &env{cfg: &config.Config{Agent: "me"}, defaultPath: filepath.Join(t.TempDir(), "config.yaml")}
	for name, env := range map[string]map[string]string{
		"under a runner": {"MYCO_MESSAGE_ID": "msg_1", "CLAUDE_CODE_SESSION_ID": "s"},
		"turned off":     {"MYCO_NO_TRACE_FOLLOW": "1", "CLAUDE_CODE_SESSION_ID": "s"},
		"no session":     {},
	} {
		e.in = invocationWithEnv(env)
		if got := e.startFollower("t1"); got != "" {
			t.Fatalf("%s: started a follower for %s", name, got)
		}
	}
}

func invocationWithEnv(env map[string]string) *cmdkit.Invocation {
	return &cmdkit.Invocation{Getenv: func(k string) string { return env[k] }}
}

func TestStepsOfPicksTheCLIByFile(t *testing.T) {
	codexLine := []byte(`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}}`)
	if evs := stepsOf("/x/.codex/sessions/2026/11/14/rollout-2026-11-14T08-00-00-t.jsonl")(codexLine); len(evs) != 1 || evs[0].Text != "hi" {
		t.Fatalf("codex: %+v", evs)
	}
	if evs := stepsOf("/x/.claude/projects/p/s.jsonl")([]byte(sessionThinking)); len(evs) != 1 || evs[0].Kind != "thinking" {
		t.Fatalf("claude: %+v", evs)
	}
}
