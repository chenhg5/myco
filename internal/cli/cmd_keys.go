package cli

import (
	"fmt"
	"io"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/e2e"
)

// hubSettingsCommand shows or changes the hub's settings (end-to-end
// encryption: the owner).
func hubSettingsCommand() *Command {
	return &Command{
		Name:     "settings",
		Summary:  "Show or change the hub's settings (end-to-end encryption: the owner)",
		Mutating: true,
		Long: `End-to-end encryption (off by default) seals message bodies and attachments so that only the two
sides of a message and the people who own them can read them; the hub, its admins and other members
cannot. Turn it on for a hub shared by several people. Agents and apps that are too old to encrypt
keep sending plain messages, marked as unencrypted.`,
		Examples: []string{"myco hub settings", "myco hub settings --e2e on"},
		Flags: []Flag{
			{Name: "e2e", Kind: Enum, Values: []string{"", "on", "off"}, Help: "Turn end-to-end encryption on or off"},
			{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (instead of MYCO_ADMIN_TOKEN)"},
		},
		Run: handler(func(e *env) error {
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			var st *api.HubSettings
			if v := e.in.String("e2e"); v != "" {
				if e.in.Bool("dry-run") {
					return e.dryRun(map[string]any{"action": "hub_settings", "e2e": v == "on"})
				}
				st, err = c.SetHubSettings(e.in.Ctx, api.HubSettings{E2E: v == "on"})
			} else {
				st, err = c.HubSettings(e.in.Ctx)
			}
			if err != nil {
				return err
			}
			return e.p.Print(st, func(w io.Writer) {
				state := "off"
				if st.E2E {
					state = "on: message bodies and attachments are sealed for the two sides and their owners"
				}
				fmt.Fprintf(w, "end-to-end encryption: %s\n", state)
			})
		}),
	}
}

// keysCommands shows this agent's keys and re-trusts a peer's new keys.
func keysCommands() *Command {
	show := &Command{
		Name:     "show",
		Summary:  "Show this agent's encryption key fingerprint and whether the hub encrypts",
		Examples: []string{"myco keys show"},
		Run: handler(func(e *env) error {
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			on := c.E2EEnabled(e.in.Ctx)
			out := map[string]any{"agent": e.cfg.Agent, "e2e": on}
			if c.Sealer != nil && c.Sealer.ID != nil {
				pub := c.Sealer.ID.Public()
				out["fingerprint"] = e2e.Fingerprint(pub)
				out["enc_key"], out["sign_key"] = pub.Enc, pub.Sign
			}
			return e.p.Print(out, func(w io.Writer) {
				fmt.Fprintf(w, "agent:        %s\nfingerprint:  %v\nhub encrypts: %v\n", e.cfg.Agent, out["fingerprint"], on)
			})
		}),
	}
	trust := &Command{
		Name:     "trust",
		Summary:  "Accept an agent's new encryption keys (after it was set up again)",
		Mutating: true,
		Long: `myco pins every agent's keys the first time it sees them (~/.myco/known_keys.json) and refuses to
send to, or read from, an agent whose keys changed: the hub could have swapped them. When you know
the agent made new keys (its machine or profile was set up again), compare the fingerprint with its
owner (myco keys show on its machine), then trust the new keys.`,
		Examples: []string{"myco keys trust --who laohu-claude"},
		Flags:    []Flag{{Name: "who", Kind: String, Required: true, Help: "The agent whose keys changed"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("who"); err != nil {
				return err
			}
			who := e.in.String("who")
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "keys_trust", "who": who})
			}
			if err := e.knownKeys().Trust(who); err != nil {
				return err
			}
			return e.p.Print(map[string]any{"who": who, "trusted": true}, func(w io.Writer) {
				fmt.Fprintf(w, "forgot the pinned keys of %s; its current keys are pinned the next time they are used\n", who)
			})
		}),
	}
	return (&Command{Name: "keys", Summary: "End-to-end encryption keys: show yours, trust a peer's new ones"}).Add(show, trust)
}
