package console

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/client"
)

// LocalAgent is an agent CLI on this computer: whether it is installed and
// signed in, and the commands that install it or sign in
// (GET /app/local/agents; docs/design/client-plan.md 5.1).
type LocalAgent struct {
	CLI  string `json:"cli"`  // claude | codex | gemini | opencode
	Name string `json:"name"` // Claude Code, Codex, ...
	// State: missing (not installed), signed_out, ready, or unknown
	// (installed; whether it is signed in cannot be told without spending
	// quota). Unknown never blocks: the app offers to sign in, nothing more.
	State   string `json:"state"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	// Install is the official way to install it; InstallMirror the same
	// through npm's China mirror, for when the official download is slow.
	// Login signs in. Each is a command the app opens a terminal with.
	Install       string `json:"install"`
	InstallMirror string `json:"install_mirror,omitempty"`
	Login         string `json:"login"`
	// Needs names a program the install command needs (npm), when it does.
	Needs string `json:"needs,omitempty"`
}

const npmMirror = " --registry=https://registry.npmmirror.com"

type agentCLI struct {
	cli, name, binary        string
	install, installWin, npm string
	login                    string
}

// agentCLIs are the agent CLIs the app helps with, in the order it lists them.
var agentCLIs = []agentCLI{
	{cli: "claude", name: "Claude Code", binary: "claude",
		install: "curl -fsSL https://claude.ai/install.sh | bash", installWin: "irm https://claude.ai/install.ps1 | iex",
		npm: "npm install -g @anthropic-ai/claude-code", login: "claude"},
	{cli: "codex", name: "Codex", binary: "codex", npm: "npm install -g @openai/codex", login: "codex login"},
	{cli: "gemini", name: "Gemini CLI", binary: "gemini", npm: "npm install -g @google/gemini-cli", login: "gemini"},
	{cli: "opencode", name: "opencode", binary: "opencode",
		install: "curl -fsSL https://opencode.ai/install | bash", npm: "npm install -g opencode-ai", login: "opencode auth login"},
	{cli: "qoder", name: "Qoder CLI", binary: "qodercli", npm: "npm install -g @qoder-ai/qodercli", login: "qodercli"},
	{cli: "qwen", name: "Qwen Code", binary: "qwen", npm: "npm install -g @qwen-code/qwen-code", login: "qwen"},
	{cli: "kimi", name: "Kimi Code", binary: "kimi", npm: "npm install -g @moonshot-ai/kimi-code", login: "kimi"},
	{cli: "copilot", name: "GitHub Copilot", binary: "copilot", npm: "npm install -g @github/copilot", login: "copilot"},
	{cli: "auggie", name: "Auggie", binary: "auggie", npm: "npm install -g @augmentcode/auggie", login: "auggie"},
	{cli: "codebuddy", name: "CodeBuddy Code", binary: "codebuddy", npm: "npm install -g @tencent-ai/codebuddy-code", login: "codebuddy"},
	{cli: "kilo", name: "Kilo Code", binary: "kilo", npm: "npm install -g @kilocode/cli", login: "kilo"},
	{cli: "cline", name: "Cline", binary: "cline", npm: "npm install -g cline", login: "cline"},
	{cli: "grok", name: "Grok", binary: "grok", npm: "npm install -g @xai-official/grok", login: "grok"},
	{cli: "goose", name: "goose", binary: "goose", install: "curl -fsSL https://github.com/block/goose/releases/download/stable/download_cli.sh | bash", login: "goose configure"},
}

func findAgentCLI(cli string) (agentCLI, bool) {
	for _, a := range agentCLIs {
		if a.cli == cli {
			return a, true
		}
	}
	return agentCLI{}, false
}

// commands are the install and sign-in commands for this computer.
func (a agentCLI) commands() (install, mirror, needs string) {
	install = a.install
	if runtime.GOOS == "windows" {
		install = a.installWin
	}
	npm := a.npm
	if npm != "" && runtime.GOOS != "windows" {
		// Into this user's ~/.local (bin on the App's search path): npm's
		// global folder often belongs to whoever installed Homebrew or Node,
		// and another user of this Mac gets "EACCES: permission denied".
		npm = strings.Replace(npm, "npm install -g ", `npm install -g --prefix "$HOME/.local" `, 1)
	}
	if install == "" {
		install, needs = npm, "npm"
	}
	if npm != "" {
		mirror = npm + npmMirror
	}
	return install, mirror, needs
}

// LocalAgents reports every agent CLI the app helps with.
func (l *Local) LocalAgents(ctx context.Context) []LocalAgent {
	out := make([]LocalAgent, len(agentCLIs))
	var wg sync.WaitGroup
	for i, a := range agentCLIs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = l.localAgent(ctx, a)
		}()
	}
	wg.Wait()
	return out
}

func (l *Local) localAgent(ctx context.Context, a agentCLI) LocalAgent {
	install, mirror, needs := a.commands()
	la := LocalAgent{CLI: a.cli, Name: a.name, State: "missing", Install: install, InstallMirror: mirror, Login: a.loginHere(headless()), Needs: needs}
	la.Path = l.findAgent(a.binary)
	if la.Path == "" {
		return la
	}
	la.Version = l.version(ctx, la.Path)
	la.State = l.signedIn(ctx, a.cli, la.Path)
	return la
}

// loginHere is the sign-in command for this computer: where no browser can
// open (a server, over SSH), Codex signs in with a code shown in the
// terminal instead.
func (a agentCLI) loginHere(headless bool) string {
	if headless && a.cli == "codex" {
		return "codex login --device-auth"
	}
	return a.login
}

// headless: no browser can open here (an SSH session, or Linux without a
// display, as on a server).
func headless() bool {
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		return true
	}
	return runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
}

// version is what the CLI at path says its version is, remembered while
// the file stays the same (size and modification time): running --version
// takes a second or more for some, and the list is asked for often.
func (l *Local) version(ctx context.Context, path string) string {
	key := ""
	if real, err := filepath.EvalSymlinks(path); err == nil {
		if fi, err := os.Stat(real); err == nil {
			key = fmt.Sprintf("%s|%d|%d", real, fi.Size(), fi.ModTime().UnixNano())
		}
	}
	if key != "" {
		if v, ok := l.versions.Load(key); ok {
			return v.(string)
		}
	}
	vctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := l.runAgent(vctx, path, "--version")
	if err != nil {
		return ""
	}
	v := versionLine(out)
	if key != "" && v != "" {
		l.versions.Store(key, v)
	}
	return v
}

// findAgent looks for an agent CLI on PATH, in the login shell's PATH and
// in the places installers put them.
func (l *Local) findAgent(binary string) string {
	var dirs []string
	if sp := l.loginShellPath(); sp != "" {
		dirs = append(dirs, filepath.SplitList(sp)...)
	}
	dirs = append(dirs, l.binDirs()...)
	dirs = append(dirs, l.agentDirs()...)
	if runtime.GOOS == "windows" {
		if ad := os.Getenv("APPDATA"); ad != "" {
			dirs = append(dirs, filepath.Join(ad, "npm"))
		}
		for _, ext := range []string{".cmd", ".exe"} {
			if p := l.find(binary, l.inExt(dirs, binary, ext)...); p != "" {
				return p
			}
		}
		return ""
	}
	return l.find(binary, l.in(dirs, binary)...)
}

func (l *Local) inExt(dirs []string, name, ext string) []string {
	var out []string
	for _, d := range dirs {
		out = append(out, filepath.Join(d, name+ext))
	}
	return out
}

func (l *Local) runAgent(ctx context.Context, bin string, args ...string) ([]byte, error) {
	run := l.Run
	if run == nil {
		run = runCommand
	}
	return run(ctx, l.env(), bin, args...)
}

// versionLine is the version a CLI printed: its last non-empty line, short.
func versionLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	v := strings.TrimSpace(lines[len(lines)-1])
	if len(v) > 60 {
		v = v[:60]
	}
	return v
}

// signedIn tells whether an installed CLI is signed in, without spending
// quota: from its own status command or the files it keeps its sign-in in.
// When it cannot tell it says unknown, never signed_out.
func (l *Local) signedIn(ctx context.Context, cli, path string) string {
	switch cli {
	case "claude":
		if l.claudeSignedIn(ctx) {
			return "ready"
		}
		return "signed_out"
	case "codex":
		sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		out, err := l.runAgent(sctx, path, "login", "status")
		if err == nil {
			return "ready"
		}
		// Another provider set in its config (cc-switch, an API key) works
		// without a ChatGPT sign-in.
		if b, rerr := os.ReadFile(filepath.Join(l.codexHome(), "config.toml")); rerr == nil && bytes.Contains(b, []byte("model_provider")) {
			return "unknown"
		}
		if bytes.Contains(bytes.ToLower(out), []byte("not logged in")) {
			return "signed_out"
		}
		return "unknown"
	case "gemini":
		dir := filepath.Join(l.Home, ".gemini")
		if fileExists(filepath.Join(dir, "oauth_creds.json")) || fileHas(filepath.Join(dir, "settings.json"), "selectedAuthType") {
			return "ready"
		}
		return "unknown" // an API key in the shell's environment is invisible here
	case "opencode":
		if fileExists(filepath.Join(l.Home, ".local", "share", "opencode", "auth.json")) {
			return "ready"
		}
		return "unknown"
	}
	return "unknown"
}

// claudeSignedIn: Claude Code keeps its sign-in in the macOS keychain or a
// credentials file, and the account in .claude.json; a provider set in its
// settings (an API key, cc-switch) works without one.
func (l *Local) claudeSignedIn(ctx context.Context) bool {
	for _, d := range l.claudeDirs() {
		if fileExists(filepath.Join(d, ".credentials.json")) {
			return true
		}
		var s struct {
			Env          map[string]string `json:"env"`
			APIKeyHelper string            `json:"apiKeyHelper"`
		}
		if b, err := os.ReadFile(filepath.Join(d, "settings.json")); err == nil && json.Unmarshal(b, &s) == nil {
			if s.APIKeyHelper != "" || s.Env["ANTHROPIC_API_KEY"] != "" || s.Env["ANTHROPIC_AUTH_TOKEN"] != "" {
				return true
			}
		}
	}
	var acct struct {
		OAuthAccount json.RawMessage `json:"oauthAccount"`
	}
	if b, err := os.ReadFile(filepath.Join(l.Home, ".claude.json")); err == nil && json.Unmarshal(b, &acct) == nil &&
		len(acct.OAuthAccount) > 0 && string(acct.OAuthAccount) != "null" {
		return true
	}
	if runtime.GOOS == "darwin" {
		// Reads the item's attributes only, never the secret (no -w).
		kctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		run := l.Run
		if run == nil {
			run = runCommand
		}
		if _, err := run(kctx, os.Environ(), "/usr/bin/security", "find-generic-password", "-s", "Claude Code-credentials"); err == nil {
			return true
		}
	}
	return false
}

func (l *Local) codexHome() string {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return h
	}
	return filepath.Join(l.Home, ".codex")
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func fileHas(p, s string) bool {
	b, err := os.ReadFile(p)
	return err == nil && bytes.Contains(b, []byte(s))
}

// getLocalAgents is GET /app/local/agents.
func (b *Backend) getLocalAgents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"agents": b.Local.LocalAgents(r.Context())})
}

// postAgentTerminal is POST /app/local/agents/{cli}/terminal
// {"action": "install"|"install_mirror"|"login"}: opens a terminal window
// running that command, so the person sees what it does and answers what it
// asks (sign-in opens a browser). The command is always one of ours, never
// from the request. {opened: false} means no terminal could be opened: the
// app shows the command to copy instead.
func (b *Backend) postAgentTerminal(w http.ResponseWriter, r *http.Request) {
	a, ok := findAgentCLI(r.PathValue("cli"))
	if !ok {
		writeErr(w, &client.Error{Status: http.StatusNotFound, Code: "not_found", Message: "no such agent CLI"})
		return
	}
	var in struct {
		Action string `json:"action"`
	}
	if !decode(w, r, &in) {
		return
	}
	install, mirror, _ := a.commands()
	cmd := map[string]string{"install": install, "install_mirror": mirror, "login": a.login}[in.Action]
	if cmd == "" {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "action must be install, install_mirror or login"})
		return
	}
	open := b.Local.OpenTerminal
	if open == nil {
		open = openTerminal
	}
	err := open(cmd, b.Local.env())
	writeJSON(w, http.StatusOK, map[string]any{"opened": err == nil, "command": cmd})
}
