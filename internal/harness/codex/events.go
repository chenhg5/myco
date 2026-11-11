package codex

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/harness"
)

// streamLine is one line of `codex exec --json`, for the live view.
type streamLine struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error"`
	Usage *struct {
		InputTokens       int64 `json:"input_tokens"`
		CachedInputTokens int64 `json:"cached_input_tokens"`
		OutputTokens      int64 `json:"output_tokens"`
	} `json:"usage"`
	Item struct {
		ID               string `json:"id"`
		Type             string `json:"type"`
		Text             string `json:"text"`
		Message          string `json:"message"`
		Command          string `json:"command"`
		AggregatedOutput string `json:"aggregated_output"`
		ExitCode         *int   `json:"exit_code"`
		Status           string `json:"status"`
		Changes          []struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"changes"`
		Server string `json:"server"`
		Tool   string `json:"tool"`
		Query  string `json:"query"`
		Items  []struct {
			Text      string `json:"text"`
			Completed bool   `json:"completed"`
		} `json:"items"`
	} `json:"item"`
}

// eventStream turns Codex's lines into steps. A command is announced when it
// starts and finished when it completes; items that only complete get both.
type eventStream struct {
	start   time.Time
	started map[string]bool
}

func newEventStream(start time.Time) *eventStream {
	return &eventStream{start: start, started: map[string]bool{}}
}

func (s *eventStream) line(b []byte) []harness.Event {
	var l streamLine
	if json.Unmarshal(b, &l) != nil {
		return nil
	}
	switch l.Type {
	case "item.started", "item.updated", "item.completed":
		return s.item(l)
	case "turn.completed":
		if l.Usage == nil {
			return nil
		}
		return []harness.Event{{Kind: "usage", InputTokens: l.Usage.InputTokens, OutputTokens: l.Usage.OutputTokens,
			DurationMS: time.Since(s.start).Milliseconds()}}
	case "turn.failed":
		if l.Error != nil && l.Error.Message != "" {
			return []harness.Event{{Kind: "error", Text: unwrapAPIError(l.Error.Message)}}
		}
	case "error":
		if l.Message != "" {
			return []harness.Event{{Kind: "error", Text: unwrapAPIError(l.Message)}}
		}
	}
	return nil
}

func (s *eventStream) item(l streamLine) []harness.Event {
	it := l.Item
	done := l.Type == "item.completed"
	switch it.Type {
	case "agent_message":
		if done && strings.TrimSpace(it.Text) != "" {
			return []harness.Event{{Kind: "text", Text: it.Text}}
		}
	case "reasoning":
		if done && strings.TrimSpace(it.Text) != "" {
			return []harness.Event{{Kind: "thinking", Text: it.Text}}
		}
	case "command_execution":
		call := harness.Event{Kind: "tool", Tool: "Bash", ID: it.ID, Input: unwrapShell(it.Command)}
		failed := it.Status == "failed" || it.Status == "declined" || (it.ExitCode != nil && *it.ExitCode != 0)
		return s.call(call, done, harness.Event{Kind: "tool_result", ID: it.ID, Output: it.AggregatedOutput, Failed: failed})
	case "file_change":
		var paths, lines []string
		for _, c := range it.Changes {
			paths = append(paths, c.Path)
			lines = append(lines, c.Kind+" "+c.Path)
		}
		call := harness.Event{Kind: "tool", Tool: "Edit", ID: it.ID, Input: strings.Join(paths, ", ")}
		return s.call(call, done, harness.Event{Kind: "tool_result", ID: it.ID, Output: strings.Join(lines, "\n"), Failed: it.Status == "failed"})
	case "mcp_tool_call":
		name := it.Tool
		if it.Server != "" {
			name = it.Server + "." + it.Tool
		}
		call := harness.Event{Kind: "tool", Tool: name, ID: it.ID}
		return s.call(call, done, harness.Event{Kind: "tool_result", ID: it.ID, Failed: it.Status == "failed"})
	case "web_search":
		call := harness.Event{Kind: "tool", Tool: "WebSearch", ID: it.ID, Input: it.Query}
		return s.call(call, done, harness.Event{Kind: "tool_result", ID: it.ID})
	case "todo_list":
		// The plan as it starts and as it ends (updates in between are noise).
		if l.Type == "item.updated" {
			return nil
		}
		var items []string
		for _, t := range it.Items {
			mark := "[ ] "
			if t.Completed {
				mark = "[x] "
			}
			items = append(items, mark+t.Text)
		}
		id := it.ID + "/start"
		if done {
			id = it.ID + "/end"
		}
		return []harness.Event{{Kind: "tool", Tool: "TodoWrite", ID: id, Input: strings.Join(items, "\n")}, {Kind: "tool_result", ID: id}}
	case "error":
		if done && it.Message != "" {
			return []harness.Event{{Kind: "error", Text: it.Message}}
		}
	}
	return nil
}

// call announces a step once, and finishes it when it completes.
func (s *eventStream) call(call harness.Event, done bool, result harness.Event) []harness.Event {
	var out []harness.Event
	if !s.started[call.ID] {
		s.started[call.ID] = true
		out = append(out, call)
	}
	if done {
		out = append(out, result)
	}
	return out
}

// unwrapShell shows the command Codex ran, not its `bash -lc '…'` wrapper.
func unwrapShell(cmd string) string {
	for _, p := range []string{"/bin/bash -lc ", "bash -lc ", "/bin/zsh -lc ", "zsh -lc ", "/bin/sh -c ", "sh -c "} {
		if rest, ok := strings.CutPrefix(cmd, p); ok {
			rest = strings.TrimSpace(rest)
			if len(rest) >= 2 && (rest[0] == '\'' || rest[0] == '"') && rest[len(rest)-1] == rest[0] {
				rest = rest[1 : len(rest)-1]
			}
			return rest
		}
	}
	return cmd
}
