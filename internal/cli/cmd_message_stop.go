package cli

import (
	"fmt"
	"io"

	. "github.com/chenhg5/myco/internal/cli/cmdkit"
)

func stopMessageCommand() *Command {
	return &Command{Name: "stop", Summary: "Stop a task you sent; its runner will stop when it next checks", Mutating: true,
		Examples: []string{"myco message stop --id msg_abc123"},
		Flags:    []Flag{{Name: "id", Kind: String, Required: true, Help: "ID of the task you sent"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			dry := e.in.Bool("dry-run")
			out, err := c.StopTask(e.in.Ctx, e.in.String("id"), dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "stop", "message": out.Message})
			}
			return e.p.Print(out, func(w io.Writer) { fmt.Fprintf(w, "stopped task %s\n", out.Message.ID) })
		}),
	}
}
