package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
			"myco agent register --hub https://hub.example.com --name coder --harness claude --capability code,github",
			"myco agent register --name reviewer --harness claude --session-id 7f3c...   # adopt an existing session",
			"myco agent register --name ci --delivery github_dispatch --wake-param repo=me/app --wake-secret-file gh.token",
			"myco agent register --name cloud-worker --detect --no-save --format json",
		},
		Flags: []Flag{
			{Name: "name", Kind: String, Required: true, Help: "Agent name: lowercase letters, digits, . _ -"},
			{Name: "description", Kind: String, Help: "What this agent does (used by orchestrators to pick agents)"},
			{Name: "capability", Kind: List, Help: "Capability tags, repeatable or comma-separated"},
			{Name: "allow-from", Kind: List, Help: "Only these agents may send tasks/notes to it (default: anyone)"},
			{Name: "harness", Kind: String, Help: "Agent runtime: claude, codex, shell, claude-cloud, codex-cloud, ..."},
			{Name: "session-id", Kind: String, Help: "Existing harness session this agent is bound to (default: created on first run)"},
			{Name: "environment", Kind: String, Help: "Where the session lives (default: this host name)"},
			{Name: "workdir", Kind: String, Help: "Working directory of the session"},
			{Name: "detect", Kind: Bool, Help: "Detect harness/session/environment from the current process (e.g. inside a Claude Code cloud session)"},
			{Name: "delivery", Kind: String, Default: "pull", Help: "How the hub wakes it: pull | webhook | claude_routine | github_dispatch | github_issue"},
			{Name: "wake-url", Kind: String, Help: "Wake endpoint (webhook URL or routine fire URL)"},
			{Name: "wake-secret-file", Kind: String, Help: "File with the wake secret (HMAC key, routine token, GitHub token)"},
			{Name: "wake-param", Kind: List, Help: "Mode-specific key=value, e.g. repo=owner/name, mention=@codex"},
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
			delivery, err := e.deliveryFromFlags()
			if err != nil {
				return err
			}
			res, err := c.Register(e.in.Ctx, api.RegisterRequest{
				Name: e.in.String("name"), Description: e.in.String("description"),
				Capabilities: e.in.List("capability"), AllowFrom: e.in.List("allow-from"),
				RotateToken: e.in.Bool("rotate-token"), Binding: e.bindingFromFlags(), Delivery: delivery,
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

	bind := &Command{
		Name:     "bind",
		Summary:  "Bind yourself to a harness session in an environment",
		Mutating: true,
		Long: `An agent is one session in one environment. The runner binds automatically when it
creates the session; use this to adopt an existing session or to move an agent.`,
		Examples: []string{
			"myco agent bind --detect                       # inside a Claude Code cloud session",
			"myco agent bind --harness claude --session-id 7f3c2a... --workdir ~/src/app",
		},
		Flags: []Flag{
			{Name: "harness", Kind: String, Help: "Agent runtime (claude, codex, shell, claude-cloud, ...)"},
			{Name: "session-id", Kind: String, Help: "Harness session ID"},
			{Name: "environment", Kind: String, Help: "Where the session lives (default: this host name)"},
			{Name: "workdir", Kind: String, Help: "Working directory of the session"},
			{Name: "detect", Kind: Bool, Help: "Detect from the current process environment"},
		},
		Run: handler(func(e *env) error {
			b := e.bindingFromFlags()
			if b == (api.Binding{}) {
				return errorf("invalid_usage", ExitUsage, "pass --detect or at least one of --session-id, --harness, --environment, --workdir",
					"nothing to bind")
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "bind", "binding": b})
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			a, err := c.Bind(e.in.Ctx, b)
			if err != nil {
				return err
			}
			return e.p.Print(a, func(w io.Writer) {
				fmt.Fprintf(w, "%s bound to %s session %q in %s\n", a.Name, a.Binding.Harness, a.Binding.SessionID, a.Binding.Environment)
			})
		}),
	}

	return (&Command{Name: "agent", Summary: "Register, bind and discover agents"}).Add(register, agentEnrollCommand(), bind, list, get, remove)
}

// bindingFromFlags merges --detect results with explicit flags.
func (e *env) bindingFromFlags() api.Binding {
	var b api.Binding
	if e.in.Bool("detect") {
		b = detectBinding(e.in.Getenv)
	}
	if v := e.in.String("harness"); v != "" {
		b.Harness = v
	}
	if v := e.in.String("session-id"); v != "" {
		b.SessionID = v
	}
	if v := e.in.String("environment"); v != "" {
		b.Environment = v
	}
	if v := e.in.String("workdir"); v != "" {
		if abs, err := filepath.Abs(v); err == nil {
			v = abs
		}
		b.Workdir = v
	}
	if b != (api.Binding{}) && b.Environment == "" {
		b.Environment = hostname()
	}
	return b
}

// detectBinding recognizes known runtimes from environment variables.
func detectBinding(getenv func(string) string) api.Binding {
	if id := getenv("CLAUDE_CODE_REMOTE_SESSION_ID"); id != "" {
		// cse_XXXX (process env) and session_XXXX (session tools) name the same session.
		if rest, ok := strings.CutPrefix(id, "cse_"); ok {
			id = "session_" + rest
		}
		envType := getenv("CLAUDE_CODE_REMOTE_ENVIRONMENT_TYPE")
		if envType == "" {
			envType = "cloud"
		}
		wd, _ := os.Getwd()
		return api.Binding{Harness: "claude-cloud", SessionID: id, Environment: "claude-" + envType, Workdir: wd}
	}
	wd, _ := os.Getwd()
	return api.Binding{Environment: hostname(), Workdir: wd}
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown-host"
	}
	return h
}

// deliveryFromFlags builds the delivery settings from register flags.
func (e *env) deliveryFromFlags() (api.Delivery, error) {
	d := api.Delivery{Mode: e.in.String("delivery"), URL: e.in.String("wake-url")}
	if f := e.in.String("wake-secret-file"); f != "" {
		if err := safePath(f); err != nil {
			return d, err
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return d, errorf("not_found", ExitNotFound, "check the path", "cannot read --wake-secret-file: %v", err)
		}
		d.Secret = strings.TrimSpace(string(b))
	}
	for _, kv := range e.in.List("wake-param") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return d, errorf("invalid_usage", ExitUsage, "use --wake-param key=value", "invalid --wake-param %q", kv)
		}
		if d.Params == nil {
			d.Params = map[string]string{}
		}
		d.Params[k] = v
	}
	return d, nil
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
	fmt.Fprintln(tw, "NAME\tONLINE\tHARNESS\tSESSION\tENVIRONMENT\tDELIVERY\tCAPABILITIES\tLAST SEEN\tDESCRIPTION")
	for _, a := range agents {
		seen := "never"
		if a.LastSeenAt != nil {
			seen = time.Since(*a.LastSeenAt).Round(time.Second).String() + " ago"
		}
		online := "no"
		if a.Online {
			online = "yes"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", a.Name, online, dash(a.Binding.Harness), dash(truncate(a.Binding.SessionID, 14)),
			dash(a.Binding.Environment), a.Delivery.Mode, strings.Join(a.Capabilities, ","), seen, truncate(a.Description, 40))
	}
	tw.Flush()
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len([]rune(s)) > n {
		return string([]rune(s)[:n-1]) + "…"
	}
	return s
}
