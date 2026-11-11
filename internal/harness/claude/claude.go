// Package claude adapts Claude Code's headless mode (`claude -p`).
package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chenhg5/myco/internal/harness"
)

// Name is the registry key.
const Name = "claude"

// keepEnv lists CLAUDE* variables that are user configuration (where
// sessions live, how to authenticate) and must survive env cleaning.
var keepEnv = map[string]bool{
	"CLAUDE_CONFIG_DIR": true, "CLAUDE_CODE_OAUTH_TOKEN": true,
	"CLAUDE_CODE_USE_BEDROCK": true, "CLAUDE_CODE_USE_VERTEX": true, "CLAUDE_CODE_USE_FOUNDRY": true,
}

// inheritedSessionEnv returns the variables to drop when the runner itself
// runs inside a Claude Code session (CLAUDECODE is set): the parent's
// runtime/session state would otherwise leak into the headless claude, which
// can then attach to the *parent's* session instead of the agent's.
func inheritedSessionEnv(environ []string) []string {
	nested := false
	var names []string
	for _, kv := range environ {
		k, _, _ := strings.Cut(kv, "=")
		if k == "CLAUDECODE" {
			nested = true
		}
		if strings.HasPrefix(k, "CLAUDE") && !keepEnv[k] {
			names = append(names, k)
		}
	}
	if !nested {
		return nil
	}
	return names
}

type claude struct {
	opts harness.Options
	*harness.TranscriptFinder
}

// New is the harness.Factory for Claude Code.
func New(opts harness.Options) (harness.Harness, error) {
	if opts.Binary == "" {
		opts.Binary = "claude"
	}
	return &claude{opts: opts, TranscriptFinder: &harness.TranscriptFinder{Patterns: transcriptPatterns}}, nil
}

// transcriptPatterns: ~/.claude/projects/<project>/<session>.jsonl
func transcriptPatterns(id string) []string {
	return []string{filepath.Join(harness.HomeDir("CLAUDE_CONFIG_DIR", ".claude"), "projects", "*", id+".jsonl")}
}

func (c *claude) Name() string { return Name }

// CanFork implements harness.Forker (claude --resume <id> --fork-session).
func (c *claude) CanFork() {}

// AppliesProvider implements harness.ProviderAware (via --settings).
func (c *claude) AppliesProvider() {}

// accessArgs maps access levels to Claude Code permission flags.
var accessArgs = map[harness.Access][]string{
	harness.AccessReadonly:  {"--permission-mode", "default", "--disallowedTools", "Edit,Write,NotebookEdit,Bash"},
	harness.AccessWorkspace: {"--permission-mode", "acceptEdits"},
	harness.AccessFull:      {"--permission-mode", "bypassPermissions"},
}

// Check implements harness.Checker.
func (c *claude) Check(ctx context.Context) harness.Check {
	return harness.CheckBinary(ctx, c.opts.Binary, "--version")
}

// configDir is Claude Code's config directory for a run with env.
func configDir(env []string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(env[i], "CLAUDE_CONFIG_DIR="); ok && v != "" {
			return v
		}
	}
	return harness.HomeDir("CLAUDE_CONFIG_DIR", ".claude")
}

// providerKey reports whether an env name configures the model provider.
func providerKey(k string) bool {
	if strings.HasPrefix(k, "ANTHROPIC_") || strings.HasPrefix(k, "CLAUDE_CODE_USE_") {
		return true
	}
	switch k {
	case "CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CODE_SUBAGENT_MODEL", "API_TIMEOUT_MS", "AWS_REGION", "AWS_PROFILE", "CLOUD_ML_REGION":
		return true
	}
	return false
}

// providerSettingsFile writes the provider as a private settings file for
// --settings, which overrides ~/.claude/settings.json key by key. Provider
// keys the user's settings set but this provider does not (e.g. another
// provider's ANTHROPIC_MODEL left by cc-switch) are blanked, which Claude
// Code treats as unset. A file keeps keys out of the process list.
func providerSettingsFile(p *harness.ProviderSettings, dir string) (string, error) {
	env := map[string]string{}
	if b, err := os.ReadFile(filepath.Join(dir, "settings.json")); err == nil {
		var user struct {
			Env map[string]any `json:"env"`
		}
		if json.Unmarshal(b, &user) == nil {
			for k := range user.Env {
				if providerKey(k) {
					env[k] = ""
				}
			}
		}
	}
	for k, v := range p.Env {
		env[k] = v
	}
	s := map[string]any{"env": env}
	if p.Model != "" {
		s["model"] = p.Model
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "myco-provider-*.json") // mode 0600
	if err != nil {
		return "", fmt.Errorf("provider settings: %w", err)
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), f.Close()
}

// event is the subset of `--output-format stream-json` events we use.
type event struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	SessionID string `json:"session_id"`
	Result    string `json:"result"`
	IsError   bool   `json:"is_error"`
	Message   struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

func (c *claude) Run(ctx context.Context, req harness.Request) (harness.Result, error) {
	// Prompt goes through stdin (no argv limits, no flag injection).
	args := []string{"-p", "--output-format", "stream-json", "--verbose"}
	if req.SessionID != "" {
		args = append(args, "--resume", req.SessionID)
		if req.Fork {
			args = append(args, "--fork-session")
		}
	}
	if c.opts.Model != "" {
		args = append(args, "--model", c.opts.Model)
	}
	args = append(args, accessArgs[c.opts.Access]...)
	// myco is the agent's own messaging channel (reply, hold, delegate):
	// headless runs cannot ask for approval, so allow it explicitly. Other
	// commands stay governed by the access level.
	args = append(args, "--allowedTools", "Bash(myco:*)")
	for _, d := range req.ExtraDirs {
		args = append(args, "--add-dir", d)
	}
	args = append(args, c.opts.Args...)
	if req.Provider != nil {
		file, err := providerSettingsFile(req.Provider, configDir(req.Env))
		if err != nil {
			return harness.Result{}, err
		}
		defer os.Remove(file)
		args = append(args, "--settings", file)
	}
	proc := harness.Process{
		Binary: c.opts.Binary, Args: args, Stdin: req.Prompt, Workdir: req.Workdir,
		Env: req.Env, UnsetEnv: inheritedSessionEnv(os.Environ()),
	}
	if req.OnEvent != nil {
		proc.OnStdoutLine = func(line []byte) {
			for _, ev := range lineEvents(line) {
				req.OnEvent(ev)
			}
		}
	}
	out, runErr := harness.Exec(ctx, proc)
	traceEvents("claude", out.Stdout)
	res, final, ok := parseStream(out.Stdout)
	if req.Fork && res.SessionID == req.SessionID && runErr == nil {
		return res, fmt.Errorf("claude did not create a copy of session %s", req.SessionID)
	}
	if parent := os.Getenv("CLAUDE_CODE_SESSION_ID"); parent != "" && res.SessionID == parent && req.SessionID != parent {
		// Never let the agent adopt the session the runner itself runs in.
		return harness.Result{}, fmt.Errorf("claude attached to the runner's own session %s; run the runner outside Claude Code or report this", parent)
	}
	switch {
	case ok && final.IsError:
		msg := final.Result
		if msg == "" {
			msg = final.Subtype
		}
		return res, fmt.Errorf("claude reported an error: %s", msg)
	case runErr != nil:
		return res, runErr
	case !ok && res.Output == "":
		return res, errors.New("claude: no result event in output")
	}
	return res, nil
}

// parseStream extracts the session id, the final answer (result event, or
// the last assistant text as a fallback) and the result event itself.
func parseStream(stdout string) (harness.Result, event, bool) {
	var (
		res   harness.Result
		final event
		found bool
	)
	sc := bufio.NewScanner(strings.NewReader(stdout))
	sc.Buffer(make([]byte, 1<<20), 32<<20)
	for sc.Scan() {
		var ev event
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if ev.SessionID != "" {
			res.SessionID = ev.SessionID
		}
		switch ev.Type {
		case "assistant":
			for _, c := range ev.Message.Content {
				if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
					res.Output = c.Text
				}
			}
		case "result":
			final, found = ev, true
			if ev.Result != "" {
				res.Output = ev.Result
			}
		}
	}
	return res, final, found
}

// traceEvents appends raw harness output to $MYCO_TRACE_DIR/<name>.jsonl
// for debugging, when that variable is set.
func traceEvents(name, stdout string) {
	dir := os.Getenv("MYCO_TRACE_DIR")
	if dir == "" || stdout == "" {
		return
	}
	_ = os.MkdirAll(dir, 0o700)
	f, err := os.OpenFile(filepath.Join(dir, name+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(stdout)
}
