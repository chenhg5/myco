package cli

import (
	"errors"
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

// memberCommands manages the people using a hub (owner/admin, with the hub's
// admin token).
func memberCommands() *Command {
	adminFlag := Flag{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (instead of MYCO_ADMIN_TOKEN)"}
	list := &Command{
		Name:     "list",
		Summary:  "List the people on this hub and their roles",
		Examples: []string{"myco member list", "myco member list --all   # include removed people"},
		Flags:    []Flag{{Name: "all", Kind: Bool, Help: "Include removed members"}, adminFlag},
		Run: handler(func(e *env) error {
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			ms, err := c.Members(e.in.Ctx, e.in.Bool("all"))
			if err != nil {
				return err
			}
			return e.p.Print(api.MemberList{Members: ms}, func(w io.Writer) {
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tDISPLAY NAME\tROLE\tSINCE")
				for _, m := range ms {
					role := m.Role
					if m.Removed {
						role += " (removed)"
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", m.Name, orDefault(m.DisplayName, "-"), role, m.CreatedAt.Format("2006-01-02"))
				}
				tw.Flush()
			})
		}),
	}
	invite := &Command{
		Name:     "invite",
		Summary:  "Invite someone: prints a one-time link to join as them, with the app or one command",
		Mutating: true,
		Long: `Adds a person to the hub and prints a one-time link (24 hours). They use it one of two ways:
  - without the app: their agent runs "curl -fsSL <link> | bash" on their machine, which installs
    myco, signs in as them, and adds that machine and that agent session as theirs;
  - in the myco app (Mac or Windows): opening the link there signs the app in as them.
Members see every node and conversation and talk to any node, but manage only what they add;
admins manage everyone's. Keep the link private.`,
		Examples: []string{
			`myco member invite --name laohu --display-name "老胡"`,
			"myco member invite --name matthew --role admin",
		},
		Flags: []Flag{
			{Name: "name", Kind: String, Required: true, Help: "Handle, e.g. laohu (shown as the sender of what they send)"},
			{Name: "display-name", Kind: String, Help: "Full name"},
			{Name: "role", Kind: Enum, Values: []string{"member", "admin"}, Default: "member", Help: "member: manages own nodes; admin: manages everything and everyone"},
			adminFlag,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name"); err != nil {
				return err
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			res, err := c.InviteMember(e.in.Ctx, api.InviteRequest{Name: e.in.String("name"), DisplayName: e.in.String("display-name"), Role: e.in.String("role")})
			if err != nil {
				return err
			}
			return e.p.Print(res, func(w io.Writer) {
				fmt.Fprintf(w, "invited %s as %s. Send them this (private, works once within 24 hours):\n\n", res.Member.Name, res.Member.Role)
				fmt.Fprintf(w, "  Without the app: have your agent run\n    %s\n", inviteCommand(res.Link))
				fmt.Fprintf(w, "  Or open this link in the myco app:\n    %s\n", res.Link)
			})
		}),
	}
	update := &Command{
		Name:     "update",
		Summary:  "Rename someone or change their role (role owner hands over the hub)",
		Mutating: true,
		Examples: []string{
			"myco member update --name owner --new-name glenn",
			"myco member update --name laohu --role admin",
			"myco member update --name matthew --role owner   # hand over ownership",
		},
		Flags: []Flag{
			{Name: "name", Kind: String, Required: true, Help: "The member's current handle"},
			{Name: "new-name", Kind: String, Help: "New handle"},
			{Name: "display-name", Kind: String, Help: "New display name"},
			{Name: "role", Kind: Enum, Values: []string{"", "member", "admin", "owner"}, Help: "New role"},
			adminFlag,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name"); err != nil {
				return err
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			req := api.MemberUpdateRequest{Name: e.in.String("new-name"), Role: e.in.String("role")}
			if e.in.IsSet("display-name") {
				d := e.in.String("display-name")
				req.DisplayName = &d
			}
			m, err := c.UpdateMember(e.in.Ctx, e.in.String("name"), req)
			if err != nil {
				return err
			}
			return e.p.Print(m, func(w io.Writer) { fmt.Fprintf(w, "%s: %s\n", m.Name, m.Role) })
		}),
	}
	remove := &Command{
		Name:     "remove",
		Summary:  "Take someone off the hub (their app links stop working; their nodes stay)",
		Mutating: true,
		Examples: []string{"myco member remove --name laohu"},
		Flags:    []Flag{{Name: "name", Kind: String, Required: true, Help: "The member's handle"}, adminFlag},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name"); err != nil {
				return err
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			m, err := c.RemoveMember(e.in.Ctx, e.in.String("name"))
			if err != nil {
				return err
			}
			return e.p.Print(m, func(w io.Writer) {
				fmt.Fprintf(w, "removed %s; their app access is revoked. Their nodes and machines stay; give them to someone else with: myco agent owner --name <node> --owner <member>\n", m.Name)
			})
		}),
	}
	return (&Command{Name: "member", Summary: "The people on this hub: invite, accept an invite, roles, remove"}).Add(list, invite, memberAcceptCommand(), memberPairCommand(), update, remove)
}

// agentOwnerCommand gives a node to another member.
func agentOwnerCommand() *Command {
	return &Command{
		Name:     "owner",
		Summary:  "Give a node to another person on the hub (owner or admin)",
		Mutating: true,
		Examples: []string{"myco agent owner --name laohu-claude --owner glenn"},
		Flags: []Flag{
			{Name: "name", Kind: String, Required: true, Help: "Node (agent) name"},
			{Name: "owner", Kind: String, Required: true, Help: "Member handle"},
			{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (instead of MYCO_ADMIN_TOKEN)"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name", "owner"); err != nil {
				return err
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			a, err := c.SetAgentOwner(e.in.Ctx, e.in.String("name"), e.in.String("owner"))
			if err != nil {
				return err
			}
			return e.p.Print(a, func(w io.Writer) { fmt.Fprintf(w, "%s now belongs to %s\n", a.Name, a.Owner) })
		}),
	}
}

// auditCommand reads the hub's audit log.
func auditCommand() *Command {
	return &Command{
		Name:    "audit",
		Summary: "Who did what on this hub: invites, removals, links, sessions started, tasks sent from the app",
		Long: `Reads the hub's audit log (owner or admin), newest first. It records administrative actions and
tasks people send from the app, never message bodies; messages between agents are in their threads.`,
		Examples: []string{"myco audit", "myco audit --actor laohu --limit 50", "myco audit --target laohu-claude"},
		Flags: []Flag{
			{Name: "actor", Kind: String, Help: "Only actions by this member (or agent, or admin)"},
			{Name: "target", Kind: String, Help: "Only actions on this node, member, machine or link"},
			{Name: "limit", Kind: Int, Default: "50", Help: "How many entries (max 500)"},
			{Name: "before", Kind: String, Help: "Only entries before this time (RFC 3339), to page back"},
			{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (instead of MYCO_ADMIN_TOKEN)"},
		},
		Run: handler(func(e *env) error {
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			f := client.AuditFilter{Actor: e.in.String("actor"), Target: e.in.String("target"), Limit: e.in.Int("limit")}
			if b := e.in.String("before"); b != "" {
				t, err := time.Parse(time.RFC3339, b)
				if err != nil {
					return &Error{Code: "invalid_usage", Message: "--before must be an RFC 3339 time, e.g. 2026-11-08T10:00:00Z", Exit: ExitUsage}
				}
				f.Before = t
			}
			list, err := c.Audit(e.in.Ctx, f)
			if err != nil {
				return err
			}
			return e.p.Print(api.AuditList{Entries: list}, func(w io.Writer) {
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "TIME\tWHO\tACTION\tTARGET\tDETAIL")
				for _, x := range list {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", x.At.Local().Format("01-02 15:04:05"), x.Actor, x.Action, x.Target, strings.ReplaceAll(x.Detail, "\n", " "))
				}
				tw.Flush()
			})
		}),
	}
}

// inviteCommand is the one-liner that accepts invite link on the command line.
func inviteCommand(link string) string { return "curl -fsSL " + link + " | bash" }

// memberAcceptCommand joins a hub as the invited person, without the app.
func memberAcceptCommand() *Command {
	return &Command{
		Name:     "accept",
		Summary:  "Join a hub as yourself with an invite link, without the app (what the link's script runs)",
		Mutating: true,
		Long: `Trades the invite's one-time code for your own credential on the hub (stored in ~/.myco/machine.yaml,
mode 600), adds this machine as yours, and installs the background runner service. Then run
"myco join" inside each agent session that should be reachable: those nodes are yours too.
Later, "myco member pair" prints a link to also open the hub in the myco app as you.
Safe to re-run on the same machine.`,
		Examples: []string{
			"curl -fsSL https://hub.example.com/c/myco_pair_... | bash      # the usual way",
			"myco member accept --link https://hub.example.com/c/myco_pair_...",
		},
		Flags: []Flag{
			{Name: "link", Kind: String, Help: "Invite link (default: MYCO_HUB + MYCO_INVITE_CODE, as set by the link's script)"},
			{Name: "machine", Kind: String, Help: "Machine name (default: this host's name; \"cloud\" in claude.ai)"},
			{Name: "device", Kind: String, Help: "Name of this device in your device list (default: the machine name)"},
			{Name: "no-service", Kind: Bool, Help: "Do not install the background runner service"},
		},
		Run: handler(func(e *env) error {
			hub, code := strings.TrimRight(e.in.Getenv(config.EnvHub), "/"), e.in.Getenv(config.EnvInviteCode)
			if l := strings.TrimSpace(e.in.String("link")); l != "" {
				i := strings.LastIndex(l, "/c/")
				if i < 0 || !strings.HasPrefix(l, "http") {
					return errorf("invalid_argument", ExitUsage, "pass the whole link, e.g. https://hub.example.com/c/myco_pair_...", "not an invite link: %q", l)
				}
				hub, code = l[:i], strings.Trim(l[i+3:], "/ ")
			}
			if hub == "" || code == "" {
				return errorf("not_configured", ExitUsage, "pass --link <invite link>, or run the link's script: curl -fsSL <link> | bash", "no invite link given")
			}
			if !strings.HasPrefix(code, pairingCodePrefix) {
				return errorf("invalid_argument", ExitUsage, "ask the hub's owner for an invite: myco member invite --name <you>",
					"this is not an invite link (it does not carry a one-time invite code)")
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "member_accept", "hub": hub})
			}
			// Re-running: the code is used up, but this machine already holds
			// the person's credential.
			var (
				token  string
				member *api.Member
				reused bool
			)
			if t := e.memberToken(hub); t != "" {
				if c, err := client.New(hub, t); err == nil {
					if who, err := c.Whoami(e.in.Ctx); err == nil && who.Member != nil {
						token, member, reused = t, who.Member, true
					}
				}
			}
			if token == "" {
				device := e.in.String("device")
				if device == "" {
					device = orDefault(e.in.String("machine"), hostname())
					if e.in.Getenv("CLAUDE_CODE_REMOTE_SESSION_ID") != "" {
						device = "cloud"
					}
				}
				anon, err := client.New(hub, "")
				if err != nil {
					return err
				}
				res, err := anon.Pair(e.in.Ctx, code, device)
				if err != nil {
					var ce *client.Error
					if errors.As(err, &ce) && (ce.Code == "unauthenticated" || ce.Code == "not_found") {
						return errorf("invite_unusable", ExitPermission, "ask whoever invited you for a new link (invites work once, within 24 hours)",
							"this invite has expired or was already used")
					}
					return err
				}
				if res.Member == nil {
					return errorf("invalid_argument", ExitUsage, "use a link from: myco member invite", "this code is not an invite for a person")
				}
				token, member = res.Token, res.Member
			}
			if reused {
				// Already done here: keep this machine's membership as it is.
				if mf, _ := config.LoadMachine(e.defaultPath); mf != nil {
					if h, ok := mf.For(hub); ok {
						return e.p.Print(map[string]any{"member": member.Name, "machine": h.Machine, "hub": hub, "created": false}, func(w io.Writer) {
							fmt.Fprintf(w, "==> already joined %s as %s (machine %q); nothing to do\n", hub, member.Name, h.Machine)
						})
					}
				}
			}
			mc, err := client.New(hub, token)
			if err != nil {
				return err
			}
			// A short-lived, one-machine link of the person's own: what this
			// machine (and the sessions joined on it) adds is theirs.
			link, err := mc.CreateEnrollment(e.in.Ctx, api.CreateEnrollmentRequest{Kind: "link", MaxUses: 1, TTLSeconds: 600,
				Note: "invite accepted on the command line"}, false)
			if err != nil {
				return err
			}
			return e.joinMachine(hub, link.Token, func(out map[string]any) []string {
				out["member"] = member.Name
				if err := e.saveMemberToken(hub, token); err != nil {
					return []string{"could not store your credential (" + err.Error() + "); run this again"}
				}
				who := member.Name
				if member.DisplayName != "" {
					who += " (" + member.DisplayName + ")"
				}
				return []string{"you joined as " + who + "; nodes added from this machine are yours to manage",
					"to also use the myco app later: myco member pair"}
			})
		}),
	}
}

// pairingCodePrefix marks an invite's (or add-device link's) one-time code.
const pairingCodePrefix = "myco_pair_"

// saveMemberToken stores the person's credential with this machine's
// membership of hub.
func (e *env) saveMemberToken(hub, token string) error {
	mf, err := config.LoadMachine(e.defaultPath)
	if err != nil {
		return err
	}
	if mf == nil {
		return errors.New("machine.yaml missing")
	}
	h, ok := mf.For(hub)
	if !ok {
		return errors.New("this machine is not in " + hub)
	}
	h.MemberToken = token
	mf.Put(h)
	return config.SaveMachine(e.defaultPath, mf)
}

// memberPairCommand prints a one-time link to open the hub in the app as you.
func memberPairCommand() *Command {
	return &Command{
		Name:     "pair",
		Summary:  "Print a one-time link to open this hub in the myco app as you (another device)",
		Mutating: true,
		Long: `Creates a one-time link (24 hours) for the myco app on another device. Opening it there signs that
device in as you; it shows up in your device list, where you can remove it. Works with the
credential "myco member accept" stored on this machine, or with the hub's admin token (the owner).`,
		Examples: []string{"myco member pair"},
		Flags:    []Flag{{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (instead of MYCO_ADMIN_TOKEN)"}},
		Run: handler(func(e *env) error {
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "member_pair"})
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			pl, err := c.CreatePairing(e.in.Ctx)
			if err != nil {
				return err
			}
			return e.p.Print(pl, func(w io.Writer) {
				fmt.Fprintf(w, "Open this link in the myco app (works once, until %s):\n  %s\n", pl.ExpiresAt.Local().Format("2006-01-02 15:04"), pl.Link)
			})
		}),
	}
}
