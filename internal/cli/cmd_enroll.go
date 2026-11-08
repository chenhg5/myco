package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/harness/execnode"
	"github.com/chenhg5/myco/internal/osutil"
)

func enrollmentCommands() *Command {
	adminFlag := Flag{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (instead of MYCO_ADMIN_TOKEN)"}
	create := &Command{
		Name:     "create",
		Summary:  "Issue an enrollment token that lets sessions join by themselves (admin)",
		Mutating: true,
		Long: `Put the token in an environment's secrets as MYCO_ENROLL_TOKEN (with MYCO_HUB). Any
session started there then becomes an agent on its first myco command, and gets the
same agent back after a restart. The token can only enroll the calling session, under
--name-prefix, with pull delivery; it cannot administer the hub.`,
		Examples: []string{
			"myco enrollment create --name-prefix cloud- --harness claude-cloud --note \"claude.ai env\"",
			"myco enrollment create --name-prefix ci- --max-uses 10 --expires 720h --allow-from orchestrator",
			"myco enrollment create --name-prefix wsl- --harness exec --allow-from cloud-1 --max-uses 1 --expires 24h   # one exec node",
		},
		Flags: []Flag{
			{Name: "name-prefix", Kind: String, Required: true, Help: "Agent names created with this token start with it, e.g. cloud-"},
			{Name: "harness", Kind: String, Help: "Only admit this harness (e.g. claude-cloud)"},
			{Name: "allow-from", Kind: List, Help: "allow-from list given to enrolled agents"},
			{Name: "capability", Kind: List, Help: "Default capabilities of enrolled agents"},
			{Name: "label", Kind: List, Help: "Labels given to every agent enrolled with this token, e.g. machine=mac"},
			{Name: "max-uses", Kind: Int, Default: "0", Help: "New agents it may create (0 = unlimited; recovery is free)"},
			{Name: "expires", Kind: Duration, Default: "0s", Help: "Lifetime, e.g. 720h (0 = never)"},
			{Name: "note", Kind: String, Help: "Free-text note (where the token is used)"},
			adminFlag,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name-prefix"); err != nil {
				return err
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			dry := e.in.Bool("dry-run")
			res, err := c.CreateEnrollment(e.in.Ctx, api.CreateEnrollmentRequest{
				NamePrefix: e.in.String("name-prefix"), Harness: e.in.String("harness"),
				AllowFrom: e.in.List("allow-from"), Capabilities: e.in.List("capability"), Labels: e.in.List("label"),
				MaxUses: e.in.Int("max-uses"), TTLSeconds: int64(e.in.Duration("expires").Seconds()), Note: e.in.String("note"),
			}, dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "create_enrollment", "enrollment": res.Enrollment})
			}
			return e.p.Print(res, func(w io.Writer) {
				fmt.Fprintf(w, "enrollment %s created (prefix %s)\ntoken: %s\n(store it now; it is not shown again)\n",
					res.Enrollment.ID, res.Enrollment.NamePrefix, res.Token)
			})
		}),
	}

	list := &Command{
		Name:     "list",
		Summary:  "List enrollments and their usage (admin)",
		Examples: []string{"myco enrollment list"},
		Flags:    []Flag{adminFlag},
		Run: handler(func(e *env) error {
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			list, err := c.ListEnrollments(e.in.Ctx)
			if err != nil {
				return err
			}
			if list == nil {
				list = []api.Enrollment{}
			}
			return e.p.Print(api.EnrollmentList{Enrollments: list}, func(w io.Writer) {
				tw := newTable(w)
				fmt.Fprintln(tw, "ID\tACTIVE\tPREFIX\tHARNESS\tUSES\tEXPIRES\tNOTE")
				for _, en := range list {
					uses := fmt.Sprintf("%d/%d", en.Uses, en.MaxUses)
					if en.MaxUses == 0 {
						uses = fmt.Sprintf("%d/∞", en.Uses)
					}
					exp := "never"
					if en.ExpiresAt != nil {
						exp = en.ExpiresAt.Local().Format("2006-01-02 15:04")
					}
					fmt.Fprintf(tw, "%s\t%v\t%s\t%s\t%s\t%s\t%s\n", en.ID, en.Active, en.NamePrefix, dash(en.Harness), uses, exp, truncate(en.Note, 40))
				}
				tw.Flush()
			})
		}),
	}

	revoke := &Command{
		Name:     "revoke",
		Summary:  "Disable an enrollment token (agents it created keep working) (admin)",
		Mutating: true,
		Examples: []string{"myco enrollment revoke --id enr_abc123 --dry-run", "myco enrollment revoke --id enr_abc123 --force"},
		Flags: []Flag{
			{Name: "id", Kind: String, Required: true, Help: "Enrollment ID"},
			{Name: "force", Kind: Bool, Help: "Required to actually revoke"},
			adminFlag,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			dry := e.in.Bool("dry-run")
			if !dry && !e.in.Bool("force") {
				return errorf("confirmation_required", ExitUsage, "re-run with --force (or --dry-run to preview)",
					"revoking enrollment %s stops new sessions from joining with it", e.in.String("id"))
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			res, err := c.RevokeEnrollment(e.in.Ctx, e.in.String("id"), dry)
			if err != nil {
				return err
			}
			if dry {
				res["action"] = "revoke_enrollment"
				return e.dryRun(res)
			}
			return e.p.Print(res, func(w io.Writer) { fmt.Fprintf(w, "enrollment %s revoked\n", e.in.String("id")) })
		}),
	}
	return (&Command{Name: "enrollment", Summary: "Manage self-service enrollment tokens (admin)"}).Add(create, list, revoke)
}

func agentEnrollCommand() *Command {
	return &Command{
		Name:     "enroll",
		Summary:  "Join the network as the current session using an enrollment token",
		Mutating: true,
		Long: `Idempotent: the same session always gets the same agent back (with a fresh token), so
it is safe to run on every container start. Other myco commands do this automatically
when MYCO_ENROLL_TOKEN is set and no agent token is configured.`,
		Examples: []string{
			"myco agent enroll                                # detect the session, derive the name",
			"myco agent enroll --name cloud-reviewer --description \"reviews PRs\" --capability review",
			"myco agent enroll --harness claude --session-id 7f3c... --enroll-token-file enroll.token",
			"myco agent enroll --harness exec --name wsl-shell --workdir ~/exec     # exec node (needs an exec enrollment)",
		},
		Flags: []Flag{
			{Name: "name", Kind: String, Help: "Agent name (must carry the enrollment's prefix; default: derived from the session id)"},
			{Name: "display-name", Kind: String, Help: "Human-friendly name shown in lists and the dashboard"},
			{Name: "description", Kind: String, Help: "What this agent does"},
			{Name: "capability", Kind: List, Help: "Capability tags (default: the enrollment's)"},
			{Name: "label", Kind: List, Help: "Labels added to the enrollment's defaults, e.g. group=loops"},
			{Name: "detect", Kind: Bool, Default: "true", Help: "Detect harness/session/environment from this process (--no-detect to disable)"},
			{Name: "harness", Kind: String, Help: "Harness (overrides detection)"},
			{Name: "session-id", Kind: String, Help: "Session ID (overrides detection)"},
			{Name: "environment", Kind: String, Help: "Environment (overrides detection)"},
			{Name: "workdir", Kind: String, Help: "Working directory"},
			{Name: "enroll-token-file", Kind: String, Help: "File with the enrollment token (instead of MYCO_ENROLL_TOKEN)"},
		},
		Run: handler(func(e *env) error {
			hub, err := e.hubURL()
			if err != nil {
				return err
			}
			tok := e.in.Getenv(config.EnvEnrollToken)
			if f := e.in.String("enroll-token-file"); f != "" {
				if err := safePath(f); err != nil {
					return err
				}
				b, err := os.ReadFile(f)
				if err != nil {
					return errorf("not_found", ExitNotFound, "check the path", "cannot read --enroll-token-file: %v", err)
				}
				tok = strings.TrimSpace(string(b))
			}
			if tok == "" {
				return errorf("not_configured", ExitUsage, "set MYCO_ENROLL_TOKEN or pass --enroll-token-file",
					"no enrollment token")
			}
			dry := e.in.Bool("dry-run")
			binding := e.bindingFromFlags()
			exec := e.in.String("harness") == execnode.Name
			if exec {
				if e.in.String("name") == "" {
					return errorf("invalid_argument", ExitUsage, "pass --name, e.g. --name wsl-shell (it must start with the enrollment's prefix)",
						"exec nodes need an explicit --name")
				}
				b, err := execBinding(e, e.in.String("name"), e.in.String("workdir"))
				if err != nil {
					return err
				}
				binding = b
			}
			res, err := e.enroll(hub, tok, api.EnrollRequest{
				Name: e.in.String("name"), DisplayName: e.in.String("display-name"), Description: e.in.String("description"),
				Capabilities: e.in.List("capability"), Labels: e.in.List("label"), Binding: binding,
			}, dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "enroll", "agent": res.Agent, "would_create": res.Created})
			}
			out := map[string]any{"agent": res.Agent, "created": res.Created, "config": e.cfgPath}
			return e.p.Print(out, func(w io.Writer) {
				verb := "enrolled"
				if !res.Created {
					verb = "recovered (new token issued)"
				}
				fmt.Fprintf(w, "agent %s %s; credentials saved to %s\n", res.Agent.Name, verb, e.cfgPath)
				if exec {
					fmt.Fprintf(w, "exec node: commands from %s will run here as %s in %s\n",
						strings.Join(res.Agent.AllowFrom, ", "), orDefault(osutil.UserName(e.in.Getenv), "this user"), res.Agent.Binding.Workdir)
					fmt.Fprintln(w, "a running `myco runner service` picks it up within 30s; otherwise run: myco runner service install")
				}
			})
		}),
	}
}
