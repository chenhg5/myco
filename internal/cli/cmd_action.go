package cli

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
)

// Agent actions (hubs from v0.21.21): an agent asks a computer's owner to
// let it act there (see the screen, use the keyboard and mouse), then runs
// actions the computer's App carries out.

func actionCommands() *Command {
	request := &Command{
		Name: "request", Summary: "Ask a computer's owner to let you act on it (they approve in the myco app)", Mutating: true,
		Long: `The computer's owner gets a card in the myco app showing you, the computer, your reason and what
you asked for; they may allow less, or say no. With --wait, the command waits for their decision.
A grant lasts 5 minutes by default (30 at most) and ends with your task (--task) if you name it.
remote.desktop.view lets you take screenshots; remote.desktop.control also lets you type and click.
For an enabled SSH profile, use action targets to get --machine, --ssh-profile and --binding.
server.ssh.read inspects Linux status/logs; server.ssh.manage also starts/stops/restarts services
and containers. The owner approves the resolved SSH endpoint; no arbitrary shell is offered.
A shared page you can see (--page) needs no approval: page.export lets its app make a file of it
(an SVG, PNG or what the app offers), run by the App of a person who sees the page.`,
		Examples: []string{
			`myco action grant request --machine studio --permission remote.desktop.view --reason "check the build window" --wait 5m`,
			`myco action grant request --machine studio --permission remote.desktop.control --reason "fill the form" --task $MYCO_MESSAGE_ID`,
			`myco action grant request --page pg_abc --permission page.export --reason "export the board as SVG"`,
		},
		Flags: []Flag{
			{Name: "machine", Kind: String, Help: "The computer: its name or id (or --page)"},
			{Name: "ssh-profile", Kind: String, Help: "An enabled SSH profile id from action targets (requires --machine and --binding)"},
			{Name: "binding", Kind: String, Help: "The SSH profile SHA256 binding from action targets"},
			{Name: "page", Kind: String, Help: "A shared page (pg_...) instead of a computer"},
			{Name: "permission", Kind: List, Required: true, Help: "remote.desktop.view or remote.desktop.control for a computer; server.ssh.read/manage for a bound SSH profile; page.export for a page (repeatable)"},
			{Name: "reason", Kind: String, Required: true, Help: "Why: its owner reads this before approving"},
			{Name: "ttl", Kind: Duration, Default: "5m", Help: "How long you need it (at most 30m; its owner may give less)"},
			{Name: "task", Kind: String, Help: "The task it is for (msg_...): the grant ends with it"},
			{Name: "wait", Kind: Duration, Default: "0s", Help: "Wait this long for the owner's decision"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("permission", "reason"); err != nil {
				return err
			}
			target := api.ActionTarget{Kind: "machine", ID: e.in.String("machine")}
			if p := e.in.String("page"); p != "" {
				target = api.ActionTarget{Kind: "page", ID: p}
			}
			if (e.in.String("machine") == "") == (e.in.String("page") == "") {
				return errorf("invalid_usage", ExitUsage, "pass --machine <computer> or --page <pg_...>", "name one target")
			}
			if id := e.in.String("ssh-profile"); id != "" || e.in.String("binding") != "" {
				if target.Kind != "machine" || id == "" || e.in.String("binding") == "" {
					return errorf("invalid_usage", ExitUsage, "SSH needs --machine, --ssh-profile and --binding", "name the bound target")
				}
				target.Resource = &api.ActionResource{Provider: "server.ssh", ID: id, Binding: e.in.String("binding")}
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			req := api.ActionGrantRequest{Target: target, Permissions: e.in.List("permission"),
				Reason: e.in.String("reason"), MessageID: e.in.String("task"), TTLSeconds: int(e.in.Duration("ttl") / time.Second)}
			if e.in.Bool("dry-run") {
				g, err := c.RequestActionGrant(e.in.Ctx, req, true)
				if err != nil {
					return err
				}
				return e.dryRun(map[string]any{"action": "action_grant_request", "grant": g})
			}
			g, err := c.RequestActionGrant(e.in.Ctx, req, false)
			if err != nil {
				return err
			}
			if wait := e.in.Duration("wait"); wait > 0 {
				deadline := time.Now().Add(wait)
				for g.State == api.GrantPending && time.Now().Before(deadline) {
					step := time.Until(deadline)
					if step > 55*time.Second {
						step = 55 * time.Second
					}
					if g, err = c.ActionGrant(e.in.Ctx, g.ID, step); err != nil {
						return err
					}
				}
			}
			return e.p.Print(g, func(w io.Writer) { printGrant(w, g) })
		}),
	}
	list := &Command{
		Name: "list", Summary: "Your grants: asked, active and ended",
		Examples: []string{"myco action grant list"},
		Run: handler(func(e *env) error {
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			gs, err := c.ActionGrants(e.in.Ctx)
			if err != nil {
				return err
			}
			return e.p.Print(api.ActionGrantList{Grants: gs}, func(w io.Writer) {
				if len(gs) == 0 {
					fmt.Fprintln(w, "no grants")
				}
				for i := range gs {
					printGrant(w, &gs[i])
				}
			})
		}),
	}
	get := &Command{
		Name: "get", Summary: "One of your grants; --wait waits while its owner decides",
		Examples: []string{"myco action grant get --id agr_abc123 --wait 2m"},
		Flags: []Flag{{Name: "id", Kind: String, Required: true, Help: "The grant (agr_...)"},
			{Name: "wait", Kind: Duration, Default: "0s", Help: "Wait this long while it is pending (max 60s per call)"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			g, err := c.ActionGrant(e.in.Ctx, e.in.String("id"), e.in.Duration("wait"))
			if err != nil {
				return err
			}
			return e.p.Print(g, func(w io.Writer) { printGrant(w, g) })
		}),
	}
	end := &Command{
		Name: "end", Summary: "End one of your grants (do this as soon as you are done)", Mutating: true,
		Examples: []string{"myco action grant end --id agr_abc123"},
		Flags:    []Flag{{Name: "id", Kind: String, Required: true, Help: "The grant (agr_...)"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			g, err := c.EndActionGrant(e.in.Ctx, e.in.String("id"), e.in.Bool("dry-run"))
			if err != nil {
				return err
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "action_grant_end", "grant": g})
			}
			return e.p.Print(g, func(w io.Writer) { printGrant(w, g) })
		}),
	}
	grant := (&Command{Name: "grant", Summary: "Grants: what a computer's owner lets you do there, for how long"}).Add(request, list, get, end)

	actions := &Command{
		Name: "list", Summary: "What a grant lets you do on its computer now (names, versions, input schemas)",
		Examples: []string{"myco action list --grant agr_abc123"},
		Flags:    []Flag{{Name: "grant", Kind: String, Required: true, Help: "The grant (agr_...)"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("grant"); err != nil {
				return err
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			list, err := c.GrantActions(e.in.Ctx, e.in.String("grant"))
			if err != nil {
				return err
			}
			return e.p.Print(api.ActionDescriptorList{Actions: list}, func(w io.Writer) {
				if len(list) == 0 {
					fmt.Fprintln(w, "nothing available now (is the myco app open on that computer, with remote control on?)")
				}
				for _, d := range list {
					fmt.Fprintf(w, "%s v%d  %s  (%s)\n", d.Name, d.Version, d.Title, d.Permission)
				}
			})
		}),
	}
	run := &Command{
		Name: "run", Summary: "Run an action on a grant's computer and wait for it; screenshots are saved to a file",
		Long: `The computer's App carries the action out. A screenshot (or other file) the action makes is saved
(--save, or a temporary file) and its path printed: look at it before deciding the next step.
Retrying with the same --key returns the first job instead of running it twice. When an input
fails with outcome_unknown, observe again before trying anything else.`,
		Examples: []string{
			`myco action run --grant agr_abc123 --action remote.desktop.observe --save screen.jpg`,
			`myco action run --grant agr_abc123 --action remote.desktop.input --args '{"frame_id":"f1","events":[{"kind":"text","text":"hello"}]}'`,
		},
		Flags: []Flag{
			{Name: "grant", Kind: String, Required: true, Help: "The grant (agr_...)"},
			{Name: "action", Kind: String, Required: true, Help: "e.g. remote.desktop.observe"},
			{Name: "version", Kind: Int, Default: "1", Help: "The action's version"},
			{Name: "args", Kind: String, Default: "{}", Help: "Its arguments, JSON"},
			{Name: "key", Kind: String, Help: "Idempotency key (default: a new one)"},
			{Name: "wait", Kind: Duration, Default: "60s", Help: "Wait this long for it to finish (0: return at once)"},
			{Name: "save", Kind: String, Help: "Where to save what it makes (a file; with several, a folder)"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("grant", "action"); err != nil {
				return err
			}
			args := json.RawMessage(e.in.String("args"))
			if !json.Valid(args) {
				return errorf("invalid_argument", ExitUsage, `pass --args as JSON, e.g. '{"max_width":1440}'`, "--args is not JSON")
			}
			key := e.in.String("key")
			if key == "" {
				var b [8]byte
				_, _ = rand.Read(b[:])
				key = "cli-" + hex.EncodeToString(b[:])
			}
			inv := api.ActionInvocation{GrantID: e.in.String("grant"), Action: e.in.String("action"), Version: e.in.Int("version"), Args: args, IdempotencyKey: key}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "action_run", "invocation": inv})
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			job, err := c.RunAction(e.in.Ctx, inv)
			if err != nil {
				return err
			}
			deadline := time.Now().Add(e.in.Duration("wait"))
			for (job.State == api.JobQueued || job.State == api.JobRunning) && time.Now().Before(deadline) {
				step := time.Until(deadline)
				if step > 55*time.Second {
					step = 55 * time.Second
				}
				if job, err = c.ActionJob(e.in.Ctx, job.ID, step); err != nil {
					return err
				}
			}
			saved := []string{}
			for i, a := range job.Artifacts {
				data, _, err := c.ActionArtifact(e.in.Ctx, a.ID)
				if err != nil {
					return err
				}
				path := artifactPath(e.in.String("save"), a, i, len(job.Artifacts))
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					return err
				}
				if err := os.WriteFile(path, data, 0o600); err != nil {
					return err
				}
				saved = append(saved, path)
			}
			out := map[string]any{"job": job, "saved": saved}
			if job.State == api.JobQueued || job.State == api.JobRunning {
				out["hint"] = "still running: myco action job --id " + job.ID + " --wait 60s"
			}
			return e.p.Print(out, func(w io.Writer) {
				fmt.Fprintf(w, "%s %s: %s\n", job.ID, job.Action, job.State)
				if job.Error != nil {
					fmt.Fprintf(w, "  %s: %s\n", job.Error.Code, job.Error.Message)
				}
				if len(job.Result) > 0 {
					fmt.Fprintf(w, "  result: %s\n", oneLine(string(job.Result), 400))
				}
				for _, p := range saved {
					fmt.Fprintf(w, "  saved: %s\n", p)
				}
			})
		}),
	}
	job := &Command{
		Name: "job", Summary: "One of your jobs; --wait waits for it to finish",
		Examples: []string{"myco action job --id ajb_abc123 --wait 60s"},
		Flags: []Flag{{Name: "id", Kind: String, Required: true, Help: "The job (ajb_...)"},
			{Name: "wait", Kind: Duration, Default: "0s", Help: "Wait this long for it to finish (max 60s)"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			j, err := c.ActionJob(e.in.Ctx, e.in.String("id"), e.in.Duration("wait"))
			if err != nil {
				return err
			}
			return e.p.Print(j, func(w io.Writer) { fmt.Fprintf(w, "%s %s: %s\n", j.ID, j.Action, j.State) })
		}),
	}
	cancel := &Command{
		Name: "cancel", Summary: "Cancel a job that has not run, or ask a running one to stop (what it did stays done)", Mutating: true,
		Examples: []string{"myco action cancel --id ajb_abc123"},
		Flags:    []Flag{{Name: "id", Kind: String, Required: true, Help: "The job (ajb_...)"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "action_cancel", "job": e.in.String("id")})
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			j, err := c.CancelActionJob(e.in.Ctx, e.in.String("id"))
			if err != nil {
				return err
			}
			return e.p.Print(j, func(w io.Writer) { fmt.Fprintf(w, "%s %s: %s\n", j.ID, j.Action, j.State) })
		}),
	}
	targets := &Command{Name: "targets", Summary: "Enabled private resources on your computers (request a grant before use)", Run: handler(func(e *env) error {
		c, err := e.agentClient()
		if err != nil {
			return err
		}
		ts, err := c.ActionTargets(e.in.Ctx)
		if err != nil {
			return err
		}
		return e.p.Print(api.ActionTargetList{Targets: ts}, func(w io.Writer) {
			for _, t := range ts {
				fmt.Fprintf(w, "%s  %s  %s  %s\n", t.ID, t.Resource.ID, t.Resource.Binding, t.Resource.Summary)
			}
		})
	})}
	return (&Command{Name: "action", Summary: "Act on a computer with its owner's approval: screenshots, keyboard and mouse"}).Add(grant, targets, actions, run, job, cancel)
}

func printGrant(w io.Writer, g *api.ActionGrant) {
	line := fmt.Sprintf("%s  %-7s %s on %s", g.ID, g.State, strings.Join(g.Permissions, ","), g.TargetName)
	if g.State == api.GrantPending {
		line = fmt.Sprintf("%s  %-7s asked %s on %s (waiting for %s)", g.ID, g.State, strings.Join(g.Requested, ","), g.TargetName, g.Owner)
	}
	if g.ExpiresAt != nil && g.State == api.GrantActive {
		line += ", until " + g.ExpiresAt.Local().Format("15:04:05")
	}
	if g.EndReason != "" {
		line += " (" + g.EndReason + ")"
	}
	fmt.Fprintln(w, line)
}

// artifactPath is where the i-th of n artifacts goes: --save as given for
// one, a folder for several, else a temporary file.
func artifactPath(save string, a api.ActionArtifact, i, n int) string {
	ext := ".bin"
	switch a.MediaType {
	case "image/png":
		ext = ".png"
	case "image/jpeg":
		ext = ".jpg"
	case "image/webp":
		ext = ".webp"
	case "text/plain":
		ext = ".txt"
	case "application/json":
		ext = ".json"
	}
	switch {
	case save != "" && n == 1:
		return save
	case save != "":
		return filepath.Join(save, fmt.Sprintf("%d-%s%s", i+1, a.ID, ext))
	}
	return filepath.Join(os.TempDir(), "myco-actions", a.ID+ext)
}
