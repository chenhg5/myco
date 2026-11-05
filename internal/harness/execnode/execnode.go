// Package execnode is the "exec" harness: each task's body is a shell
// command, run on this machine, and the reply is an api.ExecResult (exit
// code, stdout, stderr). It is remote command execution by design, so it
// is fenced in: only the agent's allow_from senders, only tasks, an
// optional command allowlist, a clean environment (never the agent token),
// a timeout, output caps and an audit log.
package execnode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

// Name is the registry key.
const Name = "exec"

// Exit codes for commands the node does not run to completion (same
// conventions as GNU timeout and POSIX shells).
const (
	ExitTimedOut = 124
	ExitDenied   = 126
)

const (
	defaultTimeout   = 60 * time.Second
	defaultMaxOutput = 64 << 10
)

// DefaultInheritEnv are the variables copied from the runner by default.
var DefaultInheritEnv = []string{"PATH", "HOME", "USER", "LANG", "LC_ALL", "TZ"}

// blockedEnv never reaches commands, even if passed in by the runner.
var blockedEnv = []string{"MYCO_TOKEN", "MYCO_CONFIG", "MYCO_ADMIN_TOKEN", "MYCO_ENROLL_TOKEN"}

type node struct {
	spec    harness.ExecSpec
	timeout time.Duration
	shell   string
	mu      sync.Mutex // serializes audit writes
}

// New is the harness.Factory for exec nodes.
func New(opts harness.Options) (harness.Harness, error) {
	spec := harness.ExecSpec{}
	if opts.Exec != nil {
		spec = *opts.Exec
	}
	n := &node{spec: spec, timeout: defaultTimeout, shell: spec.Shell}
	if spec.Timeout != "" {
		d, err := time.ParseDuration(spec.Timeout)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("runner.exec.timeout: invalid duration %q", spec.Timeout)
		}
		n.timeout = d
	}
	if n.spec.MaxOutput <= 0 {
		n.spec.MaxOutput = defaultMaxOutput
	}
	if n.shell == "" {
		n.shell = opts.Binary
	}
	if n.shell == "" {
		n.shell = "/bin/sh"
	}
	if len(n.spec.InheritEnv) == 0 {
		n.spec.InheritEnv = DefaultInheritEnv
	}
	for _, a := range n.spec.Allow {
		if strings.TrimSpace(a) == "" || strings.TrimSpace(a) == "*" {
			return nil, errors.New(`runner.exec.allow: empty or "*" entries are not allowed (leave allow unset to permit any command)`)
		}
	}
	return n, nil
}

func (n *node) Name() string       { return Name }
func (n *node) ExecutesBody() bool { return true }

// Check implements harness.Checker for the shell.
func (n *node) Check(ctx context.Context) harness.Check {
	return harness.CheckBinary(ctx, n.shell, "-c", "echo ok")
}

// Run executes req.Body. Refusals and failing commands are results, not
// errors: the sender always gets an ExecResult back.
func (n *node) Run(ctx context.Context, req harness.Request) (harness.Result, error) {
	cmdline := strings.TrimSpace(req.Body)
	start := time.Now()
	var res api.ExecResult
	switch {
	case len(n.spec.Senders) > 0 && !slices.Contains(n.spec.Senders, req.From):
		res = denied(fmt.Sprintf("sender %q is not allowed to run commands on this node", req.From))
	case len(n.spec.Senders) == 0:
		res = denied("this exec node has no allowed senders configured (set allow_from on the agent)")
	case cmdline == "":
		res = denied("empty command: put the command line in the message body")
	default:
		if reason := n.checkAllow(cmdline); reason != "" {
			res = denied(reason)
			break
		}
		res = n.run(ctx, cmdline, req)
	}
	res.DurationMS = time.Since(start).Milliseconds()
	n.audit(req, cmdline, res)
	b, err := json.Marshal(res)
	if err != nil {
		return harness.Result{}, err
	}
	return harness.Result{Output: string(b)}, nil
}

func (n *node) run(ctx context.Context, cmdline string, req harness.Request) api.ExecResult {
	runCtx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()
	out, err := harness.Exec(runCtx, harness.Process{
		Binary: n.shell, Args: []string{"-c", cmdline}, Workdir: req.Workdir,
		Env: n.env(req.Env), CleanEnv: true, MaxOutput: n.spec.MaxOutput,
	})
	res := api.ExecResult{ExitCode: out.ExitCode, Stdout: out.Stdout, Stderr: out.Stderr, Truncated: out.Truncated}
	switch {
	case runCtx.Err() != nil:
		res.ExitCode, res.TimedOut = ExitTimedOut, true
		res.Error = fmt.Sprintf("killed after %s (the node's timeout)", n.timeout)
		if ctx.Err() != nil {
			res.Error = "killed: the runner stopped or its per-message timeout passed"
		}
	case !out.Started:
		res.ExitCode, res.Error = ExitDenied, fmt.Sprintf("cannot start %s: %v", n.shell, err)
	case out.ExitCode < 0:
		res.ExitCode, res.Error = 1, "terminated by a signal"
	}
	return res
}

// checkAllow returns why cmdline is refused by the allowlist ("" = allowed).
func (n *node) checkAllow(cmdline string) string {
	if len(n.spec.Allow) == 0 {
		return ""
	}
	if i := strings.IndexAny(cmdline, ";&|`$<>(){}\n\r\\"); i >= 0 {
		return fmt.Sprintf("character %q is not allowed when this node has a command allowlist", cmdline[i])
	}
	norm := strings.Join(strings.Fields(cmdline), " ")
	for _, a := range n.spec.Allow {
		a = strings.Join(strings.Fields(a), " ")
		if prefix, ok := strings.CutSuffix(a, " *"); ok {
			if norm == prefix || strings.HasPrefix(norm, prefix+" ") {
				return ""
			}
		} else if norm == a {
			return ""
		}
	}
	return "command is not in this node's allowlist (allowed: " + strings.Join(n.spec.Allow, ", ") + ")"
}

// env builds the command's environment from scratch: inherited basics, then
// the runner's extra variables (env_file, MYCO_* context) minus credentials.
func (n *node) env(extra []string) []string {
	var out []string
	for _, k := range n.spec.InheritEnv {
		if v, ok := os.LookupEnv(k); ok && !slices.Contains(blockedEnv, k) {
			out = append(out, k+"="+v)
		}
	}
	for _, kv := range extra {
		k, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(blockedEnv, k) {
			out = append(out, kv)
		}
	}
	return out
}

func denied(reason string) api.ExecResult {
	return api.ExecResult{ExitCode: ExitDenied, Denied: true, Error: reason}
}

func (n *node) audit(req harness.Request, cmdline string, res api.ExecResult) {
	if n.spec.AuditLog == "" {
		return
	}
	var msgID string
	for _, kv := range req.Env {
		if v, ok := strings.CutPrefix(kv, "MYCO_MESSAGE_ID="); ok {
			msgID = v
		}
	}
	line, _ := json.Marshal(map[string]any{
		"time": time.Now().UTC().Format(time.RFC3339), "from": req.From, "message_id": msgID, "command": cmdline,
		"exit_code": res.ExitCode, "duration_ms": res.DurationMS, "denied": res.Denied, "timed_out": res.TimedOut,
	})
	n.mu.Lock()
	defer n.mu.Unlock()
	_ = os.MkdirAll(filepath.Dir(n.spec.AuditLog), 0o700)
	f, err := os.OpenFile(n.spec.AuditLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}
