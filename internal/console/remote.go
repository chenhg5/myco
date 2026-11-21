package console

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Reaching another computer from this one, to set myco up there or look at
// it: over SSH with the person's own ssh and ssh-agent, or into a WSL
// distribution of this Windows computer. The app never reads or keeps keys: BatchMode makes ssh
// fail instead of asking for a password, and each failure is told in words
// with the command to run in a terminal.

// SSHHost is a host in the person's SSH config, as `ssh -G` resolves it.
type SSHHost struct {
	Name string `json:"name"`
	User string `json:"user,omitempty"`
	Host string `json:"host"`
	Port int    `json:"port,omitempty"`
	Jump string `json:"jump,omitempty"`
	// Connected is the computer it became on the workspace asked about
	// (?hub=), when this App connected it.
	Connected string `json:"connected,omitempty"`
}

// WSLDistro is a WSL distribution on this Windows computer.
type WSLDistro struct {
	Name    string `json:"name"`
	Default bool   `json:"default,omitempty"`
	Running bool   `json:"running,omitempty"`
	Version int    `json:"version,omitempty"`
	// Connected: as for SSHHost.
	Connected string `json:"connected,omitempty"`
}

// Route is how to reach a computer: "ssh" to a config name or user@host
// (Port when given by hand), or "wsl" into a distribution.
type Route struct {
	Via    string `json:"via"`
	Target string `json:"target"`
	Port   int    `json:"port,omitempty"`
}

// RemoteError is a failure reaching or setting up a computer: a code the
// App words, and the command to try in a terminal.
type RemoteError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Command string `json:"command,omitempty"`
}

func (e *RemoteError) Error() string { return e.Message }

// remoteExec runs a command with stdin, calling line for every line it
// prints to stdout; it returns what it printed to stderr and its exit code
// (tests replace it).
type remoteExec func(ctx context.Context, name string, args []string, stdin string, line func(string)) (stderr string, code int, err error)

func runRemoteCommand(ctx context.Context, name string, args []string, stdin string, line func(string)) (string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	cmd.Stdin = strings.NewReader(stdin)
	var errb strings.Builder
	cmd.Stderr = &limitWriter{w: &errb, n: 64 << 10}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return "", -1, err
	}
	if err := cmd.Start(); err != nil {
		return "", -1, err
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		if line != nil {
			line(sc.Text())
		}
	}
	err = cmd.Wait()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code, err = ee.ExitCode(), nil
	}
	return errb.String(), code, err
}

type limitWriter struct {
	w io.Writer
	n int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		q := p
		if len(q) > l.n {
			q = q[:l.n]
		}
		l.n -= len(q)
		_, _ = l.w.Write(q)
	}
	return len(p), nil
}

func (l *Local) remote() remoteExec {
	if l.Remote != nil {
		return l.Remote
	}
	return runRemoteCommand
}

// sshArgs are the options every connection uses: no terminal, no prompts,
// nothing forwarded, give up on a dead network.
func sshArgs(rt Route, cmd string) []string {
	args := []string{"-T", "-a", "-x", "-o", "BatchMode=yes", "-o", "ConnectTimeout=15",
		"-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3", "-o", "ClearAllForwardings=yes", "-o", "RemoteCommand=none"}
	if rt.Port > 0 {
		args = append(args, "-p", strconv.Itoa(rt.Port))
	}
	return append(args, "--", rt.Target, cmd)
}

// command is the program and arguments that run a POSIX shell reading its
// script from stdin on the computer rt reaches.
func (rt Route) command() (string, []string) {
	if rt.Via == "wsl" {
		return "wsl.exe", []string{"-d", rt.Target, "--cd", "~", "--", "sh", "-s"}
	}
	return "ssh", sshArgs(rt, "sh -s")
}

// TerminalCommand is what the person runs to try the same connection.
func (rt Route) TerminalCommand() string {
	if rt.Via == "wsl" {
		return "wsl -d " + rt.Target
	}
	if rt.Port > 0 && rt.Port != 22 {
		return "ssh -p " + strconv.Itoa(rt.Port) + " " + rt.Target
	}
	return "ssh " + rt.Target
}

// runScript runs script on the computer rt reaches. ssh's own failures
// (exit 255 with its message) come back as a RemoteError.
func (l *Local) runScript(ctx context.Context, rt Route, script string, line func(string)) (stderr string, code int, err error) {
	name, args := rt.command()
	stderr, code, err = l.remote()(ctx, name, args, script, line)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || strings.Contains(err.Error(), "executable file not found") {
			if rt.Via == "wsl" {
				return stderr, code, &RemoteError{Code: "wsl_missing", Message: "WSL is not installed on this computer"}
			}
			return stderr, code, &RemoteError{Code: "ssh_missing", Message: "ssh is not installed on this computer"}
		}
		if ctx.Err() != nil {
			return stderr, code, &RemoteError{Code: "timeout", Message: "it took too long", Command: rt.TerminalCommand()}
		}
		return stderr, code, err
	}
	if rt.Via == "ssh" && code == 255 {
		return stderr, code, classifySSH(stderr, rt)
	}
	if rt.Via == "wsl" && code != 0 && strings.Contains(stderr, "WSL_E_DISTRO_NOT_FOUND") {
		return stderr, code, &RemoteError{Code: "wsl_missing", Message: "no WSL distribution named " + rt.Target, Command: "wsl -l -v"}
	}
	return stderr, code, nil
}

// classifySSH words what ssh printed when it could not log in.
func classifySSH(stderr string, rt Route) *RemoteError {
	s := strings.ToLower(stderr)
	e := &RemoteError{Command: rt.TerminalCommand(), Message: lastLines(stderr, 3)}
	switch {
	case strings.Contains(s, "remote host identification has changed") || strings.Contains(s, "host key for") && strings.Contains(s, "has changed"):
		e.Code = "host_changed"
	case strings.Contains(s, "host key verification failed") || strings.Contains(s, "no ") && strings.Contains(s, "host key is known"):
		e.Code = "host_unknown"
	case strings.Contains(s, "permission denied") || strings.Contains(s, "too many authentication failures"):
		e.Code = "auth"
	case strings.Contains(s, "connection refused"):
		e.Code = "refused"
	case strings.Contains(s, "could not resolve hostname") || strings.Contains(s, "name or service not known"):
		e.Code = "unknown_host"
	default:
		e.Code = "unreachable"
	}
	return e
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(strings.ReplaceAll(s, "\r", ""), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// ── SSH config ──

// gitHosts are hosts people keep in their SSH config only for git.
var gitHosts = map[string]bool{"github.com": true, "ssh.github.com": true, "gitlab.com": true, "altssh.gitlab.com": true,
	"bitbucket.org": true, "altssh.bitbucket.org": true, "gitee.com": true, "codeberg.org": true, "ssh.dev.azure.com": true, "vs-ssh.visualstudio.com": true}

// sshConfigNames reads the Host names (without wildcards) from the SSH
// config and the files it includes, in order, each once.
func sshConfigNames(home string) []string {
	var names []string
	seen := map[string]bool{}
	visited := map[string]bool{}
	var read func(path string, depth int)
	read = func(path string, depth int) {
		if depth > 16 || visited[path] {
			return
		}
		visited[path] = true
		b, err := os.ReadFile(path)
		if err != nil {
			return
		}
		for _, raw := range strings.Split(string(b), "\n") {
			key, vals := configLine(raw)
			switch key {
			case "host":
				for _, v := range vals {
					if v == "" || strings.ContainsAny(v, "*?! \t") || seen[v] {
						continue
					}
					seen[v] = true
					names = append(names, v)
					break // the first plain name is how the host is called
				}
			case "include":
				for _, v := range vals {
					if strings.HasPrefix(v, "~/") {
						v = filepath.Join(home, v[2:])
					} else if !filepath.IsAbs(v) {
						v = filepath.Join(home, ".ssh", v)
					}
					matches, _ := filepath.Glob(v)
					sort.Strings(matches)
					for _, m := range matches {
						read(m, depth+1)
					}
				}
			}
		}
	}
	read(filepath.Join(home, ".ssh", "config"), 0)
	return names
}

// configLine splits an ssh_config line into its keyword (lower case) and
// values, honouring "key=value" and double quotes.
func configLine(line string) (string, []string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", nil
	}
	var toks []string
	var cur strings.Builder
	quoted, have := false, false
	for i, r := range line {
		switch {
		case r == '"':
			quoted, have = !quoted, true
		case !quoted && (r == ' ' || r == '\t' || (r == '=' && len(toks) == 0 && i > 0)):
			if have {
				toks = append(toks, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteRune(r)
			have = true
		}
	}
	if have {
		toks = append(toks, cur.String())
	}
	if len(toks) == 0 {
		return "", nil
	}
	return strings.ToLower(toks[0]), toks[1:]
}

// parseSSHG reads what `ssh -G name` prints.
func parseSSHG(name, out string) SSHHost {
	h := SSHHost{Name: name}
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		switch k {
		case "user":
			h.User = v
		case "hostname":
			h.Host = v
		case "port":
			h.Port, _ = strconv.Atoi(v)
		case "proxyjump":
			if v != "none" {
				h.Jump = v
			}
		}
	}
	if h.Host == "" {
		h.Host = name
	}
	return h
}

// SSHHosts lists the hosts in the SSH config, resolved by the system ssh (a
// few at a time; at most 200), leaving out the ones only used for git.
func (l *Local) SSHHosts(ctx context.Context) ([]SSHHost, error) {
	if _, err := l.lookPath("ssh"); err != nil {
		return nil, &RemoteError{Code: "ssh_missing", Message: "ssh is not installed on this computer"}
	}
	names := sshConfigNames(l.Home)
	if len(names) > 200 {
		names = names[:200]
	}
	out := make([]SSHHost, len(names))
	ok := make([]bool, len(names))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, n := range names {
		if gitHosts[strings.ToLower(n)] {
			continue
		}
		wg.Add(1)
		go func(i int, n string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			c, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			var b strings.Builder
			_, code, err := l.remote()(c, "ssh", []string{"-G", "--", n}, "", func(s string) { b.WriteString(s + "\n") })
			if err != nil || code != 0 {
				out[i], ok[i] = SSHHost{Name: n, Host: n}, true
				return
			}
			h := parseSSHG(n, b.String())
			if gitHosts[strings.ToLower(h.Host)] {
				return
			}
			out[i], ok[i] = h, true
		}(i, n)
	}
	wg.Wait()
	hosts := []SSHHost{}
	for i := range out {
		if ok[i] {
			hosts = append(hosts, out[i])
		}
	}
	return hosts, nil
}

// ── WSL ──

var wslLine = regexp.MustCompile(`^(\*?)\s*(\S+)\s+(\S+)\s+(\d+)\s*$`)

// parseWSLList reads `wsl -l -v` (UTF-16 on most Windows versions; its
// header is in the system's language, so it is skipped by shape).
func parseWSLList(raw []byte) []WSLDistro {
	s := strings.ReplaceAll(string(raw), "\x00", "")
	s = strings.TrimPrefix(s, "\ufeff")
	var out []WSLDistro
	for _, line := range strings.Split(s, "\n") {
		m := wslLine.FindStringSubmatch(strings.TrimSpace(strings.TrimRight(line, "\r")))
		if m == nil {
			continue
		}
		v, _ := strconv.Atoi(m[4])
		name := m[2]
		if strings.HasPrefix(strings.ToLower(name), "docker-desktop") {
			continue
		}
		out = append(out, WSLDistro{Name: name, Default: m[1] == "*", Running: strings.EqualFold(m[3], "running"), Version: v})
	}
	return out
}

// WSLDistros lists this computer's WSL distributions (none off Windows).
func (l *Local) WSLDistros(ctx context.Context) ([]WSLDistro, error) {
	if !wslHost {
		return nil, nil
	}
	if _, err := l.lookPath("wsl.exe"); err != nil {
		return nil, nil
	}
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var b strings.Builder
	_, _, err := l.remote()(c, "wsl.exe", []string{"-l", "-v"}, "", func(s string) { b.WriteString(s + "\n") })
	if err != nil {
		return nil, err
	}
	return parseWSLList([]byte(b.String())), nil
}

// ── handlers ──

// getSSHHosts is GET /app/local/ssh-hosts.
func (b *Backend) getSSHHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := b.Local.SSHHosts(r.Context())
	var re *RemoteError
	if errors.As(err, &re) {
		writeJSON(w, http.StatusOK, map[string]any{"hosts": []SSHHost{}, "error": re})
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	conn := b.connectedVia(r.URL.Query().Get("hub"), "ssh")
	for i := range hosts {
		hosts[i].Connected = conn[hosts[i].Name]
	}
	writeJSON(w, http.StatusOK, map[string]any{"hosts": hosts})
}

// connectedVia maps the targets this App connected over via to the
// computers they became on hub.
func (b *Backend) connectedVia(hub, via string) map[string]string {
	out := map[string]string{}
	if hub == "" {
		return out
	}
	reachMu.Lock()
	defer reachMu.Unlock()
	for k, rt := range b.reaches() {
		h, mid, ok := strings.Cut(k, "/")
		if ok && h == hub && rt.Via == via && rt.Port == 0 {
			out[rt.Target] = mid
		}
	}
	return out
}

// getWSL is GET /app/local/wsl: {supported, distros}.
func (b *Backend) getWSL(w http.ResponseWriter, r *http.Request) {
	ds, err := b.Local.WSLDistros(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if ds == nil {
		ds = []WSLDistro{}
	}
	conn := b.connectedVia(r.URL.Query().Get("hub"), "wsl")
	for i := range ds {
		ds[i].Connected = conn[ds[i].Name]
	}
	writeJSON(w, http.StatusOK, map[string]any{"supported": wslHost, "distros": ds})
}
