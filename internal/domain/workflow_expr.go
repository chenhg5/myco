package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// The conditions on edges and the {{...}} in prompts are small expressions:
//
//	output.approved            a value (missing: null)
//	steps.review.output.score  another step's latest result
//	inputs.task                the run's inputs
//	output.items[0], output["中文 key"]
//	== != < <= > >=  && || !  and or not  in  ( )
//	+ - * / %  (numbers; + also joins text)
//	"text" 'text' 12 3.5 true false null
//	len(x) contains(x, y) startsWith(s, p) endsWith(s, p) matches(s, re)
//	lower(s) upper(s) trim(s)
//
// Nothing else: no assignments, no calls out, no loops. A step id with a
// dash reads as one name when a letter follows the dash (steps.write-a);
// put spaces around a minus.

// Expr is a parsed expression.
type Expr struct{ root exprNode }

type exprNode interface {
	eval(env map[string]any) (any, error)
}

// ParseExpr parses s.
func ParseExpr(s string) (*Expr, error) {
	p := &exprParser{src: s}
	if err := p.lex(); err != nil {
		return nil, err
	}
	n, err := p.or()
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.toks) {
		return nil, fmt.Errorf("unexpected %q in %q", p.toks[p.pos].text, s)
	}
	return &Expr{root: n}, nil
}

// Eval evaluates e against env.
func (e *Expr) Eval(env map[string]any) (any, error) { return e.root.eval(env) }

// EvalBool parses and evaluates s, and says whether the result holds.
func EvalBool(s string, env map[string]any) (bool, error) {
	e, err := ParseExpr(s)
	if err != nil {
		return false, err
	}
	v, err := e.Eval(env)
	if err != nil {
		return false, err
	}
	return Truthy(v), nil
}

// Truthy: false, null, 0, "" and empty lists and objects are false.
func Truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
}

type tok struct {
	kind string // num str ident op
	text string
	num  float64
}

type exprParser struct {
	src  string
	toks []tok
	pos  int
}

func (p *exprParser) lex() error {
	rs := []rune(p.src)
	for i := 0; i < len(rs); {
		c := rs[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '"' || c == '\'':
			j := i + 1
			var b strings.Builder
			for ; j < len(rs) && rs[j] != c; j++ {
				if rs[j] == '\\' && j+1 < len(rs) {
					j++
				}
				b.WriteRune(rs[j])
			}
			if j >= len(rs) {
				return fmt.Errorf("unclosed string in %q", p.src)
			}
			p.toks = append(p.toks, tok{kind: "str", text: b.String()})
			i = j + 1
		case unicode.IsDigit(c):
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.') {
				j++
			}
			n, err := strconv.ParseFloat(string(rs[i:j]), 64)
			if err != nil {
				return fmt.Errorf("bad number %q", string(rs[i:j]))
			}
			p.toks = append(p.toks, tok{kind: "num", text: string(rs[i:j]), num: n})
			i = j
		case unicode.IsLetter(c) || c == '_' || c == '$':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_' || rs[j] == '$' ||
				(rs[j] == '-' && j+1 < len(rs) && unicode.IsLetter(rs[j+1]))) {
				j++
			}
			p.toks = append(p.toks, tok{kind: "ident", text: string(rs[i:j])})
			i = j
		default:
			two := ""
			if i+1 < len(rs) {
				two = string(rs[i : i+2])
			}
			switch two {
			case "==", "!=", "<=", ">=", "&&", "||":
				p.toks = append(p.toks, tok{kind: "op", text: two})
				i += 2
				continue
			}
			if strings.ContainsRune("<>!().[],+-*/%", c) {
				p.toks = append(p.toks, tok{kind: "op", text: string(c)})
				i++
				continue
			}
			return fmt.Errorf("unexpected %q in %q", string(c), p.src)
		}
	}
	return nil
}

func (p *exprParser) peek(text string) bool {
	return p.pos < len(p.toks) && p.toks[p.pos].kind == "op" && p.toks[p.pos].text == text
}

func (p *exprParser) eat(text string) bool {
	if p.peek(text) {
		p.pos++
		return true
	}
	return false
}

type binNode struct {
	op   string
	l, r exprNode
}

// word eats a keyword (and, or, not, in).
func (p *exprParser) word(w string) bool {
	if p.pos < len(p.toks) && p.toks[p.pos].kind == "ident" && p.toks[p.pos].text == w {
		p.pos++
		return true
	}
	return false
}

func (p *exprParser) or() (exprNode, error) {
	l, err := p.and()
	for err == nil && (p.eat("||") || p.word("or")) {
		var r exprNode
		if r, err = p.and(); err == nil {
			l = &binNode{"||", l, r}
		}
	}
	return l, err
}

func (p *exprParser) and() (exprNode, error) {
	l, err := p.cmp()
	for err == nil && (p.eat("&&") || p.word("and")) {
		var r exprNode
		if r, err = p.cmp(); err == nil {
			l = &binNode{"&&", l, r}
		}
	}
	return l, err
}

func (p *exprParser) cmp() (exprNode, error) {
	l, err := p.sum()
	if err != nil {
		return nil, err
	}
	for _, op := range []string{"==", "!=", "<=", ">=", "<", ">"} {
		if p.eat(op) {
			r, err := p.sum()
			if err != nil {
				return nil, err
			}
			return &binNode{op, l, r}, nil
		}
	}
	if p.word("in") {
		r, err := p.sum()
		if err != nil {
			return nil, err
		}
		return &callNode{fn: "contains", args: []exprNode{r, l}}, nil
	}
	if p.pos+1 < len(p.toks) && p.toks[p.pos].text == "not" && p.toks[p.pos+1].text == "in" {
		p.pos += 2
		r, err := p.sum()
		if err != nil {
			return nil, err
		}
		return &notNode{&callNode{fn: "contains", args: []exprNode{r, l}}}, nil
	}
	return l, nil
}

func (p *exprParser) sum() (exprNode, error) {
	l, err := p.product()
	for err == nil {
		op := ""
		switch {
		case p.eat("+"):
			op = "+"
		case p.eat("-"):
			op = "-"
		default:
			return l, nil
		}
		var r exprNode
		if r, err = p.product(); err == nil {
			l = &arithNode{op, l, r}
		}
	}
	return l, err
}

func (p *exprParser) product() (exprNode, error) {
	l, err := p.unary()
	for err == nil {
		op := ""
		switch {
		case p.eat("*"):
			op = "*"
		case p.eat("/"):
			op = "/"
		case p.eat("%"):
			op = "%"
		default:
			return l, nil
		}
		var r exprNode
		if r, err = p.unary(); err == nil {
			l = &arithNode{op, l, r}
		}
	}
	return l, err
}

// arithNode: numbers; + also joins text (or text and a number).
type arithNode struct {
	op   string
	l, r exprNode
}

func (n *arithNode) eval(env map[string]any) (any, error) {
	l, err := n.l.eval(env)
	if err != nil {
		return nil, err
	}
	r, err := n.r.eval(env)
	if err != nil {
		return nil, err
	}
	lf, lok := l.(float64)
	rf, rok := r.(float64)
	if !lok || !rok {
		if n.op == "+" {
			_, ls := l.(string)
			_, rs := r.(string)
			if ls || rs {
				return ValueText(l) + ValueText(r), nil
			}
		}
		return nil, nil // not numbers: null
	}
	switch n.op {
	case "+":
		return lf + rf, nil
	case "-":
		return lf - rf, nil
	case "*":
		return lf * rf, nil
	case "/":
		if rf == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return lf / rf, nil
	}
	if rf == 0 {
		return nil, fmt.Errorf("division by zero")
	}
	return math.Mod(lf, rf), nil
}

type negNode struct{ x exprNode }

func (n *negNode) eval(env map[string]any) (any, error) {
	v, err := n.x.eval(env)
	if f, ok := v.(float64); ok {
		return -f, err
	}
	return nil, err
}

type notNode struct{ x exprNode }

func (n *notNode) eval(env map[string]any) (any, error) {
	v, err := n.x.eval(env)
	return !Truthy(v), err
}

func (p *exprParser) unary() (exprNode, error) {
	if p.eat("-") {
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		return &negNode{x}, nil
	}
	if p.eat("!") || (p.pos+1 < len(p.toks) && p.toks[p.pos].kind == "ident" && p.toks[p.pos].text == "not" && p.toks[p.pos+1].text != "in" && p.word("not")) {
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		return &notNode{x}, nil
	}
	return p.primary()
}

type litNode struct{ v any }

func (n *litNode) eval(map[string]any) (any, error) { return n.v, nil }

// pathNode reads a value: a name, then .field / [index] / ["key"].
type pathNode struct {
	name  string
	parts []exprNode
}

func (n *pathNode) eval(env map[string]any) (any, error) {
	v := env[n.name]
	for _, part := range n.parts {
		k, err := part.eval(env)
		if err != nil {
			return nil, err
		}
		v = index(v, k)
	}
	return v, nil
}

func index(v, k any) any {
	switch x := v.(type) {
	case map[string]any:
		if s, ok := k.(string); ok {
			return x[s]
		}
	case []any:
		if f, ok := k.(float64); ok && f >= 0 && int(f) < len(x) && f == math.Trunc(f) {
			return x[int(f)]
		}
	}
	return nil
}

type callNode struct {
	fn   string
	args []exprNode
}

func (n *callNode) eval(env map[string]any) (any, error) {
	var vs []any
	for _, a := range n.args {
		v, err := a.eval(env)
		if err != nil {
			return nil, err
		}
		vs = append(vs, v)
	}
	switch n.fn {
	case "len":
		switch x := vs[0].(type) {
		case string:
			return float64(len([]rune(x))), nil
		case []any:
			return float64(len(x)), nil
		case map[string]any:
			return float64(len(x)), nil
		}
		return float64(0), nil
	case "contains":
		switch x := vs[0].(type) {
		case string:
			s, ok := vs[1].(string)
			return ok && strings.Contains(x, s), nil
		case []any:
			for _, e := range x {
				if equal(e, vs[1]) {
					return true, nil
				}
			}
			return false, nil
		case map[string]any:
			s, ok := vs[1].(string)
			_, has := x[s]
			return ok && has, nil
		}
		return false, nil
	}
	str := func(i int) string { s, _ := vs[i].(string); return s }
	switch n.fn {
	case "startsWith":
		return strings.HasPrefix(str(0), str(1)), nil
	case "endsWith":
		return strings.HasSuffix(str(0), str(1)), nil
	case "lower":
		return strings.ToLower(str(0)), nil
	case "upper":
		return strings.ToUpper(str(0)), nil
	case "trim":
		return strings.TrimSpace(str(0)), nil
	case "matches":
		re, err := regexp.Compile(str(1))
		if err != nil {
			return nil, fmt.Errorf("matches: %v", err)
		}
		return re.MatchString(str(0)), nil
	}
	return nil, fmt.Errorf("unknown function %s", n.fn)
}

var exprFuncs = map[string]int{"len": 1, "contains": 2, "startsWith": 2, "endsWith": 2, "matches": 2, "lower": 1, "upper": 1, "trim": 1}

func (p *exprParser) primary() (exprNode, error) {
	if p.pos >= len(p.toks) {
		return nil, fmt.Errorf("%q ends too early", p.src)
	}
	t := p.toks[p.pos]
	p.pos++
	switch t.kind {
	case "num":
		return &litNode{t.num}, nil
	case "str":
		return &litNode{t.text}, nil
	case "op":
		if t.text == "(" {
			n, err := p.or()
			if err != nil {
				return nil, err
			}
			if !p.eat(")") {
				return nil, fmt.Errorf("missing ) in %q", p.src)
			}
			return n, nil
		}
		return nil, fmt.Errorf("unexpected %q in %q", t.text, p.src)
	}
	switch t.text {
	case "true":
		return &litNode{true}, nil
	case "false":
		return &litNode{false}, nil
	case "null", "nil":
		return &litNode{nil}, nil
	}
	if n, ok := exprFuncs[t.text]; ok && p.peek("(") {
		p.pos++
		c := &callNode{fn: t.text}
		for !p.eat(")") {
			if len(c.args) > 0 && !p.eat(",") {
				return nil, fmt.Errorf("expected , in %q", p.src)
			}
			a, err := p.or()
			if err != nil {
				return nil, err
			}
			c.args = append(c.args, a)
		}
		if len(c.args) != n {
			return nil, fmt.Errorf("%s takes %d argument(s)", t.text, n)
		}
		return c, nil
	}
	n := &pathNode{name: t.text}
	for {
		switch {
		case p.eat("."):
			if p.pos >= len(p.toks) || (p.toks[p.pos].kind != "ident" && p.toks[p.pos].kind != "num") {
				return nil, fmt.Errorf("expected a field name after . in %q", p.src)
			}
			f := p.toks[p.pos]
			p.pos++
			if f.kind == "num" {
				n.parts = append(n.parts, &litNode{f.num})
			} else {
				n.parts = append(n.parts, &litNode{f.text})
			}
		case p.eat("["):
			k, err := p.or()
			if err != nil {
				return nil, err
			}
			if !p.eat("]") {
				return nil, fmt.Errorf("missing ] in %q", p.src)
			}
			n.parts = append(n.parts, k)
		default:
			return n, nil
		}
	}
}

func (n *binNode) eval(env map[string]any) (any, error) {
	l, err := n.l.eval(env)
	if err != nil {
		return nil, err
	}
	switch n.op {
	case "&&":
		if !Truthy(l) {
			return false, nil
		}
		r, err := n.r.eval(env)
		return Truthy(r), err
	case "||":
		if Truthy(l) {
			return true, nil
		}
		r, err := n.r.eval(env)
		return Truthy(r), err
	}
	r, err := n.r.eval(env)
	if err != nil {
		return nil, err
	}
	switch n.op {
	case "==":
		return equal(l, r), nil
	case "!=":
		return !equal(l, r), nil
	}
	lf, lok := l.(float64)
	rf, rok := r.(float64)
	if lok && rok {
		switch n.op {
		case "<":
			return lf < rf, nil
		case "<=":
			return lf <= rf, nil
		case ">":
			return lf > rf, nil
		case ">=":
			return lf >= rf, nil
		}
	}
	ls, lok := l.(string)
	rs, rok := r.(string)
	if lok && rok {
		switch n.op {
		case "<":
			return ls < rs, nil
		case "<=":
			return ls <= rs, nil
		case ">":
			return ls > rs, nil
		case ">=":
			return ls >= rs, nil
		}
	}
	return false, nil // comparing different kinds (or null) never holds
}

func equal(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// Templates: {{expr}} puts a value in (text as is, other values as JSON);
// {{#if expr}}…{{else}}…{{/if}} keeps one part.

type tmplNode struct {
	text  string
	expr  *Expr
	cond  *Expr
	then  []tmplNode
	other []tmplNode
}

func parseTemplate(s string) ([]tmplNode, error) {
	nodes, rest, end, err := parseTmplUntil(s)
	if err != nil {
		return nil, err
	}
	if end != "" || rest != "" {
		return nil, fmt.Errorf("{{%s}} without {{#if}}", end)
	}
	return nodes, nil
}

// parseTmplUntil reads nodes until {{else}} or {{/if}} (end says which).
func parseTmplUntil(s string) (nodes []tmplNode, rest, end string, err error) {
	for s != "" {
		i := strings.Index(s, "{{")
		if i < 0 {
			nodes = append(nodes, tmplNode{text: s})
			return nodes, "", "", nil
		}
		if i > 0 {
			nodes = append(nodes, tmplNode{text: s[:i]})
		}
		j := strings.Index(s[i:], "}}")
		if j < 0 {
			return nil, "", "", fmt.Errorf("{{ without }}")
		}
		tag := strings.TrimSpace(s[i+2 : i+j])
		s = s[i+j+2:]
		switch {
		case tag == "else" || tag == "/if":
			return nodes, s, tag, nil
		case strings.HasPrefix(tag, "#if "):
			cond, err := ParseExpr(strings.TrimSpace(tag[4:]))
			if err != nil {
				return nil, "", "", err
			}
			n := tmplNode{cond: cond}
			var e string
			if n.then, s, e, err = parseTmplUntil(s); err != nil {
				return nil, "", "", err
			}
			if e == "else" {
				if n.other, s, e, err = parseTmplUntil(s); err != nil {
					return nil, "", "", err
				}
			}
			if e != "/if" {
				return nil, "", "", fmt.Errorf("{{#if %s}} without {{/if}}", tag[4:])
			}
			nodes = append(nodes, n)
		default:
			x, err := ParseExpr(tag)
			if err != nil {
				return nil, "", "", err
			}
			nodes = append(nodes, tmplNode{expr: x})
		}
	}
	return nodes, "", "", nil
}

// CheckTemplate reports whether s is a valid template.
func CheckTemplate(s string) error {
	_, err := parseTemplate(s)
	return err
}

// RenderTemplate fills s from env.
func RenderTemplate(s string, env map[string]any) (string, error) {
	nodes, err := parseTemplate(s)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := renderNodes(&b, nodes, env); err != nil {
		return "", err
	}
	return b.String(), nil
}

func renderNodes(b *strings.Builder, nodes []tmplNode, env map[string]any) error {
	for _, n := range nodes {
		switch {
		case n.cond != nil:
			v, err := n.cond.Eval(env)
			if err != nil {
				return err
			}
			part := n.other
			if Truthy(v) {
				part = n.then
			}
			if err := renderNodes(b, part, env); err != nil {
				return err
			}
		case n.expr != nil:
			v, err := n.expr.Eval(env)
			if err != nil {
				return err
			}
			b.WriteString(ValueText(v))
		default:
			b.WriteString(n.text)
		}
	}
	return nil
}

// ValueText is how a value reads in text: strings as they are, whole
// numbers without a point, the rest as JSON.
func ValueText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// Refs lists the paths the expression reads, as their leading names
// (output.items[0].x is [output items]; a computed key ends it).
func (e *Expr) Refs() [][]string {
	var out [][]string
	collectRefs(e.root, &out)
	return out
}

func collectRefs(n exprNode, out *[][]string) {
	switch x := n.(type) {
	case *pathNode:
		p := []string{x.name}
		for _, part := range x.parts {
			lit, ok := part.(*litNode)
			if !ok {
				break
			}
			s, isStr := lit.v.(string)
			if !isStr {
				break
			}
			p = append(p, s)
		}
		for _, part := range x.parts {
			if _, ok := part.(*litNode); !ok {
				collectRefs(part, out)
			}
		}
		*out = append(*out, p)
	case *binNode:
		collectRefs(x.l, out)
		collectRefs(x.r, out)
	case *arithNode:
		collectRefs(x.l, out)
		collectRefs(x.r, out)
	case *notNode:
		collectRefs(x.x, out)
	case *negNode:
		collectRefs(x.x, out)
	case *callNode:
		for _, a := range x.args {
			collectRefs(a, out)
		}
	}
}

func tmplRefs(nodes []tmplNode) [][]string {
	var out [][]string
	for _, n := range nodes {
		if n.expr != nil {
			out = append(out, n.expr.Refs()...)
		}
		if n.cond != nil {
			out = append(out, n.cond.Refs()...)
		}
		out = append(out, tmplRefs(n.then)...)
		out = append(out, tmplRefs(n.other)...)
	}
	return out
}

// RenderValue fills a map value: text "{{expr}}" alone gives the value
// itself (keeping its type; missing: ""), other text is a template, and
// lists and objects are filled inside.
func RenderValue(v any, env map[string]any) (any, error) {
	switch x := v.(type) {
	case string:
		t := strings.TrimSpace(x)
		if strings.HasPrefix(t, "{{") && strings.HasSuffix(t, "}}") && strings.Count(t, "{{") == 1 && !strings.HasPrefix(t, "{{#") {
			e, err := ParseExpr(strings.TrimSpace(t[2 : len(t)-2]))
			if err != nil {
				return nil, err
			}
			r, err := e.Eval(env)
			if r == nil {
				r = ""
			}
			return r, err
		}
		return RenderTemplate(x, env)
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			r, err := RenderValue(e, env)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			out[k] = r
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			r, err := RenderValue(e, env)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	}
	return v, nil
}
