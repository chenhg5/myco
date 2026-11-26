package api

import (
	"bytes"
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const BrowserReplayFeature = "agent_replay"
const BrowserReplayGraphMax = 192 << 10
const BrowserReplayNodesMax = 200

// BrowserReplayGraph uses the same node JSON as recorded drafts. Validation
// permits a deliberately smaller execution surface than approved M2 scripts.
type BrowserReplayGraph struct {
	Nodes []json.RawMessage `json:"nodes"`
	Edges [][]string        `json:"edges"`
}
type BrowserReplayStep struct {
	Node  string `json:"node"`
	State string `json:"state"`
	MS    int64  `json:"ms"`
	Error string `json:"error,omitempty"`
}
type BrowserControlTab struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	Active bool   `json:"active"`
}
type BrowserReplayResult struct {
	OK        bool                       `json:"ok"`
	State     string                     `json:"state"`
	ErrorNode string                     `json:"error_node,omitempty"`
	Error     *ActionError               `json:"error,omitempty"`
	Steps     []BrowserReplayStep        `json:"steps"`
	Output    map[string]json.RawMessage `json:"output"`
	URL       string                     `json:"url"`
	Tabs      []BrowserControlTab        `json:"tabs"`
}

// BrowserReplayValidationError identifies a safe, well-formed node ID only;
// it never includes raw graph fields, locator contents or typed values.
type BrowserReplayValidationError struct{ Node string }

func (e *BrowserReplayValidationError) Error() string { return "invalid browser replay graph" }

var replayID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)
var replayInto = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,39}$`)

var replayNodeFields = map[string]string{
	"navigate": "url", "back": "", "forward": "", "reload": "",
	"click": "target button count", "type": "target value clear sensitive",
	"select": "target value", "press": "target key", "hover": "target",
	"scroll": "target dy value", "wait": "target wait value ms",
	"assert": "target check value", "extract": "target what attr into",
	"screenshot": "target clip full into", "tab": "tab url", "dialog": "dialog", "note": "note",
}

func replayStringsLiteral(value any) bool {
	switch v := value.(type) {
	case string:
		return !strings.Contains(v, "${")
	case map[string]any:
		for k, item := range v {
			if strings.Contains(k, "${") || !replayStringsLiteral(item) {
				return false
			}
		}
	case []any:
		for _, item := range v {
			if !replayStringsLiteral(item) {
				return false
			}
		}
	}
	return true
}
func replayObject(raw []byte, out any) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out) == nil
}
func replayString(fields map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(fields[key], &value)
	return value
}
func replayTarget(raw json.RawMessage) bool {
	var t struct {
		Locators      []struct{ Kind, Value, Role, Name string }
		Frame, Shadow []string
		Snapshot      string
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || !replayObject(raw, &t) || len(t.Locators) < 1 || len(t.Locators) > 12 || len(t.Frame) > 16 || len(t.Shadow) > 16 || len(t.Snapshot) > 256 {
		return false
	}
	for _, group := range [][]string{t.Frame, t.Shadow} {
		for _, value := range group {
			if value == "" || len(value) > 4096 {
				return false
			}
		}
	}
	for _, l := range t.Locators {
		if len(l.Value) > 8192 || len(l.Name) > 1024 || len(l.Role) > 80 {
			return false
		}
		switch l.Kind {
		case "testid", "label", "placeholder", "text", "css", "xpath":
			if l.Value == "" {
				return false
			}
		case "role":
			if l.Role == "" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// ValidateBrowserReplayGraph validates every node before any operation is
// dispatched. A component independently validates this untrusted boundary.
func ValidateBrowserReplayGraph(raw json.RawMessage) error {
	bad := func(id string) error {
		if !replayID.MatchString(id) {
			id = ""
		}
		return &BrowserReplayValidationError{Node: id}
	}
	if len(raw) > BrowserReplayGraphMax || !json.Valid(raw) {
		return bad("")
	}
	var decoded any
	if json.Unmarshal(raw, &decoded) != nil {
		return bad("")
	}
	var graph BrowserReplayGraph
	if !replayObject(raw, &graph) || len(graph.Nodes) == 0 || len(graph.Nodes) > BrowserReplayNodesMax || len(graph.Edges) > BrowserReplayNodesMax-1 {
		return bad("")
	}
	ids := map[string]bool{}
	for _, rawNode := range graph.Nodes {
		var fields map[string]json.RawMessage
		if json.Unmarshal(rawNode, &fields) != nil || fields == nil {
			return bad("")
		}
		id, kind := replayString(fields, "id"), replayString(fields, "type")
		var literal any
		if json.Unmarshal(rawNode, &literal) != nil || !replayStringsLiteral(literal) {
			return bad(id)
		}
		if !replayID.MatchString(id) || ids[id] {
			return bad(id)
		}
		ids[id] = true
		specific, allowed := replayNodeFields[kind]
		if !allowed {
			return bad(id)
		}
		allowedFields := "id type pos title timeout_ms on_error " + specific
		for key, value := range fields {
			if !containsBrowserField(allowedFields, key) || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return bad(id)
			}
			var destination any
			switch key {
			case "target":
				if !replayTarget(value) {
					return bad(id)
				}
				continue
			case "pos":
				var pos []float64
				if json.Unmarshal(value, &pos) != nil || len(pos) != 2 {
					return bad(id)
				}
				continue
			case "clip":
				var coordinates map[string]json.RawMessage
				if json.Unmarshal(value, &coordinates) != nil || len(coordinates) != 4 || coordinates["x"] == nil || coordinates["y"] == nil || coordinates["width"] == nil || coordinates["height"] == nil {
					return bad(id)
				}
				var clip struct{ X, Y, Width, Height float64 }
				if !replayObject(value, &clip) || clip.X < 0 || clip.Y < 0 || clip.Width <= 0 || clip.Height <= 0 || clip.Width > 16384 || clip.Height > 16384 {
					return bad(id)
				}
				continue
			case "timeout_ms", "ms", "count":
				destination = new(int)
			case "dy":
				destination = new(float64)
			case "full", "clear", "sensitive":
				destination = new(bool)
			default:
				destination = new(string)
			}
			if json.Unmarshal(value, destination) != nil {
				return bad(id)
			}
		}
		number := func(key string) int { var n int; _ = json.Unmarshal(fields[key], &n); return n }
		if fields["timeout_ms"] != nil && (number("timeout_ms") < 1 || number("timeout_ms") > 300000) {
			return bad(id)
		}
		policy := replayString(fields, "on_error")
		if policy != "" && policy != "stop" && policy != "skip" {
			n, err := strconv.Atoi(strings.TrimPrefix(policy, "retry:"))
			if !strings.HasPrefix(policy, "retry:") || err != nil || n < 1 || n > 5 || policy != "retry:"+strconv.Itoa(n) {
				return bad(id)
			}
		}
		target := fields["target"] != nil
		requiredText := func(key string) bool { return fields[key] != nil && replayString(fields, key) != "" }
		switch kind {
		case "navigate":
			if len(replayString(fields, "url")) > 8192 || !browserControlURL(replayString(fields, "url")) {
				return bad(id)
			}
		case "click":
			if !target || number("count") < 0 || number("count") > 2 || replayString(fields, "button") != "" && replayString(fields, "button") != "left" {
				return bad(id)
			}
		case "type", "select":
			var sensitive bool
			_ = json.Unmarshal(fields["sensitive"], &sensitive)
			if !target || fields["value"] == nil || sensitive {
				return bad(id)
			}
		case "press":
			if !requiredText("key") || !replayKeyAllowed(replayString(fields, "key")) {
				return bad(id)
			}
		case "hover":
			if !target {
				return bad(id)
			}
		case "scroll":
			var dy float64
			_ = json.Unmarshal(fields["dy"], &dy)
			value := replayString(fields, "value")
			if fields["dy"] == nil || math.Abs(dy) > 100000 || value != "" && value != "absolute" {
				return bad(id)
			}
		case "wait":
			switch replayString(fields, "wait") {
			case "element":
				if !target {
					return bad(id)
				}
			case "url":
				if !requiredText("value") {
					return bad(id)
				}
			case "network":
			case "time":
				if number("ms") < 1 || number("ms") > 60000 {
					return bad(id)
				}
			default:
				return bad(id)
			}
		case "assert":
			switch replayString(fields, "check") {
			case "visible", "hidden":
				if !target {
					return bad(id)
				}
			case "text":
				if !target || !requiredText("value") {
					return bad(id)
				}
			case "url":
				if !requiredText("value") {
					return bad(id)
				}
			default:
				return bad(id)
			}
		case "extract":
			if !target {
				return bad(id)
			}
			switch replayString(fields, "what") {
			case "text", "html", "table":
			case "attribute":
				if !requiredText("attr") || len(replayString(fields, "attr")) > 160 {
					return bad(id)
				}
			default:
				return bad(id)
			}
		case "screenshot":
			var full bool
			_ = json.Unmarshal(fields["full"], &full)
			if (target && fields["clip"] != nil) || (full && (target || fields["clip"] != nil)) {
				return bad(id)
			}
		case "tab":
			tab := replayString(fields, "tab")
			if tab != "new" && tab != "popup" && tab != "opener" && tab != "close" {
				index, err := strconv.Atoi(tab)
				if err != nil || index < 0 || index > 31 {
					return bad(id)
				}
			}
			if fields["url"] != nil {
				if tab != "new" || len(replayString(fields, "url")) > 8192 || !browserControlURL(replayString(fields, "url")) {
					return bad(id)
				}
			}
		case "dialog":
			value := replayString(fields, "dialog")
			if value != "accept" && value != "dismiss" {
				return bad(id)
			}
		}
		if fields["into"] != nil && !replayInto.MatchString(replayString(fields, "into")) {
			return bad(id)
		}
	}
	incoming, next := map[string]int{}, map[string]string{}
	for _, edge := range graph.Edges {
		if len(edge) != 2 || !ids[edge[0]] || !ids[edge[1]] || edge[0] == edge[1] || next[edge[0]] != "" {
			return bad("")
		}
		next[edge[0]] = edge[1]
		incoming[edge[1]]++
		if incoming[edge[1]] > 1 {
			return bad(edge[1])
		}
	}
	start := ""
	for id := range ids {
		if incoming[id] == 0 {
			if start != "" {
				return bad("")
			}
			start = id
		}
	}
	visited := map[string]bool{}
	for id := start; id != ""; id = next[id] {
		if visited[id] {
			return bad(id)
		}
		visited[id] = true
	}
	if len(visited) != len(ids) {
		return bad("")
	}
	return nil
}

func replayKeyAllowed(value string) bool {
	parts := strings.Split(value, "+")
	key := parts[len(parts)-1]
	modifiers := parts[:len(parts)-1]
	if key == "" && strings.HasSuffix(value, "+") {
		key = "+"
		if len(modifiers) > 0 && modifiers[len(modifiers)-1] == "" {
			modifiers = modifiers[:len(modifiers)-1]
		}
	}
	if len(modifiers) > 4 {
		return false
	}
	for _, modifier := range modifiers {
		switch modifier {
		case "Alt", "Control", "Ctrl", "Meta", "Command", "Shift":
		default:
			return false
		}
	}
	lower := strings.ToLower(value)
	if strings.Contains(lower, "+v") || lower == "shift+insert" {
		return false
	}
	switch key {
	case "Enter", "Tab", "Escape", "Backspace", "Delete", "ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "Home", "End", "PageUp", "PageDown", "Insert":
		return true
	}
	return len(key) == 1 && key[0] >= 32 && key[0] <= 126
}
