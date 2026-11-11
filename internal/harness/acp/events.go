package acp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/harness"
)

// ACP session/update payloads (the subset myco shows).
type update struct {
	SessionUpdate string          `json:"sessionUpdate"`
	Content       json.RawMessage `json:"content"` // a block for chunks, a list for tool calls
	ToolCallID    string          `json:"toolCallId"`
	Title         string          `json:"title"`
	Kind          string          `json:"kind"`
	Status        string          `json:"status"`
	Name          string          `json:"name"`
	Locations     []struct {
		Path string `json:"path"`
	} `json:"locations"`
	RawInput  json.RawMessage `json:"rawInput"`
	RawOutput json.RawMessage `json:"rawOutput"`
	Entries   []planEntry     `json:"entries"`
	Cost      *struct {
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
	} `json:"cost"`
}

type planEntry struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

type block struct {
	Type string `json:"type"`
	Text string `json:"text"`
	URI  string `json:"uri"`
}

type toolContent struct {
	Type    string  `json:"type"` // content, diff, terminal
	Content block   `json:"content"`
	Path    string  `json:"path"`
	OldText *string `json:"oldText"`
	NewText string  `json:"newText"`
}

// promptResponse is session/prompt's result. usage is not in every agent's
// answer (Gemini CLI sends it).
type promptResponse struct {
	StopReason string `json:"stopReason"`
	Usage      *struct {
		InputTokens      int64 `json:"inputTokens"`
		OutputTokens     int64 `json:"outputTokens"`
		TotalTokens      int64 `json:"totalTokens"`
		CachedReadTokens int64 `json:"cachedReadTokens"`
	} `json:"usage"`
}

// toolState is what is known of a tool call so far: agents announce a call
// before they know much (opencode titles it "bash" until it runs) and fill
// it in with updates.
func (r *promptResponse) usage() *struct {
	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	TotalTokens      int64 `json:"totalTokens"`
	CachedReadTokens int64 `json:"cachedReadTokens"`
} {
	if r == nil {
		return nil
	}
	return r.Usage
}

type toolState struct {
	call    update
	emitted bool
	done    bool
}

// merge takes the fields an update sets.
func (t *toolState) merge(u update) {
	if u.Title != "" {
		t.call.Title = u.Title
	}
	if u.Kind != "" {
		t.call.Kind = u.Kind
	}
	if u.Name != "" {
		t.call.Name = u.Name
	}
	if len(u.Locations) > 0 {
		t.call.Locations = u.Locations
	}
	if len(u.RawInput) > 0 && string(u.RawInput) != "null" && string(u.RawInput) != "{}" {
		t.call.RawInput = u.RawInput
	}
}

// stream turns a prompt turn's updates into the run's steps and its answer.
// Text and thoughts arrive in small chunks: they are joined and reported as
// one step each when something else starts.
type stream struct {
	mu      sync.Mutex
	emit    func(harness.Event) // nil: only the answer is kept
	workdir string
	start   time.Time
	on      bool // the prompt was sent: updates before it replay history

	text, thought strings.Builder
	answer        string // the latest finished text: the reply
	tools         map[string]*toolState
	plan          []planEntry
	cost          float64
}

func newStream(emit func(harness.Event), workdir string) *stream {
	return &stream{emit: emit, workdir: workdir, tools: map[string]*toolState{}}
}

// begin is called right before the prompt is sent.
func (s *stream) begin() {
	s.mu.Lock()
	s.on, s.start = true, time.Now()
	s.mu.Unlock()
}

func (s *stream) send(ev harness.Event) {
	if s.emit != nil {
		s.emit(ev)
	}
}

func (s *stream) update(raw json.RawMessage) {
	var u update
	if json.Unmarshal(raw, &u) != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.on {
		return
	}
	switch u.SessionUpdate {
	case "agent_message_chunk":
		s.flushThought()
		var b block
		if json.Unmarshal(u.Content, &b) == nil && b.Type == "text" {
			s.text.WriteString(b.Text)
		}
	case "agent_thought_chunk":
		s.flushText()
		var b block
		if json.Unmarshal(u.Content, &b) == nil && b.Type == "text" {
			s.thought.WriteString(b.Text)
		}
	case "tool_call", "tool_call_update":
		s.flushText()
		s.flushThought()
		s.tool(u)
	case "plan":
		s.flushText()
		s.flushThought()
		if s.plan == nil && len(u.Entries) > 0 {
			s.send(harness.Event{Kind: "tool", Tool: "TodoWrite", ID: "acp-plan", Input: fmt.Sprintf("%d steps", len(u.Entries))})
		}
		if len(u.Entries) > 0 {
			s.plan = u.Entries
		}
	case "usage_update":
		if u.Cost != nil && strings.EqualFold(u.Cost.Currency, "USD") {
			s.cost = u.Cost.Amount
		}
	}
}

func (s *stream) flushText() {
	if t := strings.TrimSpace(s.text.String()); t != "" {
		s.answer = t
		s.send(harness.Event{Kind: "text", Text: t})
	}
	s.text.Reset()
}

func (s *stream) flushThought() {
	if t := strings.TrimSpace(s.thought.String()); t != "" {
		s.send(harness.Event{Kind: "thinking", Text: t})
	}
	s.thought.Reset()
}

// tool follows a call: it is shown once it runs, asks for permission or
// ends (by then the agent has said what it does), then its result.
func (s *stream) tool(u update) {
	if u.ToolCallID == "" {
		return
	}
	st := s.state(u.ToolCallID)
	st.merge(u)
	if u.Status == "pending" || u.Status == "" && !st.emitted && u.SessionUpdate == "tool_call" {
		return
	}
	s.show(u.ToolCallID, st)
	if st.done || (u.Status != "completed" && u.Status != "failed") {
		return
	}
	st.done = true
	s.send(harness.Event{Kind: "tool_result", ID: u.ToolCallID, Output: toolOutput(u), Failed: u.Status == "failed"})
}

func (s *stream) state(id string) *toolState {
	st := s.tools[id]
	if st == nil {
		st = &toolState{}
		s.tools[id] = st
	}
	return st
}

func (s *stream) show(id string, st *toolState) {
	if st.emitted {
		return
	}
	st.emitted = true
	s.send(harness.Event{Kind: "tool", Tool: toolName(st.call), ID: id, Input: s.toolInput(st.call)})
}

// asked: the agent asks permission for a call, so it is about to run it
// (or be refused).
func (s *stream) asked(u update) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.on || u.ToolCallID == "" {
		return
	}
	s.flushText()
	s.flushThought()
	st := s.state(u.ToolCallID)
	st.merge(u)
	s.show(u.ToolCallID, st)
}

// toolName maps ACP's tool kinds to the names the app knows (Claude Code's).
func toolName(u update) string {
	switch u.Kind {
	case "execute":
		return "Bash"
	case "read":
		return "Read"
	case "edit", "delete", "move":
		return "Edit"
	case "search":
		return "Grep"
	case "fetch":
		return "WebFetch"
	case "think":
		return "Think"
	}
	if u.Name != "" {
		return u.Name
	}
	return "Tool"
}

// toolInput: the command for a shell call, the file for file tools (relative
// to the working directory), else the agent's own title.
func (s *stream) toolInput(u update) string {
	if u.Kind == "execute" {
		if c := command(u); c != "" {
			return c
		}
	}
	if u.Kind != "execute" && len(u.Locations) > 0 && u.Locations[0].Path != "" {
		p := u.Locations[0].Path
		if s.workdir != "" {
			if rel, err := filepath.Rel(s.workdir, p); err == nil && !strings.HasPrefix(rel, "..") {
				p = rel
			}
		}
		return p
	}
	if u.Title != "" {
		return u.Title
	}
	if len(u.RawInput) > 0 && string(u.RawInput) != "null" {
		return string(u.RawInput)
	}
	return ""
}

func toolOutput(u update) string {
	var items []toolContent
	_ = json.Unmarshal(u.Content, &items)
	var parts []string
	for _, c := range items {
		switch c.Type {
		case "content":
			if c.Content.Text != "" {
				parts = append(parts, c.Content.Text)
			} else if c.Content.URI != "" {
				parts = append(parts, c.Content.URI)
			}
		case "diff":
			parts = append(parts, diffSummary(c))
		}
	}
	if len(parts) == 0 && len(u.RawOutput) > 0 && string(u.RawOutput) != "null" {
		parts = append(parts, string(u.RawOutput))
	}
	return strings.Join(parts, "\n")
}

// diffSummary lists a file change's removed and added lines (a rough diff:
// lines that are only in the old or only in the new text).
func diffSummary(c toolContent) string {
	old := ""
	if c.OldText != nil {
		old = *c.OldText
	}
	oldLines, newLines := strings.Split(old, "\n"), strings.Split(c.NewText, "\n")
	in := func(lines []string) map[string]int {
		m := map[string]int{}
		for _, l := range lines {
			m[l]++
		}
		return m
	}
	oldSet, newSet := in(oldLines), in(newLines)
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", c.Path)
	for _, l := range oldLines {
		if newSet[l] == 0 && old != "" {
			b.WriteString("- " + l + "\n")
		}
	}
	for _, l := range newLines {
		if oldSet[l] == 0 {
			b.WriteString("+ " + l + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// finish closes the turn: what is left of the text, the plan, the usage.
func (s *stream) finish(resp *promptResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushThought()
	s.flushText()
	for id, st := range s.tools {
		s.show(id, st) // announced but never run (the turn ended first)
	}
	if s.plan != nil {
		var b strings.Builder
		for _, e := range s.plan {
			mark := "[ ]"
			switch e.Status {
			case "completed":
				mark = "[x]"
			case "in_progress":
				mark = "[~]"
			}
			b.WriteString(mark + " " + e.Content + "\n")
		}
		s.send(harness.Event{Kind: "tool_result", ID: "acp-plan", Output: strings.TrimSpace(b.String())})
	}
	ev := harness.Event{Kind: "usage", CostUSD: s.cost}
	if !s.start.IsZero() {
		ev.DurationMS = time.Since(s.start).Milliseconds()
	}
	if u := resp.usage(); u != nil {
		// Input counts cached tokens too, as Claude Code's does: the
		// total less the output where the agent gives a total.
		ev.InputTokens, ev.OutputTokens = u.InputTokens+u.CachedReadTokens, u.OutputTokens
		if u.TotalTokens > u.OutputTokens {
			ev.InputTokens = u.TotalTokens - u.OutputTokens
		}
	}
	s.send(ev)
}

func (s *stream) failed(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.send(harness.Event{Kind: "error", Text: msg})
}

// output is the reply: the agent's last message.
func (s *stream) output() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.answer
}
