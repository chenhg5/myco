package cli

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/harness/builtin"
	"github.com/chenhg5/myco/internal/harness/execnode"
	"github.com/chenhg5/myco/internal/harness/machinenode"
	"github.com/chenhg5/myco/internal/runner"
)

func runnerCommands() *Command {
	harnesses := builtin.Registry()
	start := &Command{
		Name:    "start",
		Summary: "Serve agents' inboxes by resuming their sessions in an agent CLI",
		Long: `Without --all it serves the current identity; flags override its 'runner' config.
With --all it serves every agent profile on this machine (~/.myco/agents/*.yaml) from one
process, picks up newly enrolled agents automatically, and caps parallel CLI runs with
--max-concurrent. Each agent (= one session) handles one message at a time; if its session
is being used elsewhere (e.g. open in the interactive UI) the message is retried shortly
instead of failing. Tasks are answered with the CLI's final output; replies, events and
notes are never auto-answered. Logs go to stderr.`,
		Examples: []string{
			"myco runner start --all                          # every agent on this machine",
			"myco runner start --harness claude --workdir ~/src/app",
			"myco runner start --harness codex --arg=--model --arg=gpt-5.6",
			"myco runner start --once                         # process what's queued, then exit",
		},
		Flags: []Flag{
			{Name: "all", Kind: Bool, Help: "Serve every agent profile in ~/.myco/agents"},
			{Name: "max-concurrent", Kind: Int, Default: "3", Help: "With --all: max agent CLIs running at once"},
			{Name: "harness", Kind: Enum, Values: harnesses.Names(), Help: "Agent CLI adapter (default from config, else claude); see: myco harness list"},
			{Name: "access", Kind: Enum, Values: []string{"readonly", "workspace", "full"}, Help: "Permission level mapped to CLI flags (default: config, else the CLI's own headless default)"},
			{Name: "model", Kind: String, Help: "Model in the CLI's own naming"},
			{Name: "binary", Kind: String, Help: "Path to the agent CLI executable"},
			{Name: "arg", Kind: List, Help: "Extra argument for the agent CLI (repeatable), e.g. --arg=--permission-mode --arg=acceptEdits"},
			{Name: "command", Kind: String, Help: "Shell command line for --harness shell (prompt on stdin, reply on stdout)"},
			{Name: "workdir", Kind: String, Help: "Working directory for the agent CLI (default: config, else current dir)"},
			{Name: "timeout", Kind: Duration, Help: "Max time per message (default 30m)"},
			{Name: "once", Kind: Bool, Help: "Process currently queued messages once and exit"},
			{Name: "session-scope", Kind: Enum, Values: []string{"agent", "thread"}, Default: "agent",
				Help: "agent: this agent is one session, every message resumes it; thread: one session per thread"},
			{Name: "busy-window", Kind: Duration, Default: "90s", Help: "Treat the session as in use if someone else wrote to it this recently"},
			{Name: "busy-retry", Kind: Duration, Default: "2m", Help: "Max delay between retries while the session is busy (starts at 10s, doubles)"},
			{Name: "rebind", Kind: Bool, Help: "Allow taking over an agent bound to another environment or harness"},
			{Name: "log-level", Kind: Enum, Values: []string{"debug", "info", "warn", "error"}, Default: "info", Help: "Log verbosity"},
		},
		Run: handler(func(e *env) error {
			log := newLogger(e.in.String("log-level"))
			shared := runnerShared{
				once: e.in.Bool("once"), busyWindow: e.in.Duration("busy-window"), busyRetry: e.in.Duration("busy-retry"),
				scope: runner.SessionScope(e.in.String("session-scope")), rebind: e.in.Bool("rebind"), registry: harnesses,
			}
			e.watchVersions(log)
			if !e.in.Bool("all") {
				r, err := e.buildRunner(e.cfgPath, e.cfg, shared, e.overrides(), log)
				if err != nil {
					return err
				}
				return r.Run(e.in.Ctx)
			}
			if n := e.in.Int("max-concurrent"); n > 0 {
				shared.slots = make(chan struct{}, n)
			}
			return e.runAll(shared, log)
		}),
	}
	return (&Command{Name: "runner", Summary: "Turn agent CLIs into always-reachable workers"}).Add(start, serviceCommands(), runnerEnvCommands(), runnerProviderCommands())
}

// runnerShared holds settings common to every agent a process serves.
type runnerShared struct {
	once       bool
	busyWindow time.Duration
	busyRetry  time.Duration
	scope      runner.SessionScope
	rebind     bool
	slots      chan struct{}
	registry   *harness.Registry
	// rescan, when set (runner --all), asks for an immediate profile scan,
	// e.g. right after a machine node spawned a new agent.
	rescan chan struct{}
}

// runnerOverrides are per-invocation flags (single-agent mode only).
type runnerOverrides struct {
	harness, binary, command, workdir, access, model string
	args                                             []string
	argsSet                                          bool
	timeout                                          time.Duration
}

func (e *env) overrides() runnerOverrides {
	o := runnerOverrides{harness: e.in.String("harness"), binary: e.in.String("binary"), command: e.in.String("command"),
		workdir: e.in.String("workdir"), args: e.in.List("arg"), argsSet: e.in.IsSet("arg"),
		access: e.in.String("access"), model: e.in.String("model")}
	if e.in.IsSet("timeout") {
		o.timeout = e.in.Duration("timeout")
	}
	return o
}

// buildRunner prepares a runner for one agent identity.
func (e *env) buildRunner(cfgPath string, cfg *config.Config, sh runnerShared, ov runnerOverrides, log *slog.Logger) (*runner.Runner, error) {
	rc := cfg.Runner
	first := func(vals ...string) string {
		for _, v := range vals {
			if v != "" {
				return v
			}
		}
		return ""
	}
	name := first(ov.harness, rc.Harness, runnerHarnessFor(cfg.Harness), "claude")
	args := rc.Args
	if ov.argsSet {
		args = ov.args
	}
	access, err := harness.ParseAccess(first(ov.access, rc.Access))
	if err != nil {
		return nil, errorf("invalid_config", ExitUsage, "use readonly, workspace or full", "%s: %v", cfgPath, err)
	}
	if cfg.Hub == "" || cfg.Token == "" {
		return nil, errorf("not_configured", ExitUsage, "enroll or register this agent first", "%s has no hub/token", cfgPath)
	}
	c, err := client.New(cfg.Hub, cfg.Token)
	if err != nil {
		return nil, err
	}
	c.Via = "runner" // yields to a session that keeps `inbox wait` running
	me, err := c.Me(e.in.Ctx)
	if err != nil {
		return nil, err
	}
	var execSpec *harness.ExecSpec
	if name == execnode.Name {
		// Remote command execution: refuse to serve anyone-can-send agents.
		if len(me.AllowFrom) == 0 {
			return nil, errorf("permission_denied", ExitPermission,
				"restrict senders first (admin): myco agent register --name "+me.Name+" --rotate-token --no-save --allow-from <agent>[,<agent>...]",
				"exec agent %s accepts messages from anyone; exec nodes require allow_from", me.Name)
		}
		spec := harness.ExecSpec{}
		if rc.Exec != nil {
			spec = *rc.Exec
		}
		spec.Senders = me.AllowFrom
		spec.CurrentSenders = cachedAllowFrom(e.in.Ctx, c, 30*time.Second)
		spec.AuditLog = filepath.Join(config.Dir(e.defaultPath), "logs", me.Name+".exec.jsonl")
		execSpec = &spec
	}
	var h harness.Harness
	if name == machinenode.Name {
		if h, err = e.machineNode(cfg.Hub, me, c, sh); err != nil {
			return nil, err
		}
	} else {
		h, err = sh.registry.New(name, harness.Options{Binary: first(ov.binary, rc.Binary), Args: args, Command: first(ov.command, rc.Command),
			Model: first(ov.model, rc.Model), Access: access, Template: rc.Template, Exec: execSpec})
		if err != nil {
			return nil, errorf("invalid_argument", ExitUsage, "available harnesses: "+strings.Join(sh.registry.Names(), ", "), "%v", err)
		}
	}
	// The provider (runner.provider, or provider variables in the env file)
	// is resolved again before every run, like the env file itself.
	currentProvider := func() providerChoice {
		ref, envFile := rc.Provider, rc.EnvFile
		if c, err := config.Load(cfgPath); err == nil {
			ref = c.Runner.Provider
			if c.Runner.EnvFile != "" {
				envFile = c.Runner.EnvFile
			}
		}
		return resolveProvider(h.Name(), ref, envFile)
	}
	initial := currentProvider()
	var rep *runtimeReporter
	if !sh.once {
		rep = e.reportToHub(c, me, h, config.Runner{Model: first(ov.model, rc.Model)}, access, initial, log)
	}
	workdir, err := filepath.Abs(first(ov.workdir, rc.Workdir, "."))
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(workdir); err != nil || !st.IsDir() {
		return nil, errorf("not_found", ExitNotFound, "create it or set runner.workdir", "workdir %s does not exist", workdir)
	}
	timeout := ov.timeout
	if timeout == 0 {
		if timeout, err = time.ParseDuration(orDefault(rc.Timeout, "30m")); err != nil {
			return nil, errorf("invalid_config", ExitUsage, "", "runner.timeout: %v", err)
		}
	}
	pollWait, err := time.ParseDuration(orDefault(rc.PollWait, "30s"))
	if err != nil {
		return nil, errorf("invalid_config", ExitUsage, "", "runner.poll_wait: %v", err)
	}
	host := hostname()
	if err := checkBinding(me, name, host, sh.rebind); err != nil {
		// The same machine under a new name (e.g. macOS renamed itself
		// after a network change): take over when the session provably
		// lives here. Exec nodes have no shared session to protect.
		b := me.Binding
		local := (name == execnode.Name || name == machinenode.Name) && b.Harness == name
		if p, ok := h.(harness.BusyProber); ok && b.Harness == name && b.SessionID != "" {
			_, local = p.LastActivity(b.SessionID)
		}
		if !local {
			return nil, err
		}
		log.Info("machine name changed; the session is on this machine, taking it over",
			"agent", me.Name, "was", b.Environment, "now", host)
	}
	if me, err = c.Bind(e.in.Ctx, api.Binding{Harness: name, Environment: host, Workdir: workdir}); err != nil {
		return nil, err
	}
	var sessions runner.SessionStore = runner.NewHubSessions(c, me.Binding.SessionID)
	if sh.scope == runner.ScopeThread {
		if sessions, err = runner.NewFileSessions(filepath.Join(config.Dir(cfgPath), "sessions", me.Name+".json")); err != nil {
			return nil, err
		}
	}
	cfgAbs, _ := filepath.Abs(cfgPath)
	env := []string{config.EnvConfig + "=" + cfgAbs, config.EnvHub + "=" + cfg.Hub, config.EnvToken + "=" + cfg.Token}
	// The agent must call the same myco as the runner (not an older copy
	// elsewhere on PATH): put our directory first and expose MYCO_BIN.
	if self, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(self); err == nil {
			self = real
		}
		env = append(env, "MYCO_BIN="+self, "PATH="+filepath.Dir(self)+string(os.PathListSeparator)+e.in.Getenv("PATH"))
	}
	if rc.EnvFile != "" {
		if _, err := config.ReadEnvFile(rc.EnvFile); err != nil {
			return nil, errorf("invalid_config", ExitUsage, "fix runner.env_file in "+cfgPath, "runner.env_file: %v", err)
		}
	}
	alog := log.With("agent", me.Name)
	// The env file is read again before every turn, and so is the profile's
	// runner.env_file setting: re-running `myco runner env capture`, or
	// adding env_file to the profile, applies to the next message without
	// restarting the runner.
	envFunc := func() []string {
		file := rc.EnvFile
		if c, err := config.Load(cfgPath); err == nil && c.Runner.EnvFile != "" {
			file = c.Runner.EnvFile
		}
		if file == "" {
			return env
		}
		extra, err := config.ReadEnvFile(file)
		if err != nil {
			alog.Warn("cannot read runner.env_file; running without it", "env_file", file, "err", err)
			return env
		}
		return append(extra, env...) // myco's own variables win
	}
	if execSpec != nil {
		allow := "any command"
		if len(execSpec.Allow) > 0 {
			allow = strings.Join(execSpec.Allow, " | ")
		}
		alog.Warn("exec node: tasks from these agents run as shell commands on this machine",
			"senders", strings.Join(me.AllowFrom, ","), "allow", allow, "audit_log", execSpec.AuditLog)
	}
	if access == harness.AccessFull {
		alog.Warn("access=full: the agent CLI runs without approvals or sandbox; messages from other agents drive it")
	}
	if initial.warn != "" {
		alog.Warn("provider: "+initial.warn, "provider", initial.name)
	}
	alog.Info("runner ready", "hub", cfg.Hub, "provider", initial.name, "provider_source", initial.source, "harness", h.Name(), "access", orDefault(string(access), "default"), "workdir", workdir, "environment", host,
		"session_scope", sh.scope, "session_id", me.Binding.SessionID)
	return runner.New(c, h, sessions, runner.DefaultPrompt{Owner: me.Owner}, runner.Options{
		Self: me.Name, Timeout: timeout, PollWait: pollWait, Workdir: workdir, Once: sh.once, Scope: sh.scope,
		BusyRetry: sh.busyRetry, BusyWindow: sh.busyWindow, Slots: sh.slots,
		StatePath:      filepath.Join(config.Dir(e.defaultPath), "state", me.Name+".json"),
		AttachmentsDir: filepath.Join(config.Dir(e.defaultPath), "attachments", me.Name),
		// Lets the agent call `myco` as itself from inside the harness.
		Env: env, EnvFunc: envFunc,
		Provider: func() *harness.ProviderSettings {
			ch := currentProvider()
			if ch.warn != "" {
				alog.Warn("provider: "+ch.warn, "provider", ch.name)
			}
			rep.Provider(ch)
			return ch.settings
		},
	}, alog), nil
}

// runAll serves every profile, rescanning for new ones until ctx ends.
func (e *env) runAll(sh runnerShared, log *slog.Logger) error {
	dir := config.ProfilesDir(e.defaultPath)
	var (
		wg      sync.WaitGroup
		started = map[string]bool{}
		failed  = map[string]string{} // profile -> last start error
		machine = false               // machine nodes ensured
		nodes   = map[string]bool{}   // hub -> machine node ensured
	)
	if sh.rescan == nil {
		sh.rescan = make(chan struct{}, 1)
	}
	scan := func() int {
		if !machine {
			machine = e.ensureMachineNodesLogged(log, nodes)
		}
		profiles, err := config.ListProfiles(dir)
		if err != nil {
			log.Warn("reading profiles failed", "dir", dir, "err", err)
		}
		for _, p := range profiles {
			if started[p.Path] {
				continue
			}
			if runnerHarnessFor(p.Config.Harness) == "" && p.Config.Runner.Harness == "" {
				started[p.Path] = true
				log.Info("skipping agent without a local runner harness", "profile", p.Path, "harness", p.Config.Harness)
				continue
			}
			r, err := e.buildRunner(p.Path, p.Config, sh, runnerOverrides{}, log)
			if err != nil {
				// Retried on every scan (the hub may be restarting); log
				// only when the reason changes.
				msg := toError(err, "").Message
				if failed[p.Path] != msg {
					log.Error("cannot start agent; retrying every 30s", "profile", p.Path, "err", msg)
					failed[p.Path] = msg
				}
				continue
			}
			if _, ok := failed[p.Path]; ok {
				log.Info("agent started after earlier failures", "profile", p.Path)
				delete(failed, p.Path)
			}
			started[p.Path] = true
			wg.Add(1)
			go func() { defer wg.Done(); _ = r.Run(e.in.Ctx) }()
		}
		return len(profiles)
	}
	n := scan()
	if n == 0 && sh.once {
		return errorf("not_found", ExitNotFound, "enroll an agent first: myco agent enroll", "no agent profiles in %s", dir)
	}
	log.Info("runner serving all agents", "profiles_dir", dir, "profiles", n, "max_concurrent", cap(sh.slots))
	if n == 0 {
		hint := "enroll or register an agent on this machine (myco agent enroll / myco agent register --harness exec ...)"
		if _, err := os.Stat(e.defaultPath); err == nil {
			hint = e.defaultPath + " holds an identity but runner --all only serves " + dir + "/*.yaml; re-run the register/enroll/login command with this myco version to create the profile"
		}
		log.Warn("no agent profiles yet; checking again every 30s", "hint", hint)
	}
	if sh.once {
		wg.Wait()
		return nil
	}
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-e.in.Ctx.Done():
			wg.Wait()
			return nil
		case <-t.C:
			scan()
		case <-sh.rescan:
			scan()
		}
	}
}

// cachedAllowFrom returns the agent's current allow_from, asking the hub at
// most once per ttl, so an admin's change reaches a running exec node.
func cachedAllowFrom(ctx context.Context, c *client.Client, ttl time.Duration) func() ([]string, error) {
	var (
		mu   sync.Mutex
		last []string
		at   time.Time
	)
	return func() ([]string, error) {
		mu.Lock()
		defer mu.Unlock()
		if last != nil && time.Since(at) < ttl {
			return last, nil
		}
		me, err := c.Me(ctx)
		if err != nil {
			return nil, err
		}
		last, at = me.AllowFrom, time.Now()
		if last == nil {
			last = []string{}
		}
		return last, nil
	}
}
