package cli

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/harness"
	"github.com/myco-ai/myco/internal/harness/builtin"
	"github.com/myco-ai/myco/internal/harness/execnode"
	"github.com/myco-ai/myco/internal/harness/machinenode"
	"github.com/myco-ai/myco/internal/logkeep"
	"github.com/myco-ai/myco/internal/processwatch"
	"github.com/myco-ai/myco/internal/quota"
	"github.com/myco-ai/myco/internal/runner"
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
			{Name: "max-concurrent", Kind: Int, Default: "3", Help: "With --all: max agent CLIs running at once, shared fairly between workspaces"},
			{Name: "harness", Kind: Enum, Values: harnesses.Names(), Help: "Agent CLI adapter (default from config, else claude); see: myco harness list"},
			{Name: "access", Kind: Enum, Values: []string{"readonly", "workspace", "full"}, Help: "Permission level mapped to CLI flags (default: config, else the CLI's own headless default)"},
			{Name: "model", Kind: String, Help: "Model in the CLI's own naming"},
			{Name: "binary", Kind: String, Help: "Path to the agent CLI executable"},
			{Name: "arg", Kind: List, Help: "Extra argument for the agent CLI (repeatable), e.g. --arg=--permission-mode --arg=acceptEdits"},
			{Name: "command", Kind: String, Help: "Shell command line for --harness shell (prompt on stdin, reply on stdout)"},
			{Name: "workdir", Kind: String, Help: "Working directory for the agent CLI (default: config, else current dir)"},
			{Name: "timeout", Kind: Duration, Help: "Stop a run after this long without any activity from the agent CLI (default 30m; a run that keeps working may take up to 6h)"},
			{Name: "once", Kind: Bool, Help: "Process currently queued messages once and exit"},
			{Name: "session-scope", Kind: Enum, Values: []string{"agent", "thread"}, Default: "agent",
				Help: "agent: this agent is one session, every message resumes it; thread: one session per thread"},
			{Name: "busy-window", Kind: Duration, Default: "90s", Help: "Treat the session as in use if someone else wrote to it this recently"},
			{Name: "busy-retry", Kind: Duration, Default: "2m", Help: "Max delay between retries while the session is busy (starts at 10s, doubles)"},
			{Name: "rebind", Kind: Bool, Help: "Allow taking over an agent bound to another environment or harness"},
			{Name: "log-level", Kind: Enum, Values: []string{"debug", "info", "warn", "error"}, Default: "info", Help: "Log verbosity"},
			{Name: "log-file", Kind: String, Help: "Append the log to this file instead of stderr (used by the Windows background runner)"},
			{Name: "trace", Kind: Enum, Values: []string{"full", "summary", "off"}, Default: "full", Help: "How much of a task's run the sender sees live in the app: full (what the agent says, thinks and does), summary (which tools ran), off"},
		},
		Run: handler(func(e *env) error {
			_ = os.Remove(runnerRestartPath(e.defaultPath)) // this is the replacement process
			if runtime.GOOS == "windows" {
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				ctx, cancel := watchRunnerStop(e.in.Ctx, filepath.Join(home, ".myco", "runner-stop-"+strconv.Itoa(os.Getpid())))
				defer cancel()
				e.in.Ctx = ctx
			}
			log := newLogger(e.in.String("log-level"))
			// Its log file (--log-file, or runner.log the service writes its
			// output to) is kept to two weeks, trimmed daily or past 20 MB.
			keep := make(chan struct{})
			defer close(keep)
			if path := e.in.String("log-file"); path != "" {
				f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
				if err != nil {
					return errorf("invalid_argument", ExitUsage, "pass a writable file to --log-file", "cannot open log file: %v", err)
				}
				defer f.Close()
				log = newLoggerTo(f, e.in.String("log-level"))
				go logkeep.Run(path, logkeep.Policy{}, keep)
			} else if lp := filepath.Join(config.Dir(e.defaultPath), "runner.log"); logkeep.IsStderr(lp) {
				go logkeep.Run(lp, logkeep.Policy{}, keep)
			}
			processes := processwatch.NewPersistent(runnerInstance, filepath.Join(config.Dir(e.defaultPath), "process-scopes"))
			defer processes.Close()
			healthCtx, stopHealth := context.WithCancel(e.in.Ctx)
			defer stopHealth()
			go processes.Run(healthCtx)
			shared := runnerShared{
				processes: processes,
				once:      e.in.Bool("once"), busyWindow: e.in.Duration("busy-window"), busyRetry: e.in.Duration("busy-retry"),
				scope: runner.SessionScope(e.in.String("session-scope")), rebind: e.in.Bool("rebind"), registry: harnesses,
				trace: e.in.String("trace"), locks: runner.NewSessionLocks(filepath.Join(config.Dir(e.defaultPath), "locks")),
			}
			e.watchVersions(log)
			// The agent CLIs it wakes read the skill of this version (a runner
			// restarted by myco upgrade puts the new one in place).
			if !e.in.Bool("once") {
				if written, _, err := installSkill(skillTargets(e.in.Getenv), false); err != nil {
					log.Debug("installing the myco skill failed", "err", err)
				} else if len(written) > 0 {
					log.Info("installed the myco skill", "version", Version, "paths", strings.Join(written, ", "))
				}
				if changed, _, err := installMemory(memoryTargets(e.in.Getenv), memoryRefresh); err == nil && len(changed) > 0 {
					log.Info("updated the myco lines in the agent CLIs' instructions", "paths", strings.Join(changed, ", "))
				}
			}
			if !e.in.Bool("all") {
				r, err := e.buildRunner(e.cfgPath, e.cfg, shared, e.overrides(), log)
				if err != nil {
					return err
				}
				return r.Run(e.in.Ctx)
			}
			if n := e.in.Int("max-concurrent"); n > 0 {
				shared.slots = runner.NewSlotPool(n)
			}
			return e.runAll(shared, log)
		}),
	}
	return (&Command{Name: "runner", Summary: "Turn agent CLIs into always-reachable workers"}).Add(start, serviceCommands(), runnerEnvCommands(), runnerProviderCommands())
}

// runnerShared holds settings common to every agent a process serves.
type runnerShared struct {
	processes  *processwatch.Manager
	once       bool
	busyWindow time.Duration
	busyRetry  time.Duration
	scope      runner.SessionScope
	rebind     bool
	trace      string
	slots      *runner.SlotPool
	// locks: one turn at a time per CLI session, across every agent
	// (several workspaces can share a session) and runner process.
	locks    *runner.SessionLocks
	registry *harness.Registry
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
	return e.buildRunnerContext(e.in.Ctx, cfgPath, cfg, sh, ov, log)
}

// startup only bounds the initial Hub handshake. Runtime reporters and inbox
// polling retain the service context after startup succeeds.
func (e *env) buildRunnerContext(startup context.Context, cfgPath string, cfg *config.Config, sh runnerShared, ov runnerOverrides, log *slog.Logger) (*runner.Runner, error) {
	startupStart := time.Now()
	startupStamp := func(stage string) {
		log.Info("runner startup latency", "agent", cfg.Agent, "stage", stage, "elapsed_ms", time.Since(startupStart).Milliseconds(), "observed_at_ms", time.Now().UnixMilli())
	}
	startupStamp("profile_build_start")
	defer startupStamp("profile_build_end")
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
	maxAccess := e.machineMaxAccess()
	if cfg.Hub == "" || cfg.Token == "" {
		return nil, errorf("not_configured", ExitUsage, "enroll or register this agent first", "%s has no hub/token", cfgPath)
	}
	c, err := client.New(cfg.Hub, cfg.Token)
	if err != nil {
		return nil, err
	}
	e.withSealer(c, cfgPath, cfg)
	c.Via = "runner" // yields to a session that keeps `inbox wait` running
	me, err := c.Me(startup)
	if err != nil {
		return nil, err
	}
	startupStamp("identity_ready")
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
		spec.CurrentSenders = cachedExecSenders(e.in.Ctx, c, 30*time.Second)
		spec.AuditLog = filepath.Join(config.Dir(e.defaultPath), "logs", me.Name+".exec.jsonl")
		execSpec = &spec
	}
	var h harness.Harness
	if name == machinenode.Name {
		if h, err = e.machineNode(cfg.Hub, me, c, sh); err != nil {
			return nil, err
		}
	} else {
		h, err = sh.registry.New(name, harness.Options{ClientVersion: Version, Binary: first(ov.binary, rc.Binary), Args: args, Command: first(ov.command, rc.Command),
			Model: first(ov.model, rc.Model), Access: access, Template: rc.Template, Exec: execSpec, ACPTransport: rc.ACPTransport})
		if err != nil {
			return nil, errorf("invalid_argument", ExitUsage, "available harnesses: "+strings.Join(sh.registry.Names(), ", "), "%v", err)
		}
	}
	_, commandExecutor := h.(harness.CommandExecutor)
	c.WakeHeld = !commandExecutor
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
	startupStamp("initial_provider_ready")
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
	if me, err = c.Bind(startup, api.Binding{Harness: name, Environment: host, Workdir: workdir}); err != nil {
		return nil, err
	}
	startupStamp("binding_ready")
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
	var rep *runtimeReporter
	if !sh.once {
		rep = e.reportToHub(c, me, h, config.Runner{Model: first(ov.model, rc.Model)}, access, maxAccess, initial, workdir, log)
		if sh.processes != nil {
			go rep.watchHealth(e.in.Ctx, sh.processes, cfg.Hub, me.OwnerID)
		}
		// (a change in machine.yaml, from the app's "Allow" or by hand, is
		// reported within seconds; each run reads it again: liveMaxAccess)
		go rep.watchMaxAccess(e.in.Ctx, 3*time.Second, maxAccess, e.machineMaxAccess)
		if sh.slots != nil {
			go rep.watchSlots(e.in.Ctx, sh.slots, runner.SlotOwner(cfg.Hub, me.Name))
		}
	}
	if rep != nil && e.in.Getenv("CLAUDE_CODE_REMOTE_SESSION_ID") == "" && !strings.HasPrefix(me.Binding.Environment, "claude-cloud") && (h.Name() == "claude" || h.Name() == "codex" || h.Name() == "opencode") {
		home, _ := os.UserHomeDir()
		rep.startQuota(e.in.Ctx, config.Dir(e.defaultPath), func(ch providerChoice) quota.Input {
			env := quotaEnvironment(envFunc())
			var overrides map[string]string
			pinned := false
			if ch.settings != nil {
				overrides = ch.settings.Env
				for k, v := range overrides {
					env[k] = v
				}
				pinned = strings.Contains(strings.Join(ch.settings.ConfigArgs, " "), "model_provider")
			}
			return quota.Input{CLI: h.Name(), Workdir: workdir, Binary: first(ov.binary, rc.Binary), Home: home, Host: ch.host, Model: first(rep.quotaModel(), ov.model, rc.Model), Env: env, Overrides: overrides, Pinned: pinned, UnmodeledLaunch: len(args) > 0}
		}, currentProvider)
	}
	if rep != nil && e.in.Getenv("CLAUDE_CODE_REMOTE_SESSION_ID") == "" && !strings.HasPrefix(me.Binding.Environment, "claude-cloud") && (h.Name() == "claude" || h.Name() == "codex") {
		home, _ := os.UserHomeDir()
		rep.startModels(e.in.Ctx, config.Dir(e.defaultPath), func(ch providerChoice) (providerTarget, bool) {
			return providerTargetOf(h.Name(), home, quotaEnvironment(envFunc()), ch)
		}, currentProvider)
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
	// The cap is on agent CLIs: a command node's shell commands and a
	// machine node's own work (upgrade, start an agent) never wait for one.
	slots := sh.slots
	if h.Name() == execnode.Name || h.Name() == machinenode.Name {
		slots = nil
	}
	alog.Info("runner ready", "hub", cfg.Hub, "provider", initial.name, "provider_source", initial.source, "harness", h.Name(), "access", orDefault(string(access), "default"), "workdir", workdir, "environment", host,
		"session_scope", sh.scope, "session_id", me.Binding.SessionID)
	return runner.New(c, h, sessions, runner.DefaultPrompt{Owner: me.Owner}, runner.Options{
		Processes: sh.processes, ProcessWorkspace: cfg.Hub, ProcessMember: me.OwnerID,
		Self: me.Name, Timeout: timeout, PollWait: pollWait, Workdir: workdir, Once: sh.once, Scope: sh.scope,
		BusyRetry: sh.busyRetry, BusyWindow: sh.busyWindow, SlotPool: slots, OnSlots: rep.Slots, Trace: sh.trace, SessionLocks: sh.locks,
		StatePath:      runnerStatePath(e.defaultPath, me.Name),
		HubURL:         cfg.Hub,
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
		OnRunSettings:         rep.RunSettings,
		ModelEnv:              rep.ModelEnv,
		OnModelsRefresh:       rep.ModelsRefresh,
		ApplyProviderSettings: remoteProviderApplier(cfgPath, c),
		OnQuota:               rep.Quota,
		OnStop:                rep.Goodbye,
		Access:                access,
		MaxAccess:             maxAccess,
		MaxAccessFunc:         e.machineMaxAccess,
		CommandRunnerID:       runnerInstance,
		CommandEnvironment:    host,
		CommandWatchAllowed: func() bool {
			return name != execnode.Name && name != machinenode.Name && e.machineMaxAccess() == harness.AccessFull
		},
	}, alog), nil
}

// Handshakes have a bounded retry window, separate from long-lived workers.
const runnerStartupTimeout = 20 * time.Second

// runAll serves every profile, rescanning for new ones until ctx ends.
func (e *env) runAll(sh runnerShared, log *slog.Logger) error {
	dir := config.ProfilesDir(e.defaultPath)
	var (
		wg          sync.WaitGroup
		mu          sync.Mutex            // started: a runner ends by itself when its node is taken over
		said        = map[string]bool{}   // replaced profiles already logged (path + token)
		refused     = map[string]string{} // profile -> the credential the hub refused
		started     = map[string]bool{}
		failed      = map[string]string{} // profile -> last start error
		nodes       = map[string]bool{}   // hub -> machine node ensured
		nodePending = map[string]bool{}
		starts      = map[string]chan struct{}{} // per Hub; a slow Hub cannot consume another's startup slots
	)
	if sh.rescan == nil {
		sh.rescan = make(chan struct{}, 1)
	}
	scan := func() int {
		if sh.once {
			e.ensureMachineNodesLogged(log, nodes)
		} else if _, hubs, err := e.machineNodeHubs(); err != nil {
			log.Warn("reading machine.yaml failed", "err", err)
		} else {
			for _, hub := range hubs {
				mu.Lock()
				if nodes[hub.Hub] || nodePending[hub.Hub] {
					mu.Unlock()
					continue
				}
				nodePending[hub.Hub] = true
				mu.Unlock()
				wg.Add(1)
				go func() {
					defer wg.Done()
					ctx, cancel := context.WithTimeout(e.in.Ctx, runnerStartupTimeout)
					defer cancel()
					done := e.ensureMachineNodeLogged(ctx, log, hub)
					mu.Lock()
					nodes[hub.Hub] = done
					delete(nodePending, hub.Hub)
					mu.Unlock()
					if done {
						select {
						case sh.rescan <- struct{}{}:
						default:
						}
					}
				}()
			}
		}
		profiles, err := config.ListProfiles(dir)
		if err != nil {
			log.Warn("reading profiles failed", "dir", dir, "err", err)
		}
		mu.Lock()
		defer mu.Unlock()
		for _, p := range profiles {
			if started[p.Path] || (p.Config.Harness == machinenode.Name && nodePending[p.Config.Hub]) {
				continue
			}
			if tok, ok := refused[p.Path]; ok {
				if tok == p.Config.Token {
					continue // still the refused credential: its runner said why
				}
				delete(refused, p.Path) // a new identity was saved here
			}
			if p.Config.Replaced != "" {
				// Another session took this node over: not woken here any
				// more (until a new identity is saved in this profile).
				if k := p.Path + "\x00" + p.Config.Token; !said[k] {
					said[k] = true
					log.Info("skipping agent taken over by another session", "profile", p.Path, "agent", p.Config.Agent)
				}
				continue
			}
			if p.Config.Harness == "inbox" {
				started[p.Path] = true
				log.Info("skipping inbox-only agent", "profile", p.Path)
				continue
			}
			if runnerHarnessFor(p.Config.Harness) == "" && p.Config.Runner.Harness == "" {
				started[p.Path] = true
				log.Info("skipping agent without a local runner harness", "profile", p.Path, "harness", p.Config.Harness)
				continue
			}
			started[p.Path] = true // includes a pending handshake; no duplicate scans
			slots := starts[p.Config.Hub]
			if slots == nil {
				slots = make(chan struct{}, 4)
				starts[p.Config.Hub] = slots
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case slots <- struct{}{}:
				case <-e.in.Ctx.Done():
					return
				}
				ctx, cancel := context.WithTimeout(e.in.Ctx, runnerStartupTimeout)
				r, err := e.buildRunnerContext(ctx, p.Path, p.Config, sh, runnerOverrides{}, log)
				cancel()
				<-slots
				mu.Lock()
				if err != nil {
					delete(started, p.Path)
					if runner.IsReplaced(err) {
						markReplaced(p.Path, p.Config.Token, err)
						refused[p.Path] = p.Config.Token
						log.Info("skipping agent taken over by another session", "profile", p.Path, "agent", p.Config.Agent)
						mu.Unlock()
						return
					}
					if runner.IsCredentialRejected(err) {
						refused[p.Path] = p.Config.Token
						log.Error("这台机器的凭据已被 Hub 拒绝，可能已从 Hub 移除；如需重新加入，请使用加入链接，或运行 myco runner service uninstall", "hub", p.Config.Hub, "profile", p.Path, "err", err)
						mu.Unlock()
						return
					}
					// Retried on every scan (the hub may be restarting); log
					// only when the reason changes.
					msg := toError(err, "").Message
					if failed[p.Path] != msg {
						log.Error("cannot start agent; retrying every 30s", "profile", p.Path, "err", msg)
						failed[p.Path] = msg
					}
					mu.Unlock()
					return
				}
				if _, ok := failed[p.Path]; ok {
					log.Info("agent started after earlier failures", "profile", p.Path)
					delete(failed, p.Path)
				}
				mu.Unlock()
				path, token := p.Path, p.Config.Token
				if err := r.Run(e.in.Ctx); runner.IsCredentialRejected(err) {
					// Taken over (a handover) or its credential replaced:
					// look again, as a new identity may have been saved in
					// this profile (a new session on this machine).
					if runner.IsReplaced(err) {
						markReplaced(path, token, err)
					}
					mu.Lock()
					delete(started, path)
					refused[path] = token
					mu.Unlock()
					select {
					case sh.rescan <- struct{}{}:
					default:
					}
				}
			}()
		}
		return len(profiles)
	}
	n := scan()
	if !sh.once {
		// Chat-app bots configured here (myco im connect) run with the
		// agents, so connecting one needs no other service.
		go e.serveBridges(e.in.Ctx, log)
	}
	if n == 0 && sh.once {
		return errorf("not_found", ExitNotFound, "enroll an agent first: myco agent enroll", "no agent profiles in %s", dir)
	}
	limit := 0
	if sh.slots != nil {
		limit = sh.slots.Snapshot("").MaxConcurrent
	}
	log.Info("runner serving all agents", "profiles_dir", dir, "profiles", n, "max_concurrent", limit)
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

// markReplaced records in a profile that the hub said its session was
// taken over, unless the profile already holds another identity (the new
// session took it over on this machine).
func markReplaced(path, token string, err error) {
	cfg, lerr := config.Load(path)
	if lerr != nil || cfg.Token != token {
		return
	}
	msg := err.Error()
	if ae, ok := client.AsError(err); ok {
		msg = ae.Message
	}
	cfg.Replaced = msg
	_ = config.Save(path, cfg)
}

// cachedAllowFrom returns the agent's current allow_from, asking the hub at
// most once per ttl, so an admin's change reaches a running machine node.
func cachedAllowFrom(ctx context.Context, c *client.Client, ttl time.Duration) func() ([]string, error) {
	return cachedSenders(ctx, c, ttl, false)
}

// cachedExecSenders derives the owner's console identity only from the Hub's
// authenticated /me response. Keep it out of the static fallback: if refresh
// fails, execnode must not retain an expired owner exception.
func cachedExecSenders(ctx context.Context, c *client.Client, ttl time.Duration) func() ([]string, error) {
	return cachedSenders(ctx, c, ttl, true)
}

func cachedSenders(ctx context.Context, c *client.Client, ttl time.Duration, execOwner bool) func() ([]string, error) {
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
		last, at = append([]string{}, me.AllowFrom...), time.Now()
		// An empty allow_from still requires the existing exec-risk setup;
		// knowing the owner must not turn an unconfigured node into an executor.
		if execOwner && len(last) > 0 && me.Owner != "" {
			last = append(last, "@console:"+me.Owner)
		}
		return last, nil
	}
}

// machineMaxAccess is the highest access this machine lets an agent's owner
// choose from the app: spawn.max_access in ~/.myco/machine.yaml, default
// workspace.
func (e *env) machineMaxAccess() harness.Access {
	if mf, err := config.LoadMachine(e.defaultPath); err == nil && mf != nil {
		if a, err := harness.ParseAccess(mf.Spawn.MaxAccess); err == nil && a != harness.AccessDefault {
			return a
		}
	}
	return harness.AccessWorkspace
}
