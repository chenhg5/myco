package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/harness/builtin"
	"github.com/chenhg5/myco/internal/harness/machinenode"
	"github.com/chenhg5/myco/internal/osutil"
	"github.com/chenhg5/myco/internal/release"
)

func harnessCommands() *Command {
	list := &Command{
		Name:     "list",
		Summary:  "List available harnesses (agent CLI adapters) and whether their CLI is installed",
		Examples: []string{"myco harness list", "myco harness list --format json"},
		Run: handler(func(e *env) error {
			reg := builtin.Registry()
			type row struct {
				Name         string `json:"name"`
				Kind         string `json:"kind"`
				Binary       string `json:"binary,omitempty"`
				Installed    bool   `json:"installed"`
				Version      string `json:"version,omitempty"`
				Experimental bool   `json:"experimental"`
			}
			var rows []row
			for _, n := range reg.Names() {
				r := row{Name: n, Kind: "native", Experimental: builtin.Experimental[n]}
				if _, ok := builtin.Presets[n]; ok {
					r.Kind = "preset"
				}
				if _, ok := builtin.ACPPresets[n]; ok {
					r.Kind = "acp"
				}
				opts := harness.Options{Command: "true"}
				if n == "template" {
					r.Kind = "generic"
					rows = append(rows, r)
					continue
				}
				if n == "shell" || n == "exec" || n == "acp" {
					r.Kind = "generic"
				}
				if n == "acp" {
					rows = append(rows, r)
					continue
				}
				if h, err := reg.New(n, opts); err == nil {
					if c, ok := h.(harness.Checker); ok {
						ck := c.Check(e.in.Ctx)
						r.Binary, r.Installed, r.Version = ck.Binary, ck.Error == "", ck.Version
					}
				}
				rows = append(rows, r)
			}
			return e.p.Print(map[string]any{"harnesses": rows}, func(w io.Writer) {
				tw := newTable(w)
				fmt.Fprintln(tw, "NAME\tKIND\tBINARY\tINSTALLED\tVERSION")
				for _, r := range rows {
					name := r.Name
					if r.Experimental {
						name += " (experimental)"
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\t%v\t%s\n", name, r.Kind, dash(r.Binary), r.Installed, truncate(r.Version, 40))
				}
				tw.Flush()
			})
		}),
	}
	return (&Command{Name: "harness", Summary: "Inspect agent CLI adapters"}).Add(list)
}

// checkResult is one line of `myco doctor`.
type checkResult struct {
	Scope      string `json:"scope"`
	Check      string `json:"check"`
	Status     string `json:"status"` // ok | warn | fail
	Detail     string `json:"detail"`
	Suggestion string `json:"suggestion,omitempty"`
}

func doctorCommand() *Command {
	return &Command{
		Name:    "doctor",
		Summary: "Check hub connectivity, identity and every local agent's CLI setup (no quota spent)",
		Long:    "Exits 1 if any check fails. With --all it checks every profile in ~/.myco/agents.",
		Examples: []string{
			"myco doctor",
			"myco doctor --all --format json",
		},
		Flags: []Flag{{Name: "all", Kind: Bool, Help: "Check every agent profile on this machine"}},
		Run: handler(func(e *env) error {
			var results []checkResult
			add := func(scope, check, status, detail, suggestion string) {
				results = append(results, checkResult{scope, check, status, detail, suggestion})
			}
			targets := []config.Profile{{Path: e.cfgPath, Config: e.cfg}}
			if e.in.Bool("all") {
				ps, err := config.ListProfiles(config.ProfilesDir(e.defaultPath))
				if err != nil {
					add("machine", "profiles", "fail", err.Error(), "fix or remove the broken profile file")
				}
				targets = ps
				if len(ps) == 0 {
					add("machine", "profiles", "warn", "no agent profiles in "+config.ProfilesDir(e.defaultPath), "enroll a session: myco agent enroll")
				}
			}
			for _, t := range targets {
				doctorProfile(e.in.Ctx, t, add)
			}
			failed := 0
			for _, r := range results {
				if r.Status == "fail" {
					failed++
				}
			}
			if err := e.p.Print(map[string]any{"ok": failed == 0, "checks": results}, func(w io.Writer) {
				tw := newTable(w)
				fmt.Fprintln(tw, "SCOPE\tCHECK\tSTATUS\tDETAIL")
				for _, r := range results {
					d := r.Detail
					if r.Suggestion != "" && r.Status != "ok" {
						d += " → " + r.Suggestion
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.Scope, r.Check, strings.ToUpper(r.Status), d)
				}
				tw.Flush()
			}); err != nil {
				return err
			}
			if failed > 0 {
				return &Error{Code: "checks_failed", Message: fmt.Sprintf("%d check(s) failed", failed), Exit: ExitError,
					Suggestion: "see the suggestions above"}
			}
			return nil
		}),
	}
}

func doctorProfile(ctx context.Context, p config.Profile, add func(scope, check, status, detail, suggestion string)) {
	cfg := p.Config
	scope := orDefault(cfg.Agent, filepath.Base(p.Path))
	if cfg.Hub == "" {
		add(scope, "hub", "fail", "no hub configured in "+p.Path, "set MYCO_HUB or enroll/register")
		return
	}
	c, err := client.New(cfg.Hub, cfg.Token)
	if err != nil {
		add(scope, "hub", "fail", err.Error(), "")
		return
	}
	start := time.Now()
	health, err := c.Health(ctx)
	if err != nil {
		add(scope, "hub", "fail", toError(err, "").Message, "check the URL, TLS and network policy (allowed domains)")
		return
	}
	add(scope, "hub", "ok", fmt.Sprintf("%s reachable in %s", cfg.Hub, time.Since(start).Round(time.Millisecond)), "")
	const upgradeHub = "on the hub's machine: sudo myco upgrade (or re-run install-hub.sh)"
	switch {
	case health.ServerVersion == "":
		add(scope, "hub version", "warn", "the hub does not report its version: it runs myco older than v0.16.6", upgradeHub)
	case release.Newer(Version, health.ServerVersion):
		add(scope, "hub version", "warn", fmt.Sprintf("the hub runs %s, older than this machine's %s", health.ServerVersion, Version), upgradeHub)
	default:
		add(scope, "hub version", "ok", "the hub runs "+health.ServerVersion, "")
	}
	if release.Newer(health.LatestVersion, Version) {
		add(scope, "update", "warn", fmt.Sprintf("myco %s is available (this machine has %s)", health.LatestVersion, Version), "myco upgrade")
	}
	if cfg.Token == "" {
		add(scope, "identity", "warn", "no agent token", "enroll or register this agent")
		return
	}
	me, err := c.Me(ctx)
	if err != nil {
		add(scope, "identity", "fail", toError(err, "").Message, "re-enroll: myco agent enroll")
		return
	}
	add(scope, "identity", "ok", fmt.Sprintf("agent %s (%s session %s in %s)", me.Name, dash(me.Binding.Harness),
		dash(me.Binding.SessionID), dash(me.Binding.Environment)), "")
	if me.Binding.Environment != "" && me.Binding.Environment != hostname() && !strings.HasPrefix(me.Binding.Environment, "claude-") {
		add(scope, "environment", "warn", fmt.Sprintf("bound to %s, this host is %s", me.Binding.Environment, hostname()),
			"run this agent's runner on "+me.Binding.Environment)
	}

	rc := cfg.Runner
	name := rc.Harness
	if name == "" {
		name = runnerHarnessFor(me.Binding.Harness)
	}
	if name == "" {
		add(scope, "runner", "ok", "no local runner needed for harness "+dash(me.Binding.Harness), "")
		return
	}
	access, err := harness.ParseAccess(rc.Access)
	if err != nil {
		add(scope, "runner.access", "fail", err.Error(), "use readonly, workspace or full")
	}
	if name == machinenode.Name {
		var found []string
		for _, c := range machinenode.SpawnableCLIs {
			if _, err := exec.LookPath(spawnBinary(c)); err == nil {
				found = append(found, c)
			}
		}
		if len(found) == 0 {
			add(scope, "harness", "warn", "machine node: no supported agent CLI is installed here, so agent spawn has nothing to start",
				"install claude or codex on this machine")
		} else {
			add(scope, "harness", "ok", "machine node: agent spawn can start "+strings.Join(found, ", "), "")
		}
		return
	}
	h, err := builtin.Registry().New(name, harness.Options{Binary: rc.Binary, Args: rc.Args, Command: rc.Command, Model: rc.Model, Access: access, Template: rc.Template})
	if err != nil {
		add(scope, "harness", "fail", err.Error(), "fix runner.harness / runner.template")
		return
	}
	if ck, ok := h.(harness.Checker); ok {
		r := ck.Check(ctx)
		if r.Error != "" {
			add(scope, "harness", "fail", fmt.Sprintf("%s: %s", r.Binary, r.Error), "install it or set runner.binary to its path")
		} else {
			status := "ok"
			if builtin.Experimental[name] {
				status = "warn"
			}
			add(scope, "harness", status, fmt.Sprintf("%s %s (%s)", name, r.Version, r.Path), map[bool]string{true: "experimental preset: verify output parsing on a test message"}[status == "warn"])
		}
	}
	if rc.Workdir != "" {
		if st, err := os.Stat(rc.Workdir); err != nil || !st.IsDir() {
			add(scope, "workdir", "fail", rc.Workdir+" does not exist", "create it or fix runner.workdir")
		} else {
			add(scope, "workdir", "ok", rc.Workdir, "")
		}
	}
	if bp, ok := h.(harness.BusyProber); ok && me.Binding.SessionID != "" {
		if last, found := bp.LastActivity(me.Binding.SessionID); found {
			add(scope, "session", "ok", "transcript found, last activity "+time.Since(last).Round(time.Second).String()+" ago", "")
		} else {
			add(scope, "session", "warn", "transcript for session "+me.Binding.SessionID+" not found",
				"busy detection is off for this agent; check the session id and CLAUDE_CONFIG_DIR / CODEX_HOME")
		}
	}
	if rc.EnvFile != "" {
		path := rc.EnvFile
		if _, ok := osutil.HomeRelative(path); ok {
			home, _ := os.UserHomeDir()
			path = osutil.ExpandHome(path, home)
		}
		if vars, err := config.ReadEnvFile(path); err != nil {
			add(scope, "env_file", "fail", err.Error(), "fix runner.env_file")
		} else if st, _ := os.Stat(path); osutil.UnixPermissions && st != nil && st.Mode().Perm()&0o077 != 0 {
			add(scope, "env_file", "warn", fmt.Sprintf("%s is readable by others (%v)", path, st.Mode().Perm()), "chmod 600 "+path)
		} else {
			names := make([]string, 0, len(vars))
			for _, kv := range vars {
				k, _, _ := strings.Cut(kv, "=")
				names = append(names, k)
			}
			add(scope, "env_file", "ok", fmt.Sprintf("%s: %s", path, strings.Join(names, ", ")), "")
		}
	}
	if access == harness.AccessFull {
		add(scope, "access", "warn", "full: no approvals, no sandbox", "use workspace unless this machine is isolated")
	}
}
