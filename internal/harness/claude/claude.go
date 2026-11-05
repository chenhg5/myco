// Package claude adapts Claude Code's headless mode (`claude -p`).
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chenhg5/myco/internal/harness"
)

// Name is the registry key.
const Name = "claude"

type claude struct{ opts harness.Options }

// New is the harness.Factory for Claude Code.
func New(opts harness.Options) (harness.Harness, error) {
	if opts.Binary == "" {
		opts.Binary = "claude"
	}
	return &claude{opts: opts}, nil
}

func (c *claude) Name() string { return Name }

// result mirrors the fields we need from `--output-format json`.
type result struct {
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
	IsError   bool   `json:"is_error"`
	Subtype   string `json:"subtype"`
}

func (c *claude) Run(ctx context.Context, req harness.Request) (harness.Result, error) {
	// Prompt goes through stdin to avoid argv length limits.
	args := []string{"-p", "--output-format", "json"}
	if req.SessionID != "" {
		args = append(args, "--resume", req.SessionID)
	}
	args = append(args, c.opts.Args...)
	out, err := harness.Exec(ctx, harness.Process{
		Binary: c.opts.Binary, Args: args, Stdin: req.Prompt, Workdir: req.Workdir, Env: req.Env,
	})
	var r result
	if jerr := json.Unmarshal([]byte(lastJSONLine(out.Stdout)), &r); jerr != nil {
		if err != nil {
			return harness.Result{}, err
		}
		return harness.Result{}, fmt.Errorf("claude: cannot parse JSON output: %v", jerr)
	}
	if r.IsError {
		msg := r.Result
		if msg == "" {
			msg = r.Subtype
		}
		return harness.Result{SessionID: r.SessionID}, fmt.Errorf("claude reported an error: %s", msg)
	}
	if err != nil {
		return harness.Result{SessionID: r.SessionID}, err
	}
	return harness.Result{Output: r.Result, SessionID: r.SessionID}, nil
}

// lastJSONLine tolerates stray non-JSON lines before the result object.
func lastJSONLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n{"); i >= 0 {
		return s[i+1:]
	}
	return s
}
