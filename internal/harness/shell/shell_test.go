package shell

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/testutil"
)

func newShell(t *testing.T, command string, args ...string) harness.Harness {
	t.Helper()
	h, err := New(harness.Options{Command: command, Args: args})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestNewRequiresCommand(t *testing.T) {
	for _, cmd := range []string{"", "   ", "\n\t"} {
		if _, err := New(harness.Options{Command: cmd}); err == nil {
			t.Errorf("New(%q): want error", cmd)
		}
	}
	h, err := New(harness.Options{Command: "cat"})
	if err != nil || h.Name() != "shell" || h.Name() != Name {
		t.Fatalf("New: %v %v", h, err)
	}
	if got := h.(*shell).opts.Binary; got != harness.DefaultShell() {
		t.Fatalf("default binary = %q, want %s", got, harness.DefaultShell())
	}
}

func TestRunPromptOnStdinStdoutIsReply(t *testing.T) {
	testutil.NeedsPOSIXShell(t)
	h := newShell(t, `tr a-z A-Z`)
	res, err := h.Run(context.Background(), harness.Request{Prompt: "hello\nworld\n"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "HELLO\nWORLD" { // trimmed
		t.Fatalf("output = %q", res.Output)
	}
	if res.SessionID != "" {
		t.Fatalf("shell has no sessions, got %q", res.SessionID)
	}
}

func TestRunEnvWorkdirAndArgs(t *testing.T) {
	testutil.NeedsPOSIXShell(t)
	dir := t.TempDir()
	h := newShell(t, `printf '%s|%s|%s|%s|%s' "$0" "$1" "$2" "$MYCO_FROM" "$(pwd)"; echo noise >&2`, "first arg", "x;y")
	res, err := h.Run(context.Background(), harness.Request{
		Prompt: "ignored", Workdir: dir, Env: []string{"MYCO_FROM=alice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(dir)
	want := "myco-shell|first arg|x;y|alice|"
	if !strings.HasPrefix(res.Output, want) || !(strings.HasSuffix(res.Output, dir) || strings.HasSuffix(res.Output, real)) {
		t.Fatalf("output = %q, want %q + %s (stderr must not leak into the reply)", res.Output, want, dir)
	}
}

func TestRunNonZeroExitIsError(t *testing.T) {
	testutil.NeedsPOSIXShell(t)
	h := newShell(t, `echo partial; echo "it broke" >&2; exit 3`)
	res, err := h.Run(context.Background(), harness.Request{Prompt: "x"})
	if err == nil {
		t.Fatal("non-zero exit must be an error")
	}
	if !strings.Contains(err.Error(), "it broke") {
		t.Fatalf("error should carry stderr: %v", err)
	}
	if res.Output != "partial" {
		t.Fatalf("partial stdout = %q", res.Output)
	}
}

func TestRunMissingBinary(t *testing.T) {
	h, err := New(harness.Options{Command: "true", Binary: filepath.Join(t.TempDir(), "no-such-sh")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Run(context.Background(), harness.Request{}); err == nil {
		t.Fatal("missing binary must be an error")
	}
}

func TestRunTimeoutAndCancel(t *testing.T) {
	testutil.NeedsPOSIXShell(t)
	h := newShell(t, `exec sleep 30`)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := h.Run(ctx, harness.Request{Prompt: "x"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout: want DeadlineExceeded, got %v", err)
	}
	if d := time.Since(start); d > 4*time.Second {
		t.Fatalf("timeout took %s; the process was not interrupted promptly", d)
	}

	ctx2, cancel2 := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel2)
	if _, err := h.Run(ctx2, harness.Request{Prompt: "x"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: want Canceled, got %v", err)
	}
}

func TestCheck(t *testing.T) {
	h := newShell(t, "cat")
	c, ok := h.(harness.Checker)
	if !ok {
		t.Fatal("shell should implement harness.Checker")
	}
	got := c.Check(context.Background())
	if got.Error != "" || got.Version != "ok" || got.Binary != harness.DefaultShell() {
		t.Fatalf("Check = %+v, want version ok from %s", got, harness.DefaultShell())
	}
	bad, _ := New(harness.Options{Command: "cat", Binary: filepath.Join(t.TempDir(), "nope")})
	if got := bad.(harness.Checker).Check(context.Background()); got.Error == "" {
		t.Fatalf("missing shell must fail the check: %+v", got)
	}
}
