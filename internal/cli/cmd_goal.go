package cli

import (
	"fmt"
	"github.com/myco-ai/myco/internal/api"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/client"
	"io"
	"strings"
	"time"
)

func goalError(e error) error {
	if ce, ok := client.AsError(e); ok && ce.Status == 404 && (ce.Code == "internal" || strings.Contains(ce.Message, "not enabled")) {
		return errorf("unsupported", ExitUsage, "upgrade the Hub to use goals", "this Hub does not provide goal endpoints")
	}
	return e
}
func goalPrint(e *env, v any, err error) error {
	if err != nil {
		return goalError(err)
	}
	if e.in.Bool("dry-run") {
		return e.dryRun(v)
	}
	return e.p.Print(v, func(w io.Writer) { fmt.Fprintf(w, "%+v\n", v) })
}
func goalSeconds(e *env, name string) (int, error) {
	d := e.in.Duration(name)
	if d < time.Second || d%time.Second != 0 || d > 8*time.Hour {
		return 0, errorf("invalid_argument", ExitUsage, "use whole seconds from 1s to 8h", "invalid --%s", name)
	}
	return int(d / time.Second), nil
}
func goalCommands() *Command {
	root := &Command{Name: "goal", Summary: "Set a bounded objective and schedule idle turns toward it"}
	id := Flag{Name: "id", Kind: String, Required: true, Help: "Goal id (gol_...)"}
	admin := Flag{Name: "admin-token-file", Kind: String, Help: "File containing the owner's/admin token"}
	every := Flag{Name: "every", Kind: Duration, Help: "15m, 30m, or 60m"}
	max := Flag{Name: "max-rounds", Kind: Int, Help: "Maximum turns, 1..256"}
	duration := Flag{Name: "for", Kind: Duration, Help: "Duration from now, within eight hours of the original start"}
	controls := []string{"pause", "resume", "stop"}
	for _, action := range controls {
		root.Add(&Command{Name: action, Summary: action + " a goal (creator, owner, or project coordinator)", Mutating: true, Flags: []Flag{admin, id, {Name: "reason", Kind: String, Help: "Reason for this change"}}, Run: handler(func(e *env) error {
			c, err := e.watchClient()
			if err != nil {
				return err
			}
			var g *api.Goal
			switch action {
			case "pause":
				g, err = c.PauseGoal(e.in.Ctx, e.in.String("id"), e.in.String("reason"), e.in.Bool("dry-run"))
			case "resume":
				g, err = c.ResumeGoal(e.in.Ctx, e.in.String("id"), e.in.String("reason"), e.in.Bool("dry-run"))
			default:
				g, err = c.StopGoal(e.in.Ctx, e.in.String("id"), e.in.String("reason"), e.in.Bool("dry-run"))
			}
			return goalPrint(e, g, err)
		})})
	}
	root.Add(&Command{Name: "start", Summary: "Set an immutable objective for another agent (default 30m, 8h, 16 rounds)", Mutating: true, Flags: []Flag{admin, {Name: "agent", Kind: String, Required: true, Help: "Target agent"}, {Name: "objective", Kind: String, Required: true, Help: "Immutable goal text"}, {Name: "done-when", Kind: String, Required: true, Help: "Success criteria"}, {Name: "thread", Kind: String, Help: "Existing message involving the target"}, {Name: "project", Kind: String, Help: "Project name"}, every, max, duration}, Run: handler(func(e *env) error {
		c, err := e.watchClient()
		if err != nil {
			return err
		}
		in := api.GoalCreateRequest{Agent: e.in.String("agent"), Objective: e.in.String("objective"), DoneWhen: e.in.String("done-when"), Thread: e.in.String("thread"), Project: e.in.String("project")}
		if e.in.IsSet("every") {
			in.EverySec, err = goalSeconds(e, "every")
			if err != nil {
				return err
			}
		}
		if e.in.IsSet("for") {
			in.ForSec, err = goalSeconds(e, "for")
			if err != nil {
				return err
			}
		}
		if e.in.IsSet("max-rounds") {
			in.MaxRounds = e.in.Int("max-rounds")
			if in.MaxRounds < 1 {
				return errorf("invalid_argument", ExitUsage, "use --max-rounds 1..256", "invalid round limit")
			}
		}
		g, err := c.CreateGoal(e.in.Ctx, in, e.in.Bool("dry-run"))
		return goalPrint(e, g, err)
	})})
	root.Add(&Command{Name: "list", Summary: "List goals you may access", Flags: []Flag{admin, {Name: "agent", Kind: String, Help: "Filter by agent"}, {Name: "project", Kind: String, Help: "Filter by project"}, {Name: "all", Kind: Bool, Help: "Include terminated goals"}, {Name: "dry-run", Kind: Bool, Help: "Preview this query"}}, Run: handler(func(e *env) error {
		c, err := e.watchClient()
		if err != nil {
			return err
		}
		gs, err := c.Goals(e.in.Ctx, e.in.String("agent"), e.in.String("project"), e.in.Bool("all"))
		return goalPrint(e, api.GoalList{Goals: gs}, err)
	})})
	root.Add(&Command{Name: "get", Summary: "Read a goal and its round history", Flags: []Flag{admin, id, {Name: "dry-run", Kind: Bool, Help: "Preview this query"}}, Run: handler(func(e *env) error {
		c, err := e.watchClient()
		if err != nil {
			return err
		}
		g, err := c.Goal(e.in.Ctx, e.in.String("id"))
		return goalPrint(e, g, err)
	})})
	root.Add(&Command{Name: "set", Summary: "Change the interval, round limit, or bounded deadline", Mutating: true, Flags: []Flag{admin, id, every, max, duration}, Run: handler(func(e *env) error {
		c, err := e.watchClient()
		if err != nil {
			return err
		}
		in := api.GoalUpdateRequest{}
		if e.in.IsSet("every") {
			n, err := goalSeconds(e, "every")
			if err != nil {
				return err
			}
			in.EverySec = &n
		}
		if e.in.IsSet("for") {
			n, err := goalSeconds(e, "for")
			if err != nil {
				return err
			}
			in.ForSec = &n
		}
		if e.in.IsSet("max-rounds") {
			n := e.in.Int("max-rounds")
			in.MaxRounds = &n
		}
		if in.EverySec == nil && in.MaxRounds == nil && in.ForSec == nil {
			return errorf("invalid_argument", ExitUsage, "set --every, --for, or --max-rounds", "no changes specified")
		}
		g, err := c.UpdateGoal(e.in.Ctx, e.in.String("id"), in, e.in.Bool("dry-run"))
		return goalPrint(e, g, err)
	})})
	root.Add(&Command{Name: "report", Summary: "Report this goal turn before replying to its wake task", Mutating: true, Flags: []Flag{id, {Name: "message", Kind: String, Help: "Round task id (default MYCO_MESSAGE_ID)"}, {Name: "summary", Kind: String, Required: true, Help: "Progress summary, up to 4000 characters"}, {Name: "next", Kind: String, Help: "Next step"}, {Name: "progressed", Kind: Bool, Help: "Made progress"}, {Name: "no-progress", Kind: Bool, Help: "Made no progress"}, {Name: "blocked", Kind: Bool, Help: "Pause and ask the creator for help"}, {Name: "achieved", Kind: Bool, Help: "Goal achieved"}, {Name: "abandoned", Kind: Bool, Help: "Goal cannot be achieved"}, {Name: "reason", Kind: String, Help: "Required for achieved or abandoned"}}, Run: handler(func(e *env) error {
		if e.in.Bool("progressed") == e.in.Bool("no-progress") {
			return errorf("invalid_argument", ExitUsage, "choose --progressed or --no-progress", "one progress outcome is required")
		}
		m := e.in.String("message")
		if m == "" {
			m = e.in.Getenv("MYCO_MESSAGE_ID")
		}
		if m == "" {
			return errorf("invalid_argument", ExitUsage, "supply --message <round-task-id> outside a runner turn", "no goal round message")
		}
		c, err := e.agentClient()
		if err != nil {
			return err
		}
		progress := e.in.Bool("progressed")
		g, err := c.ReportGoal(e.in.Ctx, e.in.String("id"), api.GoalReportRequest{Message: m, Summary: e.in.String("summary"), NextStep: e.in.String("next"), Progressed: &progress, Blocked: e.in.Bool("blocked"), Achieved: e.in.Bool("achieved"), Abandoned: e.in.Bool("abandoned"), Reason: e.in.String("reason")}, e.in.Bool("dry-run"))
		return goalPrint(e, g, err)
	})})
	return root
}
