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

type codex struct{ opts harness.Options }

// New is the harness.Factory for Codex.
func New(opts harness.Options) (harness.Harness, error) {
	if opts.Binary == "" {
		opts.Binary = "codex"
	}
	return &codex{opts: opts}, nil
}

func (c *codex) Name() string { return Name }

// event is the subset of `codex exec --json` JSONL events we use.
type event struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id"`
	Item     struct {
		Type string `json:"type"`
		Text string `json:"text"`
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
	args = append(args, c.opts.Args...)
	if req.SessionID != "" {
		args = append(args, "resume", req.SessionID)
	}
	args = append(args, "-")

	out, runErr := harness.Exec(ctx, harness.Process{
		Binary: c.opts.Binary, Args: args, Stdin: req.Prompt, Workdir: req.Workdir, Env: req.Env,
	})
	res := parseEvents(out.Stdout)
	if b, err := os.ReadFile(lastMsg); err == nil && strings.TrimSpace(string(b)) != "" {
		res.Output = strings.TrimSpace(string(b))
	}
	if req.SessionID != "" && res.SessionID == "" {
		res.SessionID = req.SessionID
	}
	if runErr != nil && strings.Contains(runErr.Error(), "already has an active writer") {
		return res, fmt.Errorf("%w: codex thread %s is open elsewhere (interactive session?)", harness.ErrSessionBusy, req.SessionID)
	}
	return res, runErr
}

func parseEvents(stdout string) harness.Result {
	var res harness.Result
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
		}
	}
	return res
}
