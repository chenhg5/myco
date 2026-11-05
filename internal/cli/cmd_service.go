package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/service"
)

// serviceEnvKeys are copied into the service definition when set: they
// decide where CLIs and sessions are found. Tokens are never copied.
var serviceEnvKeys = []string{"PATH", "CLAUDE_CONFIG_DIR", "CODEX_HOME", config.EnvConfig}

func serviceCommands() *Command {
	manager := func(e *env) (service.Manager, error) {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		m, err := service.Detect(home, service.ExecRunner{})
		if err != nil {
			return nil, errorf("unsupported", ExitError, "run `myco runner start --all` under your own process manager", "%v", err)
		}
		return m, nil
	}

	install := &Command{
		Name:     "install",
		Summary:  "Run `myco runner start --all` in the background, at login and after crashes",
		Mutating: true,
		Long: `macOS: a LaunchAgent (~/Library/LaunchAgents/ai.myco.runner.plist). Linux: a systemd user unit
(~/.config/systemd/user/myco-runner.service). Your current PATH is recorded so the service finds
claude/codex (e.g. under nvm); re-run install after changing it. Re-running is safe.`,
		Examples: []string{
			"myco runner service install --dry-run   # show the file and commands",
			"myco runner service install --max-concurrent 2",
		},
		Flags: []Flag{
			{Name: "max-concurrent", Kind: Int, Default: "3", Help: "Max agent CLIs running at once"},
			{Name: "log-level", Kind: Enum, Values: []string{"debug", "info", "warn", "error"}, Default: "info", Help: "Runner log verbosity"},
		},
		Run: handler(func(e *env) error {
			m, err := manager(e)
			if err != nil {
				return err
			}
			bin, err := os.Executable()
			if err != nil {
				return err
			}
			if real, err := filepath.EvalSymlinks(bin); err == nil {
				bin = real
			}
			spec := service.Spec{
				Binary: bin,
				Args: []string{"runner", "start", "--all", "--max-concurrent", strconv.Itoa(e.in.Int("max-concurrent")),
					"--log-level", e.in.String("log-level")},
				Env:     map[string]string{},
				LogPath: filepath.Join(config.Dir(e.defaultPath), "runner.log"),
			}
			for _, k := range serviceEnvKeys {
				if v := e.in.Getenv(k); v != "" {
					spec.Env[k] = v
				}
			}
			plan, err := m.PlanInstall(spec)
			if err != nil {
				return err
			}
			profiles, _ := config.ListProfiles(config.ProfilesDir(e.defaultPath))
			if len(profiles) == 0 {
				plan.Notes = append(plan.Notes, "no agent profiles yet; the runner will pick them up as sessions enroll (myco agent enroll)")
			}
			if strings.HasPrefix(bin, os.TempDir()) {
				plan.Notes = append(plan.Notes, "warning: myco runs from a temporary directory ("+bin+"); install it permanently first")
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "install_service", "plan": plan})
			}
			if err := os.MkdirAll(filepath.Dir(spec.LogPath), 0o700); err != nil {
				return err
			}
			if err := service.Apply(plan, service.ExecRunner{}); err != nil {
				return errorf("service_failed", ExitError, "inspect with --dry-run and the system logs", "%v", err)
			}
			// launchd/systemd report transient states (xpcproxy, activating)
			// right after start; wait briefly for the process to be up.
			st, _ := m.Status()
			for i := 0; i < 10 && !st.Running; i++ {
				time.Sleep(500 * time.Millisecond)
				st, _ = m.Status()
			}
			return e.p.Print(map[string]any{"installed": true, "plan": plan, "status": st}, func(w io.Writer) {
				fmt.Fprintf(w, "runner service installed (%s)\nfile: %s\nlogs: %s\n", plan.Manager, plan.File, spec.LogPath)
				printStatus(w, st)
				for _, n := range plan.Notes {
					fmt.Fprintln(w, "note:", n)
				}
			})
		}),
	}

	uninstall := &Command{
		Name:     "uninstall",
		Summary:  "Stop the background runner and remove its service definition",
		Mutating: true,
		Examples: []string{"myco runner service uninstall"},
		Run: handler(func(e *env) error {
			m, err := manager(e)
			if err != nil {
				return err
			}
			plan := m.PlanUninstall()
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "uninstall_service", "plan": plan})
			}
			if err := service.Apply(plan, service.ExecRunner{}); err != nil {
				return err
			}
			if err := service.RemoveFile(plan); err != nil {
				return err
			}
			return e.p.Print(map[string]any{"uninstalled": true, "file": plan.File}, func(w io.Writer) {
				fmt.Fprintf(w, "runner service removed (%s)\n", plan.File)
			})
		}),
	}

	status := &Command{
		Name:     "status",
		Summary:  "Show whether the background runner is installed and running",
		Examples: []string{"myco runner service status"},
		Run: handler(func(e *env) error {
			m, err := manager(e)
			if err != nil {
				return err
			}
			st, err := m.Status()
			if err != nil {
				return err
			}
			st.LogPath = filepath.Join(config.Dir(e.defaultPath), "runner.log")
			return e.p.Print(st, func(w io.Writer) { printStatus(w, st) })
		}),
	}

	restart := &Command{
		Name:     "restart",
		Summary:  "Restart the background runner (e.g. after upgrading myco)",
		Examples: []string{"myco runner service restart"},
		Run: handler(func(e *env) error {
			m, err := manager(e)
			if err != nil {
				return err
			}
			if err := m.Restart(); err != nil {
				return errorf("service_failed", ExitError, "is it installed? myco runner service status", "%v", err)
			}
			st, _ := m.Status()
			return e.p.Print(map[string]any{"restarted": true, "status": st}, func(w io.Writer) { printStatus(w, st) })
		}),
	}

	logs := &Command{
		Name:     "logs",
		Summary:  "Print the last lines of the background runner log",
		Examples: []string{"myco runner service logs --lines 100"},
		Flags:    []Flag{{Name: "lines", Kind: Int, Default: "50", Help: "Number of lines"}},
		Run: handler(func(e *env) error {
			path := filepath.Join(config.Dir(e.defaultPath), "runner.log")
			b, err := os.ReadFile(path)
			if err != nil {
				return errorf("not_found", ExitNotFound, "is the service installed? myco runner service status", "cannot read %s: %v", path, err)
			}
			lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
			if n := e.in.Int("lines"); n > 0 && len(lines) > n {
				lines = lines[len(lines)-n:]
			}
			return e.p.Print(map[string]any{"path": path, "lines": lines}, func(w io.Writer) {
				fmt.Fprintln(w, strings.Join(lines, "\n"))
			})
		}),
	}

	return (&Command{Name: "service", Summary: "Run the runner in the background (launchd / systemd)"}).Add(install, status, restart, logs, uninstall)
}

func printStatus(w io.Writer, st service.Status) {
	state := "not installed"
	switch {
	case st.Running:
		state = fmt.Sprintf("running (pid %d)", st.PID)
	case st.Installed:
		state = "installed, not running"
	}
	fmt.Fprintf(w, "status: %s [%s%s]\n", state, st.Manager, map[bool]string{true: ", " + st.Detail, false: ""}[st.Detail != ""])
}
