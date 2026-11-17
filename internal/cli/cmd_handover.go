package cli

import (
	"fmt"
	"io"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/harness/machinenode"
)

// EnvTakeOver carries a take-over link (or its token) to `myco join`, so it
// stays out of the process list.
const EnvTakeOver = "MYCO_TAKEOVER"

// Handing a node over (「换一个来接着做」): another session answers for the
// same name; see docs/design/agent-handover.md.
func agentHandoverCommands() []*Command {
	adminFlag := Flag{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (instead of MYCO_ADMIN_TOKEN)"}
	nameFlag := Flag{Name: "name", Kind: String, Required: true, Help: "The node to hand over"}
	handover := &Command{
		Name:     "handover",
		Summary:  "Have a new session on a machine take a node over (same name and conversations)",
		Mutating: true,
		Long: `The node keeps its name, conversations, projects, aliases and who may message it; a new Claude Code or
Codex session on the machine you name answers for it from now on. The session behind it until now is
stopped: its credential is retired (it is told it was replaced), and the tasks it had not finished go
back to the queue for the new one. The new session's first prompt is a handover note: where the work
was, the open tasks and the recent messages. --back switches back to the session a handover replaced
(it resumes on its own machine and gets what happened meanwhile).
Not yet available in end-to-end encrypted workspaces. For a session on no machine of yours (another
claude.ai account, say), use handover-link. Only the node's owner or someone who manages the hub.`,
		Examples: []string{
			`myco agent handover --name docs-claude --machine mac-mini --cli codex --note "subscription ran out"`,
			"myco agent handover --name docs-claude --machine wsl --dir ~/code/myco",
			"myco agent handover --name docs-claude --back hov_...   # switch back",
		},
		Flags: []Flag{nameFlag,
			{Name: "machine", Kind: String, Help: "Machine (name or id) that starts the new session (not with --back)"},
			{Name: "cli", Kind: Enum, Values: machinenode.SpawnableCLIs, Help: "Agent CLI to start (default claude)"},
			{Name: "dir", Kind: String, Help: "Directory on that machine (default: its home directory)"},
			{Name: "model", Kind: String, Help: "Model, in the CLI's own naming"},
			{Name: "access", Kind: Enum, Values: []string{"readonly", "workspace", "full"}, Help: "Permission level (capped by the machine)"},
			{Name: "note", Kind: String, Help: "Why, for the new session (in its handover note)"},
			{Name: "back", Kind: String, Help: "Switch back to the session this handover (hov_...) replaced"},
			{Name: "timeout", Kind: Duration, Default: "5m", Help: "How long to wait for the machine to answer"},
			adminFlag},
		Run: handler(func(e *env) error {
			name := e.in.String("name")
			if e.in.String("machine") == "" && e.in.String("back") == "" {
				return errorf("invalid_argument", ExitUsage, "name the machine with --machine, or switch back with --back <hov id>; for a session elsewhere use: myco agent handover-link --name "+name,
					"say which machine starts the new session")
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			cli := e.in.String("cli")
			if cli == "" && e.in.String("back") == "" {
				cli = "claude"
			}
			req := api.HandoverRequest{Machine: e.in.String("machine"), CLI: cli, Dir: remoteDir(e.in.String("dir")),
				Model: e.in.String("model"), Access: e.in.String("access"), Note: e.in.String("note"), Back: e.in.String("back")}
			dry := e.in.Bool("dry-run")
			res, err := c.HandOver(e.in.Ctx, name, req, e.in.Duration("timeout"), dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "handover", "agent": name, "request": req, "from": res.Handover.From})
			}
			spawned := api.SpawnResult{}
			if res.Spawn != nil {
				spawned = *res.Spawn
			}
			return e.p.Print(res, func(w io.Writer) {
				fmt.Fprintf(w, "%s is now answered by a %s session on %s", name, orDefault(spawned.CLI, cli), orDefault(res.Handover.To.MachineName, req.Machine))
				if spawned.Workdir != "" {
					fmt.Fprintf(w, " in %s", spawned.Workdir)
				}
				fmt.Fprintln(w)
				if n := len(res.Handover.OpenTasks); n > 0 {
					fmt.Fprintf(w, "%d open task(s) went back to its queue\n", n)
				}
				fmt.Fprintf(w, "handover %s; switch back with: myco agent handover --name %s --back %s\n", res.Handover.ID, name, res.Handover.ID)
			})
		}),
	}
	link := &Command{
		Name:     "handover-link",
		Summary:  "Make a one-time command that lets another session take a node over",
		Mutating: true,
		Long: `Prints a command (curl ... | sh, and one for Windows PowerShell). Run it in the session that should answer
for the node from now on (another claude.ai account's cloud session, a session on any computer): it
installs myco if needed and takes the node over. The link works once, within 24 hours, and is a
credential: do not post it where others can read it. --back makes the command for switching back:
run it in the session that handover replaced.`,
		Examples: []string{
			`myco agent handover-link --name docs-claude --note "moving to my other account"`,
			"myco agent handover-link --name docs-claude --back hov_...",
		},
		Flags: []Flag{nameFlag,
			{Name: "note", Kind: String, Help: "Why, for the new session (in its handover note)"},
			{Name: "back", Kind: String, Help: "Switch back to the session this handover (hov_...) replaced"},
			adminFlag},
		Run: handler(func(e *env) error {
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			name := e.in.String("name")
			dry := e.in.Bool("dry-run")
			res, err := c.CreateHandoverLink(e.in.Ctx, name, api.HandoverLinkRequest{Note: e.in.String("note"), Back: e.in.String("back")}, dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "handover_link", "agent": name, "from": res.Handover.From})
			}
			return e.p.Print(res, func(w io.Writer) {
				fmt.Fprintf(w, "In the session that should answer for %s from now on, run (once, within 24 hours; keep it private):\n\n  %s\n\nWindows PowerShell:\n\n  %s\n\n", name, res.Command, res.CommandPS1)
				fmt.Fprintf(w, "Until then nothing changes. Then the session behind %s now stops and its open tasks go to the new one.\n", name)
			})
		}),
	}
	list := &Command{
		Name:     "handover-list",
		Summary:  "Show a node's handovers, newest first",
		Examples: []string{"myco agent handover-list --name docs-claude"},
		Flags:    []Flag{nameFlag, adminFlag},
		Run: handler(func(e *env) error {
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			list, err := c.Handovers(e.in.Ctx, e.in.String("name"))
			if err != nil {
				return err
			}
			return e.p.Print(map[string]any{"handovers": list}, func(w io.Writer) {
				if len(list) == 0 {
					fmt.Fprintln(w, "never handed over")
					return
				}
				tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tWHEN\tSTATE\tFROM\tTO\tBY")
				for _, h := range list {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", h.ID, h.At.Local().Format("2006-01-02 15:04"), h.State,
						sideText(h.From), sideText(h.To), orDefault(h.By, "-"))
				}
				tw.Flush()
			})
		}),
	}
	return []*Command{handover, link, list}
}

func sideText(s api.HandoverSide) string {
	h := s.Binding.Harness
	if s.Runtime != nil && s.Runtime.CLI != "" {
		h = s.Runtime.CLI
	}
	if h == "" {
		return "-"
	}
	where := orDefault(s.MachineName, s.Binding.Environment)
	if where != "" {
		return h + "@" + where
	}
	return h
}

// handoverCommand: what the session that took a node over reads first.
func handoverCommand() *Command {
	show := &Command{
		Name:     "show",
		Summary:  "Show the handover note of the node this session took over",
		Examples: []string{"myco handover show"},
		Run: handler(func(e *env) error {
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			res, err := c.MyHandover(e.in.Ctx)
			if err != nil {
				return err
			}
			return e.p.Print(res, func(w io.Writer) { fmt.Fprint(w, res.Brief) })
		}),
	}
	return (&Command{Name: "handover", Summary: "The node this session took over from another session"}).Add(show)
}

// takeOverTokenAndHub accepts a take-over token or its /h/ link. Never put
// the raw input in an error: it is a credential.
func takeOverTokenAndHub(raw string) (token, hub string, err error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, perr := url.Parse(raw)
		if perr == nil && u.User == nil && u.Fragment == "" {
			if i := strings.LastIndex(u.Path, "/h/"); i >= 0 {
				token, hub = u.Path[i+3:], u.Scheme+"://"+u.Host+u.Path[:i]
			}
		}
	} else {
		token = raw
	}
	if !strings.HasPrefix(token, "myco_takeover_") || strings.ContainsAny(token, "/?# \t\r\n") {
		return "", "", errorf("invalid_argument", ExitUsage, "pass the myco_takeover_ token or its /h/ link (from myco agent handover-link)", "invalid take-over link")
	}
	return token, hub, nil
}

// takeOver binds this session to the node of a take-over link and prints
// the handover note.
func (e *env) takeOver(raw string) error {
	token, hub, err := takeOverTokenAndHub(raw)
	if err != nil {
		return err
	}
	if h := e.in.String("hub"); hub == "" && h != "" {
		hub = h
	}
	if hub == "" {
		hub = e.in.Getenv(config.EnvHub)
	}
	if hub == "" {
		hub = e.cfg.Hub
	}
	if hub == "" {
		return errorf("not_configured", ExitUsage, "pass the whole /h/ link, or --hub <hub URL>", "which hub is the link for?")
	}
	b, how, err := detectSession(e)
	if err != nil {
		return err
	}
	if e.in.Bool("dry-run") {
		return e.dryRun(map[string]any{"action": "take_over", "hub": hub, "binding": b, "detected": how})
	}
	c, err := client.New(hub, "")
	if err != nil {
		return err
	}
	res, err := c.TakeOver(e.in.Ctx, api.TakeOverRequest{Token: token, Binding: b})
	if err != nil {
		return err
	}
	if b.Harness == "inbox" && !e.explicitConfig {
		// An inbox-only session has no harness variable to find its own
		// profile with: it uses the default config.
		e.cfgPath, e.explicitConfig = e.defaultPath, true
	}
	if err := e.saveIdentity(hub, res.Agent.Name, res.Token, res.Agent.Binding); err != nil {
		return fmt.Errorf("took %s over but saving the config failed: %w", res.Agent.Name, err)
	}
	cloud := b.Harness == "claude-cloud" || b.Harness == "inbox"
	out := map[string]any{"agent": res.Agent, "handover": res.Handover, "detected": how, "config": e.cfgPath, "brief": res.Brief}
	if cmd := e.ensureResume(hub, res.Token); cmd != "" {
		out["resume_command"] = cmd
	}
	installedTo, _, _ := installSkill(skillTargets(e.in.Getenv), false)
	out["skill"] = map[string]any{"read": "myco skill show", "installed": nonNilStrings(installedTo)}
	if cloud {
		out["receive"] = inboxReceiveNote
	} else {
		out["receive"] = openSessionNote
	}
	return e.p.Print(out, func(w io.Writer) {
		fmt.Fprintf(w, "took over %s (%s); the session behind it until now no longer answers for it\n", res.Agent.Name, how)
		if n := len(res.Handover.OpenTasks); n > 0 {
			fmt.Fprintf(w, "%d open task(s) are back in your inbox\n", n)
		}
		if cmd, ok := out["resume_command"].(string); ok {
			fmt.Fprintln(w, "If this sandbox is temporary, save this credential outside it (memory, project instructions or an environment variable). Run it first if myco or its config is missing; keep it private:\n  "+cmd)
		}
		fmt.Fprintln(w, out["receive"])
		fmt.Fprintln(w, "\nRead this handover note first (again any time: myco handover show):")
		fmt.Fprintln(w)
		fmt.Fprint(w, res.Brief)
	})
}
