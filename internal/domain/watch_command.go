package domain

import (
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const WatchHistoryLimit = 5

// CommandCheckTimeout leaves room between checks and bounds each process.
func CommandCheckTimeout(every int) time.Duration {
	return min(time.Duration(every)*time.Second/2, 60*time.Second)
}

var watchSecret = regexp.MustCompile(`(?i)myco_[a-z0-9_-]+|sk-[a-z0-9_-]+|gh[pousr]_[a-z0-9]+|github_pat_[a-z0-9_]+|xox[baprs]-[a-z0-9-]+|AIza[a-z0-9_-]{20,}|(?:AKIA|ASIA)[a-z0-9]{16}|eyJ[a-z0-9_-]+\.[a-z0-9_-]+\.[a-z0-9_-]+|https?://[^/@:\s]+:[^/@\s]+@|(?:bearer\s+\S+)|(?:token|secret|api[_-]?key|password|authorization)["']?\s*[:=]\s*(?:(?:bearer|basic)\s+)?(?:"(?:\\.|[^"\\\r\n])*"|'(?:\\.|[^'\\\r\n])*'|[^\s"',;]+)|[a-z0-9_+/=-]{40,}`)
var watchANSI = regexp.MustCompile("\x1b(?:\\[[0-?]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\))")

// SanitizeWatchResult redacts before truncating so a truncated credential
// cannot escape shape detection. Only the last nonempty, printable line survives.
func SanitizeWatchResult(text string) string {
	text = watchSecret.ReplaceAllString(watchANSI.ReplaceAllString(strings.ToValidUTF8(text, "�"), ""), "[redacted]")
	lines := strings.FieldsFunc(text, func(r rune) bool { return r == '\r' || r == '\n' })
	last := ""
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			last = line
		}
	}
	last = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, last)
	if len(last) > 1024 {
		last = last[:1021]
		for !utf8.ValidString(last) {
			last = last[:len(last)-1]
		}
		last += "…"
	}
	return last
}
