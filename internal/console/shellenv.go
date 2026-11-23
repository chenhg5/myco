package console

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LoginShellEnv is the environment this user's login shell sets up: what a
// terminal has and an App started from the Dock or Finder does not (keys
// and base URLs exported in ~/.zshrc, CODEX_HOME, proxies). nil on Windows,
// where an App gets the user's variables anyway, or when the shell does not
// answer in time.
func LoginShellEnv() []string {
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// A marker, since rc files may print their own lines; env -0 keeps
	// values with newlines whole.
	cmd := exec.CommandContext(ctx, sh, "-ilc", `printf '\n__MYCO_ENV__\n'; command env -0`)
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil
	}
	return parseShellEnv(out)
}

// parseShellEnv reads what follows the marker: NUL-separated KEY=VALUE.
func parseShellEnv(out []byte) []string {
	i := bytes.LastIndex(out, []byte("\n__MYCO_ENV__\n"))
	if i < 0 {
		return nil
	}
	var env []string
	for _, kv := range strings.Split(string(out[i+len("\n__MYCO_ENV__\n"):]), "\x00") {
		if k, _, ok := strings.Cut(kv, "="); ok && k != "" && !strings.ContainsAny(k, " \n") {
			env = append(env, kv)
		}
	}
	return env
}

// shellEnvTTL is how long a reading of the login shell's environment is
// used: a key exported in ~/.zshrc a moment ago is picked up soon after.
const shellEnvTTL = 2 * time.Minute

type shellEnvCache struct {
	mu  sync.Mutex
	val []string
	at  time.Time
}

// shellEnv is the login shell's environment through ShellEnv (nil asks
// nothing, as in tests), asked at most every two minutes.
func (l *Local) shellEnv() []string {
	if l.ShellEnv == nil {
		return nil
	}
	c := &l.shellEnvCache
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.at.IsZero() || time.Since(c.at) > shellEnvTTL {
		c.val, c.at = l.ShellEnv(), time.Now()
	}
	return c.val
}

// userEnv is this App's environment with the login shell's over it: what
// the person's terminal would have.
func (l *Local) userEnv() []string {
	shell := l.shellEnv()
	if len(shell) == 0 {
		return os.Environ()
	}
	have := map[string]bool{}
	for _, kv := range shell {
		k, _, _ := strings.Cut(kv, "=")
		have[k] = true
	}
	out := append([]string{}, shell...)
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); !have[k] {
			out = append(out, kv)
		}
	}
	return out
}

// userGetenv looks a variable up in userEnv.
func (l *Local) userGetenv(key string) string {
	if v := os.Getenv(key); v != "" && len(l.shellEnv()) == 0 {
		return v
	}
	for _, kv := range l.userEnv() {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v
		}
	}
	return ""
}
