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
		Summary:  "Invite someone: prints an app link that opens this hub as them",
		Mutating: true,
		Long: `Adds a person to the hub and prints a link for the myco app (Mac or Windows): opening it there
signs them in as that person. Keep the link private; it works until the person is removed.
Members see every node and conversation and talk to any node, but manage only what they add;
admins manage everyone's.`,
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
				fmt.Fprintf(w, "invited %s as %s.\n\nSend them this link (private) and ask them to open it in the myco app:\n  %s\n", res.Member.Name, res.Member.Role, res.Link)
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
	return (&Command{Name: "member", Summary: "The people on this hub: invite, roles, remove"}).Add(list, invite, update, remove)
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
