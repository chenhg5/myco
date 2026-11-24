package integration_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

func TestProjectAutomationsApprovalAndScope(t *testing.T) {
	ctx := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	console, e := admin.CreateConsoleToken(ctx, "owner-app")
	if e != nil {
		t.Fatal(e)
	}
	owner := h.client(console.Token)
	coord := h.client(h.register(api.RegisterRequest{Name: "coord", Binding: api.Binding{Harness: "inbox", SessionID: "coord"}}))
	member := h.client(h.register(api.RegisterRequest{Name: "worker", Binding: api.Binding{Harness: "inbox", SessionID: "worker"}}))
	h.register(api.RegisterRequest{Name: "outside", Binding: api.Binding{Harness: "inbox", SessionID: "outside"}})
	other, e := admin.InviteMember(ctx, api.InviteRequest{Name: "other"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = admin.SetAgentOwner(ctx, "coord", other.Member.Name); e != nil {
		t.Fatal(e)
	}
	p, e := owner.CreateProject(ctx, api.CreateProjectRequest{Name: "schedule-test"})
	if e != nil {
		t.Fatal(e)
	}
	role := "coordinator"
	if _, e = owner.PutProjectMember(ctx, p.Name, "coord", api.ProjectMemberRequest{Role: &role}); e != nil {
		t.Fatal(e)
	}
	if _, e = owner.PutProjectMember(ctx, p.Name, "worker", api.ProjectMemberRequest{}); e != nil {
		t.Fatal(e)
	}
	if p.CoordinatorAutomations {
		t.Fatal("permission must default off")
	}
	str := func(v string) *string { return &v }
	in := api.AutomationRequest{Name: str("Daily check"), To: str("worker"), Body: str("Check the project"), Schedule: &api.AutomationSchedule{Cron: "0 9 * * *"}, StartsAt: str("2026-11-26T00:00:00Z"), EndsAt: str("2026-11-29T00:00:00Z"), Once: ptr(true)}
	for _, cl := range []*client.Client{coord, member} {
		if _, e = cl.SetProjectCoordinatorAutomations(ctx, p.Name, true); e == nil {
			t.Fatal("agent changed project permission")
		}
	}
	if _, e = member.ChangeProjectAutomation(ctx, p.Name, "create", "", in); e == nil {
		t.Fatal("non-coordinator created task")
	}
	bad := in
	bad.To = str("outside")
	if _, e = coord.ChangeProjectAutomation(ctx, p.Name, "create", "", bad); e == nil {
		t.Fatal("outside target accepted")
	}
	bad = in
	bad.Project = str("another")
	if _, e = coord.ChangeProjectAutomation(ctx, p.Name, "create", "", bad); e == nil {
		t.Fatal("project could be changed")
	}
	bad = in
	bad.Webhook = ptr(true)
	if _, e = coord.ChangeProjectAutomation(ctx, p.Name, "create", "", bad); e == nil {
		t.Fatal("agent could create webhook credential")
	}
	request, e := coord.ChangeProjectAutomation(ctx, p.Name, "create", "", in)
	if e != nil || request.Status != "pending" || request.ApprovalID == "" || request.Automation != nil {
		t.Fatalf("pending %+v %v", request, e)
	}
	list, _ := coord.ProjectAutomations(ctx, p.Name)
	if len(list) != 0 {
		t.Fatal("request applied without approval")
	}
	approvals, e := owner.Approvals(ctx, "pending")
	if e != nil || len(approvals) != 1 || approvals[0].Kind != "project_automation" {
		t.Fatalf("approval %+v %v", approvals, e)
	}
	// Two simultaneous clicks approve at most one creation.
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = owner.DecideApproval(ctx, request.ApprovalID, true) }()
	}
	wg.Wait()
	list, e = coord.ProjectAutomations(ctx, p.Name)
	if e != nil || len(list) != 1 {
		t.Fatalf("applied list %+v %v", list, e)
	}
	a := list[0]
	if a.Owner != p.Owner || a.CreatedBy != "coord" || a.ApprovedBy == "" || a.Project != p.Name || a.To != "worker" || a.StartsAt == nil || a.EndsAt == nil || !a.Once {
		t.Fatalf("wrong owner or scope %+v", a)
	}
	// A different owner's node cannot see unrelated automations through this route.
	unrelated, e := owner.CreateAutomation(ctx, api.AutomationRequest{Name: str("Private"), To: str("outside"), Body: str("Private task"), Schedule: in.Schedule})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = coord.ChangeProjectAutomation(ctx, p.Name, "delete", unrelated.Automation.ID, api.AutomationRequest{}); e == nil {
		t.Fatal("deleted unrelated automation")
	}
	hook, e := owner.CreateAutomation(ctx, api.AutomationRequest{Name: str("Private webhook"), To: str("worker"), Project: &p.Name, Body: str("Webhook task"), Webhook: ptr(true)})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = coord.ChangeProjectAutomation(ctx, p.Name, "delete", hook.Automation.ID, api.AutomationRequest{}); e == nil {
		t.Fatal("coordinator could delete webhook automation")
	}
	onlySchedules, e := coord.ProjectAutomations(ctx, p.Name)
	if e != nil || len(onlySchedules) != 1 {
		t.Fatal("webhook leaked into schedule list", onlySchedules, e)
	}
	// Approving a stale request does not overwrite a human's newer change.
	pending, e := coord.ChangeProjectAutomation(ctx, p.Name, "update", a.ID, api.AutomationRequest{Body: str("Old requested content")})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = owner.UpdateAutomation(ctx, a.ID, api.AutomationRequest{Body: str("Owner's newer content")}); e != nil {
		t.Fatal(e)
	}
	if _, e = owner.DecideApproval(ctx, pending.ApprovalID, true); e == nil {
		t.Fatal("stale approval overwrote newer task")
	}
	denied, e := coord.ChangeProjectAutomation(ctx, p.Name, "delete", a.ID, api.AutomationRequest{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = owner.DecideApproval(ctx, denied.ApprovalID, false); e != nil {
		t.Fatal(e)
	}
	if _, e = owner.Automation(ctx, a.ID); e != nil {
		t.Fatal("denial deleted task")
	}
	// Direct path after enabling: change, pause and delete.
	if _, e = owner.SetProjectCoordinatorAutomations(ctx, p.Name, true); e != nil {
		t.Fatal(e)
	}
	changed, e := coord.ChangeProjectAutomation(ctx, p.Name, "update", a.ID, api.AutomationRequest{Enabled: ptr(false)})
	if e != nil || changed.Status != "applied" || changed.Automation.Enabled {
		t.Fatal("pause", changed, e)
	}
	if _, e = coord.ChangeProjectAutomation(ctx, p.Name, "delete", a.ID, api.AutomationRequest{}); e != nil {
		t.Fatal(e)
	}
	fresh, e := coord.ChangeProjectAutomation(ctx, p.Name, "create", "", in)
	if e != nil || fresh.Status != "applied" {
		t.Fatal("direct creation", fresh, e)
	}
	// Workflows are scoped by both their owner and project, not just a visible name.
	def := json.RawMessage(`{"name":"Scoped check","project":"schedule-test","steps":[{"id":"check","agent":"worker","prompt":"Check project"}],"edges":[{"from":"check","to":"$end"}]}`)
	wf, e := owner.ApplyWorkflow(ctx, def, false)
	if e != nil {
		t.Fatal("workflow fixture", e)
	}
	flowIn := in
	flowIn.To = nil
	flowIn.Body = nil
	flowIn.Workflow = str(wf.Workflow.ID)
	flow, e := coord.ChangeProjectAutomation(ctx, p.Name, "create", "", flowIn)
	if e != nil || flow.Automation.Workflow != wf.Workflow.ID {
		t.Fatal("project workflow", flow, e)
	}
	foreignDef := json.RawMessage(`{"name":"Other check","steps":[{"id":"check","agent":"worker","prompt":"Check"}],"edges":[{"from":"check","to":"$end"}]}`)
	foreign, e := owner.ApplyWorkflow(ctx, foreignDef, false)
	if e != nil {
		t.Fatal(e)
	}
	flowIn.Workflow = str(foreign.Workflow.ID)
	if _, e = coord.ChangeProjectAutomation(ctx, p.Name, "create", "", flowIn); e == nil {
		t.Fatal("workflow outside project accepted")
	}
	// Adopting an owner's existing task must not misattribute its creation.
	human, e := owner.CreateAutomation(ctx, api.AutomationRequest{Name: str("Owner task"), To: str("worker"), Project: &p.Name, Body: str("Owner content"), Schedule: in.Schedule, StartsAt: in.StartsAt, EndsAt: in.EndsAt})
	if e != nil {
		t.Fatal(e)
	}
	adopted, e := coord.ChangeProjectAutomation(ctx, p.Name, "update", human.Automation.ID, api.AutomationRequest{Name: str("Updated owner task")})
	if e != nil || adopted.Automation.CreatedBy != "" {
		t.Fatal("human task creator was overwritten", adopted, e)
	}
	// No schedule runs outside the window; loss of target membership disables it.
	if _, e = owner.RemoveProjectMember(ctx, p.Name, "worker"); e != nil {
		t.Fatal(e)
	}
	db, e := sqlite.Open(h.db)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = db.Exec(`UPDATE automations SET next_run_at=1,starts_at=0,ends_at=? WHERE id=?`, time.Now().Add(time.Hour).UnixMilli(), fresh.Automation.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`UPDATE automations SET next_run_at=1,starts_at=0,ends_at=? WHERE id=?`, time.Now().Add(time.Hour).UnixMilli(), human.Automation.ID); e != nil {
		t.Fatal(e)
	}
	h.h.Automations.RunDue(ctx)
	checked, e := owner.Automation(ctx, fresh.Automation.ID)
	if e != nil || checked.Automation.Enabled || checked.Automation.DisabledReason != "project_scope_changed" {
		t.Fatal("lost target scope still active", checked, e)
	}
	adoptedState, e := owner.Automation(ctx, human.Automation.ID)
	if e != nil || adoptedState.Automation.Enabled || adoptedState.Automation.DisabledReason != "project_scope_changed" {
		t.Fatal("adopted task escaped project scope", adoptedState, e)
	}
	audit, e := admin.Audit(ctx, client.AuditFilter{})
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(audit)
	for _, action := range []string{"project.automation.request", "project.automation.approve", "project.automation.deny", "project.automation.create", "project.automation.update", "project.automation.delete"} {
		if !strings.Contains(string(raw), action) {
			t.Fatal("missing audit", action)
		}
	}
}
