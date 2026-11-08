package cli

import (
	"fmt"
	"io"
	"time"

	. "github.com/chenhg5/myco/internal/cli/cmdkit"
)

// consoleCommands manage console tokens: per-device admin credentials for
// the console app, so the hub's admin token never leaves the hub machine.
func consoleCommands() *Command {
	adminFlag := Flag{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (instead of MYCO_ADMIN_TOKEN)"}
	create := &Command{
		Name:     "create",
		Summary:  "Issue a console token for one device (shown once)",
		Mutating: true,
		Long: `Run it on the hub machine (the admin token is read from hub.env). Paste the token into the
console app, which keeps it in the system keychain. A console token can do what the admin token
does, except manage console tokens; revoke it on its own if the device is lost.`,
		Examples: []string{
			"sudo myco console token create --name glenn-mac   # on a server installed with install-hub.sh",
			"myco console token create --name glenn-mac        # on a machine running `myco hub up`",
		},
		Flags: []Flag{
			{Name: "name", Kind: String, Required: true, Help: "Device name, e.g. glenn-mac"},
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
			res, err := c.CreateConsoleToken(e.in.Ctx, e.in.String("name"))
			if err != nil {
				return err
			}
			// On the hub machine MYCO_HUB is a loopback address; the app
			// needs the public one.
			hub, _ := e.hubURL()
			if h, err := c.Health(e.in.Ctx); err == nil && h.PublicURL != "" {
				hub = h.PublicURL
			}
			out := map[string]any{"console_token": res.ConsoleToken, "token": res.Token, "hub": hub}
			return e.p.Print(out, func(w io.Writer) {
				fmt.Fprintf(w, "console token for %s (%s), shown only this once:\n\n  %s\n\n", res.ConsoleToken.Name, res.ConsoleToken.ID, res.Token)
				fmt.Fprintf(w, "Add this hub in the console app with that token. Hub address: %s\n", hub)
				fmt.Fprintf(w, "Revoke it with: myco console token revoke --id %s\n", res.ConsoleToken.ID)
			})
		}),
	}
	list := &Command{
		Name:     "list",
		Summary:  "List console tokens and when each was last used",
		Examples: []string{"sudo myco console token list"},
		Flags:    []Flag{adminFlag},
		Run: handler(func(e *env) error {
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			tokens, err := c.ListConsoleTokens(e.in.Ctx)
			if err != nil {
				return err
			}
			return e.p.Print(map[string]any{"tokens": nonNilSlice(tokens)}, func(w io.Writer) {
				tw := newTable(w)
				fmt.Fprintln(tw, "ID\tNAME\tACTIVE\tCREATED\tLAST USED")
				for _, t := range tokens {
					used := "never"
					if t.LastUsedAt != nil {
						used = t.LastUsedAt.Local().Format(time.DateTime)
					}
					fmt.Fprintf(tw, "%s\t%s\t%v\t%s\t%s\n", t.ID, t.Name, t.Active, t.CreatedAt.Local().Format(time.DateTime), used)
				}
				tw.Flush()
			})
		}),
	}
	revoke := &Command{
		Name:     "revoke",
		Summary:  "Revoke a console token (the device loses access at once)",
		Mutating: true,
		Examples: []string{"sudo myco console token revoke --id con_abc123"},
		Flags: []Flag{
			{Name: "id", Kind: String, Required: true, Help: "Console token ID (from `myco console token list`)"},
			adminFlag,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			t, err := c.RevokeConsoleToken(e.in.Ctx, e.in.String("id"))
			if err != nil {
				return err
			}
			return e.p.Print(t, func(w io.Writer) { fmt.Fprintf(w, "console token %s (%s) revoked\n", t.ID, t.Name) })
		}),
	}
	link := &Command{
		Name:     "link",
		Summary:  "Print a one-time app link: open it in the myco app to add this hub (works once, for 24 hours)",
		Mutating: true,
		Long: `Prints https://<hub>/c/<one-time code>. Paste it into the myco app ("连接已有的 Hub"): the app
trades the code for its own credential for that device, so the link stops working once used (or
after 24 hours). It signs in as the hub's owner; invite other people with: myco member invite.
Each device gets its own credential; list and remove them in the app (设置 → 成员 → 我的设备).`,
		Examples: []string{
			"sudo myco console link                  # on a server installed with install-hub.sh",
		},
		Flags: []Flag{
			{Name: "name", Kind: String, Help: "Unused (kept for older scripts): devices are named by the app"},
			adminFlag,
		},
		Run: handler(func(e *env) error {
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			res, err := c.CreatePairing(e.in.Ctx)
			if err != nil {
				return err
			}
			hub, _ := e.hubURL()
			// The hub builds the link from its public URL when it knows it.
			out := map[string]any{"link": res.Link, "hub": hub, "expires_at": res.ExpiresAt}
			return e.p.Print(out, func(w io.Writer) {
				fmt.Fprintf(w, "App link (works once, until %s):\n\n  %s\n\n", res.ExpiresAt.Local().Format("2006-01-02 15:04"), res.Link)
				fmt.Fprintln(w, "Paste it into the myco app (连接已有的 Hub). It signs in as this hub's owner: keep it private.")
			})
		}),
	}
	token := (&Command{Name: "token", Summary: "Console tokens: per-device admin credentials for the console app"}).Add(create, list, revoke)
	return (&Command{Name: "console", Summary: "The console app's access to this hub (admin)"}).Add(link, token)
}
