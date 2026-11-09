// Package machinenode is the "machine" harness: one node per joined machine,
// served by that machine's runner without a model. Its only job is to start
// new agent sessions there on request (`myco agent spawn`): it checks the
// request against the machine's local policy, runs a short first turn in the
// requested directory to create the session, enrolls it with the machine's
// own credentials and writes its runner profile.
//
// Security (checked here, on the machine, whatever the hub says):
//   - senders: the node's allow_from (inherited from the join link); empty
//     means any agent on the hub, as for any other agent;
//   - directories: only inside the policy's roots (default: home), after
//     resolving symlinks, and never inside a credentials directory;
//   - access: capped by the policy (default cap: workspace); "full" only when
//     the machine's owner set max_access: full locally.
package machinenode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/osutil"
)

// Name is the harness name of machine nodes.
const Name = "machine"

// SpawnableCLIs are the agent CLIs a machine node can start (they all keep
// sessions, so the new agent resumes the same conversation every time).
var SpawnableCLIs = []string{"claude", "codex", "pi", "kiro", "cursor", "gemini", "opencode"}

// ExecCLI adds a shell node (harness exec): each message from its allowed
// senders is a command run in its folder. It has no session.
const ExecCLI = "exec"

// credentialDirs are never valid working directories for a spawned session.
var credentialDirs = osutil.CredentialDirs

// Policy is the machine's local spawn policy (machine.yaml).
type Policy struct {
	// Home expands "~" (the runner user's home directory).
	Home string
	// Dirs are allowed roots; empty = Home.
	Dirs []string
	// MaxAccess caps spawned sessions; "" = workspace.
	MaxAccess harness.Access
}

// Plan is a request that passed the policy.
type Plan struct {
	CLI, Name, Workdir string
	Access             harness.Access
}

// Deps are the effects a spawn needs, injected by the CLI layer.
type Deps struct {
	// NewHarness builds the agent CLI harness for the first turn.
	NewHarness func(cli string, opts harness.Options) (harness.Harness, error)
	// Installed reports whether the CLI can run here (its binary is found).
	Installed func(cli string) bool
	// Enroll registers the new session with the machine's credentials and
	// writes its runner profile; it returns the agent's final name.
	Enroll func(ctx context.Context, p Plan, req api.SpawnRequest, sessionID string) (string, error)
	// Spawned is called after a successful spawn (the runner starts serving
	// the new profile right away instead of at its next scan).
	Spawned func()
}

// Node is a machine node.
type Node struct {
	Machine string // machine name, e.g. "wsl"
	Prefix  string // agent name prefix of this machine, e.g. "wsl-"
	Policy  Policy
	Deps    Deps
	// Senders / CurrentSenders: the node's allow_from (CurrentSenders
	// re-reads it from the hub so an admin's change applies without restart).
	Senders        []string
	CurrentSenders func() ([]string, error)
	// FirstTurnTimeout bounds the session-creating first turn (default 5m).
	FirstTurnTimeout time.Duration
	// LoadPolicy, if set, re-reads the local policy before every request,
	// so a change to machine.yaml (e.g. allowing shell nodes from the app)
	// applies without restarting the runner. Policy is used if it fails.
	LoadPolicy func() (Policy, error)
}

func (n *Node) Name() string       { return Name }
func (n *Node) ExecutesBody() bool { return true }

// Run handles one spawn request. Refusals and failures are results, not
// errors: the sender always gets a SpawnResult back.
func (n *Node) Run(ctx context.Context, req harness.Request) (harness.Result, error) {
	res := n.spawn(ctx, req)
	b, err := json.Marshal(res)
	if err != nil {
		return harness.Result{}, err
	}
	return harness.Result{Output: string(b)}, nil
}

func (n *Node) spawn(ctx context.Context, req harness.Request) api.SpawnResult {
	if !n.senderAllowed(req.From) {
		return denied("sender %q may not start sessions on machine %s (not in the machine node's allow_from)", req.From, n.Machine)
	}
	var sr api.SpawnRequest
	if err := json.Unmarshal([]byte(req.Body), &sr); err != nil {
		return denied("the task body must be a spawn request (JSON), as sent by `myco agent spawn`: %v", err)
	}
	if n.LoadPolicy != nil {
		if p, err := n.LoadPolicy(); err == nil {
			n.Policy = p // one request at a time per node: no concurrent reader
		}
	}
	plan, err := n.Check(sr)
	if err != nil {
		return denied("%v", err)
	}
	if plan.CLI == ExecCLI { // no session to start: join with the hub's exec token
		name, err := n.Deps.Enroll(ctx, plan, sr, "")
		if err != nil {
			return failed(plan, "joining the shell node failed: %v", err)
		}
		if n.Deps.Spawned != nil {
			n.Deps.Spawned()
		}
		return api.SpawnResult{Agent: name, CLI: plan.CLI, Workdir: plan.Workdir, Access: string(plan.Access)}
	}
	if n.Deps.Installed != nil && !n.Deps.Installed(plan.CLI) {
		return denied("%s is not installed on machine %s", plan.CLI, n.Machine)
	}
	h, err := n.Deps.NewHarness(plan.CLI, harness.Options{Access: plan.Access, Model: sr.Model})
	if err != nil {
		return failed(plan, "cannot prepare %s: %v", plan.CLI, err)
	}
	if _, ok := h.(harness.Forker); sr.ForkFrom != "" && !ok {
		return denied("%v", harness.ErrForkUnsupported)
	}
	timeout := n.FirstTurnTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	turnCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	first := harness.Request{Prompt: n.firstPrompt(req.From, plan, sr), Workdir: plan.Workdir, Env: req.Env}
	if sr.ForkFrom != "" {
		first.SessionID, first.Fork = sr.ForkFrom, true
	}
	out, err := h.Run(turnCtx, first)
	if err != nil {
		if sr.ForkFrom != "" {
			return failed(plan, "copying session %s failed: %v", sr.ForkFrom, err)
		}
		return failed(plan, "starting the %s session failed: %v", plan.CLI, err)
	}
	if out.SessionID == "" || (sr.ForkFrom != "" && out.SessionID == sr.ForkFrom) {
		return failed(plan, "%s did not report a session id, so the session could not be joined", plan.CLI)
	}
	name, err := n.Deps.Enroll(ctx, plan, sr, out.SessionID)
	if err != nil {
		return failed(plan, "the session started (%s) but joining it failed: %v", out.SessionID, err)
	}
	if n.Deps.Spawned != nil {
		n.Deps.Spawned()
	}
	return api.SpawnResult{Agent: name, SessionID: out.SessionID, CLI: plan.CLI, Workdir: plan.Workdir,
		Access: string(plan.Access), Reply: strings.TrimSpace(out.Output)}
}

func (n *Node) senderAllowed(from string) bool {
	senders := n.Senders
	if n.CurrentSenders != nil {
		if cur, err := n.CurrentSenders(); err == nil {
			senders = cur
		}
	}
	// The hub sends as "@admin" when an administrator starts a session from
	// the console. Agent names cannot contain "@", and the hub is what
	// vouches for every sender anyway; the directory and access rules
	// below still apply.
	if from == AdminSender {
		return true
	}
	return len(senders) == 0 || slices.Contains(senders, from) || slices.Contains(senders, "*")
}

// AdminSender is the hub's sender for administrators (app.AdminSender).
const AdminSender = "@admin"

// Check validates a request against the policy and resolves it.
func (n *Node) Check(sr api.SpawnRequest) (Plan, error) {
	cli := strings.TrimSpace(sr.CLI)
	if cli == "" {
		cli = "claude"
	}
	if cli == ExecCLI {
		return n.checkShell(sr)
	}
	if !slices.Contains(SpawnableCLIs, cli) {
		return Plan{}, fmt.Errorf("cannot spawn %q: supported CLIs are %s", cli, strings.Join(SpawnableCLIs, ", "))
	}
	access, err := n.access(sr.Access)
	if err != nil {
		return Plan{}, err
	}
	dir, err := n.dir(sr.Dir)
	if err != nil {
		return Plan{}, err
	}
	return Plan{CLI: cli, Name: n.name(sr.Name, cli, dir), Workdir: dir, Access: access}, nil
}

var accessRank = map[harness.Access]int{harness.AccessReadonly: 1, harness.AccessWorkspace: 2, harness.AccessFull: 3}

func (n *Node) access(requested string) (harness.Access, error) {
	limit := n.Policy.MaxAccess
	if limit == "" {
		limit = harness.AccessWorkspace
	}
	if requested == "" {
		requested = string(harness.AccessWorkspace)
	}
	a, err := harness.ParseAccess(requested)
	if err != nil || a == "" {
		return "", fmt.Errorf("access must be readonly, workspace or full")
	}
	if accessRank[a] > accessRank[limit] {
		return "", fmt.Errorf("access %s is above this machine's limit (%s); the machine's owner can raise it with spawn.max_access in ~/.myco/machine.yaml", a, limit)
	}
	return a, nil
}

func (n *Node) expand(p string) string {
	return osutil.ExpandHome(strings.TrimSpace(p), n.Policy.Home)
}

// dir resolves the working directory: absolute, symlinks resolved, existing,
// inside an allowed root and outside credentials directories.
func (n *Node) dir(requested string) (string, error) {
	if strings.TrimSpace(requested) == "" {
		return "", errors.New("pass the directory to start the session in (--dir)")
	}
	p := n.expand(requested)
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("directory %q must be absolute or start with ~/", requested)
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", fmt.Errorf("directory %s does not exist on machine %s", requested, n.Machine)
	}
	if st, err := os.Stat(real); err != nil || !st.IsDir() {
		return "", fmt.Errorf("%s is not a directory", requested)
	}
	roots := n.Policy.Dirs
	if len(roots) == 0 {
		roots = []string{n.Policy.Home}
	}
	inside := false
	for _, r := range roots {
		root, err := filepath.EvalSymlinks(n.expand(r))
		if err != nil {
			continue
		}
		if osutil.Within(real, root) {
			inside = true
			break
		}
	}
	if !inside {
		return "", fmt.Errorf("directory %s is outside the allowed directories on machine %s (%s); the machine's owner can change spawn.dirs in ~/.myco/machine.yaml",
			requested, n.Machine, strings.Join(roots, ", "))
	}
	// By path component of the resolved directory, not by prefix of Home:
	// Home may itself be a symlink (or an 8.3 short name on Windows), and
	// then no resolved path would ever start with it.
	if osutil.InCredentialDir(real) {
		return "", fmt.Errorf("refusing to start a session inside a credentials directory (%s)", strings.Join(credentialDirs, ", "))
	}
	return real, nil
}

// name returns the agent name: the requested one with the machine prefix
// added when missing, or <prefix><dir>-<cli>.
func (n *Node) name(requested, cli, dir string) string {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested != "" {
		if strings.HasPrefix(requested, n.Prefix) {
			return requested
		}
		return n.Prefix + requested
	}
	return n.Prefix + slug(filepath.Base(dir)) + "-" + cli
}

func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "agent"
	}
	return out
}

func (n *Node) firstPrompt(from string, p Plan, sr api.SpawnRequest) string {
	var b strings.Builder
	if sr.ForkFrom != "" {
		fmt.Fprintf(&b, "[myco] This session is a copy of session %s, made by %q through myco on machine %s, in %s. You keep everything from the original conversation; the original continues separately.\n", sr.ForkFrom, from, n.Machine, p.Workdir)
	} else {
		fmt.Fprintf(&b, "[myco] This session was started by agent %q through myco on machine %s, in %s.\n", from, n.Machine, p.Workdir)
	}
	fmt.Fprintf(&b, "It will join the myco network as agent %q; other agents will send it tasks later, and each one resumes this session.\n", p.Name)
	if d := strings.TrimSpace(sr.Description); d != "" {
		fmt.Fprintf(&b, "Its role: %s\n", d)
	}
	if pr := strings.TrimSpace(sr.Prompt); pr != "" {
		fmt.Fprintf(&b, "\nFirst instructions from %s:\n%s\n", from, pr)
	}
	b.WriteString("\nDo not start any work yet. Reply with one short line confirming you are ready.\n")
	return b.String()
}

func denied(format string, args ...any) api.SpawnResult {
	return api.SpawnResult{Denied: true, Error: fmt.Sprintf(format, args...)}
}

func failed(p Plan, format string, args ...any) api.SpawnResult {
	return api.SpawnResult{CLI: p.CLI, Workdir: p.Workdir, Access: string(p.Access), Error: fmt.Sprintf(format, args...)}
}

// checkShell validates a shell node: whoever it allows runs commands with
// the runner user's full rights, so the machine must allow full access, and
// only the hub admin can ask (the hub then supplies an exec enrollment).
func (n *Node) checkShell(sr api.SpawnRequest) (Plan, error) {
	if sr.EnrollToken == "" {
		return Plan{}, errors.New("shell nodes can only be added by the hub's administrator (in the myco app)")
	}
	if n.Policy.MaxAccess != harness.AccessFull {
		limit := n.Policy.MaxAccess
		if limit == "" {
			limit = harness.AccessWorkspace
		}
		return Plan{}, fmt.Errorf("a shell node runs commands with full rights, but machine %s allows at most %s; the machine's owner can allow it with spawn.max_access: full in ~/.myco/machine.yaml",
			n.Machine, limit)
	}
	dir, err := n.dir(sr.Dir)
	if err != nil {
		return Plan{}, err
	}
	return Plan{CLI: ExecCLI, Name: n.name(sr.Name, "shell", dir), Workdir: dir, Access: harness.AccessFull}, nil
}
