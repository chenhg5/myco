package loadtest

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/pprof"
	"time"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/hubserver"
	"github.com/myco-ai/myco/internal/perf"
)

type childConfig struct {
	Dir, Marker, Admin string
	Profile            bool
}
type childReady struct {
	Addr string `json:"addr"`
}
type child struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	log   *os.File
	done  chan error
	URL   string
}

func serveChild(parent context.Context, in io.Reader, out, errOut io.Writer) error {
	dec := json.NewDecoder(io.LimitReader(in, 64<<10))
	var c childConfig
	if e := dec.Decode(&c); e != nil {
		return errors.New("invalid child configuration")
	}
	b, e := os.ReadFile(filepath.Join(c.Dir, ".loadtest"))
	if e != nil || string(b) != c.Marker || c.Marker == "" || c.Admin == "" {
		return errors.New("isolated child marker mismatch")
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	go func() { var stop struct{}; _ = dec.Decode(&stop); cancel() }()
	if c.Profile {
		f, e := os.OpenFile(filepath.Join(c.Dir, "cpu.pprof"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		if e = pprof.StartCPUProfile(f); e != nil {
			f.Close()
			return e
		}
		defer func() { pprof.StopCPUProfile(); f.Close() }()
		defer func() {
			f, e := os.OpenFile(filepath.Join(c.Dir, "heap.pprof"), os.O_CREATE|os.O_WRONLY, 0600)
			if e == nil {
				_ = pprof.WriteHeapProfile(f)
				_ = f.Close()
			}
		}()
	}
	return hubserver.Run(ctx, hubserver.Options{Addr: "127.0.0.1:0", DBPath: filepath.Join(c.Dir, "hub.db"), AdminToken: c.Admin, Policy: app.DefaultMessagePolicy(), OnlineWindow: time.Minute, Backups: -1, Perf: perf.NewTimings(), Log: slog.New(slog.NewJSONHandler(errOut, nil)), Version: "loadtest", Ready: func(addr string) { _ = json.NewEncoder(out).Encode(childReady{Addr: addr}) }})
}
func startChild(ctx context.Context, c childConfig) (*child, error) {
	exe, e := os.Executable()
	if e != nil {
		return nil, e
	}
	log, e := os.OpenFile(filepath.Join(c.Dir, "hub.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	cmd := exec.Command(exe, "serve-child")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "TMPDIR=" + os.Getenv("TMPDIR")}
	cmd.Stderr = log
	stdin, e := cmd.StdinPipe()
	if e != nil {
		log.Close()
		return nil, e
	}
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		stdin.Close()
		log.Close()
		return nil, e
	}
	if e = cmd.Start(); e != nil {
		stdin.Close()
		log.Close()
		return nil, e
	}
	ch := &child{cmd: cmd, stdin: stdin, log: log, done: make(chan error, 1)}
	go func() { ch.done <- cmd.Wait() }()
	if e = json.NewEncoder(stdin).Encode(c); e != nil {
		ch.stop()
		return nil, e
	}
	ready := make(chan childReady, 1)
	go func() { var r childReady; _ = json.NewDecoder(bufio.NewReader(stdout)).Decode(&r); ready <- r }()
	select {
	case r := <-ready:
		if r.Addr == "" {
			ch.stop()
			return nil, errors.New("isolated Hub failed; see hub.log")
		}
		ch.URL = "http://" + r.Addr
		return ch, nil
	case <-ctx.Done():
		ch.stop()
		return nil, ctx.Err()
	case <-time.After(30 * time.Second):
		ch.stop()
		return nil, errors.New("isolated Hub startup timed out")
	}
}
func (c *child) stop() error {
	if c == nil || c.stdin == nil {
		return nil
	}
	_ = json.NewEncoder(c.stdin).Encode(struct{}{})
	_ = c.stdin.Close()
	c.stdin = nil
	var e error
	select {
	case e = <-c.done:
	case <-time.After(10 * time.Second):
		_ = c.cmd.Process.Kill()
		e = <-c.done
	}
	_ = c.log.Close()
	if e != nil {
		return fmt.Errorf("isolated Hub exit: %w", e)
	}
	return nil
}
