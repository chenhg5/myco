package coordeval

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/config"
)

// CLI is a real agent CLI as the coordinator: `myco runner start` serves
// the coordinator's inbox with it, and `myco` on its PATH acts as the
// coordinator against the drill's hub. It spends tokens.
type CLI struct {
	// Harness is the CLI: claude or codex (any harness myco knows).
	Harness string
	// Myco is the myco executable (built from this tree).
	Myco string
	// Binary, Model, Access and Args go to the runner (--binary, --model,
	// --access, --arg). Access defaults to workspace.
	Binary string
	Model  string
	Access string
	Args   []string
	// Log receives the runner's log (the CLI's own output stays in it).
	Log io.Writer
}

// Name is the CLI's.
func (c CLI) Name() string { return c.Harness }

// roleNote is what the coordinator's working folder says about its job:
// who it is and how to reach the team, nothing about the scenarios.
const roleNote = `# You are the coordinator

You are ` + "`lead`" + `, the coordinator of the ` + "`drill`" + ` project in a myco workspace. A person
sends you goals. Split each goal among the agents of the project, follow
their work, check their results, and answer the person with what they asked
for. The agents are other AI agents; you reach them with the ` + "`myco`" + ` command.

- ` + "`myco project get --name drill`" + ` and ` + "`myco agent list`" + `: the team, what each does,
  whether it is online and how much it has queued.
- ` + "`myco message send --to <agent> --type task --body \"...\"`" + `: give one a task;
  ` + "`myco message wait --id <id>`" + ` or ` + "`myco message get --id <id>`" + `: its progress and reply.
- ` + "`myco message stop --id <id>`" + `: stop a task you sent.
- ` + "`myco schema`" + ` lists every command; ` + "`myco skill show`" + ` explains them.

Your final answer in this session is your reply to the person.
`

// Start runs the runner for the coordinator until stopped.
func (c CLI) Start(ctx context.Context, w *World) (func(), error) {
	if c.Myco == "" {
		return nil, errors.New("coordeval: CLI.Myco (the myco executable) is required")
	}
	myco, err := filepath.Abs(c.Myco)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(w.opts.Dir, Lead)
	work := filepath.Join(dir, "work")
	if err := os.MkdirAll(work, 0o700); err != nil {
		return nil, err
	}
	for _, f := range []string{"CLAUDE.md", "AGENTS.md"} {
		if err := os.WriteFile(filepath.Join(work, f), []byte(roleNote), 0o600); err != nil {
			return nil, err
		}
	}
	access := c.Access
	if access == "" {
		access = "workspace"
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{Hub: w.URL, Agent: Lead, Token: w.CoordToken,
		Runner: config.Runner{Harness: c.Harness, Workdir: work, Access: access, Model: c.Model, Binary: c.Binary, Args: c.Args}}
	if err := config.Save(cfgPath, cfg); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, myco, "--config", cfgPath, "runner", "start", "--log-level", "info")
	cmd.Dir = work
	cmd.Env = append(os.Environ(),
		"MYCO_CONFIG="+cfgPath,
		"MYCO_HUB="+w.URL,
		"PATH="+filepath.Dir(myco)+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Env = withoutEnv(cmd.Env, "MYCO_TOKEN", "MYCO_ADMIN_TOKEN", "MYCO_AGENT", "MYCO_PROJECT")
	logw := c.Log
	if logw == nil {
		logw = io.Discard
	}
	cmd.Stdout, cmd.Stderr = logw, logw
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("starting the runner: %w", err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	return func() { cancel(); <-done }, nil
}

// withoutEnv drops variables that would point myco at another identity.
func withoutEnv(env []string, names ...string) []string {
	out := env[:0:0]
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		drop := false
		for _, n := range names {
			if k == n {
				drop = true
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}
