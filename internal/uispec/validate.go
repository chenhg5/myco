package uispec

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Check parses and validates a block's JSON. The spec is nil when it could
// not be parsed; problems is empty when the block is good.
func Check(data []byte) (*Spec, []Problem) {
	s, err := Parse(data)
	if err != nil {
		if ps, ok := err.(Problems); ok {
			return nil, ps
		}
		return nil, []Problem{{Message: err.Error()}}
	}
	if ps := Validate(s); len(ps) > 0 {
		return s, ps
	}
	return s, nil
}

// Validate checks a spec against the catalog: known components, children
// only where allowed, props (required, types, enum values, ranges, list
// entries), the limits, and in the flat form that the elements make one
// tree from the root (ids exist, no cycles, nothing left over).
func Validate(s *Spec) []Problem {
	v := &validator{c: catalog, s: s, visited: map[string]bool{}, onPath: map[string]bool{}}
	lim := v.c.Limits
	if s.size > lim.MaxBytes {
		v.add("", fmt.Sprintf("the block is %d bytes; at most %d", s.size, lim.MaxBytes))
	}
	if n := len(s.Elements); n > lim.MaxElements {
		v.add("", fmt.Sprintf("the block has %d elements; at most %d", n, lim.MaxElements))
	}
	if s.Version != 0 && s.Version != 1 {
		v.add("version", fmt.Sprintf("version must be 1, got %d", s.Version))
	}
	root, ok := s.Elements[s.Root]
	switch {
	case s.Root == "":
		v.add("root", "no root element")
	case !ok:
		v.add("root", fmt.Sprintf("root %q is not in elements", s.Root))
	default:
		path := v.seg("", root.Type, s.Root, -1)
		if s.implicitRoot {
			path = ""
		}
		v.walk(s.Root, path, 1)
	}
	ids := s.order
	if len(ids) != len(s.Elements) {
		ids = sortedKeys(s.Elements)
	}
	for _, id := range ids {
		if _, ok := s.Elements[id]; ok && !v.visited[id] && id != s.Root {
			v.add("elements."+id, fmt.Sprintf("element %q (%s) is not reachable from the root; add its id to a parent's children or remove it", id, s.Elements[id].Type))
		}
	}
	return v.problems
}

type validator struct {
	c        *CatalogSpec
	s        *Spec
	visited  map[string]bool
	onPath   map[string]bool
	deep     bool
	problems []Problem
}

func (v *validator) add(path, msg string) { v.problems = append(v.problems, Problem{path, msg}) }

// seg is an element's path: its parent's, then "Type[i]", plus "#id" in
// the flat form.
func (v *validator) seg(parent, typ, id string, idx int) string {
	p := childPath(parent, typ, idx)
	if !v.s.generated {
		p += "#" + id
	}
	return p
}

func (v *validator) walk(id, path string, depth int) {
	v.visited[id] = true
	v.onPath[id] = true
	defer func() { v.onPath[id] = false }()
	el := v.s.Elements[id]
	where := path
	if where == "" {
		where = "(top-level array)"
	}
	if depth > v.c.Limits.MaxDepth && !v.deep {
		v.deep = true
		v.add(where, fmt.Sprintf("nested %d levels deep; at most %d", depth, v.c.Limits.MaxDepth))
	}
	comp, known := v.c.Components[el.Type]
	if !known {
		v.add(where, v.unknownType(el.Type))
	} else {
		v.props(where, el, comp)
		if len(el.Children) > 0 && !comp.Children {
			v.add(where+".children", fmt.Sprintf("%s takes no children (components that do: %s)", el.Type, strings.Join(v.containers(), ", ")))
		}
	}
	for i, cid := range el.Children {
		child, ok := v.s.Elements[cid]
		if !ok {
			v.add(fmt.Sprintf("%s.children[%d]", where, i), fmt.Sprintf("no element with id %q", cid))
			continue
		}
		cpath := v.seg(path, child.Type, cid, i)
		if v.onPath[cid] {
			v.add(cpath, fmt.Sprintf("cycle: element %q contains itself", cid))
			continue
		}
		if v.visited[cid] {
			v.add(cpath, fmt.Sprintf("element %q already has a parent; an element can be used once", cid))
			continue
		}
		if el.Type == "Tabs" {
			if s, _ := child.Props["label"].(string); strings.TrimSpace(s) == "" {
				v.add(cpath+".props.label", `a child of Tabs needs a "label" prop: the name of its tab`)
			}
		}
		v.walk(cid, cpath, depth+1)
	}
}

func (v *validator) containers() []string {
	var out []string
	for _, name := range v.c.ComponentOrder {
		if v.c.Components[name].Children {
			out = append(out, name)
		}
	}
	return out
}

func (v *validator) unknownType(t string) string {
	for _, name := range v.c.ComponentOrder {
		if strings.EqualFold(name, t) {
			return fmt.Sprintf("unknown component %q; did you mean %q? (names are case-sensitive)", t, name)
		}
	}
	return fmt.Sprintf("unknown component %q; known: %s", t, strings.Join(v.c.ComponentOrder, ", "))
}

func (v *validator) props(path string, el *Element, comp Component) {
	for _, name := range comp.PropOrder {
		if comp.Props[name].Required && el.Props[name] == nil {
			v.add(path+".props", fmt.Sprintf("missing required prop %q (%s)", name, describe(comp.Props[name])))
		}
	}
	for _, k := range sortedKeys(el.Props) {
		p, ok := comp.Props[k]
		if !ok {
			p, ok = v.c.Common[k]
		}
		if !ok {
			allowed := append(append([]string{}, comp.PropOrder...), v.c.CommonOrder...)
			v.add(path+".props."+k, fmt.Sprintf("unknown prop %q for %s (allowed: %s)", k, el.Type, strings.Join(allowed, ", ")))
			continue
		}
		if el.Props[k] == nil {
			continue
		}
		v.value(path+".props."+k, p, el.Props[k])
	}
}

// fields checks one list entry against its field map.
func (v *validator) fields(path string, items map[string]Prop, order []string, obj map[string]any) {
	for _, name := range order {
		if items[name].Required && obj[name] == nil {
			v.add(path, fmt.Sprintf("missing required field %q (%s)", name, describe(items[name])))
		}
	}
	for _, k := range sortedKeys(obj) {
		p, ok := items[k]
		if !ok {
			v.add(path+"."+k, fmt.Sprintf("unknown field %q (allowed: %s)", k, strings.Join(order, ", ")))
			continue
		}
		if obj[k] == nil {
			continue
		}
		v.value(path+"."+k, p, obj[k])
	}
}

func (v *validator) value(path string, p Prop, val any) {
	switch p.Type {
	case "string", "markdown":
		s, ok := val.(string)
		if !ok {
			v.add(path, "expected text (a JSON string), got "+kind(val))
			return
		}
		if p.Type == "markdown" && strings.Contains(s, "```"+v.c.Fence) {
			v.add(path, "a "+v.c.Fence+" block cannot hold another one")
		}
	case "number":
		f, ok := number(val)
		if !ok {
			v.add(path, "expected a number, got "+kind(val))
			return
		}
		if p.Min != nil && f < *p.Min {
			v.add(path, fmt.Sprintf("%s is below the minimum %s", fmtNum(f), fmtNum(*p.Min)))
		}
		if p.Max != nil && f > *p.Max {
			v.add(path, fmt.Sprintf("%s is above the maximum %s", fmtNum(f), fmtNum(*p.Max)))
		}
	case "boolean":
		if _, ok := val.(bool); !ok {
			v.add(path, "expected true or false, got "+kind(val))
		}
	case "enum":
		s, ok := val.(string)
		if !ok || !contains(p.Values, s) {
			v.add(path, fmt.Sprintf("expected one of %s; got %s", strings.Join(p.Values, ", "), show(val)))
		}
	case "src":
		s, ok := val.(string)
		if !ok || !validSrc(s) {
			v.add(path, fmt.Sprintf("expected a picture: attachment:<file name> (a file attached to the same message) or an https:// address; got %s", show(val)))
		}
	case "url":
		s, ok := val.(string)
		if !ok || !validURL(s) {
			v.add(path, fmt.Sprintf("expected an http:// or https:// link; got %s", show(val)))
		}
	case "list":
		arr, ok := val.([]any)
		if !ok {
			v.add(path, "expected a list (a JSON array), got "+kind(val))
			return
		}
		if len(arr) > v.c.Limits.MaxList {
			v.add(path, fmt.Sprintf("%d entries; at most %d", len(arr), v.c.Limits.MaxList))
		}
		for i, item := range arr {
			ip := fmt.Sprintf("%s[%d]", path, i)
			switch {
			case p.Of != "":
				if item == nil {
					v.add(ip, "expected "+v.c.Types[p.Of]+", got null")
					continue
				}
				v.value(ip, Prop{Type: p.Of}, item)
			case len(p.Items) > 0:
				obj, ok := item.(map[string]any)
				if !ok {
					v.add(ip, "expected an object with fields "+strings.Join(p.ItemOrder, ", ")+"; got "+kind(item))
					continue
				}
				v.fields(ip, p.Items, p.ItemOrder, obj)
			}
		}
	case "rows":
		arr, ok := val.([]any)
		if !ok {
			v.add(path, "expected rows: a list of rows, each a list of strings or numbers; got "+kind(val))
			return
		}
		if len(arr) > v.c.Limits.MaxList {
			v.add(path, fmt.Sprintf("%d rows; at most %d", len(arr), v.c.Limits.MaxList))
		}
		for i, row := range arr {
			cells, ok := row.([]any)
			if !ok {
				v.add(fmt.Sprintf("%s[%d]", path, i), "a row must be a list of strings or numbers, got "+kind(row))
				continue
			}
			if len(cells) > v.c.Limits.MaxList {
				v.add(fmt.Sprintf("%s[%d]", path, i), fmt.Sprintf("%d cells; at most %d", len(cells), v.c.Limits.MaxList))
			}
			for j, cell := range cells {
				if _, isNum := number(cell); isNum {
					continue
				}
				if _, isStr := cell.(string); !isStr {
					v.add(fmt.Sprintf("%s[%d][%d]", path, i, j), "a cell must be a string or a number, got "+kind(cell))
				}
			}
		}
	}
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func fmtNum(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func validSrc(s string) bool {
	if rest, ok := strings.CutPrefix(s, "attachment:"); ok {
		return strings.TrimSpace(rest) != ""
	}
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != ""
}

func validURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

// describe is a prop's type in a few words, for messages and the guide.
func describe(p Prop) string {
	switch p.Type {
	case "enum":
		return "one of " + strings.Join(p.Values, ", ")
	case "number":
		switch {
		case p.Min != nil && p.Max != nil:
			return fmt.Sprintf("number %s-%s", fmtNum(*p.Min), fmtNum(*p.Max))
		case p.Min != nil:
			return fmt.Sprintf("number >= %s", fmtNum(*p.Min))
		case p.Max != nil:
			return fmt.Sprintf("number <= %s", fmtNum(*p.Max))
		}
		return "number"
	case "list":
		if p.Of != "" {
			return "list of " + p.Of
		}
		return "list of {" + strings.Join(p.ItemOrder, ", ") + "}"
	case "rows":
		return "rows: [[string|number, ...], ...]"
	case "src":
		return "src: attachment:<name> or https://"
	case "url":
		return "url: http(s)://"
	}
	return p.Type
}
