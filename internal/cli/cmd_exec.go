package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
)

func execCommand() *Command {
	return &Command{
		Name:     "exec",
		Summary:  "Run a shell command on an exec node and return its output and exit code",
		Mutating: true,
		Long: `Sends the command as a task to an agent served with harness "exec" and waits for the result.
Like ssh, the remote command's stdout and stderr are printed as-is and its exit code becomes this
command's exit code (124 = killed at the node's timeout, 126 = refused by the node). Failures of
myco itself use the usual codes with a JSON error on stderr; with --format json the result object
({exit_code, stdout, stderr, duration_ms, ...}) is printed instead and the exit code is the same.

The node decides what runs: only its allow_from senders, optionally only allowlisted commands,
with its own timeout and output limits. Set one up with: myco agent register --harness exec ...`,
		Examples: []string{
			`myco exec --to vps-shell --cmd "df -h /"`,
			`myco exec --to vps-shell --cmd "systemctl status myco-hub" --timeout 2m --format json`,
			`myco exec --to vps-shell --cmd "git -C /srv/app pull" --dry-run`,
		},
		Flags: []Flag{
			{Name: "to", Kind: String, Required: true, Help: "Exec node (agent name or alias)"},
			{Name: "cmd", Kind: String, Required: true, Help: "Command line, run with sh -c on the node"},
			{Name: "timeout", Kind: Duration, Default: "5m", Help: "How long to wait for the result (the node has its own run timeout)"},
			{Name: "idempotency-key", Kind: String, Help: "Retry-safe key: re-running with the same key does not run the command twice"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("to", "cmd"); err != nil {
				return err
			}
			cmdline := strings.TrimSpace(e.in.String("cmd"))
			if cmdline == "" {
				return errorf("invalid_argument", ExitUsage, `pass the command line, e.g. --cmd "uptime"`, "empty command")
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			target, err := c.GetAgent(e.in.Ctx, e.in.String("to"))
			if err != nil {
				return err
			}
			if target.Binding.Harness != "exec" {
				return errorf("invalid_argument", ExitUsage, "send a normal task instead: myco message send --to "+target.Name+" --body ...",
					"%s is not an exec node (harness %q)", target.Name, target.Binding.Harness)
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "exec", "to": target.Name, "command": cmdline,
					"node_online": target.Online, "allow_from": target.AllowFrom})
			}
			sent, err := c.Send(e.in.Ctx, api.SendRequest{To: target.Name, Type: "task", Body: cmdline,
				IdempotencyKey: e.in.String("idempotency-key")}, false)
			if err != nil {
				return err
			}
			wait := e.in.Duration("timeout")
			reply, err := c.WaitReply(e.in.Ctx, sent.Message.ID, wait)
			if err != nil {
				return err
			}
			if reply == nil {
				hint := ""
				if !target.Online {
					hint = " (the node is offline: is its runner running?)"
				}
				return &Error{Code: "timeout", Message: fmt.Sprintf("no result from %s within %s%s", target.Name, wait, hint),
					Suggestion: "keep waiting with: myco message wait --id " + sent.Message.ID + " --timeout 10m",
					Retryable:  true, Exit: ExitTimeout}
			}
			var res api.ExecResult
			if reply.Type != "reply" || json.Unmarshal([]byte(reply.Body), &res) != nil {
				return &Error{Code: "exec_failed", Message: fmt.Sprintf("%s did not return a command result: %s", target.Name, truncate(reply.Body, 500)),
					Suggestion: "check the node's runner logs: myco runner service logs", Exit: ExitError}
			}
			if err := e.p.Print(map[string]any{"message_id": sent.Message.ID, "to": target.Name, "result": res}, func(w io.Writer) {
				io.WriteString(w, res.Stdout)
				io.WriteString(e.in.Stderr, res.Stderr)
				if res.Truncated {
					fmt.Fprintln(e.in.Stderr, "myco exec: output truncated by the node's max_output")
				}
				if res.Error != "" {
					fmt.Fprintf(e.in.Stderr, "myco exec: %s\n", res.Error)
				}
			}); err != nil {
				return err
			}
			if res.ExitCode != 0 {
				return exitStatus(res.ExitCode)
			}
			return nil
		}),
	}
}
