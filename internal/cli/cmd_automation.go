package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
)

// automationCommands manage automations: tasks the hub sends, as you, to an
// agent on a schedule or when a webhook is called (the hub's admin token, or
// a member's console token).
func automationCommands() *Command {
	adminFlag := Flag{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (instead of MYCO_ADMIN_TOKEN)"}
	idFlag := Flag{Name: "id", Kind: String, Required: true, Help: "Automation id (aut_...)"}
	list := &Command{
		Name:     "list",
		Summary:  "List automations (yours; everyone's for those who manage the hub)",
		Examples: []string{"myco automation list"},
		Flags:    []Flag{adminFlag},
		Run: handler(func(e *env) error {
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			as, err := c.Automations(e.in.Ctx)
			if err != nil {
				return err
			}
			return e.p.Print(api.AutomationList{Automations: as}, func(w io.Writer) {
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tNAME\tTO\tWHEN\tNEXT\tLAST")
				for _, a := range as {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", a.ID, a.Name, a.To, automationWhen(a), automationNext(a), automationLast(a.LastRun))
				}
				tw.Flush()
			})
		}),
	}
	show := &Command{
		Name:     "show",
		Summary:  "Show an automation and its recent runs",
		Examples: []string{"myco automation show --id aut_abc123"},
		Flags:    []Flag{idFlag, adminFlag},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			d, err := c.Automation(e.in.Ctx, e.in.String("id"))
			if err != nil {
				return err
			}
			return e.p.Print(d, func(w io.Writer) {
				printAutomation(w, d.Automation, "")
				if len(d.Runs) > 0 {
					fmt.Fprintln(w, "\nruns (newest first):")
					for _, r := range d.Runs {
						fmt.Fprintf(w, "  %s  %-8s %-7s %s%s\n", r.At.Local().Format("2006-01-02 15:04"), r.Trigger, r.Outcome, r.MessageID, sep(r.Error))
					}
				}
			})
		}),
	}
	scheduleFlags := []Flag{
		{Name: "cron", Kind: String, Help: `Schedule, 5 fields: minute hour day-of-month month day-of-week (e.g. "0 9 * * 1-5"); runs at least 10 minutes apart`},
		{Name: "tz", Kind: String, Help: "IANA time zone of the schedule, e.g. Asia/Shanghai (default UTC)"},
	}
	create := &Command{
		Name:     "create",
		Summary:  "Make an automation: send a task to an agent on a schedule and/or when its webhook is called",
		Mutating: true,
		Long: `The hub sends the message as you, an ordinary task, so it runs while your computers are off and the
answer lands in your chat with that agent. With --webhook it also gets a URL; the secret is printed once:
  curl -X POST <url> -H "Authorization: Bearer <secret>" --data 'build failed'
The call's body replaces {{payload}} in the message (or follows it). The message is stored on the hub in
plain text (not end-to-end encrypted).`,
		Examples: []string{
			`myco automation create --name "日报" --to pm --body "整理今天的进展，写成日报" --cron "0 18 * * 1-5" --tz Asia/Shanghai`,
			`myco automation create --name "CI 失败" --to backend --body "CI 失败了，看一下：{{payload}}" --webhook`,
		},
		Flags: append([]Flag{
			{Name: "name", Kind: String, Required: true, Help: "What to call it"},
			{Name: "to", Kind: String, Required: true, Help: "The agent it sends to"},
			{Name: "body", Kind: String, Help: "The message ({{payload}} = a webhook call's body)"},
			{Name: "body-file", Kind: String, Help: "Read the message from a file ('-' = stdin)"},
			{Name: "project", Kind: String, Help: "Put its tasks in this project"},
			{Name: "webhook", Kind: Bool, Help: "Give it a webhook URL (the secret is printed once)"},
			{Name: "skip-if-busy", Kind: Bool, Help: "Skip a run while the last task it sent is not finished"},
			{Name: "disabled", Kind: Bool, Help: "Make it turned off"},
		}, append(scheduleFlags, adminFlag)...),
		Run: handler(func(e *env) error {
			if err := e.in.Require("name", "to"); err != nil {
				return err
			}
			body, err := e.readBody("body", "body-file")
			if err != nil {
				return err
			}
			req := api.AutomationRequest{Name: strPtr(e.in.String("name")), To: strPtr(e.in.String("to")), Body: &body}
			if p := e.in.String("project"); p != "" {
				req.Project = &p
			}
			if cron := e.in.String("cron"); cron != "" {
				req.Schedule = &api.AutomationSchedule{Cron: cron, TZ: e.in.String("tz")}
			}
			if e.in.Bool("webhook") {
				req.Webhook = boolPtr(true)
			}
			if e.in.Bool("skip-if-busy") {
				req.SkipIfBusy = boolPtr(true)
			}
			if e.in.Bool("disabled") {
				req.Enabled = boolPtr(false)
			}
			if req.Schedule == nil && req.Webhook == nil {
				return errorf("invalid_argument", ExitUsage, `pass --cron "0 9 * * *" and/or --webhook`, "an automation needs a schedule or a webhook")
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "automation_create", "request": req})
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			res, err := c.CreateAutomation(e.in.Ctx, req)
			if err != nil {
				return err
			}
			return e.p.Print(res, func(w io.Writer) { printAutomation(w, res.Automation, res.WebhookSecret) })
		}),
	}
	update := &Command{
		Name:     "update",
		Summary:  "Change an automation (only what you pass), turn it on or off, or give its webhook a new secret",
		Mutating: true,
		Examples: []string{
			`myco automation update --id aut_abc123 --cron "30 8 * * *"`,
			"myco automation update --id aut_abc123 --disabled",
			"myco automation update --id aut_abc123 --rotate-secret",
		},
		Flags: append([]Flag{
			idFlag,
			{Name: "name", Kind: String, Help: "New name"},
			{Name: "to", Kind: String, Help: "Send to another agent"},
			{Name: "body", Kind: String, Help: "New message"},
			{Name: "body-file", Kind: String, Help: "Read the new message from a file ('-' = stdin)"},
			{Name: "project", Kind: String, Help: `Project for its tasks ("" for none)`},
			{Name: "no-schedule", Kind: Bool, Help: "Remove the schedule"},
			{Name: "webhook", Kind: Enum, Values: []string{"", "on", "off"}, Help: "Turn the webhook on (a new secret) or off"},
			{Name: "rotate-secret", Kind: Bool, Help: "A new webhook secret (the old one stops working)"},
			{Name: "skip-if-busy", Kind: Enum, Values: []string{"", "on", "off"}, Help: "Skip runs while its last task is not finished"},
			{Name: "enabled", Kind: Bool, Help: "Turn it on"},
			{Name: "disabled", Kind: Bool, Help: "Turn it off"},
		}, append(scheduleFlags, adminFlag)...),
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			var req api.AutomationRequest
			for flag, dst := range map[string]**string{"name": &req.Name, "to": &req.To, "project": &req.Project} {
				if e.in.IsSet(flag) {
					*dst = strPtr(e.in.String(flag))
				}
			}
			if e.in.IsSet("body") || e.in.IsSet("body-file") {
				body, err := e.readBody("body", "body-file")
				if err != nil {
					return err
				}
				req.Body = &body
			}
			if cron := e.in.String("cron"); cron != "" {
				req.Schedule = &api.AutomationSchedule{Cron: cron, TZ: e.in.String("tz")}
			} else if e.in.IsSet("tz") {
				return errorf("invalid_argument", ExitUsage, `pass --cron with --tz, e.g. --cron "0 9 * * *" --tz Asia/Shanghai`, "--tz goes with --cron")
			}
			req.ClearSchedule = e.in.Bool("no-schedule")
			switch e.in.String("webhook") {
			case "on":
				req.Webhook = boolPtr(true)
			case "off":
				req.Webhook = boolPtr(false)
			}
			req.RotateSecret = e.in.Bool("rotate-secret")
			switch e.in.String("skip-if-busy") {
			case "on":
				req.SkipIfBusy = boolPtr(true)
			case "off":
				req.SkipIfBusy = boolPtr(false)
			}
			if e.in.Bool("enabled") && e.in.Bool("disabled") {
				return errorf("invalid_argument", ExitUsage, "pass one of --enabled or --disabled", "--enabled and --disabled together")
			}
			if e.in.Bool("enabled") {
				req.Enabled = boolPtr(true)
			}
			if e.in.Bool("disabled") {
				req.Enabled = boolPtr(false)
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "automation_update", "id": e.in.String("id"), "request": req})
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			res, err := c.UpdateAutomation(e.in.Ctx, e.in.String("id"), req)
			if err != nil {
				return err
			}
			return e.p.Print(res, func(w io.Writer) { printAutomation(w, res.Automation, res.WebhookSecret) })
		}),
	}
	remove := &Command{
		Name:     "remove",
		Summary:  "Remove an automation and its runs (the messages it sent stay)",
		Mutating: true,
		Examples: []string{"myco automation remove --id aut_abc123"},
		Flags:    []Flag{idFlag, adminFlag},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			id := e.in.String("id")
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "automation_remove", "id": id})
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			if err := c.DeleteAutomation(e.in.Ctx, id); err != nil {
				return err
			}
			return e.p.Print(map[string]any{"removed": id}, func(w io.Writer) { fmt.Fprintf(w, "removed %s\n", id) })
		}),
	}
	run := &Command{
		Name:     "run",
		Summary:  "Run an automation now (also when it is turned off)",
		Mutating: true,
		Examples: []string{"myco automation run --id aut_abc123"},
		Flags:    []Flag{idFlag, adminFlag},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			id := e.in.String("id")
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "automation_run", "id": id})
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			r, err := c.RunAutomation(e.in.Ctx, id)
			if err != nil {
				return err
			}
			return e.p.Print(r, func(w io.Writer) {
				fmt.Fprintf(w, "%s: %s%s%s\n", id, r.Outcome, sep(r.MessageID), sep(r.Error))
			})
		}),
	}
	return (&Command{Name: "automation", Summary: "Send tasks to agents on a schedule or from a webhook (run by the hub)"}).
		Add(list, show, create, update, remove, run)
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

func sep(s string) string {
	if s == "" {
		return ""
	}
	return " · " + s
}

func automationWhen(a api.Automation) string {
	var parts []string
	if a.Schedule != nil {
		s := a.Schedule.Cron
		if a.Schedule.TZ != "" {
			s += " (" + a.Schedule.TZ + ")"
		}
		parts = append(parts, s)
	}
	if a.Webhook {
		parts = append(parts, "webhook")
	}
	if !a.Enabled {
		parts = append(parts, "off")
	}
	return strings.Join(parts, " + ")
}

func automationNext(a api.Automation) string {
	if a.NextRunAt == nil {
		return "-"
	}
	return a.NextRunAt.Local().Format("01-02 15:04")
}

func automationLast(r *api.AutomationRun) string {
	if r == nil {
		return "-"
	}
	return r.Outcome + " " + r.At.Local().Format("01-02 15:04")
}

func printAutomation(w io.Writer, a api.Automation, secret string) {
	fmt.Fprintf(w, "%s  %s → %s  (%s)\n", a.ID, a.Name, a.To, automationWhen(a))
	if a.NextRunAt != nil {
		fmt.Fprintf(w, "next run: %s\n", a.NextRunAt.Local().Format(time.RFC1123))
	}
	if a.Webhook {
		fmt.Fprintf(w, "webhook: %s\n", a.WebhookURL)
	}
	if secret != "" {
		fmt.Fprintf(w, "webhook secret (shown once, keep it private): %s\n", secret)
		fmt.Fprintf(w, "  curl -X POST %s -H \"Authorization: Bearer %s\" --data 'what happened'\n", a.WebhookURL, secret)
	}
	fmt.Fprintf(w, "message: %s\n", a.Body)
}
