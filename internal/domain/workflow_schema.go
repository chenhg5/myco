package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Schema is a JSON Schema (draft 2020-12): a workflow's inputs, and the
// input and output contract of a step. The whole definition is kept as
// written (Raw: oneOf, $ref, pattern, minimum, …) and checked with a
// complete validator (Contracts, set by the hub); the fields below are the
// common keywords, read for forms and examples.
type Schema struct {
	Type        string             `json:"type,omitempty"` // object string number integer boolean array null
	Title       string             `json:"title,omitempty"`
	Description string             `json:"description,omitempty"`
	Required    []string           `json:"required,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Items       *Schema            `json:"items,omitempty"`
	Enum        []any              `json:"enum,omitempty"`
	Default     any                `json:"default,omitempty"`
	// Format is a hint for the App's form ("textarea" for long text).
	Format string `json:"format,omitempty"`
	// Raw is the whole definition as written.
	Raw map[string]any `json:"-"`
}

// UnmarshalJSON keeps the whole definition and reads the common keywords.
// "type" may be a list (["string", "null"]): the first that is not null is
// used for forms.
func (s *Schema) UnmarshalJSON(b []byte) error {
	var raw map[string]any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return fmt.Errorf("a schema must be a JSON object: %w", err)
	}
	raw = plainNumbers(raw).(map[string]any)
	*s = Schema{Raw: raw}
	switch t := raw["type"].(type) {
	case string:
		s.Type = t
	case []any:
		for _, x := range t {
			if v, ok := x.(string); ok && v != "null" {
				s.Type = v
				break
			}
		}
	}
	s.Title, _ = raw["title"].(string)
	s.Description, _ = raw["description"].(string)
	s.Format, _ = raw["format"].(string)
	s.Default = raw["default"]
	if e, ok := raw["enum"].([]any); ok {
		s.Enum = e
	}
	if r, ok := raw["required"].([]any); ok {
		for _, x := range r {
			if v, ok := x.(string); ok {
				s.Required = append(s.Required, v)
			}
		}
	}
	if ps, ok := raw["properties"].(map[string]any); ok {
		s.Properties = map[string]*Schema{}
		for k, v := range ps {
			b, _ := json.Marshal(v)
			var p Schema
			if err := json.Unmarshal(b, &p); err != nil {
				return fmt.Errorf("properties.%s: %w", k, err)
			}
			s.Properties[k] = &p
		}
	}
	if it, ok := raw["items"].(map[string]any); ok {
		b, _ := json.Marshal(it)
		var p Schema
		if err := json.Unmarshal(b, &p); err != nil {
			return fmt.Errorf("items: %w", err)
		}
		s.Items = &p
	}
	return nil
}

// MarshalJSON writes the definition back as it was written.
func (s *Schema) MarshalJSON() ([]byte, error) {
	if s.Raw != nil {
		return json.Marshal(s.Raw)
	}
	type plain Schema
	return json.Marshal((*plain)(s))
}

// Definition is the whole schema as a JSON value (for a validator).
func (s *Schema) Definition() map[string]any {
	if s == nil {
		return nil
	}
	if s.Raw != nil {
		return s.Raw
	}
	var m map[string]any
	b, _ := s.MarshalJSON()
	_ = json.Unmarshal(b, &m)
	return m
}

// plainNumbers turns json.Number into float64 (whole numbers stay exact).
func plainNumbers(v any) any {
	switch x := v.(type) {
	case json.Number:
		f, _ := x.Float64()
		return f
	case map[string]any:
		for k, e := range x {
			x[k] = plainNumbers(e)
		}
		return x
	case []any:
		for i, e := range x {
			x[i] = plainNumbers(e)
		}
		return x
	}
	return v
}

// Contracts checks schemas completely (JSON Schema draft 2020-12). The hub
// sets one (infra/jsonschema); without it, the common keywords are checked.
type Contracts interface {
	// Compile reports whether a definition is a valid schema.
	Compile(def map[string]any) error
	// Validate lists how v does not fit def (none: it fits).
	Validate(def map[string]any, v any) []string
}

// kind is the schema's type; properties alone mean an object.
func (s *Schema) kind() string {
	if s.Type == "" && len(s.Properties) > 0 {
		return "object"
	}
	return s.Type
}

func (s *Schema) problems(where string, c Contracts) []string {
	if c != nil {
		if err := c.Compile(s.Definition()); err != nil {
			return []string{fmt.Sprintf("%s is not a valid JSON Schema: %v", where, err)}
		}
		return nil
	}
	var out []string
	switch s.kind() {
	case "", "object", "string", "number", "integer", "boolean", "array", "null":
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
			out = append(out, p.problems(where+"."+k, nil)...)
		}
	}
	if s.Items != nil {
		out = append(out, s.Items.problems(where+"[]", nil)...)
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

// CheckWith lists how v does not fit the schema, with c (complete) or the
// common keywords (c nil).
func (s *Schema) CheckWith(c Contracts, v any) []string {
	if s == nil {
		return nil
	}
	if c != nil {
		return c.Validate(s.Definition(), v)
	}
	return s.Check(v)
}

// Check lists how v does not fit the common keywords (none: it fits).
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
// fills defaults. Values it cannot read stay as they are (the check
// reports them).
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

// Example is a sample value that fits the schema's common keywords, to
// show an agent the shape to hand back.
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
