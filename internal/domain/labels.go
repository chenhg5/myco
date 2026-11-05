package domain

import (
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Labels group agents. A label is either a bare tag ("gpu") or key=value
// ("team=loops"). A key may carry several values, so groups are just labels
// such as group=loops and group=release on the same agent.
var labelPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{0,62}(=[A-Za-z0-9._/:@-]{1,63})?$`)

// MaxLabels bounds labels per agent.
const MaxLabels = 32

// ParseLabels validates and normalizes labels (sorted, deduplicated).
func ParseLabels(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, l := range in {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if !labelPattern.MatchString(l) {
			return nil, Errf(CodeInvalidArgument, "invalid label %q", l).
				WithSuggestion("use tag or key=value: lowercase key [a-z0-9._/-], value [A-Za-z0-9._/:@-], e.g. team=loops")
		}
		if !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	if len(out) > MaxLabels {
		return nil, Errf(CodeInvalidArgument, "too many labels (%d, max %d)", len(out), MaxLabels)
	}
	sort.Strings(out)
	return out, nil
}

// MergeLabels returns cur + add - remove, validated. A remove entry that is a
// bare key ("team") removes every value of that key.
func MergeLabels(cur, add, remove []string) ([]string, error) {
	add, err := ParseLabels(add)
	if err != nil {
		return nil, err
	}
	next := append(slices.Clone(cur), add...)
	var out []string
	for _, l := range next {
		k, _, _ := strings.Cut(l, "=")
		drop := false
		for _, r := range remove {
			r = strings.TrimSpace(r)
			if r == l || (!strings.Contains(r, "=") && r == k) {
				drop = true
			}
		}
		if !drop {
			out = append(out, l)
		}
	}
	return ParseLabels(out)
}

// Selector picks agents by labels. Terms are comma-separated and all must
// match: "key" (present), "!key" (absent), "key=value", "key!=value".
// Besides labels, the virtual keys name, harness and capability match the
// agent's name, binding harness and capabilities.
type Selector struct {
	terms []selTerm
	raw   string
}

type selTerm struct {
	key, value string
	op         string // has | not | eq | ne
}

var selKey = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{0,62}$`)

// ParseSelector parses a selector; an empty string matches every agent.
func ParseSelector(s string) (Selector, error) {
	sel := Selector{raw: strings.TrimSpace(s)}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var t selTerm
		switch {
		case strings.Contains(part, "!="):
			k, v, _ := strings.Cut(part, "!=")
			t = selTerm{key: strings.TrimSpace(k), value: strings.TrimSpace(v), op: "ne"}
		case strings.Contains(part, "="):
			k, v, _ := strings.Cut(part, "=")
			t = selTerm{key: strings.TrimSpace(k), value: strings.TrimSpace(v), op: "eq"}
		case strings.HasPrefix(part, "!"):
			t = selTerm{key: strings.TrimSpace(part[1:]), op: "not"}
		default:
			t = selTerm{key: part, op: "has"}
		}
		if !selKey.MatchString(t.key) || ((t.op == "eq" || t.op == "ne") && t.value == "") {
			return Selector{}, Errf(CodeInvalidArgument, "invalid selector term %q", part).
				WithSuggestion(`use comma-separated terms: key, !key, key=value, key!=value (e.g. "group=loops,machine=mac")`)
		}
		sel.terms = append(sel.terms, t)
	}
	return sel, nil
}

// Empty reports whether the selector matches everything.
func (s Selector) Empty() bool { return len(s.terms) == 0 }

func (s Selector) String() string { return s.raw }

// Matches reports whether agent a satisfies every term.
func (s Selector) Matches(a *Agent) bool {
	for _, t := range s.terms {
		vals := a.labelValues(t.key)
		has := len(vals) > 0
		var ok bool
		switch t.op {
		case "has":
			ok = has
		case "not":
			ok = !has
		case "eq":
			ok = slices.Contains(vals, t.value)
		case "ne":
			ok = !slices.Contains(vals, t.value)
		}
		if !ok {
			return false
		}
	}
	return true
}

// labelValues returns the values of key (a bare tag yields "").
func (a *Agent) labelValues(key string) []string {
	switch key {
	case "name":
		return []string{string(a.Name)}
	case "harness":
		if a.Binding.Harness == "" {
			return nil
		}
		return []string{a.Binding.Harness}
	case "capability":
		return a.Capabilities
	}
	var out []string
	for _, l := range a.Labels {
		k, v, _ := strings.Cut(l, "=")
		if k == key {
			out = append(out, v)
		}
	}
	return out
}
