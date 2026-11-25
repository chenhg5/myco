package main

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

type settings struct {
	Secret  string `json:"secret"`
	DataDir string `json:"data_dir"`
}

func run(ctx context.Context, input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 16<<10)
	if !scanner.Scan() {
		return errors.New("missing private configuration")
	}
	var cfg settings
	if json.Unmarshal(scanner.Bytes(), &cfg) != nil || len(cfg.Secret) < 32 || len(cfg.Secret) > 512 || !filepath.IsAbs(cfg.DataDir) {
		return errors.New("invalid private configuration")
	}
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return errors.New("private data directory unavailable")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	e, err := newEngine(ctx, cfg.DataDir)
	if err != nil {
		return err
	}
	defer e.close()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	server := &http.Server{Handler: secure(cfg.Secret, e.handler()), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	defer server.Close()
	if err = json.NewEncoder(output).Encode(map[string]any{"address": listener.Addr().String(), "protocol": protocol, "process_groups": browserGroupProtocol()}); err != nil {
		listener.Close()
		return err
	}
	go func() { _, _ = io.Copy(io.Discard, scannerReader(scanner)); cancel() }()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if e.idle(2 * time.Minute) {
					cancel()
					return
				}
			}
		}
	}()
	go func() { <-ctx.Done(); _ = server.Close() }()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Scanner may already have buffered bytes after the first line. Draining it
// preserves EOF ownership without interpreting further input as commands.
func scannerReader(s *bufio.Scanner) io.Reader { return &scanDrain{s: s} }

type scanDrain struct{ s *bufio.Scanner }

func (d *scanDrain) Read(p []byte) (int, error) {
	for d.s.Scan() {
	}
	if err := d.s.Err(); err != nil {
		return 0, err
	}
	return 0, io.EOF
}
func secure(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		_, hasOrigin := r.Header["Origin"]
		if err != nil || host != "127.0.0.1" || hasOrigin {
			http.Error(w, "forbidden", 403)
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if got == r.Header.Get("Authorization") || subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
		r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
		next.ServeHTTP(w, r)
	})
}
func main() {
	if handled, err := browserInternalMode(os.Args, os.Stdin); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, "myco-browser: private cleanup/launch failed")
			os.Exit(1)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "myco-browser:", err)
		os.Exit(1)
	}
}
