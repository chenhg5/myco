package browserpolicy

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func policyFixture() (Scope, Request) {
	return Scope{Workspace: "https://hub.test/w/team", Owner: "member-one", Page: "pg_browser"}, Request{ScriptID: "report", Version: 3, Profile: "profile_work", Script: json.RawMessage(`{"name":"Monthly report","profile":"profile_work","start_url":"https://example.com","params":[],"graph":{"nodes":[{"id":"confirm","type":"confirm","message":"Publish?"}],"edges":[]},"on_error":"stop"}`)}
}
func TestApprovedSnapshotsRequireExactLocalScopeAndGeneration(t *testing.T) {
	scope, req := policyFixture()
	s := New(t.TempDir())
	approval, binding, err := s.ApproveConfirmed(scope, req, time.Now())
	if err != nil || approval.Digest == "" || binding == "" {
		t.Fatalf("approve: %+v %q %v", approval, binding, err)
	}
	if _, err := s.Approved(scope, req.Profile, binding, req.ScriptID, req.Version); !errors.Is(err, ErrDenied) {
		t.Fatalf("disabled profile: %v", err)
	}
	p, err := s.SetEnabled(scope, req.Profile, true)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := s.Approved(scope, req.Profile, p.Binding, req.ScriptID, req.Version)
	if err != nil {
		t.Fatal(err)
	}
	// Neither caller-owned input nor a returned snapshot can mutate saved approval.
	req.Script[2] = 'X'
	snap.Script[2] = 'Y'
	got, err := New(s.dir).Approved(scope, req.Profile, p.Binding, req.ScriptID, req.Version)
	if err != nil || !strings.Contains(string(got.Script), "Monthly report") {
		t.Fatalf("not immutable: %s %v", got.Script, err)
	}
	for _, other := range []Scope{{Workspace: scope.Workspace, Owner: "other", Page: scope.Page}, {Workspace: "https://hub.test/w/other", Owner: scope.Owner, Page: scope.Page}, {Workspace: scope.Workspace, Owner: scope.Owner, Page: "pg_other"}} {
		if _, err := s.Approved(other, req.Profile, p.Binding, req.ScriptID, req.Version); !errors.Is(err, ErrDenied) {
			t.Fatalf("scope leaked: %+v %v", other, err)
		}
	}
	if _, err := s.Approved(scope, req.Profile, binding, req.ScriptID, req.Version); !errors.Is(err, ErrDenied) {
		t.Fatal("old policy generation accepted")
	}
	if _, err := s.Approved(scope, req.Profile, p.Binding, req.ScriptID, req.Version+1); !errors.Is(err, ErrDenied) {
		t.Fatal("unapproved version accepted")
	}
	if _, err := s.SetEnabled(scope, req.Profile, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Approved(scope, req.Profile, p.Binding, req.ScriptID, req.Version); !errors.Is(err, ErrDenied) {
		t.Fatal("disabled policy stayed executable")
	}
}
func TestApprovalVersionsAndChildSnapshotsCannotChangeSilently(t *testing.T) {
	scope, req := policyFixture()
	s := New(t.TempDir())
	now := time.Now()
	req.Script = json.RawMessage(strings.Replace(string(req.Script), `"nodes":[`, `"nodes":[{"id":"sub","type":"subflow","script_id":"child"},`, 1))
	req.Scripts = map[string]json.RawMessage{"child": json.RawMessage(`{"name":"Child","code":"original"}`)}
	a, binding, err := s.ApproveConfirmed(scope, req, now)
	if err != nil {
		t.Fatal(err)
	}
	req.Script = json.RawMessage(" \n" + string(req.Script) + "\n")
	again, b2, err := s.ApproveConfirmed(scope, req, now.Add(time.Hour))
	if err != nil || again.Digest != a.Digest || b2 != binding || !again.ApprovedAt.Equal(a.ApprovedAt) {
		t.Fatalf("idempotent approve: %+v %v", again, err)
	}
	req.Scripts["child"] = json.RawMessage(`{"name":"Child","code":"different"}`)
	if _, _, err := s.ApproveConfirmed(scope, req, now); !errors.Is(err, ErrConflict) {
		t.Fatalf("child changed behind approval: %v", err)
	}
	req.Version++
	_, newBinding, err := s.ApproveConfirmed(scope, req, now)
	if err != nil || newBinding == binding {
		t.Fatalf("new version must invalidate old grants: %v", err)
	}
	revoked, err := s.Revoke(scope, req.Profile, req.ScriptID, req.Version)
	if err != nil || revoked == newBinding {
		t.Fatalf("revoke generation: %v", err)
	}
	p, err := s.SetEnabled(scope, req.Profile, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Approved(scope, req.Profile, p.Binding, req.ScriptID, req.Version); !errors.Is(err, ErrDenied) {
		t.Fatal("revoked version accepted")
	}
	old, err := s.Approved(scope, req.Profile, p.Binding, req.ScriptID, req.Version-1)
	if err != nil || !strings.Contains(string(old.Scripts["child"]), "original") {
		t.Fatalf("old immutable child lost: %+v %v", old, err)
	}
	other := scope
	other.Page = "pg_unrelated"
	before := p.Binding
	after, err := s.Revoke(other, req.Profile, req.ScriptID, 0)
	if err != nil || before != after {
		t.Fatal("other page revoked approval")
	}
}
func TestBrowserPolicyFileLockCorruptionAndPermissions(t *testing.T) {
	scope, req := policyFixture()
	s := New(t.TempDir())
	_, _, err := s.ApproveConfirmed(scope, req, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.SetEnabled(scope, req.Profile, true)
	if err != nil {
		t.Fatal(err)
	}
	path, err := s.path(scope, req.Profile)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := lockPolicy(path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(s.dir).SetEnabled(scope, req.Profile, false); err == nil {
		t.Fatal("another desktop ignored policy file lock")
	}
	unlock()
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("private mode: %v %v", info, err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.ReplaceAll(string(data), "Monthly report", "Tampered report"))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Approved(scope, req.Profile, p.Binding, req.ScriptID, req.Version); !errors.Is(err, ErrDenied) {
		t.Fatalf("digest corruption accepted: %v", err)
	}
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetEnabled(scope, req.Profile, true); err == nil {
		t.Fatal("corrupt policy silently reset")
	}
	bad := New(filepath.Join(s.dir, "blocked"))
	if err := os.WriteFile(bad.dir, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := bad.ApproveConfirmed(scope, req, time.Now()); err == nil {
		t.Fatal("failed persistence reported approval")
	}
}

func TestDedicatedProfileRetriesActivationAndHonorsDisable(t *testing.T) {
	scope, _ := policyFixture()
	store := New(t.TempDir())
	created, attempts := 0, 0
	create := func() (string, string, error) {
		created++
		return "p_agent", "agent 专用", nil
	}
	retry := errors.New("component interrupted activation")
	activate := func(id string) error {
		if id != "p_agent" {
			t.Fatalf("activated the wrong profile: %q", id)
		}
		attempts++
		if attempts == 1 {
			return retry
		}
		return nil
	}
	if _, err := store.EnsureAgentProfile(scope, create, activate); !errors.Is(err, retry) {
		t.Fatalf("first activation: %v", err)
	}
	store = New(store.dir)
	p, err := store.EnsureAgentProfile(scope, create, activate)
	if err != nil || created != 1 || attempts != 2 || !p.Enabled || p.Kind != "agent" {
		t.Fatalf("retry: %+v %v (created %d, activation attempts %d)", p, err, created, attempts)
	}
	if _, err := store.SetEnabled(scope, p.Profile, false); err != nil {
		t.Fatal(err)
	}
	p, err = store.EnsureAgentProfile(scope, create, activate)
	if err != nil || p.Enabled || p.Kind != "agent" || attempts != 2 {
		t.Fatalf("profile re-enabled after owner disabled it: %+v %v", p, err)
	}
}
