package main

import "testing"

func TestScreenPromptRequestsOnceAndRechecks(t *testing.T) {
	var p screenPrompt
	granted := false
	requests := 0
	check := func() bool { return granted }
	request := func() bool { requests++; return granted }
	for range 3 {
		if p.allowed(check, request) {
			t.Fatal("denied access was allowed")
		}
	}
	if requests != 1 {
		t.Fatalf("prompted %d times", requests)
	}
	granted = true
	if !p.allowed(check, request) || requests != 1 {
		t.Fatal("new grant was not recognized without another dialog")
	}
	granted = false
	if p.allowed(check, request) || requests != 1 {
		t.Fatal("revocation ignored or dialog repeated")
	}
}
func TestScreenPromptSkipsDialogForExistingGrant(t *testing.T) {
	var p screenPrompt
	if !p.allowed(func() bool { return true }, func() bool { t.Fatal("unnecessary dialog"); return false }) {
		t.Fatal("existing grant rejected")
	}
}
