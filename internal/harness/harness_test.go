package harness

import "testing"

func TestErrorLinesDropsWarnings(t *testing.T) {
	in := "2026 WARN plugin x missing\n2026 WARN plugin y missing\n2026 ERROR thread busy\nError: boom"
	if got := errorLines(in); got != "2026 ERROR thread busy\nError: boom" {
		t.Fatalf("got %q", got)
	}
	if got := errorLines("2026 WARN only"); got != "2026 WARN only" {
		t.Fatalf("all-warn input must be kept: %q", got)
	}
}
