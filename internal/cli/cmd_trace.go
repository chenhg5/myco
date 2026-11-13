package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/harness/claude"
	"github.com/chenhg5/myco/internal/runner"
)

// A session that receives with inbox wait is not started by a runner, so
// nothing reports what it does while it works on a task. Its agent CLI
// writes every step to a session file anyway (Claude Code:
// ~/.claude/projects/<project>/<session>.jsonl); myco trace follow reads the
// new lines of that file and uploads them as the task's steps, the same live
// view a runner gives. myco message ack starts it in the background.

// followCheckEvery is how often a follower checks that its task is still
// open and still the one being worked on.
const followCheckEvery = 15 * time.Second

func traceCommands() *Command {
	follow := &Command{
		Name:    "follow",
		Summary: "Upload what this session does to a task as its steps, from the session's own file",
		Long: `For a session that receives with myco inbox wait (no runner reports its steps): reads the new
lines of the session file its agent CLI writes (Claude Code, found from CLAUDE_CODE_SESSION_ID)
and uploads them to the task, so its sender sees thoughts, commands and results live, as for a
runner. myco message ack starts it in the background; you rarely run it yourself. It stops when
the task is answered or failed, when another task is acked in this session, or after --for.
Set MYCO_NO_TRACE_FOLLOW=1 to turn the automatic start off.`,
		Examples: []string{"myco trace follow --id msg_abc123"},
		Flags: []Flag{
			{Name: "id", Kind: String, Required: true, Help: "The task (acked by this agent)"},
			{Name: "file", Kind: String, Help: "The session file (default: found from the session)"},
			{Name: "for", Kind: Duration, Default: "2h", Help: "Stop after this long"},
			{Name: "from-start", Kind: Bool, Help: "Read the file from its start, not from its current end"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			id := e.in.String("id")
			file := e.in.String("file")
			if file == "" {
				file = sessionFile(e.in.Getenv)
			}
			if file == "" {
				return errorf("not_found", ExitNotFound, "pass --file, or run it inside a Claude Code session", "no session file found for this session")
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "trace_follow", "task": id, "file": file})
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			n, err := e.followTrace(e.in.Ctx, &hubTask{c: c}, id, file, e.in.Duration("for"), e.in.Bool("from-start"), slog.New(slog.NewTextHandler(e.in.Stderr, nil)))
			if err != nil {
				return err
			}
			return e.p.Print(map[string]any{"task": id, "file": file, "steps": n}, func(w io.Writer) {
				fmt.Fprintf(w, "followed %s: %d steps uploaded\n", id, n)
			})
		}),
	}
	return (&Command{Name: "trace", Summary: "A task's steps, as its sender sees them live"}).Add(follow)
}

// sessionFile is this session's file, "" if the agent CLI keeps none we read.
func sessionFile(getenv func(string) string) string {
	if getenv == nil {
		return ""
	}
	return claude.TranscriptPath(getenv("CLAUDE_CODE_SESSION_ID"))
}

// tracePoster is what a follower needs from the hub.
type tracePoster interface {
	runner.TraceUploader
	TaskOpen(ctx context.Context, id string) (open bool, lastStep int, e2e bool, err error)
}

// hubTask is tracePoster over a hub client.
type hubTask struct {
	c       *client.Client
	counted bool
}

func (h *hubTask) PostTrace(ctx context.Context, id string, events []api.TraceEvent) error {
	return h.c.PostTrace(ctx, id, events)
}

// TaskOpen says whether the task is still open; the first call also says
// the last step the hub has for it (a follower started again continues
// after it).
func (h *hubTask) TaskOpen(ctx context.Context, id string) (bool, int, bool, error) {
	m, err := h.c.GetMessage(ctx, id)
	if err != nil {
		return false, 0, false, err
	}
	open := m.Status == "queued" || m.Status == "delivered" || m.Status == "acked" || m.Status == "held"
	last := 0
	if !h.counted {
		h.counted = true
		if t, err := h.c.Trace(ctx, id, 0); err == nil {
			for _, ev := range t.Events {
				last = max(last, ev.Seq)
			}
		}
	}
	return open, last, m.E2E, nil
}

// followTrace uploads the session's new steps to task id until it closes,
// another task takes over this session, the context ends, or limit passes.
func (e *env) followTrace(ctx context.Context, c tracePoster, id, file string, limit time.Duration, fromStart bool, log *slog.Logger) (int, error) {
	open, after, e2e, err := c.TaskOpen(ctx, id)
	if err != nil {
		return 0, err
	}
	if !open || e2e { // closed already, or its content must not reach the hub
		return 0, nil
	}
	f, err := os.Open(file)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	offset := int64(0)
	if !fromStart {
		if offset, err = f.Seek(0, io.SeekEnd); err != nil {
			return 0, err
		}
	}
	marker := e.followMarker()
	if marker != "" {
		_ = os.MkdirAll(filepath.Dir(marker), 0o700)
		_ = os.WriteFile(marker, []byte(id), 0o600)
	}
	tr := runner.NewTracer(ctx, c, id, runner.TraceFull, after, log)
	defer tr.Close()

	steps := 0
	end := time.Now().Add(limit)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	lastCheck := time.Now()
	var partial []byte
	for {
		// The new complete lines since offset.
		if _, err := f.Seek(offset, io.SeekStart); err == nil {
			r := bufio.NewReaderSize(f, 1<<20)
			for {
				line, err := r.ReadBytes('\n')
				offset += int64(len(line))
				if err != nil { // no newline yet: keep it for the next read
					partial = append(partial, line...)
					break
				}
				if len(partial) > 0 {
					line = append(partial, line...)
					partial = nil
				}
				for _, ev := range claude.TranscriptEvents(line) {
					tr.Add(ev)
					steps++
				}
			}
		}
		if time.Now().After(end) {
			return steps, nil
		}
		if time.Since(lastCheck) >= followCheckEvery {
			lastCheck = time.Now()
			if marker != "" {
				if b, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(b)) != id {
					return steps, nil // another task took over this session
				}
			}
			if open, _, _, err := c.TaskOpen(ctx, id); err == nil && !open {
				return steps, nil
			}
		}
		select {
		case <-ctx.Done():
			return steps, nil
		case <-tick.C:
		}
	}
}

// followMarker names the task this agent's session works on now.
func (e *env) followMarker() string {
	if e.cfg == nil || e.cfg.Agent == "" {
		return ""
	}
	return filepath.Join(config.Dir(e.defaultPath), "state", e.cfg.Agent+".following")
}

// startFollower runs `myco trace follow` for task id in the background, for
// a session no runner reports. Best effort: an ack never fails because of it.
func (e *env) startFollower(id string) string {
	g := e.in.Getenv
	if g == nil || g("MYCO_MESSAGE_ID") != "" || g("MYCO_NO_TRACE_FOLLOW") == "1" {
		return "" // a runner reports this run's steps itself, or turned off
	}
	file := sessionFile(g)
	if file == "" {
		return ""
	}
	bin, err := os.Executable()
	if err != nil {
		return ""
	}
	cmd := exec.Command(bin, "trace", "follow", "--id", id, "--file", file, "--format", "json")
	cmd.Env = append(os.Environ(), "MYCO_CONFIG="+e.cfgPath)
	if logPath := filepath.Join(config.Dir(e.defaultPath), "logs", "trace-follow.log"); os.MkdirAll(filepath.Dir(logPath), 0o700) == nil {
		if lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			cmd.Stdout, cmd.Stderr = lf, lf
			defer lf.Close()
		}
	}
	detachProcess(cmd)
	if err := cmd.Start(); err != nil {
		return ""
	}
	_ = cmd.Process.Release()
	return file
}
