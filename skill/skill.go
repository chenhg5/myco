// Package skill carries the myco skill (myco/SKILL.md) inside the binary,
// so the guide an agent reads always matches the CLI it runs.
package skill

import (
	_ "embed"
	"strings"
)

//go:embed myco/SKILL.md
var raw string

// Myco is skill/myco/SKILL.md as built into this binary, with Unix line
// endings whatever the checkout it was built from (git on Windows may
// write the file with CRLF).
var Myco = strings.ReplaceAll(raw, "\r\n", "\n")

// StructuredUIHint belongs in session/project context, not every task prompt.
const StructuredUIHint = "For test results, file changes, release checks, comparisons, key numbers or plan steps, add a myco-ui block with `--ui ui.json` on `myco message reply` / `myco message progress` with a short text conclusion; keep ordinary short answers as text. " +
	"Example ui.json: `{\"type\":\"TestResults\",\"props\":{\"passed\":3,\"failed\":0}}`; validate with `myco ui check --file ui.json`, and see `myco ui schema --format text` for components and props."
