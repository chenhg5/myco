package builtin

import (
	"github.com/myco-ai/myco/internal/harness"
	"github.com/myco-ai/myco/internal/harness/acp"
)

// Presets are TemplateSpecs for CLIs without a dedicated adapter. pi and
// kiro follow formats used in production by chenhg5/bot-connect. Any field
// can be overridden per agent with runner.template in its config.
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
	// Options go before the acp subcommand.
	"cursor": {
		Binary:    "cursor-agent",
		Sub:       []string{"acp"},
		ModelArgs: []string{"--model", "{model}"},
		AccessArgs: map[harness.Access][]string{
			harness.AccessFull: {"--force"},
		},
		DirArgs: []string{"--add-dir", "{dir}"},
		Login:   "run cursor-agent login on this machine, or set CURSOR_API_KEY",
	},
	// opencode allows every tool unless configured to ask: below full
	// access it asks, and myco answers from the access level.
	"opencode": {
		Binary:      "opencode",
		Args:        []string{"acp"},
		ModelConfig: "model",
		AccessEnv: map[harness.Access][]string{
			harness.AccessDefault:   {opencodeAsk},
			harness.AccessReadonly:  {opencodeAsk},
			harness.AccessWorkspace: {opencodeAsk},
		},
		Login: "run opencode auth login on this machine",
	},
	// More CLIs that speak ACP: myco answers their permission requests from
	// the access level, so they need only how to start them.
	"qoder":     {Binary: "qodercli", Args: []string{"--acp"}, Login: "run qodercli on this machine and sign in"},
	"qwen":      {Binary: "qwen", Args: []string{"--acp"}, Login: "run qwen on this machine and sign in (or set an API key in its settings)"},
	"kimi":      {Binary: "kimi", Args: []string{"acp"}, Login: "run kimi on this machine and sign in"},
	"goose":     {Binary: "goose", Args: []string{"acp"}, Login: "run goose configure on this machine"},
	"copilot":   {Binary: "copilot", Args: []string{"--acp"}, Login: "run copilot on this machine and sign in"},
	"auggie":    {Binary: "auggie", Args: []string{"--acp"}, Login: "run auggie login on this machine"},
	"codebuddy": {Binary: "codebuddy", Args: []string{"--acp"}, Login: "run codebuddy on this machine and sign in"},
	"kilo":      {Binary: "kilo", Args: []string{"acp"}, Login: "run kilo on this machine and sign in"},
	"cline":     {Binary: "cline", Args: []string{"--acp"}, Login: "run cline on this machine and sign in"},
	"grok":      {Binary: "grok", Args: []string{"agent", "stdio"}, Login: "run grok on this machine and sign in"},
	"atomcode":  {Binary: "atomcode", Args: []string{"acp"}, Login: "run atomcode on this machine and sign in"},
	// Google Antigravity through the community google-antigravity-acp
	// adapter, which drives the agy CLI. It does not pass permission
	// requests on over ACP: it either lets agy approve everything (full
	// access) or leaves agy's own approvals on, which a headless run cannot
	// grant.
	"antigravity": {
		Binary:    "google-antigravity-acp",
		ModelArgs: []string{"-m", "{model}"},
		AccessArgs: map[harness.Access][]string{
			harness.AccessDefault:   {"--no-skip-permissions"},
			harness.AccessReadonly:  {"--no-skip-permissions"},
			harness.AccessWorkspace: {"--no-skip-permissions"},
		},
		Login: "install agy and sign in on this machine (an AI Studio or Vertex AI key is safer than a personal Antigravity login)",
	},
}

// opencodeAsk makes opencode ask before shell commands and edits.
const opencodeAsk = `OPENCODE_CONFIG_CONTENT={"permission":{"bash":"ask","edit":"ask"}}`

// legacyTemplates are how ACP presets were driven before ACP: an agent whose
// config has runner.template still runs this way, its fields over these.
var legacyTemplates = map[string]harness.TemplateSpec{
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
