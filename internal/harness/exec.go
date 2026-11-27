package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/processwatch"
)

// Process describes a child process run by a harness.
type Process struct {
	Binary  string
	Args    []string
	Stdin   string
	Workdir string
	Env     []string
	// UnsetEnv removes inherited variables (by name) before Env is applied.
	UnsetEnv []string
	// CleanEnv starts from an empty environment instead of the runner's.
	CleanEnv bool
	// MaxOutput caps captured stdout/stderr each (default 8 MiB).
	MaxOutput int
	// OnStdoutLine, when set, gets each line of stdout as it is written
	// (without the newline), for live views; stdout is still captured.
	OnStdoutLine func(line []byte)
	// OnStderrLine is the same for stderr; the two may be called concurrently.
	OnStderrLine func(line []byte)
	// OnStarted runs after the OS has successfully started the child process.
	OnStarted func()
	// SilentStart, when set, stops a process that has written nothing to
	// stdout this long after it started (an agent CLI stuck before its
	// first event: no network to its model, a login prompt, a hung tool
	// server). The error is ErrSilentStart with the end of its stderr.
	SilentStart time.Duration
}

// ErrSilentStart: the process wrote nothing to stdout in Process.SilentStart.
var ErrSilentStart = errors.New("it started but printed nothing")

// ProcessResult holds captured output.
type ProcessResult struct {
	Stdout string
	Stderr string
	// ExitCode is the process exit status (-1 if it did not exit normally).
	ExitCode int
	// Truncated reports that stdout or stderr exceeded MaxOutput.
	Truncated bool
	// Started reports that the process ran (false: it could not start).
	Started bool
}

const maxCapture = 8 << 20

// Command prepares p (binary, arguments, directory, environment) in its own
// process group: when ctx is done the group is interrupted, then killed.
// Stdin, stdout and stderr are left for the caller.
// ManagedCommand retains its process scope after Wait succeeds.
type ManagedCommand struct {
	*exec.Cmd
	scope      *processwatch.Scope
	prepareErr error
}

func (c *ManagedCommand) Start() error {
	if c.prepareErr != nil {
		return c.prepareErr
	}
	if c.scope != nil {
		return c.scope.Start()
	}
	return c.Cmd.Start()
}

// StopForeground closes a transport after a successful turn without ending
// the scope: background children remain owned until explicit session cleanup.
func (c *ManagedCommand) StopForeground() error {
	if c.Process == nil {
		return os.ErrProcessDone
	}
	return c.Process.Kill()
}

func (c *ManagedCommand) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

func Command(ctx context.Context, p Process) *ManagedCommand {
	cmd := exec.CommandContext(ctx, p.Binary, p.Args...)
	cmd.Dir = p.Workdir
	if p.CleanEnv {
		cmd.Env = append([]string{}, p.Env...)
	} else {
		cmd.Env = append(filterEnv(os.Environ(), p.UnsetEnv), p.Env...)
	}
	setProcessGroup(cmd)
	cmd.WaitDelay = interruptGrace + 2*time.Second
	managed := &ManagedCommand{Cmd: cmd}
	if manager, owner := processwatch.FromContext(ctx); manager != nil {
		managed.scope, managed.prepareErr = manager.PrepareContext(ctx, cmd, owner)
		if managed.scope != nil {
			cmd.Cancel = managed.scope.Cancel
		}
	}
	return managed
}

// Exec runs p, killing its whole process group when ctx is done.
func Exec(parent context.Context, p Process) (ProcessResult, error) {
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	cmd := Command(ctx, p)
	cmd.Stdin = strings.NewReader(p.Stdin)
	var stdout, stderr limitedBuffer
	stdout.max, stderr.max = maxCapture, maxCapture
	if p.MaxOutput > 0 {
		stdout.max, stderr.max = p.MaxOutput, p.MaxOutput
	}
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	var lines, errLines *lineTee
	if p.OnStdoutLine != nil {
		lines = &lineTee{next: &stdout, fn: p.OnStdoutLine}
		cmd.Stdout = lines
	}
	if p.OnStderrLine != nil {
		errLines = &lineTee{next: &stderr, fn: p.OnStderrLine}
		cmd.Stderr = errLines
	}
	if p.SilentStart > 0 {
		heard := make(chan struct{})
		cmd.Stdout = &firstWrite{next: cmd.Stdout, heard: heard}
		go func() {
			t := time.NewTimer(p.SilentStart)
			defer t.Stop()
			select {
			case <-heard:
			case <-ctx.Done():
			case <-t.C:
				cancel(ErrSilentStart)
			}
		}()
	}
	err := cmd.Start()
	if err == nil {
		if p.OnStarted != nil {
			p.OnStarted()
		}
		err = cmd.Wait()
	}
	if lines != nil {
		lines.flush()
	}
	if errLines != nil {
		errLines.flush()
	}
	res := ProcessResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: -1, Truncated: stdout.cut || stderr.cut}
	if cmd.ProcessState != nil {
		res.ExitCode, res.Started = cmd.ProcessState.ExitCode(), true
	}
	if errors.Is(context.Cause(ctx), ErrSilentStart) && parent.Err() == nil {
		why := strings.TrimSpace(tail(errorLines(res.Stderr), 1500))
		if why == "" {
			why = "(nothing on stderr either)"
		}
		return res, fmt.Errorf("%s: %w in %s; stopped. Its stderr: %s", p.Binary, ErrSilentStart, p.SilentStart, why)
	}
	if ctx.Err() != nil {
		return res, fmt.Errorf("%s: %w", p.Binary, ctx.Err())
	}
	if err != nil {
		return res, fmt.Errorf("%s exited: %v: %s", p.Binary, err, tail(errorLines(res.Stderr), 2000))
	}
	return res, nil
}

// firstWrite closes heard on the first write, then passes writes on.
type firstWrite struct {
	next  io.Writer
	heard chan struct{}
	once  sync.Once
}

func (w *firstWrite) Write(p []byte) (int, error) {
	if len(p) > 0 {
		w.once.Do(func() { close(w.heard) })
	}
	return w.next.Write(p)
}

// lineTee passes writes on and calls fn for every complete line.
type lineTee struct {
	next    io.Writer
	fn      func([]byte)
	partial []byte
}

// maxLine bounds a line held back while waiting for its newline.
const maxLine = 4 << 20

func (t *lineTee) Write(p []byte) (int, error) {
	n, err := t.next.Write(p)
	rest := p
	for {
		i := bytes.IndexByte(rest, '\n')
		if i < 0 {
			break
		}
		if len(t.partial) > 0 {
			t.fn(append(t.partial, rest[:i]...))
			t.partial = t.partial[:0]
		} else {
			t.fn(rest[:i])
		}
		rest = rest[i+1:]
	}
	if len(t.partial)+len(rest) <= maxLine {
		t.partial = append(t.partial, rest...)
	} else {
		t.partial = t.partial[:0] // a runaway line: skip it
	}
	return n, err
}

func (t *lineTee) flush() {
	if len(t.partial) > 0 {
		t.fn(t.partial)
		t.partial = nil
	}
}

// limitedBuffer keeps at most max bytes. It deliberately does not embed
// bytes.Buffer: io.Copy would use the embedded ReadFrom and bypass Write.
type limitedBuffer struct {
	buf bytes.Buffer
	max int
	cut bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	room := b.max - b.buf.Len()
	switch {
	case room <= 0:
		b.cut = b.cut || len(p) > 0
	case len(p) > room:
		b.buf.Write(p[:room])
		b.cut = true
	default:
		b.buf.Write(p)
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string { return b.buf.String() }

// Tail is the last n bytes of s, trimmed (for error messages).
func Tail(s string, n int) string { return tail(s, n) }

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return "..." + s[len(s)-n:]
	}
	return s
}

// errorLines drops noisy WARN/INFO/DEBUG log lines so the actual error
// survives truncation; it returns the input unchanged if nothing remains.
func errorLines(stderr string) string {
	var keep []string
	for _, l := range strings.Split(stderr, "\n") {
		u := strings.ToUpper(l)
		if strings.Contains(u, " WARN ") || strings.Contains(u, " INFO ") || strings.Contains(u, " DEBUG ") || strings.Contains(u, " TRACE ") {
			continue
		}
		if strings.TrimSpace(l) != "" {
			keep = append(keep, l)
		}
	}
	if len(keep) == 0 {
		return stderr
	}
	return strings.Join(keep, "\n")
}

func filterEnv(env, unset []string) []string {
	if len(unset) == 0 {
		return env
	}
	drop := map[string]bool{}
	for _, k := range unset {
		drop[k] = true
	}
	out := env[:0:0]
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if !drop[k] {
			out = append(out, kv)
		}
	}
	return out
}
