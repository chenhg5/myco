// Package sshcap manages private, immutable OpenSSH profiles and a small fixed
// Linux administration vocabulary. It never accepts passwords, private keys,
// arbitrary remote commands or an instruction from shared page data.
package sshcap

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const OutputLimit = 256 << 10

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string        { return e.Message }
func fail(code, message string) error { return &Error{code, message} }

type Profile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Destination  string `json:"destination"`
	Host         string `json:"host"`
	User         string `json:"user"`
	Port         int    `json:"port"`
	Binding      string `json:"binding"`
	AgentEnabled bool   `json:"agent_enabled"`
	AgentHub     string `json:"agent_hub,omitempty"`
}
type Create struct {
	Name        string `json:"name"`
	Destination string `json:"destination"`
	User        string `json:"user,omitempty"`
	Port        int    `json:"port,omitempty"`
}
type stored struct {
	Profile
	Input  Create `json:"input"`
	Config string `json:"config"`
	Trust  string `json:"trust"`
}
type Operation struct {
	Action string `json:"action"`
	Name   string `json:"name,omitempty"`
	Lines  int    `json:"lines,omitempty"`
}
type Result struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	Truncated  bool   `json:"truncated"`
	DurationMS int64  `json:"duration_ms"`
}

type active struct {
	profile string
	agent   bool
	cancel  context.CancelFunc
}
type Manager struct {
	Home   string
	Binary string
	// Config is for isolated embedding/tests. The page cannot supply it.
	Config  string
	mu      sync.Mutex
	running map[string]active
}

func (m *Manager) root() string { return filepath.Join(m.Home, ".myco", "ssh") }

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{10,80}$`)

func (m *Manager) file(id string) (string, error) {
	if !identifier.MatchString(id) {
		return "", fail("invalid_argument", "invalid SSH profile id")
	}
	return filepath.Join(m.root(), id+".json"), nil
}
func (m *Manager) load(id string) (stored, error) {
	var p stored
	f, err := m.file(id)
	if err != nil {
		return p, err
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return p, fail("not_found", "SSH profile does not exist on this computer")
	}
	if json.Unmarshal(b, &p) != nil {
		return p, fail("invalid_config", "SSH profile cannot be read")
	}
	return p, nil
}
func (m *Manager) save(p stored) error {
	if err := os.MkdirAll(m.root(), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(m.root(), ".profile-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	b, _ := json.Marshal(p)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	path, _ := m.file(p.ID)
	return os.Rename(f.Name(), path)
}
func (m *Manager) List() ([]Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Profile{}
	files, err := filepath.Glob(filepath.Join(m.root(), "*.json"))
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		p, err := m.load(strings.TrimSuffix(filepath.Base(f), ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, p.Profile)
	}
	return out, nil
}
func (m *Manager) Get(id string) (Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, e := m.load(id)
	return p.Profile, e
}

var destination = regexp.MustCompile(`^[A-Za-z0-9_.:@\[\]-]{1,200}$`)
var userName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,80}$`)

func (m *Manager) Create(ctx context.Context, in Create) (Profile, error) {
	if !destination.MatchString(in.Destination) || strings.HasPrefix(in.Destination, "-") || strings.Count(in.Destination, "@") > 1 || in.Port < 0 || in.Port > 65535 || in.User != "" && !userName.MatchString(in.User) || len(in.Name) > 100 || strings.ContainsAny(in.Name, "\x00\r\n") {
		return Profile{}, fail("invalid_argument", "enter an SSH alias or user@host, a valid port and name; credentials and command options are not accepted")
	}
	p, err := m.resolve(ctx, in)
	if err != nil {
		return Profile{}, err
	}
	p.ID = "ssh_" + rand.Text()
	p.Destination = in.Destination
	p.Name = strings.TrimSpace(in.Name)
	if p.Name == "" {
		p.Name = in.Destination
	}
	p.Input = in
	m.mu.Lock()
	err = m.save(p)
	m.mu.Unlock()
	if err != nil {
		return Profile{}, err
	}
	return p.Profile, nil
}
func (m *Manager) SetAgent(ctx context.Context, id string, on bool) (Profile, error) {
	return m.SetAgentFor(ctx, id, on, "")
}

func (m *Manager) SetAgentFor(ctx context.Context, id string, on bool, hub string) (Profile, error) {
	// An enabled profile has actually authenticated against its frozen host trust.
	if on {
		r, err := m.Run(ctx, id, "", Operation{Action: "probe"}, false)
		if err != nil {
			return Profile{}, err
		}
		if r.ExitCode != 0 || strings.TrimSpace(r.Stdout) != "Linux" {
			return Profile{}, fail("unsupported", "the SSH target must be Linux")
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, err := m.load(id)
	if err != nil {
		return Profile{}, err
	}
	if p.AgentHub != hub {
		m.cancelLocked(id, true)
	}
	p.AgentEnabled = on
	p.AgentHub = hub
	if !on {
		p.AgentHub = ""
	}
	if !on {
		m.cancelLocked(id, true)
	}
	if err := m.save(p); err != nil {
		return Profile{}, err
	}
	return p.Profile, nil
}
func (m *Manager) cancelLocked(id string, agentsOnly bool) {
	for _, a := range m.running {
		if a.profile == id && (!agentsOnly || a.agent) {
			a.cancel()
		}
	}
}
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, err := m.load(id)
	if err != nil {
		return err
	}
	m.cancelLocked(id, false)
	f, _ := m.file(p.ID)
	return os.Remove(f)
}
func (m *Manager) command(ctx context.Context, args ...string) *exec.Cmd {
	binary := m.Binary
	if binary == "" {
		binary = "ssh"
	}
	c := exec.CommandContext(ctx, binary, args...)
	c.WaitDelay = time.Second
	c.Env = []string{}
	for _, k := range []string{"PATH", "HOME", "USER", "LOGNAME", "LANG", "LC_ALL", "SSH_AUTH_SOCK", "SYSTEMROOT", "USERPROFILE", "TEMP", "TMP"} {
		if v, ok := os.LookupEnv(k); ok {
			c.Env = append(c.Env, k+"="+v)
		}
	}
	return c
}
func quoteConfig(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}
func (m *Manager) expand(s string) string {
	if s == "~" {
		return m.Home
	}
	if strings.HasPrefix(s, "~/") {
		return filepath.Join(m.Home, s[2:])
	}
	return s
}
func (m *Manager) resolve(ctx context.Context, in Create) (stored, error) {
	args := []string{"-G", "-o", "PermitLocalCommand=no", "-o", "ForwardAgent=no", "-o", "ClearAllForwardings=yes"}
	if m.Config != "" {
		args = append(args, "-F", m.Config)
	}
	if in.User != "" {
		args = append(args, "-l", in.User)
	}
	if in.Port != 0 {
		args = append(args, "-p", strconv.Itoa(in.Port))
	}
	args = append(args, "--", in.Destination)
	call, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var out limited
	out.max = 256 << 10
	c := m.command(call, args...)
	c.Stdout = &out
	c.Stderr = io.Discard
	if err := c.Run(); err != nil || out.cut {
		return stored{}, fail("invalid_config", "OpenSSH could not resolve this profile; check the SSH configuration locally")
	}
	p := stored{}
	var config, trust strings.Builder
	for _, line := range strings.Split(out.buf.String(), "\n") {
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		// A flattened ssh -G profile cannot safely carry a second host's
		// configuration. Refuse ProxyJump until every hop can be frozen too.
		if key == "proxyjump" && value != "none" {
			return stored{}, fail("unsupported", "ProxyJump profiles are not supported yet; use a direct SSH address reachable from this computer (for example over a VPN)")
		}
		switch key {
		case "hostname":
			p.Host = value
		case "user":
			p.User = value
		case "port":
			p.Port, _ = strconv.Atoi(value)
		}
		switch key {
		case "userknownhostsfile", "globalknownhostsfile":
			// ssh -G does not quote paths. A single existing path containing spaces
			// is unambiguous; otherwise use the ordinary whitespace-separated list.
			paths := strings.Fields(value)
			if _, err := os.Stat(m.expand(value)); err == nil {
				paths = []string{value}
			}
			for _, path := range paths {
				if path == "none" {
					continue
				}
				b, err := os.ReadFile(m.expand(path))
				if err != nil {
					if os.IsNotExist(err) {
						continue
					}
					return stored{}, fail("invalid_config", "a known_hosts file cannot be read")
				}
				if len(b) > 4<<20 {
					return stored{}, fail("too_large", "known_hosts exceeds the safety limit")
				}
				trust.Write(b)
				trust.WriteByte('\n')
			}
			continue
		case "knownhostscommand", "remotecommand", "localcommand", "controlpath", "controlmaster", "controlpersist", "localforward", "remoteforward", "dynamicforward", "sendenv", "setenv":
			continue
		case "identityfile", "certificatefile", "identityagent", "revokedhostkeys", "xauthlocation":
			value = quoteConfig(value)
		}
		config.WriteString(key + " " + value + "\n")
	}
	if p.Host == "" || p.User == "" || p.Port < 1 {
		return p, fail("invalid_config", "OpenSSH did not resolve a target")
	}
	if trust.Len() == 0 {
		return p, fail("host_key_not_trusted", "no existing known_hosts trust is available; verify this server with OpenSSH in a terminal first")
	}
	p.Config, p.Trust = config.String(), trust.String()
	sum := sha256.Sum256([]byte(p.Config + "\x00" + p.Trust))
	p.Binding = hex.EncodeToString(sum[:])
	return p, nil
}

type limited struct {
	buf bytes.Buffer
	max int
	cut bool
}

func (l *limited) Write(p []byte) (int, error) {
	n := len(p)
	left := l.max - l.buf.Len()
	if len(p) > left {
		p = p[:max(left, 0)]
		l.cut = true
	}
	_, _ = l.buf.Write(p)
	return n, nil
}

func (m *Manager) Run(ctx context.Context, id, binding string, op Operation, agent bool) (Result, error) {
	return m.RunFor(ctx, id, binding, op, agent, "")
}
func (m *Manager) RunFor(ctx context.Context, id, binding string, op Operation, agent bool, hub string) (Result, error) {
	command, mutates, err := Command(op)
	if err != nil {
		return Result{}, err
	}
	m.mu.Lock()
	p, err := m.load(id)
	if err == nil && agent && (!p.AgentEnabled || p.AgentHub != hub) {
		err = fail("permission_denied", "agent access to this SSH profile is disabled")
	}
	if err != nil {
		m.mu.Unlock()
		return Result{}, err
	}
	if binding != "" && binding != p.Binding {
		m.mu.Unlock()
		return Result{}, fail("target_changed", "the SSH profile is not the approved target")
	}
	call, cancel := context.WithTimeout(ctx, 30*time.Second)
	runID := rand.Text()
	if m.running == nil {
		m.running = map[string]active{}
	}
	m.running[runID] = active{id, agent, cancel}
	m.mu.Unlock()
	defer func() { cancel(); m.mu.Lock(); delete(m.running, runID); m.mu.Unlock() }()
	current, err := m.resolve(call, p.Input)
	if err != nil {
		return Result{}, err
	}
	if current.Binding != p.Binding {
		return Result{}, fail("target_changed", "SSH configuration or host trust changed; recreate the profile and approve the new target")
	}
	dir, err := os.MkdirTemp("", "myco-ssh-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(dir)
	cfg, known := filepath.Join(dir, "config"), filepath.Join(dir, "known_hosts")
	if err := os.WriteFile(cfg, []byte(p.Config), 0600); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(known, []byte(p.Trust), 0600); err != nil {
		return Result{}, err
	}
	null := "/dev/null"
	if runtime.GOOS == "windows" {
		null = "NUL"
	}
	args := []string{"-F", cfg, "-l", p.User, "-p", strconv.Itoa(p.Port), "-n", "-T", "-o", "BatchMode=yes", "-o", "LogLevel=ERROR", "-o", "StrictHostKeyChecking=yes", "-o", "UpdateHostKeys=no", "-o", "VerifyHostKeyDNS=no", "-o", "KnownHostsCommand=none", "-o", "ForwardAgent=no", "-o", "ForwardX11=no", "-o", "PermitLocalCommand=no", "-o", "ClearAllForwardings=yes", "-o", "ControlMaster=no", "-o", "ControlPath=none", "-o", "ControlPersist=no", "-o", "ConnectTimeout=10", "-o", "ConnectionAttempts=1", "-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=2", "-o", "UserKnownHostsFile=" + quoteConfig(known), "-o", "GlobalKnownHostsFile=" + quoteConfig(null), "--", p.Destination, command}
	c := m.command(call, args...)
	stdout, stderr := &limited{max: OutputLimit - OutputLimit/4}, &limited{max: OutputLimit / 4}
	c.Stdout, c.Stderr = stdout, stderr
	started := time.Now()
	err = c.Run()
	r := Result{Stdout: stdout.buf.String(), Stderr: stderr.buf.String(), ExitCode: 0, Truncated: stdout.cut || stderr.cut, DurationMS: time.Since(started).Milliseconds()}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			r.ExitCode = exit.ExitCode()
		} else {
			r.ExitCode = -1
		}
	}
	if call.Err() != nil || r.ExitCode == 255 || r.ExitCode == -1 {
		if mutates {
			return Result{}, fail("outcome_unknown", "SSH stopped answering; the operation may have happened. Read its status before trying again")
		}
		if call.Err() != nil {
			return Result{}, fail("cancelled", "SSH operation was cancelled or timed out")
		}
		return Result{}, fail("ssh_failed", "SSH could not connect using existing trusted keys and noninteractive authentication; check OpenSSH locally")
	}
	return r, nil
}

var resource = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.@:-]{0,159}$`)

func Command(op Operation) (string, bool, error) {
	lines := op.Lines
	if lines == 0 {
		lines = 100
	}
	if lines < 1 || lines > 200 {
		return "", false, fail("invalid_argument", "logs must request between 1 and 200 lines")
	}
	if op.Name != "" && !resource.MatchString(op.Name) {
		return "", false, fail("invalid_argument", "invalid service or container name")
	}
	if strings.HasPrefix(op.Action, "services.") && op.Name != "" && !strings.HasSuffix(op.Name, ".service") {
		return "", false, fail("invalid_argument", "select a systemd .service unit; targets and other unit types are not allowed")
	}
	q := "'" + op.Name + "'"
	switch op.Action {
	case "probe":
		return "uname -s", false, nil
	case "overview":
		return overview, false, nil
	case "services.list":
		return "LC_ALL=C systemctl --no-pager --plain --no-legend list-units --all --type=service", false, nil
	case "services.status":
		if op.Name != "" {
			return "LC_ALL=C systemctl --no-pager show --property=Id,LoadState,ActiveState,SubState,Description,MainPID,MemoryCurrent,CPUUsageNSec -- " + q, false, nil
		}
	case "services.logs":
		if op.Name != "" {
			return "LC_ALL=C journalctl --no-pager --output=short-iso --lines=" + strconv.Itoa(lines) + " --unit=" + q, false, nil
		}
	case "services.start", "services.stop", "services.restart":
		if op.Name != "" {
			return "LC_ALL=C systemctl --no-ask-password " + strings.TrimPrefix(op.Action, "services.") + " -- " + q, true, nil
		}
	case "containers.list":
		return `LC_ALL=C docker ps -a --no-trunc --format '{{json .}}'`, false, nil
	case "containers.logs":
		if op.Name != "" {
			return "LC_ALL=C docker logs --timestamps --tail " + strconv.Itoa(lines) + " -- " + q, false, nil
		}
	case "containers.start", "containers.stop", "containers.restart":
		if op.Name != "" {
			return "LC_ALL=C docker " + strings.TrimPrefix(op.Action, "containers.") + " -- " + q, true, nil
		}
	}
	return "", false, fail("invalid_argument", "unsupported SSH action or missing resource name")
}

const overview = `LC_ALL=C; export LC_ALL
printf '%s\n' '@@myco:os'; uname -srm; cat /etc/os-release
printf '%s\n' '@@myco:uptime'; cat /proc/uptime /proc/loadavg
printf '%s\n' '@@myco:cpu0'; head -n 1 /proc/stat
sleep 0.2
printf '%s\n' '@@myco:cpu1'; head -n 1 /proc/stat
printf '%s\n' '@@myco:memory'; cat /proc/meminfo
printf '%s\n' '@@myco:disks'; df -Pk`

// Summary is safe approval text; private config/key paths never leave this package.
func (p Profile) Summary() string {
	return fmt.Sprintf("%s@%s:%d · strict known_hosts · profile %s", p.User, p.Host, p.Port, p.ID)
}
