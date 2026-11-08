package console

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
)

// LocalSession is a Claude Code or Codex session on this computer that can
// join a hub with one click.
type LocalSession struct {
	CLI        string    `json:"cli"` // claude | codex
	SessionID  string    `json:"session_id"`
	Workdir    string    `json:"workdir"`
	LastActive time.Time `json:"last_active"`
	// Title is the session's first request, shortened.
	Title string `json:"title,omitempty"`
	// Joined is set when the session is already an agent here.
	Joined *JoinedAs `json:"joined,omitempty"`
}

// JoinedAs names the agent a session joined as, and on which hub.
type JoinedAs struct {
	Agent string `json:"agent"`
	Hub   string `json:"hub"`
}

const (
	sessionsMax    = 40
	sessionsMaxAge = 30 * 24 * time.Hour
	titleMax       = 80
	scanLines      = 400
)

var claudeSessionFile = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.jsonl$`)

// LocalSessions lists the most recent Claude Code and Codex sessions on
// this computer (newest first), from their transcripts:
// ~/.claude/projects/<dir>/<session>.jsonl and
// ~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl.
func (l *Local) LocalSessions(now time.Time) []LocalSession {
	claudeDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if claudeDir == "" {
		claudeDir = filepath.Join(l.Home, ".claude")
	}
	codexDir := os.Getenv("CODEX_HOME")
	if codexDir == "" {
		codexDir = filepath.Join(l.Home, ".codex")
	}
	type file struct {
		path, cli string
		mod       time.Time
	}
	var files []file
	add := func(pattern, cli string, keep func(string) bool) {
		paths, _ := filepath.Glob(pattern)
		for _, p := range paths {
			if !keep(filepath.Base(p)) {
				continue
			}
			if st, err := os.Stat(p); err == nil && now.Sub(st.ModTime()) < sessionsMaxAge {
				files = append(files, file{p, cli, st.ModTime()})
			}
		}
	}
	add(filepath.Join(claudeDir, "projects", "*", "*.jsonl"), "claude", claudeSessionFile.MatchString)
	add(filepath.Join(codexDir, "sessions", "*", "*", "*", "rollout-*.jsonl"), "codex", func(string) bool { return true })
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })

	joined := l.joinedSessions()
	var out []LocalSession
	seen := map[string]bool{}
	for _, f := range files {
		if len(out) >= sessionsMax {
			break
		}
		var s LocalSession
		var ok bool
		if f.cli == "claude" {
			s, ok = readClaudeSession(f.path)
		} else {
			s, ok = readCodexSession(f.path)
		}
		key := f.cli + ":" + s.SessionID
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		s.LastActive = f.mod
		if j, ok := joined[key]; ok {
			s.Joined = &j
		}
		out = append(out, s)
	}
	return out
}

// joinedSessions maps "cli:session" to the local agent profile bound to it.
func (l *Local) joinedSessions() map[string]JoinedAs {
	out := map[string]JoinedAs{}
	profiles, err := config.ListProfiles(filepath.Join(l.Home, ".myco", "agents"))
	if err != nil {
		return out
	}
	for _, p := range profiles {
		h := p.Config.Runner.Harness
		if h == "" {
			h = p.Config.Harness
		}
		if p.Config.SessionID != "" {
			out[h+":"+p.Config.SessionID] = JoinedAs{Agent: p.Config.Agent, Hub: p.Config.Hub}
		}
	}
	return out
}

func readClaudeSession(path string) (LocalSession, bool) {
	s := LocalSession{CLI: "claude", SessionID: strings.TrimSuffix(filepath.Base(path), ".jsonl")}
	eachLine(path, func(line []byte) bool {
		var e struct {
			Type        string `json:"type"`
			Cwd         string `json:"cwd"`
			IsSidechain bool   `json:"isSidechain"`
			Message     struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &e) != nil {
			return true
		}
		if s.Workdir == "" && e.Cwd != "" {
			s.Workdir = e.Cwd
		}
		if s.Title == "" && e.Type == "user" && !e.IsSidechain {
			s.Title = shorten(contentText(e.Message.Content))
		}
		return s.Workdir == "" || s.Title == ""
	})
	return s, s.Workdir != ""
}

func readCodexSession(path string) (LocalSession, bool) {
	s := LocalSession{CLI: "codex"}
	eachLine(path, func(line []byte) bool {
		var e struct {
			Type    string `json:"type"`
			Payload struct {
				ID      string `json:"id"`
				Cwd     string `json:"cwd"`
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &e) != nil {
			return true
		}
		switch {
		case e.Type == "session_meta":
			s.SessionID, s.Workdir = e.Payload.ID, e.Payload.Cwd
		case e.Type == "event_msg" && e.Payload.Type == "user_message" && s.Title == "":
			s.Title = shorten(e.Payload.Message)
		}
		return s.SessionID == "" || s.Title == ""
	})
	return s, s.SessionID != "" && s.Workdir != ""
}

// eachLine calls fn for the first scanLines lines while it returns true.
func eachLine(path string, fn func([]byte) bool) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for n := 0; n < scanLines && sc.Scan(); n++ {
		if !fn(sc.Bytes()) {
			return
		}
	}
}

// contentText is the text of a message content: a string, or the text
// parts of a list.
func contentText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		for _, p := range parts {
			if p.Type == "text" && strings.TrimSpace(p.Text) != "" {
				return p.Text
			}
		}
	}
	return ""
}

func shorten(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	// Command wrappers and system reminders are not what the user asked.
	if strings.HasPrefix(s, "<") {
		return ""
	}
	if r := []rune(s); len(r) > titleMax {
		return string(r[:titleMax]) + "…"
	}
	return s
}

func (b *Backend) listLocalSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sessions": nonNilSessions(b.Local.LocalSessions(time.Now()))})
}

func nonNilSessions(s []LocalSession) []LocalSession {
	if s == nil {
		return []LocalSession{}
	}
	return s
}

// LocalJoinResult is POST /app/hubs/{id}/local-join.
type LocalJoinResult struct {
	Agent   string `json:"agent"`
	Created bool   `json:"created"`
	// Runner is "running" when this computer's runner service serves the
	// new agent (it picks new agents up within ~30s), else why not.
	Runner string `json:"runner"`
}

// localJoin makes a session on this computer an agent of hub {id}: a
// one-time enrollment from the hub (with the console token), `myco join`
// run in the session's directory, and the runner service started if it
// is not running.
func (b *Backend) localJoin(w http.ResponseWriter, r *http.Request, c *client.Client, hub Hub) {
	var in struct {
		CLI         string `json:"cli"`
		SessionID   string `json:"session_id"`
		Workdir     string `json:"workdir"`
		DisplayName string `json:"display_name"`
		Description string `json:"description"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.CLI != "claude" && in.CLI != "codex" {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "cli must be claude or codex"})
		return
	}
	if st, err := os.Stat(in.Workdir); in.SessionID == "" || err != nil || !st.IsDir() {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "pick a session from the list (its directory must exist)"})
		return
	}
	device := orStr(b.Local.Device, "this-mac")
	prefix := device + "-"
	name := prefix + in.CLI + "-" + lastAlnum(in.SessionID, 4)
	enr, err := c.CreateEnrollment(r.Context(), api.CreateEnrollmentRequest{NamePrefix: prefix, MaxUses: 1, TTLSeconds: 600,
		Labels: []string{"machine=" + device}, Note: "myco app: local session"}, false)
	if err != nil {
		writeErr(w, err)
		return
	}
	args := []string{"join", "--harness", in.CLI, "--session-id", in.SessionID, "--name", name}
	if in.DisplayName != "" {
		args = append(args, "--display-name", in.DisplayName)
	}
	if in.Description != "" {
		args = append(args, "--description", in.Description)
	}
	out, err := b.Local.runIn(r.Context(), in.Workdir, []string{"MYCO_HUB=" + hub.URL, "MYCO_ENROLL_TOKEN=" + enr.Token}, args...)
	if err != nil {
		writeErr(w, err)
		return
	}
	var j struct {
		Agent   api.Agent `json:"agent"`
		Created bool      `json:"created"`
	}
	if err := json.Unmarshal(out, &j); err != nil || j.Agent.Name == "" {
		writeErr(w, fmt.Errorf("unexpected output from myco join: %s", lastLine(out)))
		return
	}
	writeJSON(w, http.StatusOK, LocalJoinResult{Agent: j.Agent.Name, Created: j.Created, Runner: b.Local.ensureRunner(r.Context())})
}

// lastAlnum keeps the last n letters/digits of id, lowercased.
func lastAlnum(id string, n int) string {
	var keep []rune
	for _, r := range strings.ToLower(id) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			keep = append(keep, r)
		}
	}
	if len(keep) > n {
		keep = keep[len(keep)-n:]
	}
	return string(keep)
}
