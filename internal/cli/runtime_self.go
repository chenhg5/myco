package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

// An agent that receives in its own session (myco inbox wait) has no runner
// to say which model it runs on. When that session is Claude Code or Codex,
// the session itself says: the model of its latest turn is in the session's
// transcript, and Claude Code exports its reasoning effort (CLAUDE_EFFORT).
// `myco inbox wait` reports them, so people see them without the agent
// having to run `myco agent runtime set`.

// sessionRuntime is the CLI, model and effort of the Claude Code or Codex
// session this command runs in, as far as they can be read ("" if not).
func sessionRuntime(getenv func(string) string) (cli, model, effort string) {
	if id := getenv("CLAUDE_CODE_SESSION_ID"); id != "" {
		dir := getenv("CLAUDE_CONFIG_DIR")
		if dir == "" {
			home, _ := os.UserHomeDir()
			dir = filepath.Join(home, ".claude")
		}
		if path := newestMatch(filepath.Join(dir, "projects", "*", id+".jsonl")); path != "" {
			model = lastClaudeModel(path)
		}
		return "claude", model, strings.TrimSpace(getenv("CLAUDE_EFFORT"))
	}
	if id := getenv("CODEX_THREAD_ID"); id != "" {
		dir := getenv("CODEX_HOME")
		if dir == "" {
			home, _ := os.UserHomeDir()
			dir = filepath.Join(home, ".codex")
		}
		if path := newestMatch(filepath.Join(dir, "sessions", "*", "*", "*", "rollout-*"+id+".jsonl")); path != "" {
			model, effort = lastCodexTurn(path)
		}
		return "codex", model, effort
	}
	return "", "", ""
}

func newestMatch(pattern string) string {
	matches, _ := filepath.Glob(pattern)
	var newest string
	var at time.Time
	for _, m := range matches {
		if st, err := os.Stat(m); err == nil && st.ModTime().After(at) {
			newest, at = m, st.ModTime()
		}
	}
	return newest
}

// tailLines is the last lines of a file (at most its last 512 KiB; a cut
// first line is dropped), newest last.
func tailLines(path string) [][]byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	const max = 512 << 10
	st, err := f.Stat()
	if err != nil {
		return nil
	}
	start := int64(0)
	if st.Size() > max {
		start = st.Size() - max
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil
	}
	var lines [][]byte
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), max+1)
	for sc.Scan() {
		lines = append(lines, append([]byte(nil), sc.Bytes()...))
	}
	if start > 0 && len(lines) > 0 {
		lines = lines[1:]
	}
	return lines
}

// lastClaudeModel is the model of the latest reply in a Claude Code
// transcript (not a synthetic one, which Claude Code writes itself).
func lastClaudeModel(path string) string {
	lines := tailLines(path)
	for i := len(lines) - 1; i >= 0; i-- {
		if !bytes.Contains(lines[i], []byte(`"assistant"`)) {
			continue
		}
		var ev struct {
			Type    string `json:"type"`
			Message struct {
				Model string `json:"model"`
			} `json:"message"`
		}
		if json.Unmarshal(lines[i], &ev) != nil || ev.Type != "assistant" {
			continue
		}
		if m := strings.TrimSpace(ev.Message.Model); m != "" && !strings.HasPrefix(m, "<") {
			return m
		}
	}
	return ""
}

// lastCodexTurn is the model and effort of the latest turn in a Codex
// session file (its turn_context lines).
func lastCodexTurn(path string) (string, string) {
	lines := tailLines(path)
	for i := len(lines) - 1; i >= 0; i-- {
		if !bytes.Contains(lines[i], []byte(`"turn_context"`)) {
			continue
		}
		var ev struct {
			Type    string `json:"type"`
			Payload struct {
				Model  string `json:"model"`
				Effort string `json:"effort"`
			} `json:"payload"`
		}
		if json.Unmarshal(lines[i], &ev) != nil || ev.Type != "turn_context" {
			continue
		}
		if m := strings.TrimSpace(ev.Payload.Model); m != "" {
			return m, strings.TrimSpace(ev.Payload.Effort)
		}
	}
	return "", ""
}

// reportSessionRuntime tells the hub the model this session runs on, when
// it can be read and the hub has something else. Never for an agent its
// runner started (the runner reports), nor over a runner's report. Best
// effort: quiet, and a few seconds at most.
func reportSessionRuntime(ctx context.Context, c *client.Client, getenv func(string) string) {
	if c == nil || getenv("MYCO_MESSAGE_ID") != "" {
		return
	}
	cli, model, effort := sessionRuntime(getenv)
	if getenv("CLAUDE_CODE_REMOTE_SESSION_ID") != "" {
		cli = "claude-cloud"
	}
	account := runtimeAccount(cli, accountHome(getenv), getenv)
	if model == "" && account == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	me, err := c.Me(ctx)
	if err != nil {
		return
	}
	rt := api.Runtime{}
	if me.Runtime != nil {
		rt = *me.Runtime
	}
	if rt.ModelSource != "" && rt.ModelSource != "self" {
		if me.Binding.Harness != "claude-cloud" {
			return
		}
		model, effort = "", rt.Effort
	}
	if rt.Account == account && rt.Model == model && rt.Effort == effort && rt.ModelSource == "self" {
		return
	}
	rt.Account = runtimeAccount(me.Binding.Harness, accountHome(getenv), getenv)
	if model != "" {
		rt.Model, rt.ModelSource = model, "self"
	}
	rt.Effort, rt.EffortSource = effort, ""
	if effort != "" {
		rt.EffortSource = "self"
	}
	if h := me.Binding.Harness; h == "claude-cloud" || h == cli {
		rt.CLI = h
	} else if rt.CLI == "" {
		rt.CLI = cli
	}
	_ = c.ReportRuntime(ctx, rt)
}
