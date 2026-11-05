package harness

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
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
}

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

// Exec runs p, killing its whole process group when ctx is done.
func Exec(ctx context.Context, p Process) (ProcessResult, error) {
	cmd := exec.CommandContext(ctx, p.Binary, p.Args...)
	cmd.Dir = p.Workdir
	if p.CleanEnv {
		cmd.Env = append([]string{}, p.Env...)
	} else {
		cmd.Env = append(filterEnv(os.Environ(), p.UnsetEnv), p.Env...)
	}
	cmd.Stdin = strings.NewReader(p.Stdin)
	var stdout, stderr limitedBuffer
	stdout.max, stderr.max = maxCapture, maxCapture
	if p.MaxOutput > 0 {
		stdout.max, stderr.max = p.MaxOutput, p.MaxOutput
	}
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	setProcessGroup(cmd)
	cmd.WaitDelay = interruptGrace + 2*time.Second
	err := cmd.Run()
	res := ProcessResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: -1, Truncated: stdout.cut || stderr.cut}
	if cmd.ProcessState != nil {
		res.ExitCode, res.Started = cmd.ProcessState.ExitCode(), true
	}
	if ctx.Err() != nil {
		return res, fmt.Errorf("%s: %w", p.Binary, ctx.Err())
	}
	if err != nil {
		return res, fmt.Errorf("%s exited: %v: %s", p.Binary, err, tail(errorLines(res.Stderr), 2000))
	}
	return res, nil
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
