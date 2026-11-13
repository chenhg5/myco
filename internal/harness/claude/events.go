package claude

import (
	"encoding/json"
	"strings"

	"github.com/chenhg5/myco/internal/harness"
)

// streamLine is one line of `--output-format stream-json`, for the live view.
type streamLine struct {
	Type    string `json:"type"`
	Message struct {
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
	Usage        struct {
		InputTokens              int64 `json:"input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	} `json:"usage"`
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
