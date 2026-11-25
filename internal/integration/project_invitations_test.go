package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/hubserver"
	"github.com/myco-ai/myco/internal/infra/sqlite"
	"github.com/myco-ai/myco/internal/testutil"
)

func TestProjectInvitationsApprovalOwnershipOneUse(t *testing.T) {
	c := ctx(t)
	var logs bytes.Buffer
	dbPath := filepath.Join(t.TempDir(), "hub.db")
	built, err := hubserver.Build(hubserver.Options{DBPath: dbPath, AdminToken: adminToken, Policy: app.DefaultMessagePolicy(), OfficialDir: testutil.OfficialDir(t), OnlineWindow: time.Minute, Log: slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(built.Handler)
	h := &hub{t: t, db: dbPath, url: srv.URL, srv: srv, h: built}
	t.Cleanup(h.stop)
	admin := h.admin()
	console, err := admin.CreateConsoleToken(c, "owner-app")
	if err != nil {
		t.Fatal(err)
	}
	owner := h.client(console.Token)
	other, err := admin.InviteMember(c, api.InviteRequest{Name: "other"})
	if err != nil {
		t.Fatal(err)
	}
	coord := h.client(h.register(api.RegisterRequest{Name: "coord", Binding: api.Binding{Harness: "claude", SessionID: "coordinator"}}))
	helper := h.client(h.register(api.RegisterRequest{Name: "helper", Binding: api.Binding{Harness: "codex", SessionID: "helper"}}))
	if _, err = admin.SetAgentOwner(c, "coord", other.Member.Name); err != nil {
		t.Fatal(err)
	}
	p, err := owner.CreateProject(c, api.CreateProjectRequest{Name: "invite-test"})
	if err != nil {
		t.Fatal(err)
	}
	role := "coordinator"
	if _, err = owner.PutProjectMember(c, p.Name, "coord", api.ProjectMemberRequest{Role: &role}); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.PutProjectMember(c, p.Name, "helper", api.ProjectMemberRequest{}); err != nil {
		t.Fatal(err)
	}
	if p.CoordinatorInvites {
		t.Fatal("invites must default off")
	}
	for _, cl := range []*client.Client{coord, helper} {
		if _, err = cl.SetProjectCoordinatorInvites(c, p.Name, true); err == nil {
			t.Fatal("agent changed invite setting")
		}
	}
	if _, err = helper.InviteProjectAgent(c, p.Name, api.ProjectInviteRequest{}, false); err == nil {
		t.Fatal("non-coordinator invited")
	}
	pending, err := coord.InviteProjectAgent(c, p.Name, api.ProjectInviteRequest{DisplayName: "Reviewer", Description: "Review changes", Note: "review"}, false)
	if err != nil || pending.Status != "pending" || pending.Link != "" || pending.ApprovalID == "" {
		t.Fatalf("pending %+v %v", pending, err)
	}
	if _, err = coord.ProjectInvitation(c, p.Name, pending.ID, true); err == nil {
		t.Fatal("claimed without approval")
	}
	approvals, err := owner.Approvals(c, "pending")
	if err != nil || len(approvals) != 1 || approvals[0].Kind != "project_invite" {
		t.Fatalf("approval %+v %v", approvals, err)
	}
	if _, err = owner.DecideApproval(c, pending.ApprovalID, true); err != nil {
		t.Fatal(err)
	}
	status, err := coord.ProjectInvitation(c, p.Name, pending.ID, false)
	if err != nil || status.Status != "approved" || status.Link != "" {
		t.Fatalf("status %+v %v", status, err)
	}
	ready, err := coord.ProjectInvitation(c, p.Name, pending.ID, true)
	if err != nil || ready.Status != "ready" || ready.Link == "" || ready.Sentence != "Join myco: "+ready.Link {
		t.Fatalf("ready %s %v", ready.Status, err)
	}
	if time.Until(ready.ExpiresAt) < 23*time.Hour || time.Until(ready.ExpiresAt) > 25*time.Hour {
		t.Fatal("wrong link expiry")
	}
	if _, err = coord.ProjectInvitation(c, p.Name, pending.ID, true); err == nil {
		t.Fatal("link revealed twice")
	}
	token := strings.TrimPrefix(ready.Link, h.url+"/j/")
	response, err := http.Get(ready.Link + "?format=md")
	if err != nil {
		t.Fatal(err)
	}
	guide, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || !strings.Contains(string(guide), "one ordinary agent session") {
		t.Fatal("missing session-only guide")
	}
	if _, err = h.client(token).Join(c, "not-a-computer", false); err == nil {
		t.Fatal("invitation added a computer")
	}
	// Concurrent redeemers cannot both consume the link.
	var wg sync.WaitGroup
	var mu sync.Mutex
	var joined []*api.RegisterResponse
	for _, session := range []string{"new-one", "new-two"} {
		wg.Add(1)
		go func(session string) {
			defer wg.Done()
			out, err := h.client(token).Enroll(c, api.EnrollRequest{Binding: api.Binding{Harness: "inbox", SessionID: session}}, false)
			if err == nil {
				mu.Lock()
				joined = append(joined, out)
				mu.Unlock()
			}
		}(session)
	}
	wg.Wait()
	if len(joined) != 1 {
		t.Fatalf("joined %d agents", len(joined))
	}
	a := joined[0].Agent
	if a.OwnerID != p.OwnerID || a.InvitedBy != "coord" || a.DisplayName != "Reviewer" || a.Description != "Review changes" || a.Kind != "agent" {
		t.Fatalf("bad ownership/profile %+v", a)
	}
	after, err := owner.GetProject(c, p.Name, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range after.Members {
		if m.Name == a.Name {
			found = m.Role == "member" && m.Note == "review" && m.AddedBy == "coord"
		}
	}
	if !found {
		t.Fatal("agent not automatically added as ordinary member")
	}
	newAgent := h.client(joined[0].Token)
	if _, err = newAgent.InviteProjectAgent(c, p.Name, api.ProjectInviteRequest{}, false); err == nil {
		t.Fatal("invited member could invite")
	}
	if _, err = newAgent.PutProjectMember(c, p.Name, "helper", api.ProjectMemberRequest{Role: &role}); err == nil {
		t.Fatal("invited member could manage roles")
	}
	if _, err = h.client(token).Enroll(c, api.EnrollRequest{Binding: api.Binding{Harness: "inbox", SessionID: a.Binding.SessionID}}, false); err == nil {
		t.Fatal("one-use link allowed session recovery")
	}
	// Denied requests never mint credentials.
	denied, err := coord.InviteProjectAgent(c, p.Name, api.ProjectInviteRequest{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.DecideApproval(c, denied.ApprovalID, false); err != nil {
		t.Fatal(err)
	}
	denied, err = coord.ProjectInvitation(c, p.Name, denied.ID, false)
	if err != nil || denied.Status != "denied" {
		t.Fatal("denial not visible")
	}
	// Enabling the setting issues immediately, with no approval card.
	if _, err = owner.SetProjectCoordinatorInvites(c, p.Name, true); err != nil {
		t.Fatal(err)
	}
	direct, err := coord.InviteProjectAgent(c, p.Name, api.ProjectInviteRequest{}, false)
	if err != nil || direct.Link == "" || direct.ApprovalID != "" {
		t.Fatalf("direct %+v %v", direct, err)
	}
	db, err := sqlite.Open(h.db)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`UPDATE project_invitations SET expires_at=1 WHERE id=?`, direct.ID); err != nil {
		t.Fatal(err)
	}
	dt := strings.TrimPrefix(direct.Link, h.url+"/j/")
	if _, err = h.client(dt).Enroll(c, api.EnrollRequest{Binding: api.Binding{Harness: "inbox", SessionID: "expired"}}, false); err == nil {
		t.Fatal("expired link used")
	}
	audit, err := admin.Audit(c, client.AuditFilter{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(audit)
	for _, action := range []string{"project.invitation.create", "project.invitation.approve", "project.invitation.deny", "project.invitation.use"} {
		if !strings.Contains(string(raw), action) {
			t.Fatal("missing audit " + action)
		}
	}
	if strings.Contains(string(raw), token) || strings.Contains(string(raw), dt) {
		t.Fatal("credential leaked to audit")
	}
	h.stop()
	if strings.Contains(logs.String(), token) || strings.Contains(logs.String(), dt) {
		t.Fatal("plaintext invitation token in logs")
	}
	for _, file := range []string{h.db, h.db + "-wal"} {
		b, _ := os.ReadFile(file)
		if strings.Contains(string(b), token) || strings.Contains(string(b), dt) {
			t.Fatal("plaintext invitation token in database")
		}
	}
}
