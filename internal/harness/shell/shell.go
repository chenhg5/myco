// Package shell runs an arbitrary command line as a harness. The prompt is
// written to stdin, MYCO_* variables describe the message, and stdout becomes
// the reply. Use it to plug in any agent CLI myco has no adapter for.
package shell

import (
	"context"
	"errors"
	"strings"

	"github.com/chenhg5/myco/internal/harness"
)

// Name is the registry key.
const Name = "shell"

type shell struct{ opts harness.Options }

// New is the harness.Factory for shell commands.
func New(opts harness.Options) (harness.Harness, error) {
	if strings.TrimSpace(opts.Command) == "" {
		return nil, errors.New(`shell harness needs a command (runner.command in config, or --command)`)
	}
	if opts.Binary == "" {
		opts.Binary = harness.DefaultShell()
	}
	return &shell{opts: opts}, nil
}

func (s *shell) Name() string { return Name }

func (s *shell) Run(ctx context.Context, req harness.Request) (harness.Result, error) {
	extra := s.opts.Args
	if harness.IsPOSIXShell(s.opts.Binary) {
		extra = append([]string{"myco-shell"}, extra...) // $0
	}
	args := harness.ShellArgs(s.opts.Binary, s.opts.Command, extra...)
	out, err := harness.Exec(ctx, harness.Process{
		Binary: s.opts.Binary, Args: args, Stdin: req.Prompt, Workdir: req.Workdir, Env: req.Env,
	})
	return harness.Result{Output: strings.TrimSpace(out.Stdout)}, err
}

// Check implements harness.Checker for the shell itself.
func (s *shell) Check(ctx context.Context) harness.Check {
	return harness.CheckBinary(ctx, s.opts.Binary, harness.ShellArgs(s.opts.Binary, "echo ok")...)
}
