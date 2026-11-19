package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Schema is the part of JSON Schema workflows use: the start form's inputs
// and the result an agent step must hand back.
type Schema struct {
	Type        string             `json:"type,omitempty"` // object string number integer boolean array
	Title       string             `json:"title,omitempty"`
	Description string             `json:"description,omitempty"`
	Required    []string           `json:"required,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Items       *Schema            `json:"items,omitempty"`
	Enum        []any              `json:"enum,omitempty"`
	Default     any                `json:"default,omitempty"`
	// Format is a hint for the App's form ("textarea" for long text).
	Format string `json:"format,omitempty"`
}

// kind is the schema's type; properties alone mean an object.
func (s *Schema) kind() string {
	if s.Type == "" && len(s.Properties) > 0 {
		return "object"
	}
	return s.Type
}

func (s *Schema) problems(where string) []string {
	var out []string
	switch s.kind() {
	case "", "object", "string", "number", "integer", "boolean", "array":
	default:
		out = append(out, fmt.Sprintf("%s: unknown type %q", where, s.Type))
	}
	for _, r := range s.Required {
		if s.Properties != nil && s.Properties[r] == nil {
			out = append(out, fmt.Sprintf("%s: required %q is not in properties", where, r))
		}
	}
	for _, k := range sortedKeys(s.Properties) {
		if p := s.Properties[k]; p != nil {
			out = append(out, p.problems(where+"."+k)...)
		}
	}
	if s.Items != nil {
		out = append(out, s.Items.problems(where+"[]")...)
	}
	return out
}

func sortedKeys(m map[string]*Schema) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// Check lists how v does not fit the schema (none: it fits).
func (s *Schema) Check(v any) []string {
	var out []string
	s.check(v, "", &out)
	return out
}

func (s *Schema) check(v any, path string, out *[]string) {
	name := path
	if name == "" {
		name = "the result"
	}
	bad := func(f string, a ...any) { *out = append(*out, name+" "+fmt.Sprintf(f, a...)) }
	if len(s.Enum) > 0 {
		ok := false
		for _, e := range s.Enum {
			if equal(e, v) {
				ok = true
			}
		}
		if !ok {
			bad("must be one of %s", ValueText(s.Enum))
			return
		}
	}
	switch s.kind() {
	case "object":
		m, ok := v.(map[string]any)
		if !ok {
			bad("must be an object")
			return
		}
		for _, r := range s.Required {
			if _, has := m[r]; !has {
				*out = append(*out, fmt.Sprintf("%s is missing", join(path, r)))
			}
		}
		for _, k := range sortedKeys(s.Properties) {
			if x, has := m[k]; has && s.Properties[k] != nil {
				s.Properties[k].check(x, join(path, k), out)
			}
		}
	case "string":
		if _, ok := v.(string); !ok {
			bad("must be text")
		}
	case "number":
		if _, ok := v.(float64); !ok {
			bad("must be a number")
		}
	case "integer":
		if f, ok := v.(float64); !ok || f != math.Trunc(f) {
			bad("must be a whole number")
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			bad("must be true or false")
		}
	case "array":
		xs, ok := v.([]any)
		if !ok {
			bad("must be a list")
			return
		}
		if s.Items != nil {
			for i, x := range xs {
				s.Items.check(x, fmt.Sprintf("%s[%d]", path, i), out)
			}
		}
	}
}

func join(path, k string) string {
	if path == "" {
		return k
	}
	return path + "." + k
}

// Coerce turns text form values into the types the schema asks for ("3"
// for a number, "true" for a boolean, JSON for lists and objects), and
// fills defaults. Values it cannot read stay as they are (Check reports
// them).
func (s *Schema) Coerce(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	if s == nil {
		return out
	}
	for k, p := range s.Properties {
		if p == nil {
			continue
		}
		v, has := out[k]
		if !has || v == nil || v == "" {
			if p.Default != nil {
				out[k] = p.Default
			} else if has && v == "" && p.kind() != "string" {
				delete(out, k)
			}
			continue
		}
		t, ok := v.(string)
		if !ok {
			continue
		}
		switch p.kind() {
		case "number", "integer":
			if f, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
				out[k] = f
			}
		case "boolean":
			if b, err := strconv.ParseBool(strings.TrimSpace(t)); err == nil {
				out[k] = b
			}
		case "array", "object":
			var x any
			if json.Unmarshal([]byte(t), &x) == nil {
				out[k] = x
			}
		}
	}
	return out
}

// Example is a sample value that fits the schema, to show an agent the
// shape to hand back.
func (s *Schema) Example() any {
	if len(s.Enum) > 0 {
		return s.Enum[0]
	}
	switch s.kind() {
	case "object":
		m := map[string]any{}
		for k, p := range s.Properties {
			if p != nil {
				m[k] = p.Example()
			}
		}
		return m
	case "array":
		if s.Items != nil {
			return []any{s.Items.Example()}
		}
		return []any{}
	case "number", "integer":
		return 0
	case "boolean":
		return true
	case "string":
		if s.Description != "" {
			return "<" + s.Description + ">"
		}
		return "..."
	}
	return nil
}
