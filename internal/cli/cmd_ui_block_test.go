package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/uispec"
	"github.com/myco-ai/myco/skill"
)

const goodBlock = `{"type":"Card","props":{"title":"Release"},"children":[{"type":"Metric","props":{"label":"Tests","value":"412"}}]}`

func TestSessionUIHintExampleValid(t *testing.T) {
	_, rest, found := strings.Cut(skill.StructuredUIHint, "Example ui.json: `")
	example, _, closed := strings.Cut(rest, "`")
	if !found || !closed {
		t.Fatal("session hint lacks its minimal JSON example")
	}
	if _, problems := uispec.Check([]byte(example)); len(problems) > 0 {
		t.Fatalf("session hint example is invalid: %v", problems)
	}
}

func TestMessageUIHelpExamplesValidate(t *testing.T) {
	h := &harnessT{t, map[string]string{"MYCO_CONFIG": filepath.Join(t.TempDir(), "c.yaml")}}
	for _, name := range []string{"reply", "progress"} {
		code, out, e := h.run("message", name, "--help")
		if code != 0 {
			t.Fatalf("%s help: %d %s", name, code, e)
		}
		found := false
		for _, line := range strings.Split(out, "\n") {
			_, example, ok := strings.Cut(line, ".json: ")
			if !ok {
				continue
			}
			found = true
			if _, problems := uispec.Check([]byte(example)); len(problems) > 0 {
				t.Errorf("%s example is invalid: %v", name, problems)
			}
		}
		if !found || !strings.Contains(out, "--ui") {
			t.Errorf("%s help lacks a --ui example with its JSON", name)
		}
	}
}

func TestUICheckExplainsJSONErrors(t *testing.T) {
	h := &harnessT{t, map[string]string{"MYCO_CONFIG": filepath.Join(t.TempDir(), "c.yaml")}}
	for _, tc := range []struct{ input, want string }{
		{"```myco-ui\n{}\n```", "remove the opening and closing fence lines"},
		{"{\n\"type\":\"Card\",\n}", "line 3, column 1"},
	} {
		file := filepath.Join(t.TempDir(), "ui.json")
		if err := os.WriteFile(file, []byte(tc.input), 0600); err != nil {
			t.Fatal(err)
		}
		code, _, output := h.run("ui", "check", "--file", file)
		var ce Error
		if code != ExitUsage || json.Unmarshal([]byte(output), &ce) != nil || !strings.Contains(ce.Message, tc.want) {
			t.Fatalf("expected actionable JSON error %q: %d %s", tc.want, code, output)
		}
		if len(ce.Details) != 1 || !strings.Contains(ce.Details[0].Message, tc.want) || !strings.Contains(ce.Suggestion, "myco ui schema --format text") {
			t.Fatalf("missing details or reference: %+v", ce)
		}
	}
}

func TestUISchema(t *testing.T) {
	h := &harnessT{t, map[string]string{"MYCO_CONFIG": filepath.Join(t.TempDir(), "c.yaml")}}
	code, out, e := h.run("ui", "schema", "--format", "text")
	if code != 0 {
		t.Fatalf("schema text: %d %s", code, e)
	}
	for _, want := range []string{"```myco-ui", "Card:", "Metric:", "TestResults:", "Button:"} {
		if !strings.Contains(out, want) {
			t.Errorf("guide lacks %q", want)
		}
	}
	code, out, _ = h.run("ui", "schema")
	var cat struct {
		Fence      string
		Components map[string]any
	}
	if code != 0 || json.Unmarshal([]byte(out), &cat) != nil || cat.Fence != "myco-ui" || cat.Components["Card"] == nil {
		t.Fatalf("schema json: %d %.200s", code, out)
	}
}

func TestUICheck(t *testing.T) {
	dir := t.TempDir()
	h := &harnessT{t, map[string]string{"MYCO_CONFIG": filepath.Join(dir, "c.yaml")}}
	good := filepath.Join(dir, "good.json")
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(good, []byte(goodBlock), 0o644)
	os.WriteFile(bad, []byte(`{"type":"Card","children":[{"type":"Metric","props":{"label":"x"}},{"type":"Carousel"}]}`), 0o644)

	code, out, e := h.run("ui", "check", "--file", good)
	if code != 0 || !strings.Contains(out, `"ok": true`) || !strings.Contains(out, `"elements": 2`) {
		t.Fatalf("check good: %d %s %s", code, out, e)
	}
	code, _, e = h.run("ui", "check", "--file", bad)
	if code != ExitUsage {
		t.Fatalf("check bad: exit %d %s", code, e)
	}
	var ce Error
	if err := json.Unmarshal([]byte(e), &ce); err != nil {
		t.Fatalf("error not JSON: %s", e)
	}
	if ce.Code != "invalid_ui" || ce.Retryable || len(ce.Details) != 2 || !strings.Contains(ce.Suggestion, "myco ui check") {
		t.Fatalf("error: %+v", ce)
	}
	if ce.Details[0].Path != "Card > Metric[0].props" || !strings.Contains(ce.Details[1].Message, `unknown component "Carousel"`) {
		t.Fatalf("details: %+v", ce.Details)
	}
	if code, _, _ := h.run("ui", "check"); code != ExitUsage {
		t.Fatalf("missing --file: exit %d", code)
	}
	if code, _, _ := h.run("ui", "check", "--file", filepath.Join(dir, "nope.json")); code != ExitNotFound {
		t.Fatalf("missing file: exit %d", code)
	}
}

func TestAppendUIBlock(t *testing.T) {
	got, err := appendUIBlock("All green.\n", []byte(goodBlock))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "All green.\n\n```myco-ui\n{\n  \"type\": \"Card\"") || !strings.HasSuffix(got, "\n```") {
		t.Fatalf("body:\n%s", got)
	}
	if got, _ := appendUIBlock("", []byte(goodBlock)); !strings.HasPrefix(got, "```myco-ui\n") {
		t.Fatalf("empty body: %q", got)
	}
	_, err = appendUIBlock("x", []byte(`{"type":"Nope"}`))
	if ce, ok := err.(*Error); !ok || ce.Code != "invalid_ui" || ce.Exit != ExitUsage || len(ce.Details) != 1 {
		t.Fatalf("bad block: %#v", err)
	}
}

func TestMessageUIFlag(t *testing.T) {
	hub := newHub(t)
	dir := t.TempDir()
	mk := func(name string) *harnessT {
		return &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, name+".yaml")}}
	}
	boss, worker := mk("boss"), mk("worker")
	for name, h := range map[string]*harnessT{"boss": boss, "worker": worker} {
		if code, _, e := h.run("agent", "register", "--name", name); code != 0 {
			t.Fatalf("register %s: %d %s", name, code, e)
		}
	}
	block := filepath.Join(dir, "block.json")
	os.WriteFile(block, []byte(goodBlock), 0o644)
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte(`{"type":"Metric"}`), 0o644)

	// A bad block fails before anything is sent.
	code, _, e := boss.run("message", "send", "--to", "worker", "--body", "x", "--ui", bad)
	if code != ExitUsage || !strings.Contains(e, `"invalid_ui"`) {
		t.Fatalf("send bad ui: %d %s", code, e)
	}
	if code, out, _ := worker.run("inbox", "pull"); code != 0 || strings.Contains(out, `"id": "msg_`) {
		t.Fatalf("a rejected block must send nothing: %s", out)
	}
	// Dry run: checked and shown, not sent.
	code, out, _ := boss.run("message", "send", "--to", "worker", "--body", "Release check", "--ui", block, "--dry-run")
	if code != ExitDryRun || !strings.Contains(out, "myco-ui") {
		t.Fatalf("send dry-run: %d %s", code, out)
	}
	// The block is appended to the body.
	code, out, e = boss.run("message", "send", "--to", "worker", "--body", "Release check", "--ui", block)
	if code != 0 {
		t.Fatalf("send: %d %s", code, e)
	}
	var sent struct{ Message struct{ ID, Body string } }
	json.Unmarshal([]byte(out), &sent)
	if !strings.HasPrefix(sent.Message.Body, "Release check\n\n```myco-ui\n{") || !strings.HasSuffix(sent.Message.Body, "\n```") {
		t.Fatalf("sent body: %q", sent.Message.Body)
	}
	id := sent.Message.ID
	worker.run("inbox", "pull")

	// progress: the block alone is a body.
	code, out, e = worker.run("message", "progress", "--id", id, "--ui", block)
	if code != 0 || !strings.Contains(out, `"body": "`+"```myco-ui") {
		t.Fatalf("progress: %d %s %s", code, out, e)
	}
	if code, _, _ := worker.run("message", "progress", "--id", id); code != ExitUsage {
		t.Fatalf("progress without body or ui: %d", code)
	}
	if code, _, e := worker.run("message", "progress", "--id", id, "--ui", bad); code != ExitUsage || !strings.Contains(e, "invalid_ui") {
		t.Fatalf("progress bad ui: %d %s", code, e)
	}

	// reply with text and a block.
	if code, _, e := worker.run("message", "reply", "--id", id, "--ui", bad, "--body", "done"); code != ExitUsage || !strings.Contains(e, "invalid_ui") {
		t.Fatalf("reply bad ui: %d %s", code, e)
	}
	if code, _, e := worker.run("message", "reply", "--id", id, "--body", "All green.", "--ui", block); code != 0 {
		t.Fatalf("reply: %d %s", code, e)
	}
	code, out, _ = boss.run("message", "wait", "--id", id, "--timeout", "1s", "--format", "json")
	if code != 0 || !strings.Contains(out, `All green.\n\n`+"```myco-ui") {
		t.Fatalf("reply body: %d %s", code, out)
	}
}
