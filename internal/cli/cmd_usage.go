package cli

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
)

func usageCommands() *Command {
	show := &Command{Name: "show", Summary: "Workspace token usage reported by CLIs (member/admin credentials)", Flags: []Flag{
		{Name: "from", Kind: String, Help: "First UTC day (YYYY-MM-DD; default last 7 days)"},
		{Name: "to", Kind: String, Help: "Last UTC day (inclusive)"},
		{Name: "project", Kind: String, Help: "Only this project (- for no project)"},
		{Name: "by", Kind: String, Default: "agent", Help: "Group by agent, project, day or model"},
		{Name: "admin-token-file", Kind: String, Help: "File containing admin/member token"},
	}, Run: handler(func(e *env) error {
		by := e.in.String("by")
		if by != "agent" && by != "project" && by != "day" && by != "model" {
			return errorf("invalid_usage", ExitUsage, "use agent, project, day or model", "invalid --by")
		}
		c, err := e.adminClient()
		if err != nil {
			return err
		}
		out, err := c.Usage(e.in.Ctx, client.UsageFilter{From: e.in.String("from"), To: e.in.String("to"), Project: e.in.String("project")})
		if err != nil {
			return err
		}
		return e.p.Print(out, func(w io.Writer) { printUsage(w, out, by) })
	})}
	return (&Command{Name: "usage", Summary: "Reported workspace token and cost accounting"}).Add(show)
}
func printUsage(w io.Writer, out *api.UsageList, by string) {
	sums := map[string]api.UsageRow{}
	for _, r := range out.Rows {
		key := r.Agent
		switch by {
		case "project":
			key = r.Project
		case "day":
			key = r.Day
		case "model":
			key = r.Model
		}
		if key == "" {
			key = "-"
		}
		s := sums[key]
		s.Calls += r.Calls
		s.InputTokens += r.InputTokens
		s.CacheWriteTokens += r.CacheWriteTokens
		s.CacheReadTokens += r.CacheReadTokens
		s.OutputTokens += r.OutputTokens
		s.CostUSD += r.CostUSD
		sums[key] = s
	}
	keys := []string{}
	for k := range sums {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := sums[keys[i]], sums[keys[j]]
		ta, tb := a.InputTokens+a.CacheReadTokens+a.CacheWriteTokens+a.OutputTokens, b.InputTokens+b.CacheReadTokens+b.CacheWriteTokens+b.OutputTokens
		if ta != tb {
			return ta > tb
		}
		return keys[i] < keys[j]
	})
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "%s\tCALLS\tINPUT\tCACHE WRITE\tCACHE READ\tOUTPUT\tREPORTED USD\n", by)
	for _, k := range keys {
		s := sums[k]
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%d\t%.6f\n", k, s.Calls, s.InputTokens, s.CacheWriteTokens, s.CacheReadTokens, s.OutputTokens, s.CostUSD)
	}
	tw.Flush()
}
