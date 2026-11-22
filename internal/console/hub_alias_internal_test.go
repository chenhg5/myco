package console

import (
	"slices"
	"testing"
)

func TestHubAliasesLoopback(t *testing.T) {
	t.Setenv("MYCO_LAN_ADDR", "192.168.1.9")
	got := hubAliases("http://127.0.0.1:7777/")
	for _, want := range []string{"http://192.168.1.9:7777", "http://localhost:7777", "http://[::1]:7777"} {
		if !slices.Contains(got, want) {
			t.Errorf("hubAliases = %v, want it to contain %s", got, want)
		}
	}
	if slices.Contains(got, "http://127.0.0.1:7777") {
		t.Errorf("hubAliases = %v, should not repeat the hub's own URL", got)
	}
	// a workspace under a path keeps it
	if got := hubAliases("http://localhost:7777/w/team"); !slices.Contains(got, "http://192.168.1.9:7777/w/team") {
		t.Errorf("hubAliases with a path = %v", got)
	}
}

func TestHubAliasesRemote(t *testing.T) {
	t.Setenv("MYCO_LAN_ADDR", "192.168.1.9")
	for _, u := range []string{"https://hub.example.com", "http://192.168.1.20:7777", "not a url"} {
		if got := hubAliases(u); len(got) != 0 {
			t.Errorf("hubAliases(%q) = %v, want none", u, got)
		}
	}
}
