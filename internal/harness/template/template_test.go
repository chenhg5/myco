package template

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/testutil"
)

// fakeCLI writes a script that records its argv and prints canned output.
func fakeCLI(t *testing.T, output string) (bin, argsFile string) {
	t.Helper()
	testutil.NeedsPOSIXShell(t)
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	bin = filepath.Join(dir, "fake")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\ncat > " + filepath.Join(dir, "stdin") + "\ncat <<'EOF'\n" + output + "\nEOF\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, argsFile
}

func readArgs(t *testing.T, f string) []string {
	b, _ := os.ReadFile(f)
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

var piSpec = harness.TemplateSpec{
	Args:       []string{"-p", "--mode", "json", "{prompt}"},
	ResumeArgs: []string{"-p", "--mode", "json", "--session", "{session}", "{prompt}"},
	ModelArgs:  []string{"--model", "{model}"},
	AccessArgs: map[harness.Access][]string{harness.AccessFull: {"--yolo"}},
	Output:     "jsonl",
	Text:       &harness.Extract{Path: "message.content", When: map[string]string{"type": "message_end", "message.role": "assistant"}},
	Session:    &harness.Extract{Path: "id", When: map[string]string{"type": "session"}},
	Error:      &harness.Extract{Path: "message.errorMessage", When: map[string]string{"type": "message_end"}},
}

func TestJSONLPresetNewAndResume(t *testing.T) {
	out := `{"type":"session","id":"pi-123"}
{"type":"message_end","message":{"role":"user","content":[{"type":"text","text":"ignored"}]}}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"line 1"},{"type":"image"},{"type":"text","text":"line 2"}]}}`
	bin, argsFile := fakeCLI(t, out)
	spec := piSpec
	spec.Binary = bin
	h, err := Build("pi", spec, harness.Options{Model: "m1", Access: harness.AccessFull, Args: []string{"--extra"}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Run(context.Background(), harness.Request{Prompt: "-hello"})
	if err != nil || res.Output != "line 1\nline 2" || res.SessionID != "pi-123" {
		t.Fatalf("run: %+v %v", res, err)
	}
	want := []string{"-p", "--mode", "json", "--model", "m1", "--yolo", "--extra", "-hello"}
	if got := readArgs(t, argsFile); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("argv: %q", got)
	}
	if _, err := h.Run(context.Background(), harness.Request{Prompt: "again", SessionID: "pi-123"}); err != nil {
		t.Fatal(err)
	}
	if got := readArgs(t, argsFile); !strings.Contains(strings.Join(got, " "), "--session pi-123") || got[len(got)-1] != "again" {
		t.Fatalf("resume argv: %q", got)
	}
}

func TestErrorExtractionAndStdin(t *testing.T) {
	bin, _ := fakeCLI(t, `{"type":"message_end","message":{"role":"assistant","content":[],"errorMessage":"quota exceeded"}}`)
	spec := piSpec
	spec.Binary = bin
	h, _ := Build("pi", spec, harness.Options{})
	if _, err := h.Run(context.Background(), harness.Request{Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("want quota error, got %v", err)
	}

	// No {prompt} placeholder: the prompt goes to stdin; text output with a
	// session pattern.
	bin2, _ := fakeCLI(t, "answer text\nsession: abc-9")
	h2, err := Build("x", harness.TemplateSpec{Binary: bin2, Args: []string{"run"}, SessionPattern: `session: (\S+)`}, harness.Options{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := h2.Run(context.Background(), harness.Request{Prompt: "via stdin"})
	if err != nil || res.SessionID != "abc-9" || !strings.HasPrefix(res.Output, "answer text") {
		t.Fatalf("%+v %v", res, err)
	}
	stdin, _ := os.ReadFile(filepath.Join(filepath.Dir(bin2), "stdin"))
	if strings.TrimSpace(string(stdin)) != "via stdin" {
		t.Fatalf("stdin: %q", stdin)
	}
}

func TestSpecValidationAndOverride(t *testing.T) {
	if _, err := Build("x", harness.TemplateSpec{Binary: "b"}, harness.Options{}); err == nil {
		t.Fatal("args required")
	}
	if _, err := Build("x", harness.TemplateSpec{Binary: "b", Args: []string{"a"}, Output: "xml"}, harness.Options{}); err == nil {
		t.Fatal("bad output rejected")
	}
	// A user template overrides only the fields it sets.
	f := Preset("pi", harness.TemplateSpec{Binary: "pi", Args: []string{"-p", "{prompt}"}, Output: "jsonl"})
	h, err := f(harness.Options{Template: &harness.TemplateSpec{Binary: "/opt/pi"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := h.(*tmpl).spec; got.Binary != "/opt/pi" || got.Output != "jsonl" || len(got.Args) != 2 {
		t.Fatalf("merge: %+v", got)
	}
}
