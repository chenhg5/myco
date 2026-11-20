package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestWorkflowExpressions(t *testing.T) {
	env := map[string]any{
		"output": map[string]any{"approved": false, "score": float64(7), "tags": []any{"a", "b"}, "名字": "x", "exit_code": float64(0)},
		"inputs": map[string]any{"task": "build"},
		"steps":  map[string]any{"write-a": map[string]any{"output": map[string]any{"branch": "feat/x"}}},
	}
	for expr, want := range map[string]bool{
		"!output.approved":                                                true,
		"output.approved == false":                                        true,
		"output.score >= 7 && output.score < 8":                           true,
		"output.score > 7 || inputs.task == 'build'":                      true,
		"output.missing == null":                                          true,
		"output.missing":                                                  false,
		"len(output.tags) == 2":                                           true,
		"contains(output.tags, 'b')":                                      true,
		"contains(steps['write-a'].output.branch, 'feat/')":               true,
		"steps.write-a.output.branch == \"feat/x\"":                       true,
		"output.score - 1 == 6 and output.score * 2 > 13":                 true,
		"output.score % 2 == 1 && -output.score < 0":                      true,
		"'a' in output.tags and not ('z' in output.tags)":                 true,
		"'z' not in output.tags":                                          true,
		"startsWith(inputs.task, 'bu') or false":                          true,
		"upper(inputs.task) == 'BUILD' && matches(inputs.task, '^b.+d$')": true,
		"inputs.task + '!' == 'build!'":                                   true,
		"output.名字 == 'x'":                                                true,
		"output.tags[1] == 'b'":                                           true,
		"output.exit_code == 0":                                           true,
		"(output.score == 1) || !(output.score == 7)":                     false,
		"output.score < 'z'":                                              false, // different kinds never compare
	} {
		got, err := EvalBool(expr, env)
		if err != nil || got != want {
			t.Errorf("%s = %v (%v), want %v", expr, got, err, want)
		}
	}
	for _, bad := range []string{"output.", "a ==", "(a", "a = b", "len(a, b)", "'open"} {
		if _, err := ParseExpr(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestWorkflowTemplates(t *testing.T) {
	env := map[string]any{"inputs": map[string]any{"task": "导出", "n": float64(3), "list": []any{"a"}}, "steps": map[string]any{}}
	got, err := RenderTemplate("做 {{inputs.task}} ×{{ inputs.n }} {{inputs.list}}{{#if steps.review}} 意见{{else}} 首次{{/if}}{{#if inputs.n > 2}}!{{/if}}", env)
	if err != nil || got != `做 导出 ×3 ["a"] 首次!` {
		t.Fatalf("%q %v", got, err)
	}
	for _, bad := range []string{"{{#if a}}x", "{{/if}}", "{{a", "{{a ==}}"} {
		if err := CheckTemplate(bad); err == nil {
			t.Errorf("%q passed", bad)
		}
	}
}

func TestWorkflowSchema(t *testing.T) {
	s := &Schema{Required: []string{"approved"}, Properties: map[string]*Schema{
		"approved": {Type: "boolean"}, "n": {Type: "integer"}, "level": {Type: "string", Enum: []any{"low", "high"}},
		"items": {Type: "array", Items: &Schema{Type: "string"}},
	}}
	if p := s.Check(map[string]any{"approved": true, "n": float64(2), "level": "low", "items": []any{"x"}}); len(p) != 0 {
		t.Fatal(p)
	}
	p := s.Check(map[string]any{"n": 2.5, "level": "mid", "items": []any{float64(1)}})
	if strings.Join(p, "|") != "approved is missing|items[0] must be text|level must be one of [\"low\",\"high\"]|n must be a whole number" {
		t.Fatalf("%q", p)
	}
	in := (&Schema{Properties: map[string]*Schema{"n": {Type: "number"}, "ok": {Type: "boolean", Default: true}, "l": {Type: "array"}}}).
		Coerce(map[string]any{"n": "4.5", "l": `["a"]`})
	if in["n"] != 4.5 || in["ok"] != true || len(in["l"].([]any)) != 1 {
		t.Fatalf("%v", in)
	}
}

func TestWorkflowDefProblems(t *testing.T) {
	cases := map[string]string{
		`{"name": "x", "steps": []}`: "at least one step",
		`{"name": "x", "steps": [{"id": "a", "agent": "p", "computer": "q", "prompt": "x"}]}`:   "exactly one of",
		`{"name": "x", "steps": [{"id": "a", "agent": "p"}]}`:                                   "needs a prompt",
		`{"name": "x", "steps": [{"id": "a", "agent": "p", "role": "审查", "prompt": "x"}]}`:      "exactly one of",
		`{"name": "x", "steps": [{"id": "a", "human": "$starter", "options": ["ok"]}]}`:         "at least two options",
		`{"name": "x", "steps": [{"id": "a", "agent": "p", "prompt": "x", "timeout": "soon"}]}`: "not a duration",
		`{"name": "x", "steps": [{"id": "a", "agent": "p", "prompt": "x"}, {"id": "b", "agent": "p", "prompt": "x"}],
		  "edges": [{"from": "a", "to": "b"}, {"from": "b", "to": "a"}]}`: "give it maxIterations",
		`{"name": "x", "steps": [{"id": "a", "agent": "p", "prompt": "x"}, {"id": "b", "agent": "p", "prompt": "x"}],
		  "edges": [{"from": "a", "to": "$end"}]}`: "no edge leads to it",
		`{"name": "x", "steps": [{"id": "a", "agent": "p", "prompt": "x"}], "edges": [{"from": "a", "to": "zz"}]}`:                  "to must be a step",
		`{"name": "x", "steps": [{"id": "a", "agent": "p", "prompt": "x"}], "edges": [{"from": "a", "to": "$end", "when": "a ="}]}`: "when:",
		`{"name": "x", "steps": [{"id": "a", "agent": "p", "prompt": "{{#if x}}"}]}`:                                                "without {{/if}}",
		`{"name": "x", "bogus": 1, "steps": [{"id": "a", "agent": "p", "prompt": "x"}]}`:                                            "not valid JSON",
	}
	for def, want := range cases {
		_, err := ParseWorkflowDef([]byte(def))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s:\n got %v\nwant %q", def, err, want)
		}
	}
	ok := `{"name": "x", "steps": [{"id": "a", "agent": "p", "prompt": "x"}, {"id": "b", "agent": "p", "prompt": "y"}],
	  "edges": [{"from": "a", "to": "b"}, {"from": "b", "to": "a", "maxIterations": 2, "when": "!output.ok"}, {"from": "b", "to": "$end"}]}`
	if _, err := ParseWorkflowDef([]byte(ok)); err != nil {
		t.Fatal(err)
	}
	// No edges: steps run in order.
	d, err := ParseWorkflowDef([]byte(`{"name": "seq", "steps": [{"id": "a", "agent": "p", "prompt": "x"}, {"id": "b", "agent": "p", "prompt": "y"}]}`))
	if err != nil || d.Out("a")[0].Edge.To != "b" || d.Out("b")[0].Edge.To != EdgeEnd {
		t.Fatalf("%v %+v", err, d)
	}
}

func TestWorkflowDurations(t *testing.T) {
	for s, want := range map[string]time.Duration{"": 0, "90s": 90 * time.Second, "2h": 2 * time.Hour, "1d": 24 * time.Hour, "1d12h": 36 * time.Hour} {
		if got, err := ParseWorkflowDuration(s); err != nil || got != want {
			t.Errorf("%q: %v %v", s, got, err)
		}
	}
	if (&StepDef{Human: "x"}).TimeoutDur() != DefaultHumanTimeout {
		t.Error("decisions wait by default")
	}
}

func TestMatchesRole(t *testing.T) {
	a := &Agent{Name: "rev", DisplayName: "Reviewer", Description: "负责审查 Go 代码", Labels: []string{"role=qa", "team=web"}, Skills: []Skill{{Name: "release"}}}
	for role, want := range map[string]bool{"审查": true, "QA": true, "release": true, "reviewer": true, "设计": false, "web": false, "": false} {
		if got := MatchesRole(a, role, ""); got != want {
			t.Errorf("%q: %v", role, got)
		}
	}
	if !MatchesRole(&Agent{Name: "x"}, "前端", "这个项目里管前端") {
		t.Error("a project member's note says its role there")
	}
}

func TestWorkflowReferencesAreChecked(t *testing.T) {
	for def, want := range map[string]string{
		`{"name": "x", "inputs": {"properties": {"task": {"type": "string"}}}, "steps": [{"id": "a", "agent": "p", "prompt": "{{inputs.tsk}}"}]}`: "inputs.tsk is not one of the workflow's inputs",
		`{"name": "x", "steps": [{"id": "a", "agent": "p", "prompt": "{{steps.b.output}}"}]}`:                                                     "there is no step b",
		`{"name": "x", "steps": [{"id": "a", "agent": "p", "prompt": "{{steps.a.outptu}}"}]}`:                                                     "a step has output (result)",
		`{"name": "x", "steps": [{"id": "a", "agent": "p", "prompt": "{{output.x}}"}]}`:                                                           "only known on an edge",
		`{"name": "x", "steps": [{"id": "a", "agent": "p", "prompt": "{{foo}}"}]}`:                                                                `unknown name "foo"`,
		`{"name": "x", "steps": [{"id": "a", "agent": "p", "prompt": "x"}, {"id": "b", "agent": "p", "prompt": "y"}],
		  "edges": [{"from": "a", "to": "b", "map": {"v": "{{nodes.zz.result}}"}}]}`: "there is no step zz",
		`{"name": "x", "entry": "nope", "steps": [{"id": "a", "agent": "p", "prompt": "x"}]}`:                  `entry "nope" is not a step`,
		`{"name": "x", "steps": [{"id": "a", "agent": "p", "prompt": "x", "join": {"from": ["a"]}}]}`:          "join.from needs at least two",
		`{"name": "x", "steps": [{"id": "a", "agent": "p", "prompt": "x", "output": {}, "outputSchema": {}}]}`: "output and outputSchema are the same",
	} {
		_, err := ParseWorkflowDef([]byte(def))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s:\n got %v\nwant %q", def, err, want)
		}
	}
	ok := `{"name": "x", "title": "X", "tags": ["a"], "group": "g", "entry": "a", "allowUnboundedCycles": true,
	  "inputs": {"properties": {"task": {"type": "string"}}},
	  "steps": [{"id": "a", "agent": "p", "prompt": "{{inputs.task}} {{input.task}}", "dispatch": "all", "inputSchema": {"required": ["task"]}},
	            {"id": "b", "agent": "p", "prompt": "{{nodes.a.result.x}}"}, {"id": "c", "agent": "p", "prompt": "{{steps.a.summary}}"},
	            {"id": "d", "agent": "p", "prompt": "{{input.b.x}}", "join": {"from": ["b", "c"]}}],
	  "edges": [{"from": "a", "to": "b", "map": {"x": "{{output.y}}", "n": 1}}, {"from": "a", "to": "c"},
	            {"from": "b", "to": "d"}, {"from": "c", "to": "d"}, {"from": "d", "to": "a", "when": "status == 'done' && output.again"}]}`
	d, err := ParseWorkflowDef([]byte(ok))
	if err != nil {
		t.Fatal(err)
	}
	if d.Waits("d")[0] != "b" || d.EntryStep() != "a" {
		t.Fatalf("%+v", d)
	}
	b, _ := json.Marshal(d.Steps[3].Join)
	if string(b) != `{"from":["b","c"]}` {
		t.Fatalf("join: %s", b)
	}
}

func TestRenderValueKeepsTypes(t *testing.T) {
	env := map[string]any{"output": map[string]any{"n": float64(3), "list": []any{"a"}}}
	v, err := RenderValue(map[string]any{"n": "{{output.n}}", "list": "{{ output.list }}", "text": "n={{output.n}}", "gone": "{{output.zz}}", "k": true}, env)
	if err != nil {
		t.Fatal(err)
	}
	m := v.(map[string]any)
	if m["n"] != float64(3) || len(m["list"].([]any)) != 1 || m["text"] != "n=3" || m["gone"] != "" || m["k"] != true {
		t.Fatalf("%v", m)
	}
}

func TestSchemaKeepsTheWholeDefinition(t *testing.T) {
	var s Schema
	in := `{"type":["string","null"],"pattern":"^[a-z]+$","minLength":2,"oneOf":[{"const":"a"}]}`
	if err := json.Unmarshal([]byte(in), &s); err != nil {
		t.Fatal(err)
	}
	if s.Type != "string" {
		t.Fatalf("type %q", s.Type)
	}
	b, _ := json.Marshal(&s)
	var a, c map[string]any
	_ = json.Unmarshal([]byte(in), &a)
	_ = json.Unmarshal(b, &c)
	if !equal(a, c) {
		t.Fatalf("lost keywords: %s", b)
	}
}
