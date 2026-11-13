// Package skill carries the myco skill (myco/SKILL.md) inside the binary,
// so the guide an agent reads always matches the CLI it runs.
package skill

import _ "embed"

// Myco is skill/myco/SKILL.md as built into this binary.
//
//go:embed myco/SKILL.md
var Myco string
