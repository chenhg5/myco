// Package codex adapts OpenAI Codex CLI's non-interactive mode (`codex exec`).
package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chenhg5/myco/internal/harness"
)

// Name is the registry key.
const Name = "codex"

type codex struct {
	opts harness.Options
	*harness.TranscriptFinder
}

// New is the harness.Factory for Codex.
func New(opts harness.Options) (harness.Harness, error) {
	if opts.Binary == "" {
		opts.Binary = "codex"
	}
	return &codex{opts: opts, TranscriptFinder: &harness.TranscriptFinder{Patterns: transcriptPatterns}}, nil
}

// transcriptPatterns: $CODEX_HOME/sessions/YYYY/MM/DD/rollout-<ts>-<id>.jsonl
func transcriptPatterns(id string) []string {
	return []string{filepath.Join(harness.HomeDir("CODEX_HOME", ".codex"), "sessions", "*", "*", "*", "rollout-*"+id+".jsonl")}
}

func (c *codex) Name() string { return Name }

// accessArgs maps access levels to Codex flags. `exec resume` rejects -s,
// so the sandbox is set through -c, which both forms accept.
var accessArgs = map[harness.Access][]string{
	harness.AccessReadonly:  {"-c", `sandbox_mode="read-only"`},
	// Network on so the agent can reach the hub with `myco` (reply, hold,
	// delegate); file writes stay confined to the workspace.
	harness.AccessWorkspace: {"-c", `sandbox_mode="workspace-write"`, "-c", "sandbox_workspace_write.network_access=true"},
	harness.AccessFull:      {"--dangerously-bypass-approvals-and-sandbox"},
}

// Check implements harness.Checker.
func (c *codex) Check(ctx context.Context) harness.Check {
	return harness.CheckBinary(ctx, c.opts.Binary, "--version")
}

// event is the subset of `codex exec --json` JSONL events we use.
type event struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id"`
	Message  string `json:"message"`
	Error    *struct {
		Message string `json:"message"`
	} `json:"error"`
	Item struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Message string `json:"message"`
	} `json:"item"`
}

func (c *codex) Run(ctx context.Context, req harness.Request) (harness.Result, error) {
	dir, err := os.MkdirTemp("", "myco-codex-")
	if err != nil {
		return harness.Result{}, err
	}
	defer os.RemoveAll(dir)
	lastMsg := filepath.Join(dir, "last.txt")

	// codex exec [flags] [resume <id>] -   (prompt read from stdin)
	args := []string{"exec", "--json", "--output-last-message", lastMsg}
	if c.opts.Model != "" {
		args = append(args, "-m", c.opts.Model)
	}
	args = append(args, accessArgs[c.opts.Access]...)
	args = append(args, c.opts.Args...)
	if req.SessionID != "" {
		args = append(args, "resume", req.SessionID)
	}
	args = append(args, "-")

	out, runErr := harness.Exec(ctx, harness.Process{
		Binary: c.opts.Binary, Args: args, Stdin: req.Prompt, Workdir: req.Workdir, Env: req.Env,
	})
	res, failure := parseEvents(out.Stdout)
	if b, err := os.ReadFile(lastMsg); err == nil && strings.TrimSpace(string(b)) != "" {
		res.Output = strings.TrimSpace(string(b))
	}
	if req.SessionID != "" && res.SessionID == "" {
		res.SessionID = req.SessionID
	}
	if runErr != nil && strings.Contains(runErr.Error()+failure, "already has an active writer") {
		return res, fmt.Errorf("%w: codex thread %s is open elsewhere (interactive session?)", harness.ErrSessionBusy, req.SessionID)
	}
	if runErr != nil && failure != "" {
		// The real cause is in the JSON events; stderr is mostly warnings.
		return res, fmt.Errorf("codex failed: %s", failure)
	}
	return res, runErr
}

// parseEvents extracts the session id, the final agent message and, if the
// run failed, the most specific error in the event stream: turn.failed beats
// error events, which beat non-fatal "error" items (Codex also uses those for
// warnings such as missing model metadata).
func parseEvents(stdout string) (harness.Result, string) {
	var (
		res                         harness.Result
		turnFailed, errs, itemNotes []string
	)
	sc := bufio.NewScanner(strings.NewReader(stdout))
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var ev event
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch {
		case ev.Type == "thread.started" && ev.ThreadID != "":
			res.SessionID = ev.ThreadID
		case ev.Type == "item.completed" && ev.Item.Type == "agent_message":
			res.Output = ev.Item.Text
		case ev.Type == "turn.failed" && ev.Error != nil && ev.Error.Message != "":
			turnFailed = append(turnFailed, unwrapAPIError(ev.Error.Message))
		case ev.Type == "error" && ev.Message != "":
			errs = append(errs, unwrapAPIError(ev.Message))
		case ev.Type == "item.completed" && ev.Item.Type == "error" && ev.Item.Message != "":
			itemNotes = append(itemNotes, ev.Item.Message)
		}
	}
	for _, group := range [][]string{turnFailed, errs, itemNotes} {
		if len(group) > 0 {
			return res, strings.Join(dedupe(group), "; ")
		}
	}
	return res, ""
}

// unwrapAPIError turns `{"error":{"message":"..."}}` payloads into the
// human-readable message.
func unwrapAPIError(s string) string {
	var v struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(s), &v) == nil && v.Error.Message != "" {
		if v.Error.Code != "" {
			return v.Error.Message + " (" + v.Error.Code + ")"
		}
		return v.Error.Message
	}
	return s
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
