package cli

import (
	"fmt"
	"io"
	"strings"

	. "github.com/chenhg5/myco/internal/cli/cmdkit"
)

func machineRemoveCommand() *Command {
	return &Command{Name: "remove", Summary: "Remove a machine and all its nodes from this Hub", Mutating: true,
		Long:     "Revokes this machine's enrollment credential, removes its nodes and machine record, and keeps message history. The machine must use a join link to return.",
		Examples: []string{"myco machine remove --id mch_abc --dry-run", "myco machine remove --id mch_abc"},
		Flags:    []Flag{{Name: "id", Kind: String, Required: true, Help: "Machine ID shown in the App or GET /v1/machines"}, {Name: "admin-token-file", Kind: String, Help: "File containing the hub admin token"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			dry := e.in.Bool("dry-run")
			out, err := c.RemoveMachine(e.in.Ctx, e.in.String("id"), dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(out)
			}
			return e.p.Print(out, func(w io.Writer) {
				fmt.Fprintf(w, "removed machine %s and nodes: %s\n", out.Machine.Name, strings.Join(out.Removed, ", "))
			})
		}),
	}
}
