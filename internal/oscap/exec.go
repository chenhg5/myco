package oscap

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

func init() {
	register(
		&Cap{Name: "exec.run", About: "跑一条命令或一段 shell 脚本，跑完回退出码和输出", Fn: execRun},
		&Cap{Name: "exec.spawn", Job: true, About: "起一个程序：输出实时推回来，可以写入，可以停", Fn: execSpawn},
	)
}

type execArgs struct {
	// Argv: the program and its arguments (no shell); or Shell: a script
	// for the system's shell (sh on macOS and Linux, PowerShell on Windows).
	Argv  []string          `json:"argv"`
	Shell string            `json:"shell"`
	Cwd   string            `json:"cwd"`
	Env   map[string]string `json:"env"`
	Stdin string            `json:"stdin"`
	// StdinEncoding: "utf8" (default) or "base64".
	StdinEncoding string `json:"stdin_encoding"`
	// TimeoutMs: default 60 s for exec.run (at most an hour); exec.spawn has none unless given.
	TimeoutMs int64 `json:"timeout_ms"`
	// MaxOutput: bytes of stdout and of stderr kept by exec.run (default 1 MB, at most 16 MB).
	MaxOutput int `json:"max_output"`
}

func (env *Env) command(ctx context.Context, in *execArgs) (*exec.Cmd, error) {
	argv := in.Argv
	if in.Shell != "" {
		if len(argv) > 0 {
			return nil, errf(CodeInvalid, "give argv or shell, not both")
		}
		argv = shellArgv(env, in.Shell)
	}
	if len(argv) == 0 || argv[0] == "" {
		return nil, errf(CodeInvalid, "a command is needed (argv or shell)")
	}
	prog, err := env.LookPath(argv[0])
	if err != nil {
		return nil, errf(CodeNotFound, "%s is not installed here (not on PATH)", argv[0])
	}
	cmd := exec.CommandContext(ctx, prog, argv[1:]...)
	cmd.Env = env.Environ(in.Env)
	if in.Cwd != "" {
		d, err := env.abs(in.Cwd)
		if err != nil {
			return nil, err
		}
		cmd.Dir = d
	} else {
		cmd.Dir = env.Home
	}
	ownGroup(cmd)
	cmd.Cancel = func() error { return killTree(cmd) }
	cmd.WaitDelay = 3 * time.Second
	return cmd, nil
}

func outText(b []byte) (string, string) {
	if utf8.Valid(b) {
		return string(b), ""
	}
	return base64.StdEncoding.EncodeToString(b), "base64"
}

// capped keeps the first n bytes written to it.
type capped struct {
	buf       bytes.Buffer
	n         int
	truncated bool
}

func (c *capped) Write(p []byte) (int, error) {
	if left := c.n - c.buf.Len(); left > 0 {
		if len(p) > left {
			c.buf.Write(p[:left])
			c.truncated = true
		} else {
			c.buf.Write(p)
		}
	} else if len(p) > 0 {
		c.truncated = true
	}
	return len(p), nil
}

func execRun(ctx context.Context, env *Env, args json.RawMessage, _ Emit) (any, error) {
	var in execArgs
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	timeout := time.Duration(in.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Minute
	}
	if timeout > time.Hour {
		timeout = time.Hour
	}
	limit := clamp(in.MaxOutput, 1<<20, 1, 16<<20)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd, err := env.command(ctx, &in)
	if err != nil {
		return nil, err
	}
	stdin, err := content(in.Stdin, in.StdinEncoding)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = bytes.NewReader(stdin)
	out, errOut := &capped{n: limit}, &capped{n: limit}
	cmd.Stdout, cmd.Stderr = out, errOut
	start := time.Now()
	err = cmd.Run()
	res := map[string]any{"ms": time.Since(start).Milliseconds()}
	var ee *exec.ExitError
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		res["timed_out"] = true
		res["code"] = -1
	case errors.As(err, &ee):
		res["code"] = ee.ExitCode()
	case err != nil:
		return nil, err
	default:
		res["code"] = 0
	}
	s, enc := outText(out.buf.Bytes())
	res["stdout"] = s
	if enc != "" {
		res["stdout_encoding"] = enc
	}
	s, enc = outText(errOut.buf.Bytes())
	res["stderr"] = s
	if enc != "" {
		res["stderr_encoding"] = enc
	}
	if out.truncated || errOut.truncated {
		res["truncated"] = true
	}
	return res, nil
}

// streamer sends what a program writes as events, a piece at a time
// (joined when it writes fast), keeping UTF-8 characters whole.
type streamer struct {
	kind string
	emit Emit
	mu   *sync.Mutex
	rest []byte
}

func (s *streamer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := append(s.rest, p...)
	cut := len(b)
	// (hold back an unfinished UTF-8 character, up to 3 bytes)
	for i := 1; i <= 3 && i <= len(b); i++ {
		c := b[len(b)-i]
		if c < 0x80 {
			break
		}
		if c >= 0xC0 {
			if !utf8.FullRune(b[len(b)-i:]) {
				cut = len(b) - i
			}
			break
		}
	}
	s.rest = append([]byte(nil), b[cut:]...)
	if cut > 0 {
		s.send(b[:cut])
	}
	return len(p), nil
}

func (s *streamer) send(b []byte) {
	if t, enc := outText(b); enc == "" {
		s.emit(s.kind, t)
	} else {
		s.emit(s.kind, map[string]string{"base64": t})
	}
}

func (s *streamer) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.rest) > 0 {
		s.send(s.rest)
		s.rest = nil
	}
}

func execSpawn(ctx context.Context, env *Env, args json.RawMessage, emit Emit) (any, error) {
	var in execArgs
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	if in.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(in.TimeoutMs)*time.Millisecond)
		defer cancel()
	}
	cmd, err := env.command(ctx, &in)
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	out := &streamer{kind: "stdout", emit: emit, mu: &mu}
	errOut := &streamer{kind: "stderr", emit: emit, mu: &mu}
	cmd.Stdout, cmd.Stderr = out, errOut
	w, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	emit("started", map[string]int{"pid": cmd.Process.Pid})
	// what the page writes (myco.os job.write), then what was given at the start
	go func() {
		defer w.Close()
		if in.Stdin != "" {
			b, err := content(in.Stdin, in.StdinEncoding)
			if err == nil {
				_, _ = w.Write(b)
			}
		}
		ch := inputOf(ctx)
		if ch == nil {
			return
		}
		for {
			select {
			case b := <-ch:
				if b == nil {
					return
				}
				if _, err := w.Write(b); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	err = cmd.Wait()
	out.flush()
	errOut.flush()
	res := map[string]any{"ms": time.Since(start).Milliseconds(), "pid": cmd.Process.Pid}
	var ee *exec.ExitError
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		res["timed_out"], res["code"] = true, -1
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case errors.As(err, &ee):
		res["code"] = ee.ExitCode()
	case err != nil && !errors.Is(err, io.ErrClosedPipe) && !strings.Contains(err.Error(), "WaitDelay"):
		return nil, err
	default:
		res["code"] = cmd.ProcessState.ExitCode()
	}
	return res, nil
}
