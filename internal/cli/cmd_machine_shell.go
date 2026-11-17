package cli

import (
	"errors"
	"fmt"
	"io"

	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/config"
)

// machineShellCommand lets agents open a terminal (a shell node) on this
// computer, or stops it: spawn.max_access in machine.yaml. It is local
// policy, so it is only ever changed here, never from a hub or the app on
// another computer.
func machineShellCommand() *Command {
	return &Command{Name: "shell-access", Summary: "Allow or stop agents opening a terminal (shell node) on this computer", Mutating: true,
		Long: `A terminal (shell node) runs the commands of the agents it allows with this user's full rights, so a
computer only gets one when it allows it: spawn.max_access: full in ~/.myco/machine.yaml. This sets it
(--allow) or puts it back to the default (--deny; terminals already open keep running until removed).
Only someone on this computer can change it; the myco app on this computer has the same switch.`,
		Examples: []string{"myco machine shell-access --allow", "myco machine shell-access --deny --dry-run"},
		Flags: []Flag{
			{Name: "allow", Kind: Bool, Help: "Let agents open a terminal here (spawn.max_access: full)"},
			{Name: "deny", Kind: Bool, Help: "Stop allowing new terminals here (back to the default, workspace)"},
		},
		Run: handler(func(e *env) error {
			allow, deny := e.in.Bool("allow"), e.in.Bool("deny")
			if allow == deny {
				return errorf("invalid_usage", ExitUsage, "pass --allow or --deny", "say whether to allow terminals here: exactly one of --allow, --deny")
			}
			m, err := config.LoadMachine(e.defaultPath)
			if err != nil {
				return err
			}
			if m == nil {
				return errorf("not_configured", ExitUsage, "join this computer with a link first: curl -fsSL <link> | bash",
					"this computer has not joined a hub as a machine yet")
			}
			want := "workspace"
			if allow {
				want = config.ShellAccessFull
			}
			now := config.ShellAllowed(e.defaultPath)
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "machine_shell_access", "max_access": want, "changed": now != allow,
					"config": config.MachinePath(e.defaultPath)})
			}
			changed, err := config.SetShellAccess(e.defaultPath, allow)
			if errors.Is(err, config.ErrNotJoined) {
				return errorf("not_configured", ExitUsage, "join this computer with a link first: curl -fsSL <link> | bash", "%v", err)
			}
			if err != nil {
				return err
			}
			out := map[string]any{"max_access": want, "changed": changed}
			return e.p.Print(out, func(w io.Writer) {
				switch {
				case allow:
					fmt.Fprintln(w, "agents may now open a terminal on this computer (允许在这台电脑上开终端)")
				default:
					fmt.Fprintln(w, "agents may no longer open new terminals on this computer (不再允许在这台电脑上开新终端)")
				}
			})
		}),
	}
}
