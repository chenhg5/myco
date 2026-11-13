package httpapi_test

import (
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/app"
)

func TestResumeKeepsIdentityAndInbox(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", nil)
	id := h.send(bob, "alice", "waiting for you")
	link := h.do("POST", "/v1/me/resume", alice, nil).want(t, 200)
	token := str(link.body, "token")
	if !strings.HasPrefix(token, app.ResumeTokenPrefix) || str(link.body, "command") == "" {
		t.Fatalf("resume link: %s", link.raw)
	}
	// Rejoining from a different environment and session leaves the address
	// and all messages intact, while invalidating the old login token.
	res := h.do("POST", "/v1/resume", "", map[string]any{"token": token, "binding": map[string]string{
		"harness": "inbox", "session_id": "new-session", "environment": "new-sandbox", "workdir": "/tmp/new"}}).want(t, 200)
	newToken := str(res.body, "token")
	if str(res.body, "agent", "name") != "alice" || newToken == "" {
		t.Fatalf("resume: %s", res.raw)
	}
	h.do("GET", "/v1/me", alice, nil).wantErr(t, 401, "unauthenticated")
	me := h.do("GET", "/v1/me", newToken, nil).want(t, 200)
	if str(me.body, "binding", "environment") != "new-sandbox" || str(me.body, "resume", "last_used_env") != "new-sandbox" {
		t.Fatalf("binding and metadata: %s", me.raw)
	}
	h.do("GET", "/v1/messages/"+id, newToken, nil).want(t, 200)
	// The same recovery token is reusable after another sandbox reset.
	h.do("POST", "/v1/resume", "", map[string]any{"token": token, "binding": map[string]string{"harness": "inbox", "environment": "third-sandbox"}}).want(t, 200)
	rotated := h.do("POST", "/v1/me/resume", newToken, nil)
	rotated.wantErr(t, 401, "unauthenticated") // previous login was rotated again
}

func TestResumeRotateRevokeAndPermissions(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice, bob := h.register("alice", nil), h.register("bob", nil)
	h.do("POST", "/v1/agents/alice/resume", bob, nil).wantErr(t, 403, "permission_denied")
	h.do("DELETE", "/v1/agents/alice/resume", bob, nil).wantErr(t, 403, "permission_denied")
	first := str(h.do("POST", "/v1/me/resume", alice, nil).want(t, 200).body, "token")
	second := str(h.do("POST", "/v1/me/resume", alice, nil).want(t, 200).body, "token")
	if first == second {
		t.Fatal("rotation reused token")
	}
	use := func(tok string) resp {
		return h.do("POST", "/v1/resume", "", map[string]any{"token": tok, "binding": map[string]string{"harness": "inbox", "environment": "test"}})
	}
	use(first).wantErr(t, 401, "unauthenticated")
	use(second).want(t, 200)
	if _, ok := h.do("GET", "/v1/agents/alice", bob, nil).want(t, 200).body["resume"]; ok {
		t.Fatal("other agent saw resume metadata")
	}
	h.do("DELETE", "/v1/agents/alice/resume", adminToken, nil).want(t, 200)
	use(second).wantErr(t, 401, "unauthenticated")
}

func TestResumeScriptNoOtherCredentials(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	link := h.do("POST", "/v1/me/resume", alice, nil).want(t, 200)
	tok := str(link.body, "token")
	script := string(h.do("GET", "/r/"+tok, "", nil).want(t, 200).raw)
	if !strings.Contains(script, "agent resume --token") || !strings.Contains(script, tok) || strings.Contains(script, alice) || strings.Contains(script, adminToken) || strings.Contains(script, "echo "+tok) {
		t.Fatalf("unexpected resume script: %s", script)
	}
	h.do("GET", "/r/bad", "", nil).want(t, 404)
}
