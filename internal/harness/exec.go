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
}

// ProcessResult holds captured output.
type ProcessResult struct {
	Stdout string
	Stderr string
}

const maxCapture = 8 << 20

// Exec runs p, killing its whole process group when ctx is done.
func Exec(ctx context.Context, p Process) (ProcessResult, error) {
	cmd := exec.CommandContext(ctx, p.Binary, p.Args...)
	cmd.Dir = p.Workdir
	cmd.Env = append(os.Environ(), p.Env...)
	cmd.Stdin = strings.NewReader(p.Stdin)
	var stdout, stderr limitedBuffer
	stdout.max, stderr.max = maxCapture, maxCapture
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	setProcessGroup(cmd)
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	res := ProcessResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if ctx.Err() != nil {
		return res, fmt.Errorf("%s: %w", p.Binary, ctx.Err())
	}
	if err != nil {
		return res, fmt.Errorf("%s exited: %v: %s", p.Binary, err, tail(res.Stderr, 2000))
	}
	return res, nil
}

type limitedBuffer struct {
	bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.Len(); room > 0 {
		if len(p) > room {
			b.Buffer.Write(p[:room])
		} else {
			b.Buffer.Write(p)
		}
	}
	return len(p), nil
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return "..." + s[len(s)-n:]
	}
	return s
}
