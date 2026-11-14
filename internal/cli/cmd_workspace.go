package cli

import (
	"fmt"
	"io"
	"strings"

	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
)

// workspaceCommands manage the workspaces one hub serves: each is isolated
// (its own members, agents, projects and messages) at <hub>/w/<name>, on the
// hub's own address, port and certificate.
func workspaceCommands() *Command {
	adminFlag := Flag{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (instead of MYCO_ADMIN_TOKEN)"}
	create := &Command{
		Name:     "create",
		Summary:  "Add a workspace to this hub and print its owner's one-time app link",
		Mutating: true,
		Long: `A workspace is isolated: its own members, agents, projects, messages and files, in its own
database on the hub (<data dir>/workspaces/<name>/). It is served on the hub's own address, at
<hub>/w/<name>, so it needs no new port, domain, certificate or service. The hub itself stays the
default workspace, as before.

Run it on the hub's server (the admin token is read from hub.env), or anywhere with the admin
token. It prints the app link that signs in as the new workspace's owner; machines and agents join
it with links made there, as with any hub.`,
		Examples: []string{
			"sudo myco workspace create --name team-b     # on a server installed with install-hub.sh",
			"myco workspace create --name side-project    # on a machine running `myco hub up`",
		},
		Flags: []Flag{
			{Name: "name", Kind: String, Required: true, Help: "Workspace name: lowercase letters, digits and '-', up to 40"},
			adminFlag,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name"); err != nil {
				return err
			}
			c, tok, err := e.serverAdmin()
			if err != nil {
				return err
			}
			name := strings.TrimSpace(e.in.String("name"))
			if e.in.Bool("dry-run") {
				ws, err := c.CreateWorkspace(e.in.Ctx, name, true)
				if err != nil {
					return err
				}
				return e.dryRun(map[string]any{"action": "workspace_create", "workspace": ws})
			}
			ws, err := c.CreateWorkspace(e.in.Ctx, name, false)
			if err != nil {
				return err
			}
			out := map[string]any{"workspace": ws}
			// Its owner's way in: the same one-time app link a new hub gives.
			wc, err := client.New(ws.URL, tok)
			if err == nil {
				if link, err := wc.CreatePairing(e.in.Ctx); err == nil {
					out["link"] = link.Link
					out["expires_at"] = link.ExpiresAt
				}
			}
			return e.p.Print(out, func(w io.Writer) {
				fmt.Fprintf(w, "workspace %s created: %s\n", ws.Name, ws.URL)
				if l, ok := out["link"].(string); ok {
					fmt.Fprintf(w, "\nApp link for its owner (works once, for 24 hours):\n\n  %s\n\n", l)
					fmt.Fprintln(w, "Paste it into the myco app (连接已有的 Hub). Keep it private: it signs in as the owner.")
				} else {
					fmt.Fprintf(w, "Get its owner's app link with: myco console link --hub %s\n", ws.URL)
				}
			})
		}),
	}
	list := &Command{
		Name:    "list",
		Summary: "List the workspaces this hub serves (besides itself, the default one)",
		Flags:   []Flag{adminFlag},
		Run: handler(func(e *env) error {
			c, _, err := e.serverAdmin()
			if err != nil {
				return err
			}
			ws, err := c.Workspaces(e.in.Ctx)
			if err != nil {
				return err
			}
			return e.p.Print(map[string]any{"workspaces": nonNilSlice(ws)}, func(w io.Writer) {
				if len(ws) == 0 {
					fmt.Fprintln(w, "only the default workspace (the hub itself); add one with: myco workspace create --name <name>")
					return
				}
				for _, x := range ws {
					fmt.Fprintf(w, "%-24s %s  (since %s)\n", x.Name, x.URL, x.CreatedAt.Local().Format("2006-01-02"))
				}
			})
		}),
	}
	remove := &Command{
		Name:     "remove",
		Summary:  "Take a workspace off this hub (its data is kept aside on the server)",
		Mutating: true,
		Long: `Everyone in it loses access at once: its agents, machines and app links stop working. Its
data is not deleted: it moves to <data dir>/workspaces/.removed/<name>-<time>/ on the server, from
where it can be moved back (stop the hub, move the folder to workspaces/<name>, start the hub).`,
		Examples: []string{"sudo myco workspace remove --name team-b"},
		Flags: []Flag{
			{Name: "name", Kind: String, Required: true, Help: "The workspace to remove"},
			adminFlag,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name"); err != nil {
				return err
			}
			c, _, err := e.serverAdmin()
			if err != nil {
				return err
			}
			name := strings.TrimSpace(e.in.String("name"))
			if e.in.Bool("dry-run") {
				ws, err := c.Workspaces(e.in.Ctx)
				if err != nil {
					return err
				}
				for _, x := range ws {
					if x.Name == name {
						return e.dryRun(map[string]any{"action": "workspace_remove", "workspace": x})
					}
				}
				return errorf("not_found", ExitNotFound, "list them: myco workspace list", "no workspace %q on this hub", name)
			}
			if err := c.RemoveWorkspace(e.in.Ctx, name); err != nil {
				return err
			}
			return e.p.Print(map[string]any{"removed": name}, func(w io.Writer) {
				fmt.Fprintf(w, "workspace %s removed; its data is kept in workspaces/.removed/ on the server\n", name)
			})
		}),
	}
	return (&Command{Name: "workspace", Summary: "Workspaces this hub serves: each isolated, all on its one address (server owner)"}).Add(create, list, remove)
}

// serverAdmin is a client for the hub's own address (a workspace URL in
// MYCO_HUB is taken back to the hub) with the admin token, and the token.
func (e *env) serverAdmin() (*client.Client, string, error) {
	hub, tok, err := e.adminCredentials()
	if err != nil {
		return nil, "", err
	}
	if i := strings.Index(hub, "/w/"); i > 0 {
		hub = hub[:i]
	}
	c, err := client.New(hub, tok)
	return c, tok, err
}
