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
