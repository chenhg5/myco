package httpapi_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
)

func TestResumeKeepsIdentityAndInbox(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", map[string]any{"binding": map[string]string{"harness": "inbox", "session_id": "bob-session"}})
	beforeID := str(h.do("GET", "/v1/me", alice, nil).want(t, 200).body, "id")
	admin, _ := client.New(h.url, adminToken)
	if _, err := admin.CreateProject(context.Background(), api.CreateProjectRequest{Name: "work"}); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.PutProjectMember(context.Background(), "work", "alice", api.ProjectMemberRequest{}); err != nil {
		t.Fatal(err)
	}
	id := h.send(bob, "alice", "waiting for you")
	link := h.do("POST", "/v1/me/resume", alice, nil).want(t, 200)
	token := str(link.body, "token")
	if !strings.HasPrefix(token, app.ResumeTokenPrefix) || str(link.body, "command") == "" {
		t.Fatalf("resume link: %s", link.raw)
	}
	h.do("POST", "/v1/resume", "", map[string]any{"token": token, "binding": map[string]string{"harness": "inbox", "session_id": "bob-session", "environment": "new-sandbox"}}).wantErr(t, 409, "conflict")
	// Rejoining from a different environment and session leaves the address
	// and all messages intact, while invalidating the old login token.
	res := h.do("POST", "/v1/resume", "", map[string]any{"token": token, "binding": map[string]string{
		"harness": "inbox", "session_id": "new-session", "environment": "new-sandbox", "workdir": "/tmp/new"}}).want(t, 200)
	newToken := str(res.body, "token")
	if str(res.body, "agent", "name") != "alice" || str(res.body, "agent", "id") != beforeID || newToken == "" {
		t.Fatalf("resume: %s", res.raw)
	}
	h.do("GET", "/v1/me", alice, nil).wantErr(t, 401, "unauthenticated")
	me := h.do("GET", "/v1/me", newToken, nil).want(t, 200)
	if str(me.body, "binding", "environment") != "new-sandbox" || str(me.body, "resume", "last_used_env") != "new-sandbox" {
		t.Fatalf("binding and metadata: %s", me.raw)
	}
	h.do("GET", "/v1/messages/"+id, newToken, nil).want(t, 200)
	project, err := admin.GetProject(context.Background(), "work", false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range project.Members {
		if m.Name == "alice" {
			found = true
		}
	}
	if !found {
		t.Fatal("resumed agent lost project membership")
	}
	// The same recovery token is reusable after another sandbox reset.
	h.do("POST", "/v1/resume", "", map[string]any{"token": token, "binding": map[string]string{"harness": "inbox", "session_id": "third-session", "environment": "third-sandbox"}}).want(t, 200)
	rotated := h.do("POST", "/v1/me/resume", newToken, nil)
	rotated.wantErr(t, 401, "unauthenticated") // previous login was rotated again
}

func TestResumeRotateRevokeAndPermissions(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice, bob := h.register("alice", nil), h.register("bob", nil)
	h.do("POST", "/v1/agents/alice/resume", bob, nil).wantErr(t, 403, "permission_denied")
	h.do("DELETE", "/v1/agents/alice/resume", bob, nil).wantErr(t, 403, "permission_denied")
	first := str(h.do("POST", "/v1/me/resume", alice, nil).want(t, 200).body, "token")
	if got := h.do("POST", "/v1/me/resume?if_missing=true", alice, nil).want(t, 200); str(got.body, "token") != "" {
		t.Fatalf("if_missing rotated or revealed an existing token: %s", got.raw)
	}
	if str(h.do("GET", "/v1/agents/alice", alice, nil).want(t, 200).body, "resume", "created_at") == "" {
		t.Fatal("self detail missing resume metadata")
	}
	if str(h.do("GET", "/v1/agents/alice", adminToken, nil).want(t, 200).body, "resume", "created_at") == "" {
		t.Fatal("admin detail missing resume metadata")
	}
	if str(h.do("GET", "/v1/me", alice, nil).want(t, 200).body, "resume", "created_at") == "" {
		t.Fatal("self /me missing resume metadata")
	}
	ov := h.do("GET", "/v1/overview", alice, nil).want(t, 200)
	visible := false
	for _, row := range ov.body["agents"].([]any) {
		item := row.(map[string]any)["agent"].(map[string]any)
		if item["name"] == "alice" && item["resume"] != nil {
			visible = true
		}
		if item["name"] == "bob" && item["resume"] != nil {
			t.Fatal("overview leaked another agent's resume metadata")
		}
	}
	if !visible {
		t.Fatal("overview omitted self resume metadata")
	}
	second := str(h.do("POST", "/v1/me/resume", alice, nil).want(t, 200).body, "token")
	if first == second {
		t.Fatal("rotation reused token")
	}
	use := func(tok string) resp {
		return h.do("POST", "/v1/resume", "", map[string]any{"token": tok, "binding": map[string]string{"harness": "inbox", "session_id": "test-session", "environment": "test"}})
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
	if runtime.GOOS == "windows" {
		return
	}
	// Run the script with a harmless installer and CLI on PATH. Neither the
	// shell script nor its log may print the credential while reconnecting.
	bin := t.TempDir()
	for name, body := range map[string]string{
		"curl": "#!/bin/sh\nprintf '#!/bin/bash\\n'\n",
		"myco": "#!/bin/sh\nprintf 'resumed\\n'\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("sh")
	cmd.Stdin = strings.NewReader(script)
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	output, err := cmd.CombinedOutput()
	if err != nil || strings.Contains(string(output), tok) {
		t.Fatalf("script output leaked token or failed: %v %q", err, output)
	}
}

func TestResumeLinkMemberOwnership(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	admin, _ := client.New(h.url, adminToken)
	ctx := context.Background()
	invite, err := admin.InviteMember(ctx, api.InviteRequest{Name: "member-a"})
	if err != nil {
		t.Fatal(err)
	}
	guest, _ := client.New(h.url, "")
	paired, err := guest.Pair(ctx, invite.Link[strings.Index(invite.Link, "/c/")+3:], "device-a")
	if err != nil {
		t.Fatal(err)
	}
	owner, _ := client.New(h.url, paired.Token)
	enr, err := owner.CreateEnrollment(ctx, api.CreateEnrollmentRequest{NamePrefix: "member-a-"}, false)
	if err != nil {
		t.Fatal(err)
	}
	enroller, _ := client.New(h.url, enr.Token)
	joined, err := enroller.Enroll(ctx, api.EnrollRequest{Name: "member-a-bot", Binding: api.Binding{Harness: "inbox", SessionID: "sess-a"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = joined
	h.do("POST", "/v1/agents/member-a-bot/resume", paired.Token, nil).want(t, 200)
	otherInvite, err := admin.InviteMember(ctx, api.InviteRequest{Name: "member-b"})
	if err != nil {
		t.Fatal(err)
	}
	otherPair, err := guest.Pair(ctx, otherInvite.Link[strings.Index(otherInvite.Link, "/c/")+3:], "device-b")
	if err != nil {
		t.Fatal(err)
	}
	h.do("POST", "/v1/agents/member-a-bot/resume", otherPair.Token, nil).wantErr(t, 403, "permission_denied")
	h.do("DELETE", "/v1/agents/member-a-bot/resume", otherPair.Token, nil).wantErr(t, 403, "permission_denied")
	h.do("DELETE", "/v1/agents/member-a-bot/resume", paired.Token, nil).want(t, 200)
}
