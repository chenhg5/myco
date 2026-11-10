package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
)

// agentAliasCommands manages re-pointable role addresses (admin).
func agentAliasCommands() *Command {
	adminFlag := Flag{Name: "admin-token-file", Kind: String, Help: "File containing the admin token"}

	set := &Command{
		Name:     "set",
		Summary:  "Point an alias at an agent, creating or re-pointing it (admin)",
		Mutating: true,
		Long: `An alias is an extra address: messages sent to it go to its target and are stored under the
target's real name. Aliases share the namespace with agent names and cannot point at another alias.
Re-pointing is how you hand a role (e.g. "reviewer") to a new session, or keep an old name working
after replacing an agent.`,
		Examples: []string{
			"myco agent alias set --alias reviewer --target mac-claude-loops",
			"myco agent alias set --alias reviewer --target codex-loops-01a1a9c3 --dry-run",
		},
		Flags: []Flag{
			{Name: "alias", Kind: String, Required: true, Help: "Alias name (same rules as agent names)"},
			{Name: "target", Kind: String, Required: true, Help: "Agent the alias points at"},
			adminFlag,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("alias", "target"); err != nil {
				return err
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			dry := e.in.Bool("dry-run")
			res, err := c.SetAlias(e.in.Ctx, e.in.String("alias"), e.in.String("target"), dry)
			if err != nil {
				return err
			}
			if dry {
				res["action"] = "alias_set"
				return e.dryRun(res)
			}
			return e.p.Print(res, func(w io.Writer) {
				fmt.Fprintf(w, "alias %s -> %s\n", e.in.String("alias"), e.in.String("target"))
			})
		}),
	}

	remove := &Command{
		Name:     "remove",
		Summary:  "Delete an alias (admin); the target agent is not touched",
		Mutating: true,
		Examples: []string{"myco agent alias remove --alias reviewer --force"},
		Flags: []Flag{
			{Name: "alias", Kind: String, Required: true, Help: "Alias to delete"},
			{Name: "force", Kind: Bool, Help: "Required to actually delete (senders using it will get not_found)"},
			adminFlag,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("alias"); err != nil {
				return err
			}
			dry := e.in.Bool("dry-run")
			if !dry && !e.in.Bool("force") {
				return errorf("confirmation_required", ExitUsage, "re-run with --force (or --dry-run to preview)",
					"removing alias %s breaks senders that still use it", e.in.String("alias"))
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			res, err := c.RemoveAlias(e.in.Ctx, e.in.String("alias"), dry)
			if err != nil {
				return err
			}
			if dry {
				res["action"] = "alias_remove"
				return e.dryRun(res)
			}
			return e.p.Print(res, func(w io.Writer) { fmt.Fprintf(w, "alias %s removed\n", e.in.String("alias")) })
		}),
	}

	list := &Command{
		Name:     "list",
		Summary:  "List aliases and their targets",
		Examples: []string{"myco agent alias list", "myco agent alias list --format json"},
		Run: handler(func(e *env) error {
			c, err := e.agentOrAdminClient()
			if err != nil {
				return err
			}
			aliases, err := c.ListAliases(e.in.Ctx)
			if err != nil {
				return err
			}
			return e.p.Print(api.AliasList{Aliases: aliases}, func(w io.Writer) {
				if len(aliases) == 0 {
					fmt.Fprintln(w, "no aliases")
					return
				}
				tw := newTable(w)
				fmt.Fprintln(tw, "ALIAS\tTARGET\tUPDATED")
				for _, a := range aliases {
					fmt.Fprintf(tw, "%s\t%s\t%s\n", a.Alias, a.Target, a.UpdatedAt.Local().Format("2006-01-02 15:04"))
				}
				tw.Flush()
			})
		}),
	}

	return (&Command{Name: "alias", Summary: "Manage role addresses that point at agents (admin)"}).Add(set, remove, list)
}

// agentUpdateCommand changes cosmetic profile fields.
func agentUpdateCommand() *Command {
	return &Command{
		Name:     "update",
		Summary:  "Change display name, description, or (admin) who may send to an agent",
		Mutating: true,
		Long: `The agent name is the address and never changes; the display name is only shown in lists and the
dashboard. Pass an empty value (--display-name "") to clear it. To give an agent a new address, use an alias.`,
		Examples: []string{
			`myco agent update --display-name "Mac Claude (loops)"`,
			`myco agent update --name codex-loops-01a1a9c3 --display-name "Codex reviewer" --description "reviews PRs"`,
			"myco agent update --name mac-exec --allow-from cloud-1,cloud-cc-connect   # admin; the token is unchanged",
		},
		Flags: []Flag{
			{Name: "display-name", Kind: String, Help: "Human-friendly name, up to 80 bytes"},
			{Name: "description", Kind: String, Help: "What this agent does"},
			{Name: "allow-from", Kind: List, Help: "Admin: replace who may send tasks/notes to it (exec nodes take effect within ~30s)"},
			{Name: "allow-anyone", Kind: Bool, Help: "Admin: let any agent send to it (exec nodes also need --i-understand-exec-risk)"},
			{Name: "i-understand-exec-risk", Kind: Bool, Help: "With --allow-anyone on an exec node: every agent, including future ones, may run shell commands there"},
			{Name: "name", Kind: String, Help: "Another agent (needs the admin token); default: yourself"},
			{Name: "admin-token-file", Kind: String, Help: "File containing the admin token"},
		},
		Run: handler(func(e *env) error {
			var req api.ProfileRequest
			if e.in.IsSet("display-name") {
				v := e.in.String("display-name")
				req.DisplayName = &v
			}
			if e.in.IsSet("description") {
				v := e.in.String("description")
				req.Description = &v
			}
			name := e.in.String("name")
			switch {
			case e.in.IsSet("allow-from") && e.in.Bool("allow-anyone"):
				return errorf("invalid_usage", ExitUsage, "use one of them", "--allow-from and --allow-anyone conflict")
			case e.in.IsSet("allow-from"):
				v := e.in.List("allow-from")
				req.AllowFrom = &v
			case e.in.Bool("allow-anyone"):
				v := []string{}
				req.AllowFrom = &v
				req.AcceptExecRisk = e.in.Bool("i-understand-exec-risk")
			}
			if req.AllowFrom != nil && name == "" {
				return errorf("invalid_usage", ExitUsage, "add --name <agent> (changing allow-from needs the admin token)", "--allow-from changes another agent")
			}
			if req.DisplayName == nil && req.Description == nil && req.AllowFrom == nil {
				return errorf("invalid_usage", ExitUsage, "pass --display-name, --description or --allow-from", "nothing to update")
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "update_profile", "agent": orDefault(name, e.cfg.Agent), "profile": req})
			}
			var (
				c   *client.Client
				err error
			)
			switch {
			case name != "" && req.AllowFrom == nil:
				c, err = e.cardClient() // a coordinator may change the card
			case name != "":
				c, err = e.adminClient()
			default:
				c, err = e.agentClient()
			}
			if err != nil {
				return err
			}
			a, err := c.UpdateProfile(e.in.Ctx, name, req)
			if err != nil {
				return err
			}
			return e.p.Print(a, func(w io.Writer) {
				allow := strings.Join(a.AllowFrom, ", ")
				switch allow {
				case "":
					allow = "anyone"
				case "*":
					allow = "anyone (explicitly; exec risk accepted)"
				}
				fmt.Fprintf(w, "%s: display name %q, description %q, accepts messages from %s\n", a.Name, a.DisplayName, a.Description, allow)
			})
		}),
	}
}

// agentStatusCommands publish the agent's public progress line.
func agentStatusCommands() *Command {
	set := &Command{
		Name:     "set",
		Summary:  "Publish a one-line status that every agent and the dashboard can see",
		Mutating: true,
		Long: `Use it for progress others may want to glance at ("migrating the API: 3/5 endpoints done").
For updates meant for whoever gave you a task, use myco message progress instead.`,
		Examples: []string{`myco agent status set --text "reviewing PR 12: 2 of 5 files done"`},
		Flags:    []Flag{{Name: "text", Kind: String, Required: true, Help: "One line, up to 280 bytes"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("text"); err != nil {
				return err
			}
			return e.setStatus(e.in.String("text"))
		}),
	}
	clear := &Command{
		Name:     "clear",
		Summary:  "Remove your status line",
		Mutating: true,
		Examples: []string{"myco agent status clear"},
		Run:      handler(func(e *env) error { return e.setStatus("") }),
	}
	return (&Command{Name: "status", Summary: "Your public status line (progress others can see)"}).Add(set, clear)
}

func (e *env) setStatus(text string) error {
	if e.in.Bool("dry-run") {
		return e.dryRun(map[string]any{"action": "set_status", "agent": e.cfg.Agent, "text": text})
	}
	c, err := e.agentClient()
	if err != nil {
		return err
	}
	a, err := c.SetStatus(e.in.Ctx, text)
	if err != nil {
		return err
	}
	return e.p.Print(a, func(w io.Writer) {
		if a.StatusText == "" {
			fmt.Fprintf(w, "%s: status cleared\n", a.Name)
			return
		}
		fmt.Fprintf(w, "%s: %s\n", a.Name, a.StatusText)
	})
}
