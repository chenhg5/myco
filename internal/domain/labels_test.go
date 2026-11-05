package domain

import "testing"

func TestLabelsAndSelector(t *testing.T) {
	if _, err := ParseLabels([]string{"Team=x"}); err == nil {
		t.Fatal("uppercase key must be rejected")
	}
	l, err := ParseLabels([]string{"group=loops", "gpu", "group=release", "gpu", " machine=mac "})
	if err != nil || len(l) != 4 || l[0] != "gpu" {
		t.Fatalf("parse: %v %v", l, err)
	}
	a := &Agent{Name: "codex-1", Labels: l, Capabilities: []string{"code"}, Binding: Binding{Harness: "codex"}}
	cases := map[string]bool{
		"group=loops":                   true,
		"group=release,machine=mac":     true,
		"group=ops":                     false,
		"gpu":                           true,
		"!gpu":                          false,
		"!cloud":                        true,
		"machine!=vps":                  true,
		"machine!=mac":                  false,
		"harness=codex,capability=code": true,
		"name=codex-1":                  true,
		"":                              true,
	}
	for sel, want := range cases {
		s, err := ParseSelector(sel)
		if err != nil {
			t.Fatalf("%q: %v", sel, err)
		}
		if got := s.Matches(a); got != want {
			t.Errorf("%q: got %v want %v", sel, got, want)
		}
	}
	for _, bad := range []string{"group=", "=x", "A=b", "a b"} {
		if _, err := ParseSelector(bad); err == nil {
			t.Errorf("%q should be invalid", bad)
		}
	}
	next, err := MergeLabels(l, []string{"group=ops"}, []string{"group", "gpu"})
	if err != nil || len(next) != 1 || next[0] != "machine=mac" {
		t.Fatalf("merge: %v %v", next, err)
	}
}
