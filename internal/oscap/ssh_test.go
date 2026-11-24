package oscap

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/sshcap"
	"github.com/chenhg5/myco/internal/testsshd"
)

func TestSSHNativePrivateProfilesAndFixedCalls(t *testing.T) {
	home, config := testsshd.Start(t)
	e := &Env{Home: home, Scope: "workspace-a\x00pg_test", SSH: &sshcap.Manager{Home: home, Config: config}}
	create, err := Call(context.Background(), e, "ssh.profiles", json.RawMessage(`{"op":"create","name":"Linux","destination":"fixture"}`))
	if err != nil {
		t.Fatal(err)
	}
	p := create.(sshcap.Profile)
	data, _ := json.Marshal(p)
	if strings.Contains(string(data), home) || strings.Contains(string(data), "PRIVATE KEY") || strings.Contains(string(data), "config") {
		t.Fatal("private configuration exposed")
	}
	// Unknown fields cannot silently accept a password/key or arbitrary command.
	for _, raw := range []string{`{"op":"create","destination":"fixture","password":"must-not-be-stored"}`, `{"op":"list","private_key":"must-not-be-stored"}`} {
		if _, err := Call(context.Background(), e, "ssh.profiles", json.RawMessage(raw)); err == nil || err.Code != CodeInvalid {
			t.Fatal("credentials accepted by profile API")
		}
	}
	request, _ := json.Marshal(map[string]any{"profile_id": p.ID, "binding": p.Binding, "action": "overview"})
	if _, err := Call(context.Background(), e, "ssh.run", request); err != nil {
		t.Fatal(err)
	}
	enable, _ := json.Marshal(map[string]any{"op": "agent_enabled", "id": p.ID, "agent_enabled": true})
	if _, err := Call(context.Background(), e, "ssh.profiles", enable); err != nil {
		t.Fatal(err)
	}
	got, _ := e.SSH.Get(p.ID)
	if !got.AgentEnabled || got.AgentHub != "workspace-a" {
		t.Fatalf("workspace opt-in %+v", got)
	}
	noScope := *e
	noScope.Scope = ""
	if _, err := Call(context.Background(), &noScope, "ssh.profiles", enable); err == nil || err.Code != CodeInvalid {
		t.Fatal("unscoped agent enable accepted")
	}
	if _, err := Call(context.Background(), e, "ssh.run", json.RawMessage(`{"profile_id":"anything","action":"shell","command":"touch /tmp/forbidden"}`)); err == nil {
		t.Fatal("arbitrary command accepted")
	}
}
