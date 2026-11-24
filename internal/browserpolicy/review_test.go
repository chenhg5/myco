package browserpolicy

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestReviewShowsCodeAndRejectsRuntimeCodeSubstitution(t *testing.T) {
	scope, req := policyFixture()
	req.Script = json.RawMessage(`{"name":"Report","start_url":"https://example.com","graph":{"nodes":[{"id":"read","type":"extract","what":"js","code":"return this.textContent"},{"id":"check","type":"assert","check":"expr","code":"document.title !== ''"},{"id":"child-run","type":"subflow","script_id":"child"},{"id":"visit","type":"navigate","url":"https://example.com/${path}"}]}}`)
	req.Scripts = map[string]json.RawMessage{"child": json.RawMessage(`{"name":"Child","start_url":"https://child.example.com","graph":{"nodes":[{"id":"child-code","type":"js","code":"return 1"}]}}`)}
	review, err := Prepare(scope, req)
	if err != nil || len(review.Code) != 3 || len(review.URLs) != 3 || len(review.Children) != 1 {
		t.Fatalf("review omitted executable content: %+v %v", review, err)
	}
	req.Script = json.RawMessage(strings.Replace(string(req.Script), "return this.textContent", `return '${payload}'`, 1))
	if _, err := Prepare(scope, req); err == nil {
		t.Fatal("parameter interpolation could inject code")
	}
	if _, _, err := New(t.TempDir()).ApproveConfirmed(scope, req, time.Now()); err == nil {
		t.Fatal("store bypassed code substitution guard")
	}
}
func TestPolicyNamespacingAndAmbiguousPageIDs(t *testing.T) {
	scope, req := policyFixture()
	s := New(t.TempDir())
	if _, _, err := s.ApproveConfirmed(scope, req, time.Now()); err != nil {
		t.Fatal(err)
	}
	second := scope
	second.Page = "pg_second"
	if _, _, err := s.ApproveConfirmed(second, req, time.Now()); err != nil {
		t.Fatal(err)
	}
	p, err := s.SetEnabled(scope, req.Profile, true, "Work")
	if err != nil {
		t.Fatal(err)
	}
	broad := scope
	broad.Page = ""
	if _, err := s.Approved(broad, req.Profile, p.Binding, req.ScriptID, req.Version); err == nil {
		t.Fatal("ambiguous source page was guessed")
	}
	if _, err := s.Approved(second, req.Profile, p.Binding, req.ScriptID, req.Version); err != nil {
		t.Fatal(err)
	}
	other := scope
	other.Workspace = "https://hub.test/w/another"
	if profiles, err := s.Profiles(other); err != nil || len(profiles) != 0 {
		t.Fatalf("cross-workspace enumeration: %+v %v", profiles, err)
	}
	profiles, err := s.Profiles(scope)
	if err != nil || len(profiles) != 1 || profiles[0].Name != "Work" {
		t.Fatalf("profile enumeration: %+v %v", profiles, err)
	}
}

func TestApprovalChildClosure(t *testing.T) {
	scope, req := policyFixture()
	req.Script = json.RawMessage(`{"name":"Parent","graph":{"nodes":[{"type":"subflow","script_id":"child"}]}}`)
	if _, err := Prepare(scope, req); err == nil {
		t.Fatal("missing child approved")
	}
	req.Scripts = map[string]json.RawMessage{"child": json.RawMessage(`{"name":"Child","graph":{"nodes":[]}}`)}
	if _, err := Prepare(scope, req); err != nil {
		t.Fatal(err)
	}
	req.Scripts["extra"] = json.RawMessage(`{"name":"Unused","graph":{"nodes":[]}}`)
	if _, err := Prepare(scope, req); err == nil {
		t.Fatal("unused code approved")
	}
}
