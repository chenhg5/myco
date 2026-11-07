package builtin

import (
	"cmp"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/harness"
)

// readmeHarnesses are the harnesses the README advertises.
var readmeHarnesses = []string{"claude", "codex", "cursor", "exec", "gemini", "kiro", "opencode", "pi", "shell", "template"}

func TestRegistryHasEveryAdvertisedHarness(t *testing.T) {
	got := Registry().Names()
	if !slices.Equal(got, readmeHarnesses) {
		t.Fatalf("registry = %v, want %v", got, readmeHarnesses)
	}
	if _, err := Registry().New("nope", harness.Options{}); err == nil || !strings.Contains(err.Error(), "available") {
		t.Fatalf("unknown harness: %v", err)
	}
}

func TestEveryHarnessBuildsWithItsMinimalOptions(t *testing.T) {
	r := Registry()
	opts := map[string]harness.Options{
		"shell":    {Command: "cat"},
		"template": {Template: &harness.TemplateSpec{Binary: "x", Args: []string{"{prompt}"}}},
	}
	for _, name := range readmeHarnesses {
		h, err := r.New(name, opts[name])
		if err != nil {
			t.Errorf("New(%s): %v", name, err)
			continue
		}
		if h.Name() != name {
			t.Errorf("New(%s).Name() = %q", name, h.Name())
		}
	}
	// Generic harnesses refuse to start unconfigured.
	for _, name := range []string{"shell", "template"} {
		if _, err := r.New(name, harness.Options{}); err == nil {
			t.Errorf("New(%s) without config should fail", name)
		}
	}
	// Only exec executes message bodies.
	for _, name := range readmeHarnesses {
		h, err := r.New(name, opts[name])
		if err != nil {
			continue
		}
		ce, ok := h.(harness.CommandExecutor)
		if is := ok && ce.ExecutesBody(); is != (name == "exec") {
			t.Errorf("%s: ExecutesBody = %v", name, is)
		}
	}
}

func TestPresetSpecsAreSane(t *testing.T) {
	if len(Presets) != 5 {
		t.Fatalf("presets = %d, want pi kiro cursor gemini opencode", len(Presets))
	}
	for name, spec := range Presets {
		if spec.Binary == "" || len(spec.Args) == 0 || len(spec.ResumeArgs) == 0 {
			t.Errorf("%s: binary/args/resume_args required: %+v", name, spec)
		}
		switch spec.Output {
		case "json", "jsonl":
		default:
			t.Errorf("%s: output %q", name, spec.Output)
		}
		if spec.Text == nil || spec.Session == nil || spec.Error == nil {
			t.Errorf("%s: text, session and error extraction rules required", name)
		}
		if !slices.ContainsFunc(spec.Args, hasPrompt) || !slices.ContainsFunc(spec.ResumeArgs, hasPrompt) {
			t.Errorf("%s: args and resume_args must carry {prompt}", name)
		}
		if !slices.ContainsFunc(spec.ResumeArgs, func(a string) bool { return strings.Contains(a, "{session}") }) {
			t.Errorf("%s: resume_args must carry {session}", name)
		}
		if slices.ContainsFunc(spec.Args, func(a string) bool { return strings.Contains(a, "{session}") }) {
			t.Errorf("%s: args (new session) must not reference {session}", name)
		}
		if !slices.Contains(spec.ModelArgs, "{model}") {
			t.Errorf("%s: model_args must carry {model}", name)
		}
	}
	for name := range Experimental {
		if _, ok := Presets[name]; !ok {
			t.Errorf("experimental %q is not a preset", name)
		}
	}
	for _, name := range []string{"pi", "kiro"} {
		if Experimental[name] {
			t.Errorf("%s is verified, not experimental", name)
		}
	}
}

func hasPrompt(a string) bool { return strings.Contains(a, "{prompt}") }

// fakeCLI writes an executable that records argv one per line and prints out.
func fakeCLI(t *testing.T, out string) (bin, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	bin = filepath.Join(dir, "fake")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + argsFile + "'\ncat <<'EOF'\n" + out + "\nEOF\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, argsFile
}

func readArgs(t *testing.T, f string) []string {
	t.Helper()
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

func TestPresetsRunEndToEnd(t *testing.T) {
	// Canned output in each CLI's format, in session S1. Append rules
	// (kiro, opencode) join the matching events with newlines.
	outputs := map[string]string{
		"pi":       `{"type":"session","id":"S1"}` + "\n" + `{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}`,
		"kiro":     `{"type":"content","text":"hi"}` + "\n" + `{"type":"content","text":"there"}` + "\n" + `{"type":"done","session_id":"S1"}`,
		"cursor":   `{"result":"hi","session_id":"S1","is_error":false}`,
		"gemini":   `{"response":"hi","session_id":"S1"}`,
		"opencode": `{"type":"step_start","sessionID":"S1"}` + "\n" + `{"type":"text","part":{"text":"hi"},"sessionID":"S1"}` + "\n" + `{"type":"text","part":{"text":"there"},"sessionID":"S1"}`,
	}
	want := map[string]string{"kiro": "hi\nthere", "opencode": "hi\nthere"}
	const prompt = "-fix the tests; rm -rf / $(x)"
	for name, spec := range Presets {
		t.Run(name, func(t *testing.T) {
			bin, argsFile := fakeCLI(t, outputs[name])
			workdir := t.TempDir()
			h, err := Registry().New(name, harness.Options{Binary: bin, Model: "m1", Access: harness.AccessFull})
			if err != nil {
				t.Fatal(err)
			}
			res, err := h.Run(context.Background(), harness.Request{Prompt: prompt, Workdir: workdir})
			wantOut := cmp.Or(want[name], "hi")
			if err != nil || res.Output != wantOut || res.SessionID != "S1" {
				t.Fatalf("run = %+v, %v", res, err)
			}
			args := readArgs(t, argsFile)
			checkArgv(t, args, prompt, workdir)
			if !slices.Contains(args, "m1") {
				t.Errorf("model not passed: %q", args)
			}
			for _, a := range spec.AccessArgs[harness.AccessFull] {
				if !slices.Contains(args, a) {
					t.Errorf("access arg %q missing: %q", a, args)
				}
			}
			if slices.Contains(args, "S1") {
				t.Errorf("new session must not pass a session id: %q", args)
			}

			// Resume continues the session.
			if _, err := h.Run(context.Background(), harness.Request{Prompt: prompt, Workdir: workdir, SessionID: "S1"}); err != nil {
				t.Fatal(err)
			}
			args = readArgs(t, argsFile)
			checkArgv(t, args, prompt, workdir)
			if i := slices.Index(args, "S1"); i < 1 || !strings.HasPrefix(args[i-1], "-") {
				t.Errorf("resume: session id should follow a flag: %q", args)
			}
		})
	}
}

func checkArgv(t *testing.T, args []string, prompt, workdir string) {
	t.Helper()
	if args[len(args)-1] != prompt {
		t.Errorf("prompt must be the last, single argument: %q", args)
	}
	for _, a := range args {
		if strings.Contains(a, "{") && a != prompt {
			t.Errorf("unexpanded placeholder %q in %q", a, args)
		}
		if a == "" {
			t.Errorf("empty argument in %q", args)
		}
	}
	if i := slices.Index(args, "--cwd"); i >= 0 && args[i+1] != workdir {
		t.Errorf("--cwd = %q, want %q", args[i+1], workdir)
	}
}

func TestPresetUserOverride(t *testing.T) {
	bin, argsFile := fakeCLI(t, `{"response":"ok"}`)
	h, err := Registry().New("gemini", harness.Options{Template: &harness.TemplateSpec{Binary: bin}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Run(context.Background(), harness.Request{Prompt: "p"}); err != nil {
		t.Fatal(err)
	}
	if got := readArgs(t, argsFile); !slices.Equal(got, []string{"--output-format", "json", "-p", "p"}) {
		t.Fatalf("argv = %q", got)
	}
	// The override must not mutate the shared preset.
	if Presets["gemini"].Binary != "gemini" {
		t.Fatalf("preset mutated: %+v", Presets["gemini"])
	}
}
