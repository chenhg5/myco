package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
)

func agentCommands() *Command {
	register := &Command{
		Name:     "register",
		Summary:  "Register an agent with the hub and save its credentials locally (admin)",
		Mutating: true,
		Long: `Needs the hub admin token (MYCO_ADMIN_TOKEN or --admin-token-file). The agent token is
written to the config file unless --no-save, in which case it is printed once.
Re-registering an existing name fails with exit 5 unless --rotate-token is given.`,
		Examples: []string{
			"myco agent register --hub https://hub.example.com --name coder --description \"Claude Code on VPS\" --capability code,github",
			"myco agent register --name orchestrator --allow-from glenn --rotate-token",
			"myco agent register --name cloud-worker --no-save --format json   # print token for a secret store",
		},
		Flags: []Flag{
			{Name: "name", Kind: String, Required: true, Help: "Agent name: lowercase letters, digits, . _ -"},
			{Name: "description", Kind: String, Help: "What this agent does (used by orchestrators to pick agents)"},
			{Name: "capability", Kind: List, Help: "Capability tags, repeatable or comma-separated"},
			{Name: "allow-from", Kind: List, Help: "Only these agents may send tasks/notes to it (default: anyone)"},
			{Name: "rotate-token", Kind: Bool, Help: "If the agent exists, update its profile and issue a new token"},
			{Name: "no-save", Kind: Bool, Help: "Do not write the config file; print the token instead"},
			{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (instead of MYCO_ADMIN_TOKEN)"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name"); err != nil {
				return err
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			dry := e.in.Bool("dry-run")
			res, err := c.Register(e.in.Ctx, api.RegisterRequest{
				Name: e.in.String("name"), Description: e.in.String("description"),
				Capabilities: e.in.List("capability"), AllowFrom: e.in.List("allow-from"),
				RotateToken: e.in.Bool("rotate-token"),
			}, dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "register", "agent": res.Agent, "would_create": res.Created, "config": e.cfgPath})
			}
			out := map[string]any{"agent": res.Agent, "created": res.Created}
			if e.in.Bool("no-save") {
				out["token"] = res.Token
			} else {
				e.cfg.Agent, e.cfg.Token = res.Agent.Name, res.Token
				if err := config.Save(e.cfgPath, e.cfg); err != nil {
					return fmt.Errorf("agent registered but saving config failed (token: %s): %w", res.Token, err)
				}
				out["config"] = e.cfgPath
			}
			return e.p.Print(out, func(w io.Writer) {
				verb := "registered"
				if !res.Created {
					verb = "updated (token rotated)"
				}
				fmt.Fprintf(w, "agent %s %s\n", res.Agent.Name, verb)
				if t, ok := out["token"]; ok {
					fmt.Fprintf(w, "token: %s\n(store it now; it is not shown again)\n", t)
				} else {
					fmt.Fprintf(w, "credentials saved to %s\n", e.cfgPath)
				}
			})
		}),
	}

	list := &Command{
		Name:    "list",
		Summary: "List registered agents (discovery)",
		Examples: []string{
			"myco agent list",
			"myco agent list --online --capability code --format json",
		},
		Flags: []Flag{
			{Name: "capability", Kind: String, Help: "Only agents advertising this capability"},
			{Name: "online", Kind: Bool, Help: "Only agents seen recently"},
		},
		Run: handler(func(e *env) error {
			c, err := e.agentOrAdminClient()
			if err != nil {
				return err
			}
			agents, err := c.ListAgents(e.in.Ctx, e.in.String("capability"), e.in.Bool("online"))
			if err != nil {
				return err
			}
			return e.p.Print(api.AgentList{Agents: agents}, func(w io.Writer) { agentTable(w, agents) })
		}),
	}

	get := &Command{
		Name:     "get",
		Summary:  "Show one agent (default: yourself)",
		Examples: []string{"myco agent get", "myco agent get --name coder"},
		Flags:    []Flag{{Name: "name", Kind: String, Help: "Agent name (default: the configured agent)"}},
		Run: handler(func(e *env) error {
			var (
				a   *api.Agent
				err error
			)
			if n := e.in.String("name"); n != "" {
				c, cerr := e.agentOrAdminClient()
				if cerr != nil {
					return cerr
				}
				a, err = c.GetAgent(e.in.Ctx, n)
			} else {
				c, cerr := e.agentClient()
				if cerr != nil {
					return cerr
				}
				a, err = c.Me(e.in.Ctx)
			}
			if err != nil {
				return err
			}
			return e.p.Print(a, func(w io.Writer) { agentTable(w, []api.Agent{*a}) })
		}),
	}

	remove := &Command{
		Name:     "remove",
		Summary:  "Delete an agent from the hub (admin, irreversible)",
		Mutating: true,
		Examples: []string{"myco agent remove --name old-worker --dry-run", "myco agent remove --name old-worker --force"},
		Flags: []Flag{
			{Name: "name", Kind: String, Required: true, Help: "Agent to delete"},
			{Name: "force", Kind: Bool, Help: "Required to actually delete (irreversible)"},
			{Name: "admin-token-file", Kind: String, Help: "File containing the admin token"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name"); err != nil {
				return err
			}
			dry := e.in.Bool("dry-run")
			if !dry && !e.in.Bool("force") {
				return errorf("confirmation_required", ExitUsage, "re-run with --force (or --dry-run to preview)",
					"removing agent %s is irreversible", e.in.String("name"))
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			res, err := c.RemoveAgent(e.in.Ctx, e.in.String("name"), dry)
			if err != nil {
				return err
			}
			res["reversible"] = false
			if dry {
				res["action"] = "remove"
				return e.dryRun(res)
			}
			return e.p.Print(res, func(w io.Writer) { fmt.Fprintf(w, "agent %s removed\n", e.in.String("name")) })
		}),
	}

	return (&Command{Name: "agent", Summary: "Register and discover agents"}).Add(register, list, get, remove)
}

// agentOrAdminClient prefers the agent token, falling back to the admin token.
func (e *env) agentOrAdminClient() (*client.Client, error) {
	if e.cfg.Token != "" {
		return e.agentClient()
	}
	if e.in.Getenv(config.EnvAdminToken) != "" {
		return e.adminClient()
	}
	return e.agentClient() // produces the not_configured error
}

func agentTable(w io.Writer, agents []api.Agent) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tONLINE\tCAPABILITIES\tLAST SEEN\tDESCRIPTION")
	for _, a := range agents {
		seen := "never"
		if a.LastSeenAt != nil {
			seen = time.Since(*a.LastSeenAt).Round(time.Second).String() + " ago"
		}
		online := "no"
		if a.Online {
			online = "yes"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", a.Name, online, strings.Join(a.Capabilities, ","), seen, truncate(a.Description, 60))
	}
	tw.Flush()
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len([]rune(s)) > n {
		return string([]rune(s)[:n-1]) + "…"
	}
	return s
}
