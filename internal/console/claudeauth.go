package console

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/provider"
)

// How Claude Code signs in for the agents this computer starts. People set
// Claude up in different ways: a Claude account (OAuth), a provider picked
// in cc-switch (or Magpie and the like, which write the same settings), or
// API variables exported in their shell. Agents started in the background
// see none of the shell's variables, so a terminal where Claude works can
// sit next to agents that fail with "OAuth session expired". Here the App
// says which ways this computer has and lets the person pick one for the
// agents it starts (this computer's machine nodes; started agents inherit
// it). Names and hosts only ever leave: never a key.

// ClaudeAuth is GET /app/local/claude-auth.
type ClaudeAuth struct {
	// Account: Claude Code's own sign-in (ready, signed_out, unknown).
	Account string `json:"account"`
	// CCSwitch: cc-switch's Claude providers (name and host).
	CCSwitch []provider.Provider `json:"cc_switch"`
	// Shell: API settings exported in the login shell (variable names, and
	// the endpoint's host), empty when there are none.
	Shell     []string `json:"shell,omitempty"`
	ShellHost string   `json:"shell_host,omitempty"`
	// Using: what agents started here use now: "" (Claude's own settings:
	// the account, or what cc-switch wrote), "cc-switch:<id>", or "shell".
	Using string `json:"using"`
	// UsingName names it (a cc-switch provider's name, or the host).
	UsingName string `json:"using_name,omitempty"`
}

// shellProviderVars are the shell variables that point Claude Code at an
// API (taken over when the person picks "shell").
var shellProviderVars = []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY", "ANTHROPIC_MODEL",
	"ANTHROPIC_SMALL_FAST_MODEL", "ANTHROPIC_DEFAULT_OPUS_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_HAIKU_MODEL",
	"CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_OAUTH_TOKEN", "API_TIMEOUT_MS"}

// loginShellEnv reads the provider variables a login shell of this user
// exports (tests replace it). Values stay in memory.
var loginShellEnv = func(ctx context.Context) map[string]string {
	if runtime.GOOS == "windows" {
		return nil
	}
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/zsh"
		if runtime.GOOS != "darwin" {
			sh = "/bin/bash"
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, sh, "-ilc", `printf '\n__MYCO_ENV__\n'; env -0`).Output()
	_, rest, ok := bytes.Cut(out, []byte("__MYCO_ENV__\n"))
	if !ok {
		return nil
	}
	want := map[string]bool{}
	for _, k := range shellProviderVars {
		want[k] = true
	}
	env := map[string]string{}
	for _, kv := range bytes.Split(rest, []byte{0}) {
		k, v, ok := strings.Cut(string(kv), "=")
		if ok && want[k] && strings.TrimSpace(v) != "" {
			env[k] = v
		}
	}
	return env
}

// machineNodeProfiles are this computer's machine nodes (one per hub).
func (l *Local) machineNodeProfiles() []config.Profile {
	ps, _ := config.ListProfiles(config.ProfilesDir(l.defaultConfig()))
	var out []config.Profile
	for _, p := range ps {
		if p.Config.Harness == "machine" || p.Config.Runner.Harness == "machine" {
			out = append(out, p)
		}
	}
	return out
}

func (b *Backend) getClaudeAuth(w http.ResponseWriter, r *http.Request) {
	l := b.Local
	out := ClaudeAuth{Account: "missing", CCSwitch: []provider.Provider{}}
	if path := l.findAgent("claude"); path != "" {
		out.Account = l.signedIn(r.Context(), "claude", path)
	}
	if ps, err := provider.List(provider.DirIn(l.Home), "claude"); err == nil && ps != nil {
		out.CCSwitch = ps
	}
	env := loginShellEnv(r.Context())
	for _, k := range shellProviderVars {
		if _, ok := env[k]; ok {
			out.Shell = append(out.Shell, k)
		}
	}
	out.ShellHost = hostOf(env["ANTHROPIC_BASE_URL"])
	if nodes := l.machineNodeProfiles(); len(nodes) > 0 {
		rc := nodes[0].Config.Runner
		if strings.HasPrefix(rc.Provider, provider.RefPrefix) {
			out.Using = rc.Provider
			for _, p := range out.CCSwitch {
				if p.Ref == rc.Provider {
					out.UsingName = p.Name
				}
			}
		} else if rc.EnvFile != "" {
			// Claude's variables only (a Codex endpoint may be there too).
			if vars, err := config.ReadEnvFile(rc.EnvFile); err == nil {
				for _, kv := range vars {
					k, v, _ := strings.Cut(kv, "=")
					if strings.HasPrefix(k, "ANTHROPIC_") || strings.HasPrefix(k, "CLAUDE_CODE_USE_") {
						out.Using = "shell"
						if k == "ANTHROPIC_BASE_URL" {
							out.UsingName = hostOf(v)
						}
					}
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// postClaudeAuth is POST /app/local/claude-auth {use}: "cc-switch:<id>",
// "shell" (take the shell's API variables), or "account" (Claude's own
// settings and sign-in). It applies to every machine node here.
func (b *Backend) postClaudeAuth(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Use string `json:"use"`
	}
	if !decode(w, r, &in) {
		return
	}
	l := b.Local
	nodes := l.machineNodeProfiles()
	if len(nodes) == 0 {
		writeErr(w, &client.Error{Status: http.StatusConflict, Code: "not_configured", Message: "this computer has not joined a workspace yet"})
		return
	}
	var shellVars map[string]string
	var ref string
	switch {
	case in.Use == "account":
	case in.Use == "shell":
		shellVars = loginShellEnv(r.Context())
		if shellVars["ANTHROPIC_BASE_URL"] == "" && shellVars["ANTHROPIC_AUTH_TOKEN"] == "" && shellVars["ANTHROPIC_API_KEY"] == "" &&
			shellVars["CLAUDE_CODE_USE_BEDROCK"] == "" && shellVars["CLAUDE_CODE_USE_VERTEX"] == "" {
			writeErr(w, &client.Error{Status: http.StatusConflict, Code: "not_found", Message: "your shell exports no API settings for Claude"})
			return
		}
	case strings.HasPrefix(in.Use, provider.RefPrefix):
		p, err := provider.Resolve(provider.DirIn(l.Home), in.Use, "claude")
		if err != nil {
			writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: err.Error()})
			return
		}
		ref = p.Ref
	default:
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "use is account, shell or cc-switch:<id>"})
		return
	}
	var errs []string
	for _, p := range nodes {
		cfg := p.Config
		file := cfg.Runner.EnvFile
		if file == "" {
			file = filepath.Join(l.Home, ".myco", "env", cfg.Agent+".env")
		}
		// Only Claude's variables change: whatever else the file holds stays.
		if err := writeProviderEnv(file, cfg.Agent, keepNonClaude(file, shellVars)); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		cfg.Runner.EnvFile, cfg.Runner.Provider = file, ref
		if err := config.Save(p.Path, cfg); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		writeErr(w, errors.New(strings.Join(errs, "; ")))
		return
	}
	b.getClaudeAuth(w, r)
}

// keepNonClaude is vars plus the provider variables in file that are not
// Claude's (a Codex endpoint set for the same node stays).
func keepNonClaude(file string, vars map[string]string) map[string]string {
	out := map[string]string{}
	if lines, err := config.ReadEnvFile(file); err == nil {
		for _, kv := range lines {
			k, v, _ := strings.Cut(kv, "=")
			if strings.HasPrefix(k, "OPENAI_") {
				out[k] = v
			}
		}
	}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out[k] = vars[k]
	}
	return out
}
