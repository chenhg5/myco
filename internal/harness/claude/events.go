package claude

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/harness"
)

// streamLine is one line of `--output-format stream-json`, for the live view.
type streamLine struct {
	Type    string `json:"type"`
	Message struct {
		ID      string `json:"id"`
		Content []struct {
			Type      string          `json:"type"`
			Text      string          `json:"text"`
			Thinking  string          `json:"thinking"`
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			Input     json.RawMessage `json:"input"`
			ToolUseID string          `json:"tool_use_id"`
			Content   json.RawMessage `json:"content"`
			IsError   bool            `json:"is_error"`
		} `json:"content"`
	} `json:"message"`
	// result
	IsError      bool    `json:"is_error"`
	Result       string  `json:"result"`
	DurationMS   int64   `json:"duration_ms"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	// rate_limit_event: the account's usage limits.
	RateLimitInfo *rateLimitInfo `json:"rate_limit_info"`
	Usage         struct {
		InputTokens              int64 `json:"input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

// streamEvent is the payload of a stream_event line
// (--include-partial-messages): the API's own streaming events.
type streamEvent struct {
	Type  string `json:"type"`
	Event struct {
		Type    string `json:"type"`
		Index   int    `json:"index"`
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
		Delta struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		} `json:"delta"`
	} `json:"event"`
}

// liveParser turns stream-json lines into steps, including the text and
// thinking of a block while it is being written (Kind "delta", Stream
// "text" or "thinking", ID "<message>/<block>"). The finished block still
// arrives as its own text or thinking step and replaces the deltas.
type liveParser struct{ msg string }

func (p *liveParser) events(line []byte) []harness.Event {
	var l streamEvent
	if json.Unmarshal(line, &l) != nil {
		return nil
	}
	if l.Type != "stream_event" {
		return lineEvents(line)
	}
	e := l.Event
	switch e.Type {
	case "message_start":
		p.msg = e.Message.ID
	case "content_block_delta":
		id := p.msg + "/" + strconv.Itoa(e.Index)
		switch e.Delta.Type {
		case "text_delta":
			if e.Delta.Text != "" {
				return []harness.Event{{Kind: "delta", Stream: "text", ID: id, Text: e.Delta.Text}}
			}
		case "thinking_delta":
			if e.Delta.Thinking != "" {
				return []harness.Event{{Kind: "delta", Stream: "thinking", ID: id, Text: e.Delta.Thinking}}
			}
		}
	}
	return nil
}

// lineEvents turns one stream-json line into steps.
func lineEvents(line []byte) []harness.Event {
	var l streamLine
	if json.Unmarshal(line, &l) != nil {
		return nil
	}
	var out []harness.Event
	switch l.Type {
	case "assistant":
		for _, c := range l.Message.Content {
			switch c.Type {
			case "text":
				if strings.TrimSpace(c.Text) != "" {
					out = append(out, harness.Event{Kind: "text", Text: c.Text})
				}
			case "thinking":
				if strings.TrimSpace(c.Thinking) != "" {
					out = append(out, harness.Event{Kind: "thinking", Text: c.Thinking})
				}
			case "tool_use":
				out = append(out, harness.Event{Kind: "tool", Tool: c.Name, ID: c.ID, Input: toolInput(c.Name, c.Input)})
			}
		}
	case "user":
		for _, c := range l.Message.Content {
			if c.Type == "tool_result" {
				out = append(out, harness.Event{Kind: "tool_result", ID: c.ToolUseID, Output: resultText(c.Content), Failed: c.IsError})
			}
		}
	case "rate_limit_event":
		if q := l.RateLimitInfo.quota(); q != nil {
			out = append(out, harness.Event{Kind: "quota", Quota: q})
		}
	case "result":
		u := l.Usage
		out = append(out, harness.Event{Kind: "usage", InputTokens: u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens,
			OutputTokens: u.OutputTokens, CostUSD: l.TotalCostUSD, DurationMS: l.DurationMS})
		if l.IsError && l.Result != "" {
			out = append(out, harness.Event{Kind: "error", Text: l.Result})
		}
	}
	return out
}

// toolInput summarizes a tool call: the command, path or pattern it is
// about, else its arguments as compact JSON.
func toolInput(tool string, raw json.RawMessage) string {
	var in map[string]any
	if json.Unmarshal(raw, &in) != nil {
		return ""
	}
	str := func(k string) string { s, _ := in[k].(string); return s }
	switch tool {
	case "Bash":
		return str("command")
	case "Read", "Write", "Edit", "MultiEdit", "NotebookEdit":
		return orStr(str("file_path"), str("notebook_path"))
	case "Grep", "Glob":
		if p := str("path"); p != "" {
			return str("pattern") + "  (" + p + ")"
		}
		return str("pattern")
	case "WebFetch":
		return str("url")
	case "WebSearch":
		return str("query")
	case "Task", "Agent":
		return orStr(str("description"), str("prompt"))
	case "TodoWrite":
		// The plan as a checklist, one item a line, as Codex's is: "[x]"
		// done, "[~]" being done, "[ ]" still to do (the app shows it as
		// a list being ticked off).
		if todos, ok := in["todos"].([]any); ok {
			var items []string
			for _, t := range todos {
				if m, ok := t.(map[string]any); ok {
					if c, _ := m["content"].(string); c != "" {
						mark := "[ ] "
						switch m["status"] {
						case "completed":
							mark = "[x] "
						case "in_progress":
							mark = "[~] "
						}
						items = append(items, mark+c)
					}
				}
			}
			return strings.Join(items, "\n")
		}
	}
	b, _ := json.Marshal(in)
	return string(b)
}

// resultText flattens a tool_result's content (a string, or text blocks).
func resultText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			} else if b.Type != "" {
				parts = append(parts, "["+b.Type+"]")
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// rateLimitInfo is Claude Code's rate_limit_event payload, e.g.
// {"status":"allowed","rateLimitType":"five_hour","resetsAt":1794587400,
// "unifiedWindows":{"five_hour":{"utilization":0.17,"resetsAt":...,"status":"allowed"},
// "seven_day":{...}}}.
type rateLimitInfo struct {
	Status         string `json:"status"`
	RateLimitType  string `json:"rateLimitType"`
	ResetsAt       int64  `json:"resetsAt"`
	UnifiedWindows map[string]struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    int64   `json:"resetsAt"`
		Status      string  `json:"status"`
	} `json:"unifiedWindows"`
}

func (r *rateLimitInfo) quota() *harness.Quota {
	if r == nil {
		return nil
	}
	q := &harness.Quota{Source: "claude"}
	for _, name := range []string{"five_hour", "seven_day"} {
		if w, ok := r.UnifiedWindows[name]; ok {
			q.Windows = append(q.Windows, harness.QuotaWindow{Name: name, Used: w.Utilization, ResetsAt: unix(w.ResetsAt), Status: w.Status})
		}
	}
	var others []string // any other window, after the known two, in name order
	for name := range r.UnifiedWindows {
		if name != "five_hour" && name != "seven_day" {
			others = append(others, name)
		}
	}
	sort.Strings(others)
	for _, name := range others {
		w := r.UnifiedWindows[name]
		q.Windows = append(q.Windows, harness.QuotaWindow{Name: name, Used: w.Utilization, ResetsAt: unix(w.ResetsAt), Status: w.Status})
	}
	if len(q.Windows) == 0 && r.RateLimitType != "" { // older shape: one window, no utilization
		q.Windows = append(q.Windows, harness.QuotaWindow{Name: r.RateLimitType, ResetsAt: unix(r.ResetsAt), Status: r.Status})
	}
	if len(q.Windows) == 0 {
		return nil
	}
	return q
}

func unix(s int64) time.Time {
	if s <= 0 {
		return time.Time{}
	}
	return time.Unix(s, 0).UTC()
}

// TranscriptEvents turns one line of a Claude Code session file
// (~/.claude/projects/<project>/<session>.jsonl) into steps. Those lines
// carry the same assistant and user messages as stream-json.
func TranscriptEvents(line []byte) []harness.Event {
	var out []harness.Event
	for _, ev := range lineEvents(line) {
		if ev.Kind != "quota" && ev.Kind != "usage" {
			out = append(out, ev)
		}
	}
	return out
}

// TranscriptPath is the session file of a Claude Code session, "" if none.
func TranscriptPath(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	for _, pat := range transcriptPatterns(sessionID) {
		if m, _ := filepath.Glob(pat); len(m) > 0 {
			return m[0]
		}
	}
	return ""
}
