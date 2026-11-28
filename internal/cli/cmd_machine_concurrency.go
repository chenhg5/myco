package cli

import (
	"errors"
	"fmt"
	"io"
	"time"

	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/config"
)

// machineConcurrencyCommand shows or sets how many agent CLIs this
// computer's runner runs at once: runner.max_concurrent in machine.yaml,
// which the runner reads live.
func machineConcurrencyCommand() *Command {
	return &Command{Name: "concurrency", Summary: "Show or set how many agent CLIs run at once on this computer", Mutating: true,
		Long: fmt.Sprintf(`The runner (myco runner start --all, the background service) runs at most this many agent CLIs at
once, shared between all workspaces on this computer; more tasks wait their turn. --set writes
runner.max_concurrent in ~/.myco/machine.yaml. The runner reads it within seconds, without a restart:
raising it starts waiting tasks at once, lowering it only stops new starts (running tasks finish).

Which number counts: of the runner's --max-concurrent flag and this setting, the one chosen last
(runner service install --max-concurrent N writes N here too). Neither: %d. The myco app's
Computers page shows and changes the same number.`, config.DefaultMaxConcurrent),
		Examples: []string{"myco machine concurrency", "myco machine concurrency --set 12", "myco machine concurrency --set 4 --dry-run"},
		Flags: []Flag{
			{Name: "set", Kind: Int, Help: fmt.Sprintf("The new limit (1 to %d)", config.MaxConcurrentCeiling)},
		},
		Run: handler(func(e *env) error {
			m, err := config.LoadMachine(e.defaultPath)
			if err != nil {
				return err
			}
			if m == nil {
				return errorf("not_configured", ExitUsage, "join this computer with a link first, or start the runner with --max-concurrent",
					"this computer has not joined a hub as a machine yet")
			}
			now := time.Now()
			st, _ := config.LoadRunnerSlots(e.defaultPath)
			if !st.Fresh(now) {
				st = nil
			}
			if !e.in.IsSet("set") {
				out := concurrencyView(m.Runner, st)
				return e.p.Print(out, func(w io.Writer) { printConcurrency(w, out) })
			}
			n := e.in.Int("set")
			if n < 1 || n > config.MaxConcurrentCeiling {
				return errorf("invalid_argument", ExitUsage, fmt.Sprintf("pass a number from 1 to %d", config.MaxConcurrentCeiling), "--set %d is out of range", n)
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "machine_concurrency", "max_concurrent": n, "was": m.Runner.MaxConcurrent,
					"config": config.MachinePath(e.defaultPath)})
			}
			if err := config.SetRunnerConcurrency(e.defaultPath, n, now); errors.Is(err, config.ErrNotJoined) {
				return errorf("not_configured", ExitUsage, "join this computer with a link first: curl -fsSL <link> | bash", "%v", err)
			} else if err != nil {
				return err
			}
			out := concurrencyView(config.RunnerPolicy{MaxConcurrent: n, SetAt: now}, st)
			out["changed"] = true
			return e.p.Print(out, func(w io.Writer) {
				fmt.Fprintf(w, "up to %d agent CLIs run at once on this computer now (这台电脑同时最多运行 %d 个)\n", n, n)
				if st == nil {
					fmt.Fprintln(w, "no runner here says it follows the setting: it applies when one starts (a runner older than this myco: myco runner service restart)")
				} else {
					fmt.Fprintln(w, "the runner applies it within a few seconds")
				}
			})
		}),
	}
}

// concurrencyView: the setting, and what the runner running here does
// (runner: null when none is running, or one older than this setting).
func concurrencyView(set config.RunnerPolicy, st *config.RunnerSlotsState) map[string]any {
	out := map[string]any{"setting": set.MaxConcurrent, "default": config.DefaultMaxConcurrent, "ceiling": config.MaxConcurrentCeiling, "runner": nil}
	if !set.SetAt.IsZero() {
		out["set_at"] = set.SetAt
	}
	if st != nil {
		out["runner"] = map[string]any{"limit": st.Limit, "source": st.Source, "flag": st.Flag, "in_use": st.InUse, "waiting": st.Waiting, "at": st.At}
	}
	return out
}

func printConcurrency(w io.Writer, out map[string]any) {
	r, _ := out["runner"].(map[string]any)
	if r == nil {
		if s, _ := out["setting"].(int); s > 0 {
			fmt.Fprintf(w, "setting: %d at once; no runner here says it follows it\n", s)
		} else {
			fmt.Fprintf(w, "no setting (default %d); no runner here says it follows it\n", config.DefaultMaxConcurrent)
		}
		return
	}
	from := map[string]string{config.ConcurrencyFromSetting: "machine.yaml", config.ConcurrencyFromFlag: "the runner's --max-concurrent", config.ConcurrencyFromDefault: "the default"}[fmt.Sprint(r["source"])]
	fmt.Fprintf(w, "%v at once (from %s): %v running, %v waiting\n", r["limit"], from, r["in_use"], r["waiting"])
}
