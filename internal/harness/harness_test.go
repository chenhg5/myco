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

func TestForkMarker(t *testing.T) {
	m := ForkMarker("01a182b8-8682-7251")
	if src, ok := ForkSource(m); !ok || src != "01a182b8-8682-7251" {
		t.Fatalf("%q -> %q %v", m, src, ok)
	}
	if ForkMarker("s") == ForkMarker("s") {
		t.Fatal("markers must differ")
	}
	for _, s := range []string{"", "sess-1", "fork:", "fork:x"} {
		if _, ok := ForkSource(s); ok {
			t.Errorf("%q is not a marker", s)
		}
	}
}
