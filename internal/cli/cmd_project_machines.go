package cli

import (
	"fmt"
	"io"

	. "github.com/chenhg5/myco/internal/cli/cmdkit"
)

// Which computers a project's coordinators may start agents on: a setting
// of the project, chosen by its owner (not the terminals in it).
func projectMachineCommands() (*Command, *Command) {
	list := &Command{
		Name: "machines", Summary: "The workspace's computers, and which this project's coordinators may start agents on",
		Long: `Lists the computers of the workspace: online or not, whether they can start agent sessions,
and whether this project may (allowed). A coordinator starts agents with myco agent spawn --on <computer>
on the allowed ones; for another one, ask the project's owner: myco project machine request.`,
		Examples: []string{"myco project machines --name myco"},
		Flags:    []Flag{{Name: "name", Kind: String, Required: true, Help: "The project's short name"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name"); err != nil {
				return err
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			out, err := c.ProjectMachines(e.in.Ctx, e.in.String("name"))
			if err != nil {
				return err
			}
			return e.p.Print(out, func(w io.Writer) {
				if len(out.Machines) == 0 {
					fmt.Fprintln(w, "no computers in this workspace")
					return
				}
				for _, m := range out.Machines {
					state := "offline"
					if m.Online {
						state = "online"
					}
					if !m.CanSpawn {
						state += ", cannot start agents"
					}
					mark := "  "
					if m.Allowed {
						mark = "✓ "
					}
					fmt.Fprintf(w, "%s%-20s %s (%s)\n", mark, m.Name, m.ID, state)
				}
				fmt.Fprintln(w, "✓ = this project may start agents there (myco agent spawn --on <name>)")
			})
		}),
	}
	flags := []Flag{
		{Name: "project", Kind: String, Required: true, Help: "The project's short name"},
		{Name: "machine", Kind: String, Required: true, Help: "The computer: its name or id (see myco project machines)"},
	}
	set := func(allow bool) *Command {
		verb, summary := "allow", "Let the project's coordinators start agents on a computer (people who manage the project)"
		if !allow {
			verb, summary = "disallow", "No longer let the project start agents on a computer"
		}
		return &Command{
			Name: verb, Summary: summary, Mutating: true, Flags: flags,
			Examples: []string{fmt.Sprintf("myco project machine %s --project myco --machine wsl", verb)},
			Run: handler(func(e *env) error {
				if err := e.in.Require("project", "machine"); err != nil {
					return err
				}
				if e.in.Bool("dry-run") {
					return e.dryRun(map[string]any{"action": "project_machine_" + verb, "project": e.in.String("project"), "machine": e.in.String("machine")})
				}
				c, err := e.adminClient()
				if err != nil {
					return err
				}
				p, err := c.SetProjectMachine(e.in.Ctx, e.in.String("project"), e.in.String("machine"), allow)
				if err != nil {
					return err
				}
				return e.p.Print(p, func(w io.Writer) {
					fmt.Fprintf(w, "project %s: %d computer(s) allowed\n", p.Name, len(p.Machines))
				})
			}),
		}
	}
	request := &Command{
		Name: "request", Summary: "Ask the project's owner to let it start agents on a computer (a coordinator)", Mutating: true,
		Long: `Asks the project's owner to allow a computer for this project. They see a card in your chat with
them (approve or deny); you get a note either way, and once approved you can myco agent spawn there.
Say why in --reason: it is on the card.`,
		Flags: append(append([]Flag{}, flags...), Flag{Name: "reason", Kind: String, Help: "Why (shown to the owner)"}),
		Examples: []string{
			`myco project machine request --project myco --machine wsl --reason "需要一个 Codex 在 WSL 上跑迁移"`,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("project", "machine"); err != nil {
				return err
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "project_machine_request", "project": e.in.String("project"), "machine": e.in.String("machine")})
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			ap, allowed, err := c.RequestProjectMachine(e.in.Ctx, e.in.String("project"), e.in.String("machine"), e.in.String("reason"))
			if err != nil {
				return err
			}
			if allowed {
				return e.p.Print(map[string]any{"allowed": true}, func(w io.Writer) {
					fmt.Fprintln(w, "already allowed: start agents there with myco agent spawn --on", e.in.String("machine"))
				})
			}
			return e.p.Print(ap, func(w io.Writer) {
				fmt.Fprintf(w, "asked the project's owner (%s): %s\nyou get a note when they decide; carry on or end your turn meanwhile\n", ap.ID, ap.What)
			})
		}),
	}
	machine := (&Command{Name: "machine", Summary: "Which computers the project may start agents on: allow, disallow, request"}).
		Add(set(true), set(false), request)
	return list, machine
}
