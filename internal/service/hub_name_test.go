package service

import "testing"

func TestHubIDFor(t *testing.T) {
	if HubIDFor("") != HubID {
		t.Fatal("default hub identity changed")
	}
	id := HubIDFor("work")
	if id.Label != "ai.myco.hub.work" || id.Unit != "myco-hub-work.service" {
		t.Fatalf("named hub identity: %+v", id)
	}
	for _, name := range []string{"work", "a", "123", "a-b"} {
		if !ValidHubName(name) {
			t.Fatalf("valid name rejected: %q", name)
		}
	}
	for _, name := range []string{"Bad", "space name", "../x", "a_b", "123456789012345678901234567890123"} {
		if ValidHubName(name) {
			t.Fatalf("invalid name accepted: %q", name)
		}
	}
}
