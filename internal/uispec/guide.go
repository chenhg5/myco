package uispec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Fence wraps a block's JSON in a ```myco-ui fence, indented with two
// spaces. Backticks inside strings are written as ` so the JSON can
// never close the fence early. JSON that does not parse is kept as given.
func Fence(specJSON []byte) string {
	body := bytes.TrimSpace(specJSON)
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err == nil {
		body = out.Bytes()
	}
	// In valid JSON a backtick can only be inside a string.
	s := strings.ReplaceAll(string(body), "`", "\\u0060")
	return "```" + catalog.Fence + "\n" + s + "\n```"
}

// groups is the order of component groups in the guide.
var groups = []struct{ key, title string }{
	{"layout", "Layout (these hold other components in \"children\")"},
	{"content", "Content"},
	{"media", "Pictures, charts and diagrams"},
	{"engineering", "Engineering"},
	{"action", "Actions"},
}

// Guide is the catalog written for an agent: what a myco-ui block is, how
// to write one, every component with its props, the limits and an example.
func Guide() string {
	c := catalog
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	w("# myco-ui blocks (version %d)\n\n", c.Version)
	w("%s\n\n", c.About)
	w("Put the block in a message body as a fenced code block whose info string is `%s`:\n\n", c.Fence)
	w("    ```%s\n    {\"type\": \"Callout\", \"props\": {\"tone\": \"success\", \"text\": \"All 412 tests pass.\"}}\n    ```\n\n", c.Fence)
	w("Or write the JSON to a file and pass it with --ui (myco message reply, progress, send):\n")
	w("it is checked before sending and appended to the body. Check a file alone with\n`myco ui check --file block.json`.\n\n")

	w("## Rules\n\n")
	w("- The words around the block must make sense without it: older apps, phones and other\n  agents may show the JSON source instead. Say the conclusion in words; the block shows the detail.\n")
	w("- Only the components below are drawn. Check a block with myco ui check before sending: a block with unknown components or props is not drawn as meant.\n")
	w("- Pictures: `attachment:<file name>` for a file attached to the same message (--attach), or an https:// address.\n")
	w("- Limits: at most %d bytes, %d elements, %d levels of nesting, %d entries in any list.\n\n",
		c.Limits.MaxBytes, c.Limits.MaxElements, c.Limits.MaxDepth, c.Limits.MaxList)

	w("## Two forms\n\n")
	w("Nested (recommended): an element is {\"type\": \"<Component>\", \"props\": {...}, \"children\": [<element>, ...]};\n")
	w("props and children are optional. The top level is one element, or an array of elements\n")
	w("(drawn as a vertical Stack). The top-level object may carry \"version\": 1.\n\n")
	w("Flat (json-render compatible): {\"version\": 1, \"root\": \"<id>\", \"elements\": {\"<id>\": {\"type\": ..., \"props\": {...},\n")
	w("\"children\": [\"<id>\", ...]}}}. Every element must be reachable from the root exactly once.\n\n")

	w("## Prop types\n\n")
	for _, t := range c.TypeOrder {
		w("- %s: %s\n", t, c.Types[t])
	}
	w("\nEvery element may also have:\n")
	for _, k := range c.CommonOrder {
		p := c.Common[k]
		w("- %s (%s): %s\n", k, p.Type, p.Description)
	}
	w("\n## Components\n")
	seen := map[string]bool{}
	writeGroup := func(title string, names []string) {
		if len(names) == 0 {
			return
		}
		w("\n### %s\n", title)
		for _, name := range names {
			comp := c.Components[name]
			kids := ""
			if comp.Children {
				kids = " Takes children."
			}
			w("\n%s: %s%s\n", name, comp.Description, kids)
			for _, pn := range comp.PropOrder {
				writeProp(&b, "  ", pn, comp.Props[pn])
			}
		}
	}
	for _, g := range groups {
		var names []string
		for _, name := range c.ComponentOrder {
			if c.Components[name].Group == g.key {
				names = append(names, name)
				seen[name] = true
			}
		}
		writeGroup(g.title, names)
	}
	var rest []string
	for _, name := range c.ComponentOrder {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	writeGroup("Other", rest)

	w("\n## Example\n\n")
	var ex bytes.Buffer
	if err := json.Indent(&ex, c.Example, "", "  "); err != nil {
		ex.Write(c.Example)
	}
	w("```%s\n%s\n```\n", c.Fence, ex.String())
	return b.String()
}

func writeProp(b *strings.Builder, indent, name string, p Prop) {
	req := ""
	if p.Required {
		req = "required, "
	}
	fmt.Fprintf(b, "%s- %s (%s%s)", indent, name, req, describe(p))
	if p.Description != "" {
		fmt.Fprintf(b, ": %s", p.Description)
	}
	b.WriteString("\n")
	if p.Type == "list" && len(p.Items) > 0 {
		for _, k := range p.ItemOrder {
			writeProp(b, indent+"  ", k, p.Items[k])
		}
	}
}
