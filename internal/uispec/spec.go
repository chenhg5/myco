package uispec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Spec is a myco-ui block in the flat form: elements by id, and the id of
// the root. Parse turns either form into this one.
type Spec struct {
	Version  int                 `json:"version"`
	Root     string              `json:"root"`
	Elements map[string]*Element `json:"elements"`

	// size is the length of the JSON it was parsed from (0 = unknown).
	size int
	// generated is true when the ids were made up by Parse (nested input),
	// so problem paths leave them out.
	generated bool
	// implicitRoot is true when the input was an array: the root Stack is
	// not in it, so paths start at its children.
	implicitRoot bool
	// order is the ids in the order they were read.
	order []string
}

// Element is one component in a Spec.
type Element struct {
	Type     string         `json:"type"`
	Props    map[string]any `json:"props,omitempty"`
	Children []string       `json:"children,omitempty"`
}

// Problem is one thing wrong with a block. Path says where, the way an
// agent finds it in its JSON: "Card > Grid[0] > Metric[1].props.value"
// (component types from the root, [i] = position among its parent's
// children, #id after the type in the flat form).
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (p Problem) String() string {
	if p.Path == "" {
		return p.Message
	}
	return p.Path + ": " + p.Message
}

// Problems is a list of problems; as an error it reads as the first few.
type Problems []Problem

func (ps Problems) Error() string {
	if len(ps) == 0 {
		return "no problems"
	}
	var b strings.Builder
	for i, p := range ps {
		if i == 3 {
			fmt.Fprintf(&b, "; and %d more", len(ps)-3)
			break
		}
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(p.String())
	}
	return b.String()
}

// Len is the number of elements.
func (s *Spec) Len() int { return len(s.Elements) }

// Parse reads a block's JSON in either form and returns it in the flat
// form. Ids of nested input are e1, e2, ... in document order (e1 is the
// root). A top-level array becomes a vertical Stack of its elements. A
// structural problem (not JSON, both or neither form, a malformed
// element) is returned as a Problems error; whether the components and
// props are right is Validate's job.
func Parse(data []byte) (*Spec, error) {
	trimmed := bytes.TrimSpace(data)
	if bytes.HasPrefix(trimmed, []byte("```")) || bytes.HasPrefix(trimmed, []byte("~~~")) {
		return nil, Problems{{Message: "not valid JSON: expected raw JSON without Markdown fences; remove the opening and closing fence lines"}}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var top any
	if err := dec.Decode(&top); err != nil {
		if syntax, ok := err.(*json.SyntaxError); ok {
			offset := max(0, min(int(syntax.Offset)-1, len(data)))
			line := bytes.Count(data[:offset], []byte("\n")) + 1
			start := bytes.LastIndexByte(data[:offset], '\n') + 1
			column := utf8.RuneCount(data[start:offset]) + 1
			return nil, Problems{{Message: fmt.Sprintf("not valid JSON at line %d, column %d: %s", line, column, err)}}
		}
		if err == io.ErrUnexpectedEOF {
			return nil, Problems{{Message: "not valid JSON: unexpected EOF; check for missing closing braces, brackets or quotes"}}
		}
		return nil, Problems{{Message: "not valid JSON: " + err.Error()}}
	}
	if dec.More() {
		return nil, Problems{{Message: "not valid JSON: more than one value"}}
	}
	if _, err := dec.Token(); err == nil {
		return nil, Problems{{Message: "not valid JSON: unexpected data after the value"}}
	}
	p := &parser{spec: &Spec{Version: 1, Elements: map[string]*Element{}, size: len(data)}}
	switch v := top.(type) {
	case []any:
		p.spec.generated = true
		p.spec.implicitRoot = true
		id := p.newID()
		root := &Element{Type: "Stack"}
		p.spec.Elements[id] = root
		p.spec.Root = id
		for i, c := range v {
			if cid := p.nested(c, "", i, 2); cid != "" {
				root.Children = append(root.Children, cid)
			}
		}
	case map[string]any:
		p.version(v["version"])
		_, hasRoot := v["root"]
		_, hasElems := v["elements"]
		_, hasType := v["type"]
		switch {
		case (hasRoot || hasElems) && hasType:
			p.add("", `has both "type" (nested form) and "root"/"elements" (flat form); use one`)
		case hasRoot || hasElems:
			p.flat(v)
		case hasType:
			p.spec.generated = true
			p.spec.Root = p.nested(v, "", -1, 1)
		default:
			p.add("", `expected an element {"type": ..., "props": {...}, "children": [...]}, an array of elements, or the flat form {"root": ..., "elements": {...}}`)
		}
	default:
		p.add("", "expected a JSON object or array, got "+kind(top))
	}
	if len(p.problems) > 0 {
		return nil, p.problems
	}
	return p.spec, nil
}

type parser struct {
	spec     *Spec
	next     int
	problems Problems
}

func (p *parser) add(path, msg string) { p.problems = append(p.problems, Problem{path, msg}) }

func (p *parser) newID() string {
	p.next++
	id := "e" + strconv.Itoa(p.next)
	p.spec.order = append(p.spec.order, id)
	return id
}

func (p *parser) version(v any) {
	if v == nil {
		return
	}
	if n, ok := v.(json.Number); !ok || n.String() != "1" {
		p.add("version", fmt.Sprintf("version must be 1, got %s", show(v)))
	}
}

// nested reads one nested element and returns its id ("" if it is
// unusable). parent is the parent's path ("" for the top), idx the
// element's position among its parent's children (-1 for the root).
func (p *parser) nested(v any, parent string, idx, depth int) string {
	obj, ok := v.(map[string]any)
	if !ok {
		p.add(childPath(parent, "", idx), "an element must be an object {\"type\": ...}, got "+kind(v))
		return ""
	}
	typ, _ := obj["type"].(string)
	if typ == "" {
		p.add(childPath(parent, "", idx), `element has no "type" (a component name such as "Card")`)
		return ""
	}
	path := childPath(parent, typ, idx)
	id := p.newID()
	el := &Element{Type: typ}
	p.spec.Elements[id] = el
	for _, k := range sortedKeys(obj) {
		val := obj[k]
		switch k {
		case "type":
		case "version":
			if depth != 1 {
				p.add(path, `"version" belongs at the top level only`)
			}
		case "props":
			el.Props = p.props(val, path)
		case "children":
			arr, ok := val.([]any)
			if !ok {
				if val != nil {
					p.add(path+".children", "children must be an array of elements, got "+kind(val))
				}
				continue
			}
			for i, c := range arr {
				if cid := p.nested(c, path, i, depth+1); cid != "" {
					el.Children = append(el.Children, cid)
				}
			}
		default:
			p.add(path, fmt.Sprintf(`unknown key %q in an element (an element has "type", "props" and "children"; put %q under "props")`, k, k))
		}
	}
	return id
}

func (p *parser) props(v any, path string) map[string]any {
	if v == nil {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		p.add(path+".props", "props must be an object, got "+kind(v))
		return nil
	}
	return m
}

func (p *parser) flat(top map[string]any) {
	root, ok := top["root"].(string)
	if !ok || root == "" {
		p.add("root", `the flat form needs "root": the id of the top element`)
	}
	p.spec.Root = root
	elems, ok := top["elements"].(map[string]any)
	if !ok {
		p.add("elements", `the flat form needs "elements": an object of id → element`)
		return
	}
	ids := sortedKeys(elems)
	p.spec.order = ids
	for _, id := range ids {
		path := "elements." + id
		obj, ok := elems[id].(map[string]any)
		if !ok {
			p.add(path, "an element must be an object, got "+kind(elems[id]))
			continue
		}
		typ, _ := obj["type"].(string)
		if typ == "" {
			p.add(path, `element has no "type" (a component name such as "Card")`)
			continue
		}
		el := &Element{Type: typ}
		for _, k := range sortedKeys(obj) {
			val := obj[k]
			switch k {
			case "type", "key", "parentKey":
			case "props":
				el.Props = p.props(val, path)
			case "children":
				arr, ok := val.([]any)
				if !ok {
					if val != nil {
						p.add(path+".children", "children must be an array of element ids, got "+kind(val))
					}
					continue
				}
				for i, c := range arr {
					s, ok := c.(string)
					if !ok {
						p.add(fmt.Sprintf("%s.children[%d]", path, i), "in the flat form children are element ids (strings), got "+kind(c))
						continue
					}
					el.Children = append(el.Children, s)
				}
			default:
				p.add(path, fmt.Sprintf(`unknown key %q in an element (an element has "type", "props" and "children")`, k))
			}
		}
		p.spec.Elements[id] = el
	}
}

// childPath is the path of a child: "Card > Grid[0]". idx -1 = the root;
// an empty typ marks an element whose type is unknown.
func childPath(parent, typ string, idx int) string {
	seg := typ
	if idx >= 0 {
		seg += "[" + strconv.Itoa(idx) + "]"
	}
	if parent == "" {
		return seg
	}
	return parent + " > " + seg
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func kind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "a string"
	case json.Number, float64:
		return "a number"
	case bool:
		return "a boolean"
	case []any:
		return "an array"
	case map[string]any:
		return "an object"
	}
	return fmt.Sprintf("%T", v)
}

func show(v any) string {
	b, err := json.Marshal(v)
	if err != nil || len(b) > 60 {
		return kind(v)
	}
	return string(b)
}
