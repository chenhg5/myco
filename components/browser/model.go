// myco-browser is an optional local component. Its JSON models match the
// web-automation application; no Hub credentials or shared storage are used.
package main

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const protocol = 1

var version = "0.1.1"
var variableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,39}$`)
var idPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

type profile struct {
	Settings profileSettings `json:"settings"`
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Running  bool            `json:"running"`
	Agents   bool            `json:"agents"`
}
type locator struct {
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
	Role  string `json:"role,omitempty"`
	Name  string `json:"name,omitempty"`
}
type target struct {
	Locators []locator `json:"locators"`
	Frame    []string  `json:"frame,omitempty"`
	Shadow   []string  `json:"shadow,omitempty"`
	Snapshot string    `json:"snapshot,omitempty"`
}
type node struct {
	Dialog    string     `json:"dialog,omitempty"`
	ID        string     `json:"id"`
	Type      string     `json:"type"`
	Pos       [2]float64 `json:"pos"`
	Title     string     `json:"title,omitempty"`
	Target    *target    `json:"target,omitempty"`
	To        *target    `json:"to,omitempty"`
	URL       string     `json:"url,omitempty"`
	Value     string     `json:"value,omitempty"`
	Key       string     `json:"key,omitempty"`
	Code      string     `json:"code,omitempty"`
	Message   string     `json:"message,omitempty"`
	Note      string     `json:"note,omitempty"`
	Button    string     `json:"button,omitempty"`
	Count     int        `json:"count,omitempty"`
	Clear     bool       `json:"clear,omitempty"`
	Sensitive bool       `json:"sensitive,omitempty"`
	Wait      string     `json:"wait,omitempty"`
	MS        int        `json:"ms,omitempty"`
	DY        float64    `json:"dy,omitempty"`
	Check     string     `json:"check,omitempty"`
	What      string     `json:"what,omitempty"`
	Attr      string     `json:"attr,omitempty"`
	Into      string     `json:"into,omitempty"`
	Tab       string     `json:"tab,omitempty"`
	Full      bool       `json:"full,omitempty"`
	OnError   string     `json:"on_error,omitempty"`
	TimeoutMS int        `json:"timeout_ms,omitempty"`
}
type param struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
	Default  string `json:"default,omitempty"`
}
type script struct {
	Name     string  `json:"name"`
	StartURL string  `json:"start_url"`
	Profile  string  `json:"profile"`
	Params   []param `json:"params"`
	Graph    struct {
		Nodes []node     `json:"nodes"`
		Edges [][]string `json:"edges"`
	} `json:"graph"`
	OnError string `json:"on_error"`
}
type runOptions struct {
	Uploads       []string          `json:"uploads,omitempty"`
	Script        script            `json:"script"`
	ScriptID      string            `json:"script_id"`
	ScriptVersion int               `json:"script_version"`
	Params        map[string]string `json:"params"`
	Secrets       map[string]string `json:"secrets"`
	Profile       string            `json:"profile"`
	Headless      bool              `json:"headless"`
	StartAt       string            `json:"start_at,omitempty"`
	Breakpoints   []string          `json:"breakpoints,omitempty"`
}
type runStep struct {
	Node  string `json:"node"`
	State string `json:"state"`
	MS    int64  `json:"ms,omitempty"`
	Error string `json:"error,omitempty"`
	Shot  string `json:"shot,omitempty"`
}
type runUpdate struct {
	State     string         `json:"state"`
	Node      string         `json:"node"`
	Error     string         `json:"error"`
	ErrorNode string         `json:"error_node"`
	Question  string         `json:"question,omitempty"`
	Steps     []runStep      `json:"steps"`
	Next      int            `json:"next"`
	Output    map[string]any `json:"output"`
}

// M0/M1 intentionally accept one connected acyclic sequence. Unsupported
// graph types fail before opening a browser instead of silently misrunning.
func linearOrder(s script, start string) ([]node, error) {
	if len(s.Graph.Nodes) > 128 {
		return nil, errors.New("invalid_graph")
	}
	if !validPolicy(s.OnError) {
		return nil, errors.New("unsupported_branch")
	}
	seenIDs := map[string]bool{}
	byID, incoming, next := map[string]node{}, map[string]int{}, map[string]string{}
	allowed := map[string]bool{}
	for _, k := range []string{"navigate", "click", "type", "select", "press", "hover", "drag", "scroll", "upload", "wait", "assert", "extract", "js", "screenshot", "tab", "dialog", "confirm", "note"} {
		allowed[k] = true
	}
	for _, n := range s.Graph.Nodes {
		if !idPattern.MatchString(n.ID) || seenIDs[n.ID] {
			return nil, errors.New("invalid_graph")
		}
		seenIDs[n.ID] = true
		if !validPolicy(n.OnError) {
			return nil, errors.New("unsupported_branch")
		}
		if !allowed[n.Type] {
			return nil, fmt.Errorf("unsupported_node: %s", n.Type)
		}
		if n.Sensitive && !regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]{0,39}\}$`).MatchString(n.Value) {
			return nil, errors.New("sensitive_literal")
		}
		if n.Type == "dialog" && n.Dialog != "accept" && n.Dialog != "dismiss" {
			return nil, errors.New("invalid_argument")
		}
		if n.Type == "tab" && !validTab(n.Tab) {
			return nil, errors.New("invalid_argument")
		}
		if n.Type == "wait" && n.Wait == "url" && n.Value == "" {
			return nil, errors.New("invalid_argument")
		}
		if n.Type != "note" {
			byID[n.ID] = n
		}
	}
	for _, edge := range s.Graph.Edges {
		if len(edge) < 2 || len(edge) > 3 || byID[edge[0]].ID == "" || byID[edge[1]].ID == "" || next[edge[0]] != "" {
			return nil, errors.New("invalid_graph")
		}
		if len(edge) == 3 && (byID[edge[0]].Type != "confirm" || edge[2] != "ok") {
			return nil, errors.New("unsupported_branch")
		}
		next[edge[0]] = edge[1]
		incoming[edge[1]]++
		if incoming[edge[1]] > 1 {
			return nil, errors.New("invalid_graph")
		}
	}
	root := ""
	for id := range byID {
		if incoming[id] == 0 {
			if root != "" {
				return nil, errors.New("many_starts")
			}
			root = id
		}
	}
	if root == "" {
		return nil, errors.New("invalid_graph")
	}
	ordered, seen := []node{}, map[string]bool{}
	for id := root; id != ""; id = next[id] {
		if seen[id] {
			return nil, errors.New("cycle")
		}
		seen[id] = true
		ordered = append(ordered, byID[id])
	}
	if len(seen) != len(byID) {
		return nil, errors.New("invalid_graph")
	}
	if start != "" {
		for i, n := range ordered {
			if n.ID == start {
				return ordered[i:], nil
			}
		}
		return nil, errors.New("start_not_found")
	}
	return ordered, nil
}

func validPolicy(policy string) bool {
	if policy == "" || policy == "stop" || policy == "skip" {
		return true
	}
	if strings.HasPrefix(policy, "retry:") {
		count, err := strconv.Atoi(strings.TrimPrefix(policy, "retry:"))
		return err == nil && count >= 1 && count <= 5
	}
	return false
}

func validTab(tab string) bool {
	if tab == "popup" || tab == "new" || tab == "opener" {
		return true
	}
	index, err := strconv.Atoi(tab)
	return err == nil && index >= 0 && index <= 99 && strconv.Itoa(index) == tab
}
