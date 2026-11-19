package console

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	// Title is the session's own title (one given in Claude Code or Codex,
	// or the one it made), else its first request; shortened.
	Title string `json:"title,omitempty"`
	// Joined is set when the session is already an agent here (the first,
	// for older apps); JoinedAll is every workspace it joined.
	Joined    *JoinedAs  `json:"joined,omitempty"`
	JoinedAll []JoinedAs `json:"joined_all,omitempty"`

	// Auto: a program ran it (claude -p or the Agent SDK, codex exec, myco
	// itself), not a person in a terminal. Listed after the others, folded.
	Auto bool `json:"auto,omitempty"`
}

// JoinedAs names the agent a session joined as, and on which hub (HubName:
// the hub's name in this app, when it is one of the app's).
type JoinedAs struct {
	Agent   string `json:"agent"`
	Hub     string `json:"hub"`
	HubName string `json:"hub_name,omitempty"`
}

const (
	sessionsMax    = 200
	autoMax        = 100 // of them, ones a program ran (they repeat a lot)
	sessionsMaxAge = 180 * 24 * time.Hour
	// filesMax bounds how many transcripts are read (newest first): copies
	// of one conversation count once, so a list can need more than it shows.
	filesMax  = 4000
	titleMax  = 80
	scanLines = 400
)

var claudeSessionFile = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.jsonl$`)

// claudeDirs are where Claude Code keeps its sessions on this computer: the
// configured directory and both default places (~/.claude, and
// ~/.config/claude used by newer versions), each once.
func (l *Local) claudeDirs() []string {
	var dirs []string
	seen := map[string]bool{}
	for _, d := range []string{os.Getenv("CLAUDE_CONFIG_DIR"), filepath.Join(l.Home, ".claude"), filepath.Join(l.Home, ".config", "claude")} {
		if d == "" {
			continue
		}
		if abs, err := filepath.Abs(d); err == nil {
			d = abs
		}
		if !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// LocalSessions lists the most recent Claude Code and Codex sessions on
// this computer (newest first), from their transcripts:
// <claude dir>/projects/<dir>/<session>.jsonl and
// ~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl.
//
// One conversation can have several transcripts: a copy made to join myco
// (复制一份加入), a fork, a resume that started a new file, or the same
// session under two project folders. They share the first request (its
// text, folder and time) and are shown once: the newest, marked joined if
// any of them joined.
func (l *Local) LocalSessions(now time.Time) []LocalSession {
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
	for _, d := range l.claudeDirs() {
		add(filepath.Join(d, "projects", "*", "*.jsonl"), "claude", claudeSessionFile.MatchString)
	}
	add(filepath.Join(codexDir, "sessions", "*", "*", "*", "rollout-*.jsonl"), "codex", func(string) bool { return true })
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	if len(files) > filesMax {
		files = files[:filesMax]
	}

	joined := l.joinedSessions()
	threadNames := codexThreadNames(codexDir)
	var out []LocalSession
	byID := map[string]int{}    // cli:session → index in out
	byConv := map[string]int{}  // a conversation's first request → index in out
	byText := map[string]bool{} // cli, folder and first request of a listed unjoined session
	autos := 0                  // unjoined ones a program ran, listed so far
	for _, f := range files {
		var s LocalSession
		var first string
		var ok bool
		if f.cli == "claude" {
			s, first, ok = readClaudeSession(f.path)
			if t := shorten(claudeTitle(f.path)); ok && t != "" {
				s.Title = t
			}
		} else {
			s, first, ok = readCodexSession(f.path)
			if t := shorten(threadNames[s.SessionID]); ok && t != "" {
				s.Title = t
			}
		}
		if !ok {
			continue
		}
		id := f.cli + ":" + s.SessionID
		js := joined[id]
		conv := ""
		if first != "" {
			conv = f.cli + "\x00" + s.Workdir + "\x00" + first
		}
		i, dup := byID[id]
		if !dup && conv != "" {
			i, dup = byConv[conv]
		}
		if dup {
			// An older copy of a conversation already listed (files come
			// newest first): it only adds that the conversation joined.
			byID[id] = i
			out[i].addJoined(js)
			continue
		}
		if len(js) == 0 {
			// Not joined yet: a request asked again in the same folder shows
			// once (the newest), since the rows would read alike (scripts
			// repeat one prompt many times).
			// (A person's session never hides behind a script's with the same prompt.)
			text := fmt.Sprint(f.cli, "\x00", s.Workdir, "\x00", s.Auto, "\x00", firstText(first))
			if first != "" && byText[text] {
				continue
			}
			if first != "" {
				byText[text] = true
			}
		}
		if len(out) >= sessionsMax || (s.Auto && len(js) == 0 && autos >= autoMax) {
			continue
		}
		if s.Auto && len(js) == 0 {
			autos++
		}
		s.LastActive = f.mod
		s.addJoined(js)
		byID[id] = len(out)
		if conv != "" {
			byConv[conv] = len(out)
		}
		out = append(out, s)
	}
	return out
}

// addJoined adds workspaces the session joined (each once).
func (s *LocalSession) addJoined(js []JoinedAs) {
	for _, j := range js {
		dup := false
		for _, have := range s.JoinedAll {
			if have.Agent == j.Agent && have.Hub == j.Hub {
				dup = true
			}
		}
		if !dup {
			s.JoinedAll = append(s.JoinedAll, j)
		}
	}
	if s.Joined == nil && len(s.JoinedAll) > 0 {
		first := s.JoinedAll[0]
		s.Joined = &first
	}
}

// joinedSessions maps "cli:session" to the local agent profiles bound to it
// (one per workspace the session joined).
func (l *Local) joinedSessions() map[string][]JoinedAs {
	out := map[string][]JoinedAs{}
	profiles, err := config.ListProfiles(filepath.Join(l.Home, ".myco", "agents"))
	if err != nil {
		return out
	}
	for _, p := range profiles {
		h := p.Config.Runner.Harness
		if h == "" {
			h = p.Config.Harness
		}
		if p.Config.SessionID != "" && p.Config.Replaced == "" {
			k := h + ":" + p.Config.SessionID
			out[k] = append(out[k], JoinedAs{Agent: p.Config.Agent, Hub: p.Config.Hub})
		}
	}
	return out
}

// readClaudeSession reads a Claude Code transcript's folder and first
// request; first identifies the conversation across its copies (the time
// and text of the first request, which a copy keeps).
func readClaudeSession(path string) (s LocalSession, first string, ok bool) {
	s = LocalSession{CLI: "claude", SessionID: strings.TrimSuffix(filepath.Base(path), ".jsonl")}
	entry := ""
	eachLine(path, func(line []byte) bool {
		var e struct {
			Type        string `json:"type"`
			Cwd         string `json:"cwd"`
			IsSidechain bool   `json:"isSidechain"`
			Timestamp   string `json:"timestamp"`
			Entrypoint  string `json:"entrypoint"`
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
		if entry == "" && e.Entrypoint != "" {
			// "cli" in a terminal, "claude-desktop" in the desktop app,
			// "sdk-cli" for claude -p, "sdk-ts" / "sdk-py" for the Agent SDK.
			entry = e.Entrypoint
			s.Auto = strings.HasPrefix(entry, "sdk")
		}
		if first == "" && e.Type == "user" && !e.IsSidechain {
			text := contentText(e.Message.Content)
			if t := shorten(text); t != "" {
				s.Title = t
				first = e.Timestamp + "\x00" + strings.Join(strings.Fields(text), " ")
			}
		}
		return s.Workdir == "" || first == "" || entry == ""
	})
	return s, first, s.Workdir != ""
}

func readCodexSession(path string) (s LocalSession, first string, ok bool) {
	s = LocalSession{CLI: "codex"}
	eachLine(path, func(line []byte) bool {
		var e struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
			Payload   struct {
				ID         string          `json:"id"`
				Cwd        string          `json:"cwd"`
				Originator string          `json:"originator"`
				Source     json.RawMessage `json:"source"`
				Type       string          `json:"type"`
				Message    string          `json:"message"`
				Role       string          `json:"role"`
				Content    json.RawMessage `json:"content"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &e) != nil {
			return true
		}
		switch {
		case e.Type == "session_meta":
			s.SessionID, s.Workdir = e.Payload.ID, e.Payload.Cwd
			// codex exec: originator codex_exec, source "exec".
			s.Auto = e.Payload.Originator == "codex_exec" || string(e.Payload.Source) == `"exec"`
		case first != "":
		case e.Type == "event_msg" && e.Payload.Type == "user_message",
			// Newer versions write the request only as a message item.
			e.Type == "response_item" && e.Payload.Type == "message" && e.Payload.Role == "user":
			text := e.Payload.Message
			if e.Type == "response_item" {
				text = codexInputText(e.Payload.Content)
			}
			if t := shorten(text); t != "" && !strings.HasPrefix(t, "# AGENTS.md instructions") {
				s.Title = t
				first = e.Timestamp + "\x00" + strings.Join(strings.Fields(text), " ")
			}
		}
		return s.SessionID == "" || first == ""
	})
	return s, first, s.SessionID != "" && s.Workdir != ""
}

// titleTail is how much of a transcript's end is read for its title.
const titleTail = 1 << 20

// claudeTitle is the title Claude Code keeps for a session: one given with
// /rename (custom-title), else the one it made (ai-title, or summary in
// older versions); "" when there is none. They are appended as the
// session goes on, so the last of the file's end is the current one.
func claudeTitle(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > titleTail {
		if _, err := f.Seek(st.Size()-titleTail, io.SeekStart); err != nil {
			return ""
		}
	}
	var custom, made string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.Contains(line, []byte(`title"`)) && !bytes.Contains(line, []byte(`"summary"`)) {
			continue
		}
		var e struct {
			Type        string `json:"type"`
			CustomTitle string `json:"customTitle"`
			AITitle     string `json:"aiTitle"`
			Summary     string `json:"summary"`
		}
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		switch {
		case e.Type == "custom-title" && strings.TrimSpace(e.CustomTitle) != "":
			custom = e.CustomTitle
		case e.Type == "ai-title" && strings.TrimSpace(e.AITitle) != "":
			made = e.AITitle
		case e.Type == "summary" && strings.TrimSpace(e.Summary) != "":
			made = e.Summary
		}
	}
	if custom != "" {
		return custom
	}
	return made
}

// codexThreadNames are Codex's session titles by session id, from
// session_index.jsonl (the last line for a session is its current name).
func codexThreadNames(codexDir string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(filepath.Join(codexDir, "session_index.jsonl"))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var e struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
		}
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.ID != "" && strings.TrimSpace(e.ThreadName) != "" {
			out[e.ID] = e.ThreadName
		}
	}
	return out
}

// codexInputText is the text a person typed in a Codex message item.
func codexInputText(raw json.RawMessage) string {
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	json.Unmarshal(raw, &parts)
	for _, p := range parts {
		if p.Type == "input_text" && strings.TrimSpace(p.Text) != "" {
			return p.Text
		}
	}
	return ""
}

// firstText is the request's text without its time.
func firstText(first string) string {
	_, text, _ := strings.Cut(first, "\x00")
	return text
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
	sessions := b.Local.LocalSessions(time.Now())
	// Name the workspaces as this app does.
	if hubs, err := b.Store.List(); err == nil {
		name := func(url string) string {
			for _, h := range hubs {
				if config.SameHub(h.URL, url) {
					return h.Name
				}
			}
			return ""
		}
		for i := range sessions {
			for k := range sessions[i].JoinedAll {
				sessions[i].JoinedAll[k].HubName = name(sessions[i].JoinedAll[k].Hub)
			}
			if j := sessions[i].Joined; j != nil {
				j.HubName = name(j.Hub)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": nonNilSessions(sessions)})
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
		// Fork joins a copy of the session (the original stays the user's).
		Fork bool `json:"fork"`
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
	enroll := api.CreateEnrollmentRequest{NamePrefix: prefix, MaxUses: 1, TTLSeconds: 600,
		Labels: []string{"machine=" + device}, Note: "myco app: local session"}
	enr, err := c.CreateEnrollment(r.Context(), enroll, false)
	if refused(err) && b.Local.revivedHub(r.Context(), hub.URL) {
		// This computer's own workspace had stopped: started again.
		enr, err = c.CreateEnrollment(r.Context(), enroll, false)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	args := []string{"join", "--harness", in.CLI, "--session-id", in.SessionID, "--name", name}
	if in.Fork {
		// The copy gets its own name; the original session stays the user's.
		name += "-copy"
		args = []string{"join", "--harness", in.CLI, "--session-id", in.SessionID, "--name", name, "--fork"}
	}
	if d := fitDisplayName(in.DisplayName); d != "" {
		args = append(args, "--display-name", d)
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

// fitDisplayName is s on one line, cut to what a hub takes as a display
// name (80 bytes), with "…" when cut.
func fitDisplayName(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= 80 {
		return s
	}
	r := []rune(s)
	for len(string(r))+len("…") > 80 {
		r = r[:len(r)-1]
	}
	return strings.TrimSpace(string(r)) + "…"
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
