package builtin

import (
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/harness/acp"
)

// Presets are TemplateSpecs for CLIs without a dedicated adapter. pi and
// kiro follow formats used in production by chenhg5/bot-connect; cursor and
// opencode are best-effort and marked experimental. Any field can be
// overridden per agent with runner.template in its config.
var Presets = map[string]harness.TemplateSpec{
	"pi": {
		Binary:     "pi",
		Args:       []string{"-p", "--mode", "json", "{prompt}"},
		ResumeArgs: []string{"-p", "--mode", "json", "--session", "{session}", "{prompt}"},
		ModelArgs:  []string{"--model", "{model}"},
		Output:     "jsonl",
		Text:       &harness.Extract{Path: "message.content", When: map[string]string{"type": "message_end", "message.role": "assistant"}},
		Session:    &harness.Extract{Path: "id", When: map[string]string{"type": "session"}},
		Error:      &harness.Extract{Path: "message.errorMessage", When: map[string]string{"type": "message_end"}},
	},
	"kiro": {
		Binary:     "kiro-acp",
		Args:       []string{"prompt", "--output", "json", "--cwd", "{workdir}", "{prompt}"},
		ResumeArgs: []string{"prompt", "--output", "json", "--cwd", "{workdir}", "--session", "{session}", "{prompt}"},
		ModelArgs:  []string{"--model", "{model}"},
		AccessArgs: map[harness.Access][]string{
			harness.AccessWorkspace: {"--permissions", "allow-all"},
			harness.AccessFull:      {"--permissions", "allow-all"},
		},
		Output:      "jsonl",
		Text:        &harness.Extract{Path: "text", When: map[string]string{"type": "content"}, Append: true},
		Session:     &harness.Extract{Path: "session_id"},
		Error:       &harness.Extract{Path: "message", When: map[string]string{"type": "error"}},
		Transcripts: []string{"~/.kiro/sessions/sess_%s/messages.jsonl"},
	},
	// experimental
	"cursor": {
		Binary:     "cursor-agent",
		Args:       []string{"-p", "--output-format", "json", "{prompt}"},
		ResumeArgs: []string{"-p", "--output-format", "json", "--resume", "{session}", "{prompt}"},
		ModelArgs:  []string{"--model", "{model}"},
		AccessArgs: map[harness.Access][]string{harness.AccessFull: {"--force"}},
		Output:     "json",
		Text:       &harness.Extract{Path: "result"},
		Session:    &harness.Extract{Path: "session_id"},
		Error:      &harness.Extract{Path: "result", When: map[string]string{"is_error": "true"}},
	},
	// experimental
	"opencode": {
		Binary:     "opencode",
		Args:       []string{"run", "--format", "json", "{prompt}"},
		ResumeArgs: []string{"run", "--format", "json", "--session", "{session}", "{prompt}"},
		ModelArgs:  []string{"--model", "{model}"},
		Output:     "jsonl",
		Text:       &harness.Extract{Path: "part.text", When: map[string]string{"type": "text"}, Append: true},
		Session:    &harness.Extract{Path: "sessionID"},
		Error:      &harness.Extract{Path: "error.data.message"},
	},
}

// ACPPresets are CLIs driven over the Agent Client Protocol, which reports
// every step of a run (the live view in the app).
var ACPPresets = map[string]acp.Spec{
	"gemini": {
		Binary:    "gemini",
		Args:      []string{"--acp"},
		ModelArgs: []string{"-m", "{model}"},
		AccessArgs: map[harness.Access][]string{
			harness.AccessWorkspace: {"--approval-mode", "auto_edit"},
			harness.AccessFull:      {"--yolo"},
		},
		DirArgs: []string{"--include-directories", "{dir}"},
		Login:   "run gemini once on this machine and sign in, or set GEMINI_API_KEY",
	},
}

// legacyTemplates are how ACP presets were driven before ACP: an agent whose
// config has runner.template still runs this way, its fields over these.
var legacyTemplates = map[string]harness.TemplateSpec{
	"gemini": {
		Binary:     "gemini",
		Args:       []string{"--output-format", "json", "-p", "{prompt}"},
		ResumeArgs: []string{"--output-format", "json", "--resume", "{session}", "-p", "{prompt}"},
		ModelArgs:  []string{"-m", "{model}"},
		AccessArgs: map[harness.Access][]string{
			harness.AccessWorkspace: {"--approval-mode", "auto_edit"},
			harness.AccessFull:      {"--yolo"},
		},
		Output:  "json",
		Text:    &harness.Extract{Path: "response"},
		Session: &harness.Extract{Path: "session_id"},
		Error:   &harness.Extract{Path: "error.message"},
	},
}

// Binary is the executable a preset runs ("" for other harnesses).
func Binary(name string) string {
	if p, ok := Presets[name]; ok {
		return p.Binary
	}
	if p, ok := ACPPresets[name]; ok {
		return p.Binary
	}
	return ""
}
