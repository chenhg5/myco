package cli

import (
	"encoding/json"
	"fmt"
	"io"

	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/client"
)

func printCoordination(w io.Writer, out any) {
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(w, string(b))
}
func agentStatsCommand() *Command {
	return &Command{Name: "stats", Summary: "Task outcomes and timings computed by the Hub (read-only)", Long: "Counts tasks received in the window. Timing samples without a first ACK are unknown. Only caller-readable participant messages contribute; no message bodies are returned. See docs/coordination-statistics.md for formulas.", Flags: []Flag{
		{Name: "name", Kind: String, Help: "Only this agent (name or alias; default all visible agents)"},
		{Name: "project", Kind: String, Help: "Only this project and its current members"},
		{Name: "since", Kind: String, Default: "7d", Help: "Positive duration, up to 366d (e.g. 7d, 24h)"},
	}, Run: handler(func(e *env) error {
		c, err := e.agentOrAdminClient()
		if err != nil {
			return err
		}
		out, err := c.AgentStats(e.in.Ctx, client.AgentStatsFilter{Name: e.in.String("name"), Project: e.in.String("project"), Since: e.in.String("since")})
		if err != nil {
			return err
		}
		return e.p.Print(out, func(w io.Writer) { printCoordination(w, out) })
	})}
}
func agentPickCommand() *Command {
	return &Command{Name: "pick", Summary: "Rank agents by availability, skills and recent task evidence (read-only)", Long: "Offline agents, blocked quotas and queues with at least two waiting messages rank last; --strict excludes them. Unknown quota or runner capacity is reported as unknown, not blocked. History is limited to caller-readable participant messages from the last 7 days.", Flags: []Flag{
		{Name: "skill", Kind: String, Help: "Required skill/capability or exact label"}, {Name: "cli", Kind: String, Help: "Required runtime CLI (e.g. codex or claude)"}, {Name: "project", Kind: String, Help: "Only current members of this project"}, {Name: "machine", Kind: String, Help: "Machine ID/name or exact binding environment"}, {Name: "count", Kind: Int, Default: "3", Help: "Maximum candidates, 1..100"}, {Name: "strict", Kind: Bool, Help: "Exclude offline agents, blocked quotas and full queues"},
	}, Run: handler(func(e *env) error {
		if e.in.Int("count") < 1 || e.in.Int("count") > 100 {
			return errorf("invalid_argument", ExitUsage, "use --count 3", "count must be 1..100")
		}
		c, err := e.agentOrAdminClient()
		if err != nil {
			return err
		}
		out, err := c.PickAgents(e.in.Ctx, client.AgentPickFilter{Skill: e.in.String("skill"), CLI: e.in.String("cli"), Project: e.in.String("project"), Machine: e.in.String("machine"), Count: e.in.Int("count"), Strict: e.in.Bool("strict")})
		if err != nil {
			return err
		}
		return e.p.Print(out, func(w io.Writer) {
			for _, v := range out.Candidates {
				fmt.Fprintf(w, "%s\t%.1f\t%s\n", v.Name, v.Score, v.Reason)
			}
		})
	})}
}
func statsCommands() *Command {
	return (&Command{Name: "stats", Summary: "Content-free Hub task statistics"}).Add(&Command{Name: "coordinator", Summary: "Delivery and delegation timings for one coordinator (read-only)", Long: "Person tasks are console-origin tasks. Reassignment is inferred from a sender-stopped task followed by a task to another agent in the same thread. Other stops count as withdrawals. Follow-ups are extra person tasks/steers, excluding answers to questions; this does not infer whether the person was impatient.", Flags: []Flag{{Name: "name", Kind: String, Help: "Coordinator name or alias (default yourself; required for admin/member)"}, {Name: "since", Kind: String, Default: "7d", Help: "Positive duration, up to 366d"}}, Run: handler(func(e *env) error {
		c, err := e.agentOrAdminClient()
		if err != nil {
			return err
		}
		out, err := c.CoordinatorStats(e.in.Ctx, e.in.String("name"), e.in.String("since"))
		if err != nil {
			return err
		}
		return e.p.Print(out, func(w io.Writer) { printCoordination(w, out) })
	})})
}
