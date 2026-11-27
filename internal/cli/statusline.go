package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
)

// Claude Code runs a statusLine command after most turns and gives it JSON on
// stdin, including the account's rate limits (five-hour and seven-day windows,
// percent used and when each resets). `myco agent runtime statusline` keeps
// those numbers in ~/.myco/quota/<agent>.json so `myco inbox wait` and the
// runner can report them without the agent having to remember anything. It
// never talks to the network and never fails: a broken status line would cost
// the person the line they had.

// statusLineQuotaWindow is one window as the status line wrote it: only the
// number and the reset time (a window's own status is not part of that JSON).
type statusLineQuotaWindow struct {
	Name     string     `json:"name"`
	Used     float64    `json:"used"`
	ResetsAt *time.Time `json:"resets_at,omitempty"`
	Status   string     `json:"status,omitempty"`
}

// statusLineQuotaFile is what one status line observation left on disk.
type statusLineQuotaFile struct {
	ObservedAt time.Time               `json:"observed_at"`
	Windows    []statusLineQuotaWindow `json:"windows"`
}

// statusLineReport is the throttle marker next to it: the observation last
// reported and when that report was made.
type statusLineReport struct {
	ObservedAt time.Time `json:"observed_at"`
	ReportedAt time.Time `json:"reported_at"`
}

// statusLineState keeps the statusLine value that was in Claude Code's
// settings before myco's was installed, so --uninstall can put it back.
type statusLineState struct {
	Previous json.RawMessage `json:"previous,omitempty"`
}

// statusLineMinReport is how often a session may report status line quota.
const statusLineMinReport = 5 * time.Minute

func statusLineCommand() *Command {
	return &Command{
		Name:     "statusline",
		Summary:  "Claude Code status line: keep this session's usage limits, then show the line",
		Mutating: true,
		Long: `For Claude Code, which runs a statusLine command after most turns and gives it JSON on
stdin: the account's five-hour and seven-day use (percent) and when each window resets.
This reads that JSON, writes the numbers and times to ~/.myco/quota/<agent>.json, and prints
a short line. myco inbox wait and the runner then report the quota to the hub, at most every
five minutes, so people see it without the agent running anything.

It is fast and never fails: no network, and a bad or empty stdin still leaves a line on the
screen. If a statusLine command was already configured, it is kept and chained: myco runs it
with the same JSON and prints its output instead. --install sets this command in
~/.claude/settings.json (showing what changes; --dry-run writes nothing); --uninstall puts
back the command that was there before.`,
		Examples: []string{
			"myco agent runtime statusline --install",
			"myco agent runtime statusline --install --dry-run",
			"myco agent runtime statusline --uninstall",
			"echo '<Claude Code status line JSON>' | myco agent runtime statusline",
		},
		Flags: []Flag{
			{Name: "install", Kind: Bool, Help: "Set it as Claude Code's status line in ~/.claude/settings.json"},
			{Name: "uninstall", Kind: Bool, Help: "Put back the status line that was there before"},
		},
		Run: handler(func(e *env) error {
			install, uninstall := e.in.Bool("install"), e.in.Bool("uninstall")
			switch {
			case install && uninstall:
				return errorf("invalid_argument", ExitUsage, "pass one of --install or --uninstall", "--install and --uninstall are mutually exclusive")
			case install, uninstall:
				dir := config.Dir(e.defaultPath)
				if e.in.Bool("dry-run") {
					res, err := statusLinePlan(dir, e.in.Getenv, uninstall)
					if err != nil {
						return err
					}
					action := "statusline_install"
					if uninstall {
						action = "statusline_uninstall"
					}
					return e.dryRun(map[string]any{"action": action, "path": res.Path, "command": res.Command,
						"previous": res.Previous, "chained": res.Chained, "changed": res.Changed})
				}
				res, err := statusLineApply(dir, e.in.Getenv, uninstall, false)
				if err != nil {
					return err
				}
				return e.p.Print(map[string]any{"path": res.Path, "command": res.Command, "previous": res.Previous,
					"chained": res.Chained, "changed": res.Changed}, func(w io.Writer) {
					if !res.Changed {
						fmt.Fprintf(w, "the status line in %s is already as asked\n", res.Path)
						return
					}
					if uninstall {
						fmt.Fprintf(w, "removed myco's status line from %s", res.Path)
						if res.Previous != "" {
							fmt.Fprintf(w, "; the one before is back (%s)", res.Previous)
						}
						fmt.Fprintln(w)
						return
					}
					fmt.Fprintf(w, "status line set in %s: %s\n", res.Path, res.Command)
					if res.Chained {
						fmt.Fprintf(w, "the one that was there still runs first: %s\n", res.Previous)
					}
					fmt.Fprintln(w, "restart Claude Code, or wait for its next turn, to see it")
				})
			default:
				e.statusLine()
				return nil
			}
		}),
	}
}

// statusLine is the status line itself. It must not fail and must print one
// line: anything typed by a person who ran it by hand is answered on stderr.
func (e *env) statusLine() {
	defer func() {
		if recover() != nil {
			fmt.Fprintln(e.in.Stdout, "myco")
		}
	}()
	var raw []byte
	if f, ok := e.in.Stdin.(*os.File); !ok || !isTerminal(f) {
		raw, _ = io.ReadAll(io.LimitReader(e.in.Stdin, 4<<20))
	} else {
		fmt.Fprintln(e.in.Stderr, "reads Claude Code's status line JSON on stdin; see myco agent runtime statusline --help")
	}
	windows, ok := parseStatusLineQuota(raw)
	if ok && len(windows) > 0 {
		if agent := statusLineAgent(e); agent != "" {
			_ = writeStatusLineQuota(config.Dir(e.defaultPath), agent, windows, time.Now().UTC())
		}
	}
	if out, ok := statusLinePrevious(config.Dir(e.defaultPath), raw); ok && strings.TrimSpace(out) != "" {
		fmt.Fprintln(e.in.Stdout, strings.TrimRight(out, "\n"))
		return
	}
	fmt.Fprintln(e.in.Stdout, statusLineText(windows))
}

// statusLineAgent is the agent this session joined as, and only if the
// config belongs to this session (a default config from another session
// must not receive this session's numbers).
func statusLineAgent(e *env) string {
	if e.cfg == nil || e.cfg.Agent == "" {
		return ""
	}
	if e.explicitConfig {
		return e.cfg.Agent
	}
	b := detectBinding(e.in.Getenv)
	if b.SessionID == "" || e.cfg.SessionID == b.SessionID {
		return e.cfg.Agent
	}
	return ""
}

// parseStatusLineQuota reads Claude Code's status line JSON. no rate_limits
// (older Claude Code, API-key accounts, a session with no response yet) is
// not an error: there is simply nothing to keep.
func parseStatusLineQuota(data []byte) ([]statusLineQuotaWindow, bool) {
	var in struct {
		RateLimits map[string]json.RawMessage `json:"rate_limits"`
	}
	if len(bytes.TrimSpace(data)) == 0 || json.Unmarshal(data, &in) != nil || len(in.RateLimits) == 0 {
		return nil, false
	}
	var out []statusLineQuotaWindow
	for name, raw := range in.RateLimits {
		var w struct {
			UsedPercentage *float64    `json:"used_percentage"`
			ResetsAt       json.Number `json:"resets_at"`
		}
		if json.Unmarshal(raw, &w) != nil || w.UsedPercentage == nil {
			continue
		}
		used := *w.UsedPercentage
		if math.IsNaN(used) || math.IsInf(used, 0) || used < 0 || used > 100 || strings.TrimSpace(name) == "" || len(name) > 80 {
			continue
		}
		qw := statusLineQuotaWindow{Name: name, Used: used / 100, Status: "allowed"}
		if sec, err := w.ResetsAt.Float64(); err == nil && sec > 0 && sec < 1e11 {
			t := time.Unix(int64(sec), 0).UTC()
			qw.ResetsAt = &t
		}
		out = append(out, qw)
	}
	if len(out) == 0 {
		return nil, false
	}
	sort.SliceStable(out, func(i, j int) bool { return statusLineOrder(out[i].Name) < statusLineOrder(out[j].Name) })
	return out, true
}

// statusLineOrder puts the two windows Claude Code reports first, then any
// others by name.
func statusLineOrder(name string) string {
	switch {
	case name == "five_hour":
		return "0"
	case name == "seven_day":
		return "1"
	case strings.HasPrefix(name, "seven_day_"):
		return "2" + name
	default:
		return "3" + name
	}
}

// statusLineText is the line shown when there is no previous command.
func statusLineText(windows []statusLineQuotaWindow) string {
	parts := []string{"myco"}
	for _, w := range windows {
		label := w.Name
		switch w.Name {
		case "five_hour":
			label = "5h"
		case "seven_day":
			label = "7d"
		}
		parts = append(parts, fmt.Sprintf("%s %.0f%%", label, w.Used*100))
	}
	return strings.Join(parts, " · ")
}

// mycoDir is the folder this machine's myco files live in (~/.myco).
func (e *env) mycoDir() string { return config.Dir(e.defaultPath) }

// statusLineQuotaPath is where one agent's latest observation lives.
func statusLineQuotaPath(dir, agent string) string {
	return filepath.Join(dir, "quota", agent+".json")
}

func statusLineReportPath(dir, agent string) string {
	return filepath.Join(dir, "quota", agent+".reported.json")
}

// writeStatusLineQuota keeps one observation, atomically, mode 600.
func writeStatusLineQuota(dir, agent string, windows []statusLineQuotaWindow, now time.Time) error {
	f := statusLineQuotaFile{ObservedAt: now.UTC(), Windows: windows}
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return writePrivateFile(statusLineQuotaPath(dir, agent), append(b, '\n'))
}

// readStatusLineQuota reads an agent's latest observation.
func readStatusLineQuota(dir, agent string) ([]statusLineQuotaWindow, time.Time, bool) {
	b, err := os.ReadFile(statusLineQuotaPath(dir, agent))
	if err != nil || len(b) > 64<<10 {
		return nil, time.Time{}, false
	}
	var f statusLineQuotaFile
	if json.Unmarshal(b, &f) != nil || f.ObservedAt.IsZero() || len(f.Windows) == 0 {
		return nil, time.Time{}, false
	}
	var out []statusLineQuotaWindow
	for _, w := range f.Windows {
		if math.IsNaN(w.Used) || math.IsInf(w.Used, 0) || w.Used < 0 || w.Used > 1 || w.Name == "" {
			continue
		}
		out = append(out, w)
	}
	if len(out) == 0 {
		return nil, time.Time{}, false
	}
	return out, f.ObservedAt, true
}

// writePrivateFile replaces path atomically with mode 600.
func writePrivateFile(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// statusLinePrevious runs the statusLine command that was configured before
// myco's, with the same JSON, and returns what it printed.
func statusLinePrevious(dir string, stdin []byte) (string, bool) {
	st, err := readStatusLineState(dir)
	if err != nil {
		return "", false
	}
	cmd := statusLinePreviousCommand(st.Previous)
	if cmd == "" {
		return "", false
	}
	out, err := runStatusLineCommand(cmd, stdin)
	if err != nil {
		return "", false
	}
	return string(out), true
}

// statusLineRun runs another status line command, at most two seconds.
var statusLineRun = func(ctx context.Context, command string, stdin []byte) ([]byte, error) {
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		c = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	c.Stdin = bytes.NewReader(stdin)
	var out bytes.Buffer
	c.Stdout = &out
	if err := c.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func runStatusLineCommand(command string, stdin []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return statusLineRun(ctx, command, stdin)
}

// statusLinePreviousCommand is the command in a stored statusLine value
// (an object with type command, or a bare command string).
func statusLinePreviousCommand(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var o struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	}
	if json.Unmarshal(raw, &o) == nil && o.Type == "command" {
		return strings.TrimSpace(o.Command)
	}
	return ""
}

// statusLineInstallResult says what --install/--uninstall would do (and,
// when applied, what it did).
type statusLineInstallResult struct {
	Path     string
	Command  string
	Previous string
	Chained  bool
	Changed  bool
}

// statusLineSettingsPath is Claude Code's user settings file.
func statusLineSettingsPath(getenv func(string) string) string {
	dir := ""
	if getenv != nil {
		dir = strings.TrimSpace(getenv("CLAUDE_CONFIG_DIR"))
	}
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".claude")
	}
	return filepath.Join(dir, "settings.json")
}

func statusLineStatePath(dir string) string { return filepath.Join(dir, "statusline.json") }

func readStatusLineState(dir string) (statusLineState, error) {
	var st statusLineState
	b, err := os.ReadFile(statusLineStatePath(dir))
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	return st, json.Unmarshal(b, &st)
}

func writeStatusLineState(dir string, st statusLineState) error {
	if len(st.Previous) == 0 {
		if err := os.Remove(statusLineStatePath(dir)); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return writePrivateFile(statusLineStatePath(dir), append(b, '\n'))
}

// statusLineApply installs or removes myco's status line. dry leaves both
// files untouched.
func statusLineApply(dir string, getenv func(string) string, uninstall, dry bool) (statusLineInstallResult, error) {
	res, doc, st, path, err := statusLinePlanFiles(dir, getenv, uninstall)
	if err != nil || dry || !res.Changed {
		return res, err
	}
	if uninstall {
		delete(doc, "statusLine")
		if len(st.Previous) > 0 {
			doc["statusLine"] = st.Previous
		}
	} else {
		doc["statusLine"] = statusLineSetting(res.Command)
	}
	if err := writeClaudeSettings(path, doc); err != nil {
		return res, err
	}
	return res, writeStatusLineState(dir, st)
}

// statusLinePlan is statusLineApply without writing.
func statusLinePlan(dir string, getenv func(string) string, uninstall bool) (statusLineInstallResult, error) {
	res, _, _, _, err := statusLinePlanFiles(dir, getenv, uninstall)
	return res, err
}

func statusLinePlanFiles(dir string, getenv func(string) string, uninstall bool) (statusLineInstallResult, map[string]json.RawMessage, statusLineState, string, error) {
	path := statusLineSettingsPath(getenv)
	doc, err := readClaudeSettings(path)
	if err != nil {
		return statusLineInstallResult{}, nil, statusLineState{}, path, err
	}
	st, err := readStatusLineState(dir)
	if err != nil {
		return statusLineInstallResult{}, nil, statusLineState{}, path, err
	}
	current := doc["statusLine"]
	currentCmd := statusLinePreviousCommand(current)
	ourCmd := statusLineCommandLine()
	res := statusLineInstallResult{Path: path, Command: ourCmd}
	if uninstall {
		res.Command, res.Previous = currentCmd, statusLinePreviousCommand(st.Previous)
		if statusLinePreviousCommand(st.Previous) != "" {
			res.Chained = true
		}
		res.Changed = isOurStatusLine(currentCmd)
		if !res.Changed && len(current) > 0 {
			res.Command = currentCmd
		}
		st.Previous = nil
		return res, doc, st, path, nil
	}
	res.Changed = !isOurStatusLine(currentCmd) || strings.TrimSpace(currentCmd) != ourCmd
	if !isOurStatusLine(currentCmd) && len(current) > 0 && string(current) != "null" {
		st.Previous = append(json.RawMessage(nil), current...)
	}
	res.Previous = statusLinePreviousCommand(st.Previous)
	res.Chained = res.Previous != ""
	return res, doc, st, path, nil
}

// isOurStatusLine recognizes the command myco installs (any myco's path).
func isOurStatusLine(cmd string) bool {
	return strings.HasSuffix(strings.TrimSpace(cmd), "agent runtime statusline") ||
		strings.HasSuffix(strings.TrimSpace(cmd), "agent runtime statusline --install")
}

// statusLineCommandLine is the command to put in settings: this binary, by
// its own path, so it works without the CLI's directory in PATH.
func statusLineCommandLine() string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = "myco"
	}
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(exe, `"`, `\"`) + `" agent runtime statusline`
	}
	if strings.ContainsAny(exe, " \t'\"$`\\!&|;<>()*?[]{}~#") {
		return "'" + strings.ReplaceAll(exe, "'", `'\''`) + "' agent runtime statusline"
	}
	return exe + " agent runtime statusline"
}

func statusLineSetting(command string) json.RawMessage {
	b, _ := json.Marshal(struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	}{"command", command})
	return b
}

// readClaudeSettings reads Claude Code's settings.json, keeping every other
// key as it was.
func readClaudeSettings(path string) (map[string]json.RawMessage, error) {
	doc := map[string]json.RawMessage{}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return doc, nil
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return doc, nil
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, errorf("invalid_config", ExitUsage, "fix or remove "+path, "cannot read %s: %v", path, err)
	}
	return doc, nil
}

// writeClaudeSettings replaces the file (following a symlink, keeping its
// mode), so a half-written file never breaks Claude Code.
func writeClaudeSettings(path string, doc map[string]json.RawMessage) error {
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	target, mode := path, os.FileMode(0o600)
	if real, err := filepath.EvalSymlinks(path); err == nil {
		target = real
	}
	if st, err := os.Stat(target); err == nil {
		mode = st.Mode().Perm()
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".settings-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

// reportStatusLineQuota tells the hub the quota the status line last wrote
// for agent, when it is newer than the last report, at most every five
// minutes. Only numbers and window times leave the computer. Best effort:
// quiet, and a few seconds at most.
func reportStatusLineQuota(ctx context.Context, c *client.Client, dir, agent string, now time.Time) {
	if c == nil || agent == "" {
		return
	}
	windows, at, ok := readStatusLineQuota(dir, agent)
	if !ok || at.After(now.Add(time.Minute)) {
		return
	}
	if rep, ok := readStatusLineReport(dir, agent); ok {
		if !at.After(rep.ObservedAt) || now.Sub(rep.ReportedAt) < statusLineMinReport {
			return
		}
	}
	call, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	me, err := c.Me(call)
	if err != nil {
		return
	}
	rt := api.Runtime{}
	if me.Runtime != nil {
		rt = *me.Runtime
	}
	// Something newer is already there (a runner's collection, or a later
	// session report): keep it, but mark this observation seen so a later
	// wait does not ask again.
	if rt.Quota != nil && !rt.Quota.At.Before(at) {
		writeStatusLineReport(dir, agent, statusLineReport{ObservedAt: at, ReportedAt: now})
		return
	}
	rt.Quota = statusLineAPIQuota(windows, at, now)
	if rt.CLI == "" {
		rt.CLI = me.Binding.Harness
	}
	if err := c.ReportRuntime(call, rt); err != nil {
		return
	}
	writeStatusLineReport(dir, agent, statusLineReport{ObservedAt: at, ReportedAt: now})
}

func readStatusLineReport(dir, agent string) (statusLineReport, bool) {
	var rep statusLineReport
	b, err := os.ReadFile(statusLineReportPath(dir, agent))
	if err != nil || len(b) > 4096 {
		return rep, false
	}
	if json.Unmarshal(b, &rep) != nil {
		return rep, false
	}
	return rep, true
}

func writeStatusLineReport(dir, agent string, rep statusLineReport) {
	b, err := json.Marshal(rep)
	if err != nil {
		return
	}
	_ = writePrivateFile(statusLineReportPath(dir, agent), append(b, '\n'))
}

// statusLineAPIQuota is a status line observation as the hub keeps quota.
func statusLineAPIQuota(windows []statusLineQuotaWindow, at, now time.Time) *api.Quota {
	q := &api.Quota{Source: "claude", Provider: "claude", SourceKind: "statusline", Status: "ok", At: at.UTC(), CheckedAt: now.UTC(), Windows: []api.QuotaWindow{}}
	for _, w := range windows {
		aw := api.QuotaWindow{Name: w.Name, Used: w.Used, Status: "allowed"}
		switch {
		case w.Name == "five_hour":
			aw.DurationSec = 18000
		case strings.HasPrefix(w.Name, "seven_day"):
			aw.DurationSec = 604800
		}
		if w.ResetsAt != nil {
			t := w.ResetsAt.UTC()
			aw.ResetsAt = &t
		}
		q.Windows = append(q.Windows, aw)
	}
	return q
}

// statusLineQuotaFor is the runner's version: a status line observation as
// quota, when it is newer than the last one reported (from either the runner
// or a session's inbox wait) and reports are at least five minutes apart, and
// the runner has nothing newer or healthier.
func statusLineQuotaFor(dir, agent string, now time.Time, current *api.Quota) (*api.Quota, time.Time, bool) {
	windows, at, ok := readStatusLineQuota(dir, agent)
	if !ok || at.After(now.Add(time.Minute)) {
		return nil, time.Time{}, false
	}
	if rep, ok := readStatusLineReport(dir, agent); ok {
		if !at.After(rep.ObservedAt) || now.Sub(rep.ReportedAt) < statusLineMinReport {
			return nil, time.Time{}, false
		}
	}
	if current != nil && !current.At.Before(at) {
		return nil, time.Time{}, false
	}
	// A healthy native collection is the better source: the status line is
	// for sessions (and accounts) that have none.
	if current != nil && current.ProfileRef != "" && current.Status == "ok" {
		return nil, time.Time{}, false
	}
	return statusLineAPIQuota(windows, at, now), at, true
}
