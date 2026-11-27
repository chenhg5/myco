// Command coordeval runs the coordination test (internal/coordeval)
// against a coordinator: a real Claude Code or Codex session (it spends
// tokens: `make coord-eval CLI=claude`), or the scripted baseline.
//
//	coordeval -cli claude -myco bin/myco [-scenario parallel-five,priority] [-unit 5s] [-out report.json]
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/coordeval"
)

func main() {
	cli := flag.String("cli", "claude", "coordinator: claude, codex (real sessions), baseline or naive (scripted)")
	myco := flag.String("myco", "", "the myco executable the coordinator's session uses (required for claude and codex)")
	scenarios := flag.String("scenario", "", "comma-separated scenario names (default: all)")
	unit := flag.Duration("unit", 5*time.Second, "one unit of scenario time (latencies and deadlines are in units)")
	seed := flag.Int64("seed", 1, "seed for the fake agents' latencies")
	out := flag.String("out", "coord-eval-report.json", "where to write the JSON report (the myco-ui table goes next to it, .md)")
	model := flag.String("model", "", "the CLI's model")
	access := flag.String("access", "", "the CLI's access level (default workspace)")
	binary := flag.String("binary", "", "the CLI executable")
	keep := flag.Bool("keep", false, "keep each scenario's directory (hub database, the runner's files)")
	verbose := flag.Bool("v", false, "show the runner's log")
	list := flag.Bool("list", false, "list the scenarios and exit")
	flag.Parse()

	if *list {
		for _, s := range coordeval.Scenarios() {
			fmt.Printf("%-15s %s: %s\n", s.Name, s.Title, s.Expect)
		}
		return
	}
	var coord coordeval.Coordinator
	switch *cli {
	case "baseline":
		coord = coordeval.Baseline{}
	case "naive":
		coord = coordeval.Baseline{Naive: true}
	default:
		if *myco == "" {
			fail("-myco is required for a real CLI (make coord-eval builds it)")
		}
		var logw io.Writer = io.Discard
		if *verbose {
			logw = os.Stderr
		}
		coord = coordeval.CLI{Harness: *cli, Myco: *myco, Model: *model, Access: *access, Binary: *binary, Log: logw}
	}
	var names []string
	for _, n := range strings.Split(*scenarios, ",") {
		if n = strings.TrimSpace(n); n == "" {
			continue
		}
		if coordeval.ScenarioByName(n) == nil {
			fail("no scenario " + n + " (see -list)")
		}
		names = append(names, n)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	rep, err := coordeval.RunAll(ctx, coord, coordeval.RunOptions{Options: coordeval.Options{Dir: os.TempDir(), Unit: *unit, Seed: *seed}, Keep: *keep}, names...)
	if err != nil {
		fail(err.Error())
	}
	if err := rep.Write(*out); err != nil {
		fail(err.Error())
	}
	fmt.Print(rep.Table())
	fmt.Fprintf(os.Stderr, "report: %s\n", *out)
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "coordeval:", msg)
	os.Exit(2)
}
