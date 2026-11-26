package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/myco-ai/myco/internal/api"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
)

func hubPerfCommand() *Command {
	return &Command{
		Name:    "perf",
		Summary: "Show which requests and database statements of a hub are slow",
		Long: `The hub keeps, for the last hour, how long each kind of request (by route, e.g.
"GET /v1/threads/{id}") and each database statement (by name: the repository method that ran it,
its verb and table, and a short hash of its text) took. This shows the last --minutes, slowest first
by p95: count, p50, p95, maximum and total time, and for statements the rows they read or changed.
Long polls and streams, which wait on purpose, are left out. Percentiles are estimates (at most 25%
above the true value, never above the maximum). Names only: no ids, parameters or message text.

The hub's owner only (the admin token, MYCO_ADMIN_TOKEN or --admin-token-file). The hub also logs a
statement taking 50 ms or longer as "slow statement" (myco hub serve --slow-statement).`,
		Examples: []string{
			"myco hub perf",
			"myco hub perf --minutes 60 --limit 10 --admin-token-file hub.env",
		},
		Flags: []Flag{
			{Name: "minutes", Kind: Int, Default: "15", Help: "How far back to look (1-60)"},
			{Name: "limit", Kind: Int, Default: "20", Help: "Rows of each list (1-500)"},
			{Name: "admin-token-file", Kind: String, Help: "File containing the admin token"},
		},
		Run: handler(func(e *env) error {
			minutes, limit := e.in.Int("minutes"), e.in.Int("limit")
			if minutes < 1 || minutes > 60 || limit < 1 || limit > 500 {
				return errorf("invalid_argument", ExitUsage, "--minutes 1-60, --limit 1-500", "minutes or limit out of range")
			}
			c, err := e.adminClient()
			if err != nil {
				return err
			}
			r, err := c.Perf(e.in.Ctx, minutes, limit)
			if err != nil {
				return err
			}
			return e.p.Print(r, func(w io.Writer) { printPerf(w, r) })
		}),
	}
}

func printPerf(w io.Writer, r *api.PerfReport) {
	section := func(title string, rows []api.PerfStat, statements bool) {
		fmt.Fprintf(w, "%s (last %d min, slowest first by p95)\n", title, r.Minutes)
		if len(rows) == 0 {
			fmt.Fprintln(w, "  none")
			return
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
		head := "COUNT\tP50 MS\tP95 MS\tMAX MS\tTOTAL MS\t"
		if statements {
			head += "ROWS AVG\tMAX\t"
		}
		fmt.Fprintln(tw, head+" NAME")
		for _, s := range rows {
			line := fmt.Sprintf("%d\t%.1f\t%.1f\t%.1f\t%.0f\t", s.Count, s.P50MS, s.P95MS, s.MaxMS, s.TotalMS)
			if statements {
				line += fmt.Sprintf("%d\t%d\t", s.AvgRows, s.MaxRows)
			}
			fmt.Fprintln(tw, line+" "+s.Name)
		}
		tw.Flush()
	}
	section("Requests", r.Requests, false)
	fmt.Fprintln(w)
	section("Statements", r.Statements, true)
}
