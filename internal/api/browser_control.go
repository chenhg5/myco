package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

const BrowserControlPermission = "browser.control"
const BrowserControlFeature = "agent_control"
const BrowserControlOwnerStopped = "stopped_by_owner"

const browserPagePrefix = "browser.page."

// BrowserControlDescriptors is the fixed version-one action surface.
func BrowserControlDescriptors() []ActionDescriptor {
	var out []ActionDescriptor
	for _, op := range []string{"open", "snapshot", "act", "extract", "screenshot", "close"} {
		spec := browserControlSpecs[op]
		properties := make(map[string]any)
		for _, field := range strings.Fields(spec.required + " " + spec.optional) {
			properties[field] = map[string]any{"type": browserControlFieldType(field)}
		}
		schema, _ := json.Marshal(map[string]any{
			"type": "object", "properties": properties, "required": strings.Fields(spec.required),
			"additionalProperties": false,
		})
		name := browserPagePrefix + op
		out = append(out, ActionDescriptor{Name: name, Version: 1, Title: name, Permission: BrowserControlPermission, InputSchema: schema})
	}
	return out
}

type browserControlSpec struct{ required, optional string }

var browserControlSpecs = map[string]browserControlSpec{
	"open":       {"profile", "url"},
	"snapshot":   {"session", "tab mode max_nodes include_screenshot"},
	"act":        {"session op", "tab ref value key url dy wait choice timeout_ms clear"},
	"extract":    {"session ref what", "tab attr"},
	"screenshot": {"session", "tab ref clip full"},
	"close":      {"session", ""},
}

var browserActSpecs = map[string]browserControlSpec{
	"navigate":   {"url", "tab"},
	"click":      {"ref", "tab"},
	"dblclick":   {"ref", "tab"},
	"hover":      {"ref", "tab"},
	"type":       {"ref value", "tab clear"},
	"select":     {"ref value", "tab"},
	"press":      {"key", "tab ref"},
	"scroll":     {"dy", "tab ref"},
	"back":       {"", "tab"},
	"forward":    {"", "tab"},
	"reload":     {"", "tab"},
	"switch_tab": {"tab", ""},
	"close_tab":  {"tab", ""},
	"wait":       {"wait", "tab ref value"},
	"dialog":     {"choice", "tab"},
}

func browserControlFieldType(field string) string {
	switch field {
	case "max_nodes", "timeout_ms":
		return "integer"
	case "dy":
		return "number"
	case "include_screenshot", "full", "clear":
		return "boolean"
	case "clip":
		return "object"
	default:
		return "string"
	}
}

// BrowserControlArgs are validated agent arguments. Marshal adds no identity
// fields, so only the App can supply a native grant and request identity.
type BrowserControlArgs map[string]json.RawMessage

func (a BrowserControlArgs) String(field string) string {
	var v string
	_ = json.Unmarshal(a[field], &v)
	return v
}

func DecodeBrowserControlArgs(action string, raw json.RawMessage) (BrowserControlArgs, error) {
	op := strings.TrimPrefix(action, browserPagePrefix)
	spec, ok := browserControlSpecs[op]
	var args BrowserControlArgs
	if !ok || op == action || json.Unmarshal(raw, &args) != nil || args == nil {
		return nil, fmt.Errorf("expected a browser control argument object")
	}
	if err := args.validate(spec); err != nil {
		return nil, err
	}
	if err := args.validateOperation(op); err != nil {
		return nil, err
	}
	return args, nil
}

func (a BrowserControlArgs) validate(spec browserControlSpec) error {
	for _, field := range strings.Fields(spec.required) {
		if _, ok := a[field]; !ok {
			return fmt.Errorf("missing required field %s", field)
		}
	}
	for field, raw := range a {
		if !containsBrowserField(spec.required+" "+spec.optional, field) {
			return fmt.Errorf("unknown or inapplicable field")
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("field %s cannot be null", field)
		}
		var dest any
		switch browserControlFieldType(field) {
		case "integer":
			dest = new(int)
		case "number":
			dest = new(float64)
		case "boolean":
			dest = new(bool)
		case "object":
			dest = new(map[string]json.RawMessage)
		default:
			dest = new(string)
		}
		if json.Unmarshal(raw, dest) != nil {
			return fmt.Errorf("field %s has the wrong type", field)
		}
		if str, ok := dest.(*string); ok && field != "value" && strings.TrimSpace(*str) == "" {
			return fmt.Errorf("field %s cannot be empty", field)
		}
	}
	return nil
}

func containsBrowserField(fields, field string) bool {
	for _, f := range strings.Fields(fields) {
		if f == field {
			return true
		}
	}
	return false
}

func (a BrowserControlArgs) validateOperation(op string) error {
	for _, field := range []string{"value", "url", "key"} {
		if strings.Contains(a.String(field), "${") {
			return fmt.Errorf("use a literal %s and add parameters to the draft after closing", field)
		}
	}
	if raw := a["url"]; len(raw) > 0 && !browserControlURL(a.String("url")) {
		return fmt.Errorf("url must be an absolute HTTP or HTTPS URL without user information")
	}
	switch op {
	case "act":
		action := a.String("op")
		spec, ok := browserActSpecs[action]
		if !ok {
			return fmt.Errorf("unsupported control operation")
		}
		if err := a.validate(browserControlSpec{"session op " + spec.required, "timeout_ms " + spec.optional}); err != nil {
			return err
		}
		if !a.boundedInt("timeout_ms", 30000) {
			return fmt.Errorf("timeout_ms must be between 1 and 30000")
		}
		if action == "wait" {
			return a.validateWait()
		}
		if action == "dialog" && a.String("choice") != "accept" && a.String("choice") != "dismiss" {
			return fmt.Errorf("dialog choice must be accept or dismiss")
		}
	case "snapshot":
		if mode := a.String("mode"); mode != "" && mode != "a11y" && mode != "text" {
			return fmt.Errorf("snapshot mode must be a11y or text")
		}
		if !a.boundedInt("max_nodes", 400) {
			return fmt.Errorf("max_nodes must be between 1 and 400")
		}
	case "extract":
		what := a.String("what")
		if (what != "text" && what != "html" && what != "attribute") || (what == "attribute") != (a["attr"] != nil) {
			return fmt.Errorf("what must be text, html or attribute, with attr required only for attribute")
		}
	case "screenshot":
		count := 0
		for _, field := range []string{"ref", "clip", "full"} {
			if a[field] != nil {
				count++
			}
		}
		if count > 1 {
			return fmt.Errorf("ref, clip and full are mutually exclusive")
		}
		if raw := a["clip"]; len(raw) != 0 {
			return validateBrowserClip(raw)
		}
	}
	return nil
}

func (a BrowserControlArgs) validateWait() error {
	switch a.String("wait") {
	case "element":
		if a["ref"] != nil && a["value"] == nil {
			return nil
		}
	case "url":
		if a["ref"] == nil && browserControlURL(a.String("value")) {
			return nil
		}
	case "network":
		if a["ref"] == nil && a["value"] == nil {
			return nil
		}
	}
	return fmt.Errorf("wait expects ref for element, value with an HTTP(S) URL for url, or no target for network")
}

func (a BrowserControlArgs) boundedInt(field string, max int) bool {
	var v int
	return a[field] == nil || json.Unmarshal(a[field], &v) == nil && v >= 1 && v <= max
}

func browserControlURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil
}

func validateBrowserClip(raw json.RawMessage) error {
	var clip map[string]json.RawMessage
	if json.Unmarshal(raw, &clip) != nil || len(clip) != 4 {
		return fmt.Errorf("clip needs x, y, width and height")
	}
	for _, field := range []string{"x", "y", "width", "height"} {
		var v *float64
		if json.Unmarshal(clip[field], &v) != nil || v == nil || *v < 0 || (*v == 0 && (field == "width" || field == "height")) {
			return fmt.Errorf("invalid clip coordinates")
		}
	}
	return nil
}

// ValidBrowserAuditHost accepts just a DNS hostname or IP address.
func ValidBrowserAuditHost(host string) bool {
	if host == "" || net.ParseIP(host) != nil {
		return true
	}
	host = strings.TrimSuffix(host, ".")
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range strings.ToLower(label) {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

type BrowserControlStatus struct {
	Session       string    `json:"session"`
	ExpiresAt     time.Time `json:"expires_at"`
	IdleExpiresAt time.Time `json:"idle_expires_at"`
}

type BrowserControlActor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type BrowserControlEvents struct {
	Session string                `json:"session"`
	Done    bool                  `json:"done"`
	Reason  string                `json:"reason"`
	Next    int                   `json:"next"`
	Events  []BrowserControlEvent `json:"events"`
}

type BrowserControlEvent struct {
	Host string `json:"host,omitempty"`
	Code string `json:"code,omitempty"`
}

// NormalizeBrowserHosts freezes the domain scope presented on the approval card.
func NormalizeBrowserHosts(input []string) ([]string, error) {
	if len(input) > 64 {
		return nil, fmt.Errorf("use at most 64 hosts")
	}
	var out []string
	seen := make(map[string]bool, len(input))
	for _, raw := range input {
		host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
		name := strings.TrimPrefix(host, "*.")
		if name == "" || strings.Contains(name, ":") || !ValidBrowserAuditHost(name) ||
			(name != host && net.ParseIP(name) != nil) {
			return nil, fmt.Errorf("use hostnames or subdomain patterns such as *.example.com")
		}
		if !seen[host] {
			seen[host] = true
			out = append(out, host)
		}
	}
	return out, nil
}
