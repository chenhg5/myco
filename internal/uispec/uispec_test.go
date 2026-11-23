package uispec

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogLoads(t *testing.T) {
	c := Catalog()
	if c.Version != 1 || c.Fence != "myco-ui" || c.Limits.MaxElements == 0 {
		t.Fatalf("catalog: %+v", c)
	}
	if len(c.ComponentOrder) != len(c.Components) || c.ComponentOrder[0] != "Card" {
		t.Fatalf("component order: %v", c.ComponentOrder)
	}
	for name, comp := range c.Components {
		if len(comp.PropOrder) != len(comp.Props) {
			t.Errorf("%s: prop order %v", name, comp.PropOrder)
		}
		for pn, p := range comp.Props {
			if p.Type == "list" && p.Of == "" && len(p.Items) == 0 {
				t.Errorf("%s.%s: list without of or items", name, pn)
			}
			if len(p.ItemOrder) != len(p.Items) {
				t.Errorf("%s.%s: item order", name, pn)
			}
		}
	}
}

// The App reads its own copy of the catalog; the two must not drift.
func TestWebCatalogInSync(t *testing.T) {
	web := filepath.Join("..", "..", "web", "src", "lib", "ui-catalog.json")
	b, err := os.ReadFile(web)
	if err != nil {
		t.Fatalf("cannot read %s (%v): copy it with: cp internal/uispec/catalog.json web/src/lib/ui-catalog.json", web, err)
	}
	if !bytes.Equal(b, catalogJSON) {
		t.Fatal("web/src/lib/ui-catalog.json differs from internal/uispec/catalog.json: edit catalog.json, then copy it with: cp internal/uispec/catalog.json web/src/lib/ui-catalog.json")
	}
}

func TestExampleIsValid(t *testing.T) {
	s, ps := Check(Catalog().Example)
	if len(ps) > 0 {
		t.Fatalf("the catalog's example has problems: %v", ps)
	}
	if s.Len() != 7 || s.Root != "e1" || s.Elements["e1"].Type != "Card" {
		t.Fatalf("normalized: root %s, %d elements", s.Root, s.Len())
	}
}

func TestParseNestedToFlat(t *testing.T) {
	s, err := Parse([]byte(`{"version":1,"type":"Card","props":{"title":"T"},"children":[
		{"type":"Grid","children":[{"type":"Metric","props":{"label":"a","value":"1"}},{"type":"Badge","props":{"text":"x"}}]},
		{"type":"Text","props":{"text":"hi"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Element{
		"e1": {Type: "Card", Children: []string{"e2", "e5"}},
		"e2": {Type: "Grid", Children: []string{"e3", "e4"}},
		"e3": {Type: "Metric"},
		"e4": {Type: "Badge"},
		"e5": {Type: "Text"},
	}
	if s.Root != "e1" || len(s.Elements) != len(want) {
		t.Fatalf("root %s, %d elements", s.Root, len(s.Elements))
	}
	for id, w := range want {
		got := s.Elements[id]
		if got == nil || got.Type != w.Type || strings.Join(got.Children, ",") != strings.Join(w.Children, ",") {
			t.Errorf("%s: got %+v want %+v", id, got, w)
		}
	}
	if s.Elements["e1"].Props["title"] != "T" {
		t.Errorf("props lost: %v", s.Elements["e1"].Props)
	}
	// The flat form round-trips through JSON and validates the same.
	b, _ := json.Marshal(s)
	s2, err := Parse(b)
	if err != nil {
		t.Fatalf("flat output does not parse: %v\n%s", err, b)
	}
	if ps := Validate(s2); len(ps) > 0 {
		t.Fatalf("flat output invalid: %v", ps)
	}
}

func TestParseArray(t *testing.T) {
	s, err := Parse([]byte(`[{"type":"Heading","props":{"text":"A"}},{"type":"Text","props":{"text":"b"}}]`))
	if err != nil {
		t.Fatal(err)
	}
	root := s.Elements[s.Root]
	if root.Type != "Stack" || len(root.Children) != 2 {
		t.Fatalf("array root: %+v", root)
	}
	if ps := Validate(s); len(ps) > 0 {
		t.Fatal(ps)
	}
	_, ps := Check([]byte(`[{"type":"Heading","props":{"text":"A"}},{"type":"Text"}]`))
	if len(ps) != 1 || ps[0].Path != "Text[1].props" {
		t.Fatalf("array paths: %v", ps)
	}
}

func TestParseErrors(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"not json", `{"type":`, "not valid JSON"},
		{"two values", `{"type":"Text"} {}`, "more than one value"},
		{"scalar", `42`, "expected a JSON object or array"},
		{"neither", `{"title":"x"}`, "expected an element"},
		{"both", `{"type":"Card","root":"a","elements":{}}`, "has both"},
		{"version", `{"version":2,"type":"Card"}`, "version must be 1"},
		{"no type", `{"type":"Card","children":[{"props":{}}]}`, `no "type"`},
		{"child not object", `{"type":"Card","children":["x"]}`, "must be an object"},
		{"props at top", `{"type":"Text","text":"hi"}`, `unknown key "text"`},
		{"props not object", `{"type":"Text","props":[1]}`, "props must be an object"},
		{"flat no root", `{"elements":{"a":{"type":"Card"}}}`, `needs "root"`},
		{"flat no elements", `{"root":"a"}`, `needs "elements"`},
		{"flat child not id", `{"root":"a","elements":{"a":{"type":"Card","children":[{"type":"Text"}]}}}`, "element ids"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.in))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
			if _, ok := err.(Problems); !ok {
				t.Fatalf("error is %T, not Problems", err)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	big := `{"type":"Table","props":{"columns":["a"],"rows":[` + strings.Repeat(`["x"],`, 200) + `["x"]]}}`
	var deep strings.Builder
	for range 13 {
		deep.WriteString(`{"type":"Stack","children":[`)
	}
	deep.WriteString(`{"type":"Badge","props":{"text":"x"}}`)
	for range 13 {
		deep.WriteString(`]}`)
	}
	many := `[` + strings.TrimSuffix(strings.Repeat(`{"type":"Badge","props":{"text":"x"}},`, 300), ",") + `]`
	for _, tc := range []struct {
		name, in string
		path     string // "" = any
		want     string
	}{
		{"unknown component", `{"type":"Card","children":[{"type":"Carousel"}]}`, "Card > Carousel[0]", `unknown component "Carousel"; known: Card, Stack`},
		{"case", `{"type":"card"}`, "card", `did you mean "Card"?`},
		{"children not allowed", `{"type":"Badge","props":{"text":"x"},"children":[{"type":"Card"}]}`, "Badge.children", "Badge takes no children (components that do: Card, Stack, Grid, Tabs)"},
		{"required", `{"type":"Heading"}`, "Heading.props", `missing required prop "text"`},
		{"null required", `{"type":"Heading","props":{"text":null}}`, "Heading.props", `missing required prop "text"`},
		{"unknown prop", `{"type":"Heading","props":{"text":"a","size":2}}`, "Heading.props.size", `unknown prop "size" for Heading (allowed: text, level, label)`},
		{"string type", `{"type":"Heading","props":{"text":5}}`, "Heading.props.text", "expected text"},
		{"number type", `{"type":"Grid","props":{"columns":"3"}}`, "Grid.props.columns", "expected a number"},
		{"number max", `{"type":"Grid","props":{"columns":5}}`, "Grid.props.columns", "above the maximum 4"},
		{"number min", `{"type":"Progress","props":{"value":-1}}`, "Progress.props.value", "below the minimum 0"},
		{"boolean", `{"type":"Button","props":{"label":"a","action":"reply","value":"v","primary":"yes"}}`, "Button.props.primary", "expected true or false"},
		{"enum", `{"type":"Callout","props":{"text":"a","tone":"red"}}`, "Callout.props.tone", `expected one of neutral, info, success, warning, danger; got "red"`},
		{"markdown nested fence", "{\"type\":\"Text\",\"props\":{\"text\":\"```myco-ui\\n{}\\n```\"}}", "Text.props.text", "cannot hold another one"},
		{"src scheme", `{"type":"Image","props":{"src":"http://x.test/a.png"}}`, "Image.props.src", "expected a picture"},
		{"src empty attachment", `{"type":"Image","props":{"src":"attachment:"}}`, "Image.props.src", "expected a picture"},
		{"url scheme", `{"type":"MediaList","props":{"items":[{"title":"a","href":"javascript:alert(1)"}]}}`, "MediaList.props.items[0].href", "expected an http:// or https:// link"},
		{"list type", `{"type":"Table","props":{"columns":"a","rows":[]}}`, "Table.props.columns", "expected a list"},
		{"list of", `{"type":"Table","props":{"columns":["a",2],"rows":[]}}`, "Table.props.columns[1]", "expected text"},
		{"list entry", `{"type":"Steps","props":{"items":["Build"]}}`, "Steps.props.items[0]", "expected an object with fields title, status, note"},
		{"list field required", `{"type":"Steps","props":{"items":[{"status":"done"}]}}`, "Steps.props.items[0]", `missing required field "title"`},
		{"list field unknown", `{"type":"Steps","props":{"items":[{"title":"a","state":"done"}]}}`, "Steps.props.items[0].state", `unknown field "state" (allowed: title, status, note)`},
		{"list field enum", `{"type":"Steps","props":{"items":[{"title":"a","status":"ok"}]}}`, "Steps.props.items[0].status", "expected one of done"},
		{"nested list", `{"type":"Chart","props":{"kind":"bar","labels":["a"],"series":[{"name":"s","values":[1,"2"]}]}}`, "Chart.props.series[0].values[1]", "expected a number"},
		{"rows not list", `{"type":"Table","props":{"columns":["a"],"rows":{"a":1}}}`, "Table.props.rows", "expected rows"},
		{"row not list", `{"type":"Table","props":{"columns":["a"],"rows":["x"]}}`, "Table.props.rows[0]", "a row must be a list"},
		{"cell type", `{"type":"Table","props":{"columns":["a"],"rows":[["x",true]]}}`, "Table.props.rows[0][1]", "a cell must be a string or a number"},
		{"tabs label", `{"type":"Tabs","children":[{"type":"Text","props":{"text":"a","label":"One"}},{"type":"Text","props":{"text":"b"}}]}`, "Tabs > Text[1].props.label", `needs a "label" prop`},
		{"label type", `{"type":"Text","props":{"text":"a","label":3}}`, "Text.props.label", "expected text"},
		{"list limit", big, "Table.props.rows", "201 rows; at most 200"},
		{"depth limit", deep.String(), "", "levels deep; at most 12"},
		{"element limit", many, "", "301 elements; at most 300"},
		{"byte limit", `{"type":"Code","props":{"code":"` + strings.Repeat("x", 70000) + `"}}`, "", "bytes; at most 65536"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ps := Check([]byte(tc.in))
			for _, p := range ps {
				if strings.Contains(p.Message, tc.want) && (tc.path == "" || p.Path == tc.path) {
					return
				}
			}
			t.Fatalf("want %s: %q among %v", tc.path, tc.want, ps)
		})
	}
}

func TestValidateGood(t *testing.T) {
	for _, in := range []string{
		`{"type":"Text","props":{"text":"**hi**","tone":null}}`,
		`{"type":"Image","props":{"src":"attachment:shot.png","caption":"c"}}`,
		`{"type":"Image","props":{"src":"https://example.com/a.png"}}`,
		`{"type":"Table","props":{"columns":["a","b"],"rows":[["x",1],["y",2.5]],"align":["left","right"]}}`,
		`{"type":"Tabs","children":[{"type":"Text","label":"x","props":{"text":"a","label":"One"}}]}`,
		`{"type":"MediaList","props":{"items":[{"title":"a","href":"http://x.test","badges":["new"]}]}}`,
		`{"version":1,"root":"r","elements":{"r":{"type":"Stack","children":["a"]},"a":{"type":"Badge","props":{"text":"x"},"key":"a"}}}`,
	} {
		_, ps := Check([]byte(in))
		if strings.Contains(in, `"label":"x","props"`) {
			// An unknown element key is a parse problem, not silently dropped.
			if len(ps) == 0 {
				t.Errorf("want a problem for %s", in)
			}
			continue
		}
		if len(ps) > 0 {
			t.Errorf("%s: %v", in, ps)
		}
	}
}

func TestValidateFlat(t *testing.T) {
	for _, tc := range []struct{ name, in, path, want string }{
		{"missing root", `{"root":"x","elements":{"a":{"type":"Card"}}}`, "root", `root "x" is not in elements`},
		{"missing child", `{"root":"a","elements":{"a":{"type":"Card","children":["b"]}}}`, "Card#a.children[0]", `no element with id "b"`},
		{"cycle", `{"root":"a","elements":{"a":{"type":"Card","children":["b"]},"b":{"type":"Stack","children":["a"]}}}`, "Card#a > Stack[0]#b > Card[0]#a", `cycle: element "a" contains itself`},
		{"self cycle", `{"root":"a","elements":{"a":{"type":"Card","children":["a"]}}}`, "Card#a > Card[0]#a", "cycle"},
		{"shared", `{"root":"a","elements":{"a":{"type":"Card","children":["b","b"]},"b":{"type":"Badge","props":{"text":"x"}}}}`, "Card#a > Badge[1]#b", "already has a parent"},
		{"orphan", `{"root":"a","elements":{"a":{"type":"Card"},"z":{"type":"Badge","props":{"text":"x"}}}}`, "elements.z", "not reachable from the root"},
		{"prop path", `{"root":"a","elements":{"a":{"type":"Grid","children":["m"]},"m":{"type":"Metric","props":{"label":"x","value":1}}}}`, "Grid#a > Metric[0]#m.props.value", "expected text"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ps := Check([]byte(tc.in))
			for _, p := range ps {
				if p.Path == tc.path && strings.Contains(p.Message, tc.want) {
					return
				}
			}
			t.Fatalf("want %s: %q among %v", tc.path, tc.want, ps)
		})
	}
}

func TestProblemsAreStable(t *testing.T) {
	in := []byte(`{"type":"Card","props":{"x":1,"y":2,"tone":"bad"},"children":[{"type":"Nope"},{"type":"Heading"}]}`)
	_, first := Check(in)
	for range 20 {
		_, again := Check(in)
		if len(again) != len(first) {
			t.Fatal("problem count changed")
		}
		for i := range again {
			if again[i] != first[i] {
				t.Fatalf("problem order changed: %v vs %v", again, first)
			}
		}
	}
	if len(first) != 5 {
		t.Fatalf("want 5 problems, got %v", first)
	}
}

func TestFence(t *testing.T) {
	got := Fence([]byte(` {"type":"Code","props":{"code":"` + "```go\\nx\\n```" + `"}} `))
	if !strings.HasPrefix(got, "```myco-ui\n{\n  \"type\": \"Code\"") || !strings.HasSuffix(got, "\n```") {
		t.Fatalf("fence:\n%s", got)
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(got, "```myco-ui\n"), "\n```")
	if strings.Contains(inner, "`") {
		t.Fatalf("a backtick inside could close the fence:\n%s", got)
	}
	var v struct{ Props struct{ Code string } }
	if err := json.Unmarshal([]byte(inner), &v); err != nil || v.Props.Code != "```go\nx\n```" {
		t.Fatalf("the fenced JSON must decode to the same value: %v %q", err, v.Props.Code)
	}
	if _, ps := Check([]byte(inner)); len(ps) > 0 {
		t.Fatal(ps)
	}
}

func TestGuide(t *testing.T) {
	g := Guide()
	for _, want := range []string{
		"```myco-ui", "Nested (recommended)", "Flat (json-render compatible)",
		"### Layout", "### Engineering", "Card: A framed box", "Tabs:", "Button:",
		"- kind (required, one of bar, line, pie)", "- columns (number 1-4)",
		"    - title (required, string)", "at most 65536 bytes, 300 elements",
		"must make sense without it", `"title": "Release check"`,
	} {
		if !strings.Contains(g, want) {
			t.Errorf("guide lacks %q", want)
		}
	}
	for name := range Catalog().Components {
		if !strings.Contains(g, "\n"+name+": ") {
			t.Errorf("guide lacks component %s", name)
		}
	}
}
