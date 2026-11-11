// Package acp drives agent CLIs that speak the Agent Client Protocol
// (https://agentclientprotocol.com): JSON-RPC over the CLI's stdin and
// stdout. One process per run: initialize, load (or start) the session, send
// the prompt, follow its updates, then shut the process down. Gemini CLI
// (`gemini --acp`) is a preset; any other ACP agent works with
// harness: acp, binary and args.
package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/harness"
)

// Name is the registry key of the generic ACP harness.
const Name = "acp"

// protocolVersion is the ACP version myco speaks.
const protocolVersion = 1

// ClientVersion is myco's version, told to the agent (set by the CLI).
var ClientVersion = "dev"

// Spec describes how to start an ACP agent. Placeholders: {model} in
// ModelArgs, {dir} in DirArgs. The command line is Binary, Args, the model,
// access and directory arguments, then Sub (a subcommand that must come
// after the options, as in `cursor-agent --model m acp`).
type Spec struct {
	Binary string
	Args   []string
	Sub    []string
	// ModelArgs select a model, e.g. ["-m", "{model}"]. ModelConfig instead
	// names the session config option that holds the model (set with
	// session/set_config_option once the session is open).
	ModelArgs   []string
	ModelConfig string
	// AccessArgs and AccessEnv are extra arguments and KEY=VALUE variables
	// per access level (the CLI's own approval settings). Whatever the CLI
	// still asks is answered by myco from the same access level (policy).
	AccessArgs map[harness.Access][]string
	AccessEnv  map[harness.Access][]string
	// DirArgs give the agent one more directory to read, per directory.
	DirArgs []string
	// VersionArgs print the version (default --version).
	VersionArgs []string
	// Login is shown when the agent says it needs authentication.
	Login string
}

type agent struct {
	name string
	spec Spec
	opts harness.Options
}

// New is the factory of the generic ACP harness: runner.binary is the
// agent, runner.args its arguments (e.g. binary: opencode, args: [acp]).
func New(opts harness.Options) (harness.Harness, error) {
	if opts.Binary == "" {
		return nil, errors.New("acp: set runner.binary to the agent's command (and runner.args, e.g. [acp])")
	}
	return &agent{name: Name, spec: Spec{Binary: opts.Binary, Args: opts.Args}, opts: opts}, nil
}

// Preset returns the factory for a known ACP agent; runner.binary replaces
// its binary and runner.args are added to its arguments.
func Preset(name string, spec Spec) harness.Factory {
	return func(opts harness.Options) (harness.Harness, error) {
		s := spec
		if opts.Binary != "" {
			s.Binary = opts.Binary
		}
		s.Args = append(append([]string{}, spec.Args...), opts.Args...)
		return &agent{name: name, spec: s, opts: opts}, nil
	}
}

func (a *agent) Name() string { return a.name }

// Check implements harness.Checker.
func (a *agent) Check(ctx context.Context) harness.Check {
	v := a.spec.VersionArgs
	if len(v) == 0 {
		v = []string{"--version"}
	}
	return harness.CheckBinary(ctx, a.spec.Binary, v...)
}

func (a *agent) args(req harness.Request) []string {
	args := append([]string{}, a.spec.Args...)
	if a.opts.Model != "" && a.spec.ModelConfig == "" {
		for _, s := range a.spec.ModelArgs {
			args = append(args, strings.ReplaceAll(s, "{model}", a.opts.Model))
		}
	}
	args = append(args, a.spec.AccessArgs[a.opts.Access]...)
	for _, d := range req.ExtraDirs {
		for _, s := range a.spec.DirArgs {
			args = append(args, strings.ReplaceAll(s, "{dir}", d))
		}
	}
	return append(args, a.spec.Sub...)
}

// How long the agent gets to start, and to wrap up after a cancel.
const (
	startTimeout  = 2 * time.Minute
	cancelTimeout = 10 * time.Second
	exitTimeout   = 3 * time.Second
)

func (a *agent) Run(ctx context.Context, req harness.Request) (harness.Result, error) {
	if req.Fork {
		return harness.Result{}, harness.ErrForkUnsupported
	}
	cwd := req.Workdir
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}

	// The process outlives ctx by a little: a cancelled run is cancelled
	// over the protocol first, so the agent can save its session.
	procCtx, kill := context.WithCancel(context.WithoutCancel(ctx))
	defer kill()
	env := append(append([]string{}, a.spec.AccessEnv[a.opts.Access]...), req.Env...)
	cmd := harness.Command(procCtx, harness.Process{Binary: a.spec.Binary, Args: a.args(req), Workdir: req.Workdir, Env: env})
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return harness.Result{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return harness.Result{}, err
	}
	stderr := &tailBuffer{max: 64 << 10}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return harness.Result{}, fmt.Errorf("%s: %w", a.spec.Binary, err)
	}
	exited := make(chan struct{})
	c := newConn(stdin)
	go func() {
		c.read(stdout)
		_ = cmd.Wait()
		close(exited)
	}()
	defer func() {
		_ = stdin.Close()
		select {
		case <-exited:
		case <-time.After(exitTimeout):
			kill() // interrupt, then kill the process group
			<-exited
		}
	}()

	st := newStream(req.OnEvent, cwd)
	var refused []string // read after the turn (the reader is done by then)
	pol := policy{access: a.opts.Access, dirs: append([]string{cwd}, req.ExtraDirs...), refused: &refused}
	c.onNotify = func(method string, params json.RawMessage) {
		if method != "session/update" {
			return
		}
		var p struct {
			Update json.RawMessage `json:"update"`
		}
		if json.Unmarshal(params, &p) == nil {
			st.update(p.Update)
		}
	}
	c.onRequest = func(method string, params json.RawMessage) (any, *rpcError) {
		if method == "session/request_permission" {
			var p permissionRequest
			_ = json.Unmarshal(params, &p)
			st.asked(p.ToolCall) // shown before it runs, even if refused
			return pol.answer(params), nil
		}
		// No fs/* or terminal/*: myco does not offer them (the agent uses
		// its own file system and shell).
		return nil, &rpcError{Code: codeMethodNotFound, Message: "myco does not provide " + method}
	}

	sessionID := ""
	fail := func(err error) (harness.Result, error) {
		msg := a.explain(err, stderr.String())
		st.failed(msg)
		res := harness.Result{SessionID: sessionID}
		if res.SessionID == "" {
			res.SessionID = req.SessionID
		}
		return res, errors.New(msg)
	}

	// initialize
	startCtx, cancelStart := context.WithTimeout(ctx, startTimeout)
	defer cancelStart()
	var init struct {
		ProtocolVersion   int `json:"protocolVersion"`
		AgentCapabilities struct {
			LoadSession         bool `json:"loadSession"`
			SessionCapabilities struct {
				Resume json.RawMessage `json:"resume"`
			} `json:"sessionCapabilities"`
		} `json:"agentCapabilities"`
	}
	err = c.call(startCtx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"clientCapabilities": map[string]any{
			"fs":       map[string]bool{"readTextFile": false, "writeTextFile": false},
			"terminal": false,
		},
		"clientInfo": map[string]string{"name": "myco", "title": "myco", "version": ClientVersion},
	}, &init)
	if err != nil {
		return fail(err)
	}

	// The session: continue it if the agent can, else a new one.
	if req.SessionID != "" {
		params := map[string]any{"sessionId": req.SessionID, "cwd": cwd, "mcpServers": []any{}}
		switch caps := init.AgentCapabilities; {
		case caps.LoadSession:
			err = c.call(startCtx, "session/load", params, nil)
		case len(caps.SessionCapabilities.Resume) > 0 && string(caps.SessionCapabilities.Resume) != "null":
			err = c.call(startCtx, "session/resume", params, nil)
		default:
			err = errors.New("the agent cannot continue sessions")
		}
		var re *rpcError
		if err == nil {
			sessionID = req.SessionID
		} else if errors.As(err, &re) && re.Code == codeAuthRequired || ctx.Err() != nil || errors.Is(err, errClosed) {
			return fail(err)
		}
		// Otherwise (session gone, not supported): a new session.
	}
	if sessionID == "" {
		var ns struct {
			SessionID string `json:"sessionId"`
		}
		if err := c.call(startCtx, "session/new", map[string]any{"cwd": cwd, "mcpServers": []any{}}, &ns); err != nil {
			return fail(err)
		}
		if ns.SessionID == "" {
			return fail(errors.New("the agent started no session"))
		}
		sessionID = ns.SessionID
	}
	if a.opts.Model != "" && a.spec.ModelConfig != "" {
		err := c.call(startCtx, "session/set_config_option", map[string]any{"sessionId": sessionID, "configId": a.spec.ModelConfig, "value": a.opts.Model}, nil)
		if err != nil {
			return fail(fmt.Errorf("model %s: %w", a.opts.Model, err))
		}
	}
	cancelStart()
	res := harness.Result{SessionID: sessionID}

	// The prompt turn.
	st.begin()
	type answer struct {
		resp promptResponse
		err  error
	}
	done := make(chan answer, 1)
	go func() {
		var r promptResponse
		err := c.call(context.WithoutCancel(ctx), "session/prompt", map[string]any{
			"sessionId": sessionID,
			"prompt":    []map[string]string{{"type": "text", "text": req.Prompt}},
		}, &r)
		done <- answer{r, err}
	}()
	var ans answer
	select {
	case ans = <-done:
	case <-ctx.Done():
		// Stopped or steered: ask the agent to stop, so it saves the turn.
		_ = c.notify("session/cancel", map[string]string{"sessionId": sessionID})
		select {
		case <-done:
		case <-time.After(cancelTimeout):
		}
		st.finish(nil)
		res.Output = st.output()
		return res, fmt.Errorf("%s: %w", a.spec.Binary, context.Cause(ctx))
	}
	if ans.err != nil {
		return fail(ans.err)
	}
	st.finish(&ans.resp)
	res.Output = st.output()
	if res.Output == "" && len(refused) > 0 {
		// Some agents end the turn when a call is refused: say why.
		res.Output = fmt.Sprintf("Stopped: myco refused %s. This node's access (%s) does not allow it; give the node more access to let it run.",
			strings.Join(refused, ", "), accessName(a.opts.Access))
	}
	switch ans.resp.StopReason {
	case "refusal":
		return res, errors.New("the agent refused to continue")
	case "cancelled":
		return res, errors.New("the agent cancelled the turn")
	case "max_tokens", "max_turn_requests":
		if res.Output == "" {
			return res, fmt.Errorf("the agent stopped (%s) without an answer", ans.resp.StopReason)
		}
	}
	return res, nil
}

// explain turns a failure into one line for the task's sender.
func (a *agent) explain(err error, stderr string) string {
	var re *rpcError
	switch {
	case errors.As(err, &re):
		msg := innermost(re.Message)
		if len(re.Data) > 0 && string(re.Data) != "null" && (re.Code == -32603 || re.Code == -32602) {
			msg += ": " + harness.Tail(string(re.Data), 500) // "Internal error" alone says nothing
		}
		if re.Code == codeAuthRequired {
			msg = fmt.Sprintf("%s needs a login: %s", a.spec.Binary, msg)
			if a.spec.Login != "" {
				msg += " (" + a.spec.Login + ")"
			}
		}
		return msg
	case errors.Is(err, errClosed):
		if t := harness.Tail(stderr, 1500); t != "" {
			return fmt.Sprintf("%s exited: %s", a.spec.Binary, t)
		}
		return fmt.Sprintf("%s exited (is it a version that speaks ACP?)", a.spec.Binary)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Sprintf("%s did not start in time: %s", a.spec.Binary, harness.Tail(stderr, 1000))
	}
	return err.Error()
}

// innermost digs the message out of errors that wrap JSON in JSON (Gemini
// relays the API's error object as a string).
func innermost(s string) string {
	for i := 0; i < 5; i++ {
		var v struct {
			Message string `json:"message"`
			Error   *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(s)), &v) != nil {
			return s
		}
		switch {
		case v.Error != nil && v.Error.Message != "":
			s = v.Error.Message
		case v.Message != "":
			s = v.Message
		default:
			return s
		}
	}
	return s
}

// tailBuffer keeps the last max bytes written.
type tailBuffer struct {
	mu  sync.Mutex
	b   []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > t.max {
		t.b = append([]byte{}, t.b[len(t.b)-t.max:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.b)
}

func accessName(a harness.Access) string {
	if a == harness.AccessDefault {
		return "default: read only"
	}
	return string(a)
}
