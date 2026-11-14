package codex

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chenhg5/myco/internal/harness"
)

// A Codex session file (~/.codex/sessions/YYYY/MM/DD/rollout-<ts>-<thread>.jsonl)
// records each item of a turn as {"type":"response_item","payload":{...}}:
// the agent's messages, reasoning summaries, and tool calls with their
// output. TranscriptEvents turns one such line into steps, for showing a
// session that no runner started (myco trace follow).

type rolloutLine struct {
	Type    string `json:"type"`
	Payload struct {
		Type    string          `json:"type"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
		Summary []struct {
			Text string `json:"text"`
		} `json:"summary"`
		Name      string          `json:"name"`
		Namespace string          `json:"namespace"`
		CallID    string          `json:"call_id"`
		Input     string          `json:"input"`
		Arguments string          `json:"arguments"`
		Output    json.RawMessage `json:"output"`
		Action    struct {
			Command []string `json:"command"`
		} `json:"action"`
	} `json:"payload"`
}

// TranscriptEvents turns one line of a Codex session file into steps.
//
// The end of a turn (an event_msg task_complete, or turn_aborted) is a
// "turn_end" step: the session stopped and waits.
func TranscriptEvents(line []byte) []harness.Event {
	var l rolloutLine
	if json.Unmarshal(line, &l) != nil {
		return nil
	}
	if l.Type == "event_msg" && (l.Payload.Type == "task_complete" || l.Payload.Type == "turn_aborted") {
		return []harness.Event{{Kind: "turn_end"}}
	}
	if l.Type != "response_item" {
		return nil
	}
	p := l.Payload
	switch p.Type {
	case "message":
		if p.Role != "assistant" {
			return nil // the prompt, instructions and the user's words
		}
		if text := strings.TrimSpace(contentText(p.Content)); text != "" {
			return []harness.Event{{Kind: "text", Text: text}}
		}
	case "reasoning":
		var parts []string
		for _, s := range p.Summary {
			if t := strings.TrimSpace(s.Text); t != "" {
				parts = append(parts, t)
			}
		}
		if len(parts) > 0 {
			return []harness.Event{{Kind: "thinking", Text: strings.Join(parts, "\n\n")}}
		}
	case "custom_tool_call", "function_call":
		if p.Name == "update_plan" { // its plan: shown as a checklist, as Claude Code's TodoWrite
			if plan := planChecklist(p.Arguments); plan != "" {
				return []harness.Event{{Kind: "tool", Tool: "TodoWrite", ID: p.CallID, Input: plan}}
			}
		}
		name, input := p.Name, p.Input
		if input == "" {
			input = p.Arguments
		}
		switch cmds := shellCommands(input); {
		case len(cmds) > 0: // exec wraps exec_command({cmd:...})
			name, input = "Bash", strings.Join(cmds, "\n")
		case waitsOnly(input):
			name, input = "Bash", "(waiting for the output of a running command)"
		}
		if p.Namespace != "" && name == p.Name {
			name = p.Namespace + "." + name
		}
		return []harness.Event{{Kind: "tool", Tool: name, ID: p.CallID, Input: strings.TrimSpace(input)}}
	case "local_shell_call":
		return []harness.Event{{Kind: "tool", Tool: "Bash", ID: p.CallID, Input: strings.Join(p.Action.Command, " ")}}
	case "custom_tool_call_output", "function_call_output", "local_shell_call_output":
		return []harness.Event{{Kind: "tool_result", ID: p.CallID, Output: strings.TrimSpace(contentText(p.Output))}}
	}
	return nil
}

// contentText is the text of a content field: a string, or a list of parts
// with text.
func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var out []string
	for _, p := range parts {
		if p.Text != "" {
			out = append(out, p.Text)
		}
	}
	return strings.Join(out, "")
}

// cmdArg finds exec_command({cmd: ...}) in an exec script: the command as a
// JavaScript string in double quotes, single quotes or backticks.
var cmdArg = regexp.MustCompile("\\bcmd\"?\\s*:\\s*(?:\"((?:[^\"\\\\]|\\\\.)*)\"|'((?:[^'\\\\]|\\\\.)*)'|`((?:[^`\\\\]|\\\\.)*)`)")

// shellCommands are the commands an exec script runs (exec_command({cmd:...})).
func shellCommands(script string) []string {
	var out []string
	for _, m := range cmdArg.FindAllStringSubmatch(script, -1) {
		cmd := m[1] + m[2] + m[3]
		if m[1] != "" { // a double-quoted string: undo its escapes
			var s string
			if json.Unmarshal([]byte(`"`+m[1]+`"`), &s) == nil {
				cmd = s
			}
		}
		if cmd = strings.TrimSpace(cmd); cmd != "" {
			out = append(out, cmd)
		}
	}
	return out
}

// waitsOnly: the script only reads the output of commands still running.
func waitsOnly(script string) bool {
	return strings.Contains(script, "write_stdin(") && !strings.Contains(script, "exec_command(") && !strings.Contains(script, "web__")
}

// TranscriptPath is the session file of Codex thread id, "" if none.
func TranscriptPath(id string) string {
	if id == "" {
		return ""
	}
	for _, pat := range transcriptPatterns(id) {
		if m, _ := filepath.Glob(pat); len(m) > 0 {
			return m[0]
		}
	}
	return ""
}

// planChecklist turns update_plan's arguments ({"plan":[{"step":"...",
// "status":"pending|in_progress|completed"}]}) into "[x] / [~] / [ ]" lines.
func planChecklist(args string) string {
	var a struct {
		Plan []struct {
			Step   string `json:"step"`
			Status string `json:"status"`
		} `json:"plan"`
	}
	if json.Unmarshal([]byte(args), &a) != nil {
		return ""
	}
	var lines []string
	for _, it := range a.Plan {
		if strings.TrimSpace(it.Step) == "" {
			continue
		}
		mark := "[ ] "
		switch it.Status {
		case "completed":
			mark = "[x] "
		case "in_progress":
			mark = "[~] "
		}
		lines = append(lines, mark+strings.TrimSpace(it.Step))
	}
	return strings.Join(lines, "\n")
}
