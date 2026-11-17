package integration_test

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/infra/sqlite"
)

// An automation sends its message, as its owner, to an agent: on its
// schedule (run by the hub), when its webhook is called (the call's body in
// the message), or by hand. A wrong secret learns nothing; too many calls
// are refused; a run while its last task is open can be skipped.
func TestAutomations(t *testing.T) {
	c := ctx(t)
	db := filepath.Join(t.TempDir(), "hub.db")
	h := startHub(t, db, app.DefaultMessagePolicy())
	pm := h.client(h.register(api.RegisterRequest{Name: "pm", Binding: api.Binding{Harness: "claude", SessionID: "s1"}}))
	admin := h.admin()

	// Neither schedule nor webhook; a schedule too frequent; an unknown agent.
	name, body, to := "日报", "写今天的日报", "pm"
	if _, err := admin.CreateAutomation(c, api.AutomationRequest{Name: &name, To: &to, Body: &body}); err == nil {
		t.Fatal("an automation needs a schedule or a webhook")
	}
	if _, err := admin.CreateAutomation(c, api.AutomationRequest{Name: &name, To: &to, Body: &body, Schedule: &api.AutomationSchedule{Cron: "*/5 * * * *"}}); err == nil {
		t.Fatal("runs every 5 minutes are too often")
	}
	nobody := "nobody"
	if _, err := admin.CreateAutomation(c, api.AutomationRequest{Name: &name, To: &nobody, Body: &body, Schedule: &api.AutomationSchedule{Cron: "0 9 * * *"}}); err == nil {
		t.Fatal("an unknown agent")
	}

	// A schedule: due now (the hub's clock moved on), it sends once.
	res, err := admin.CreateAutomation(c, api.AutomationRequest{Name: &name, To: &to, Body: &body,
		Schedule: &api.AutomationSchedule{Cron: "0 9 * * 1-5", TZ: "Asia/Shanghai"}, SkipIfBusy: ptr(true)})
	if err != nil {
		t.Fatal(err)
	}
	a := res.Automation
	if a.NextRunAt == nil || a.Webhook || res.WebhookSecret != "" || !strings.HasPrefix(a.From, "@console:") || !a.Enabled {
		t.Fatalf("created: %+v", res)
	}
	due := func(id string) {
		t.Helper()
		sdb, err := sqlite.Open(db)
		if err != nil {
			t.Fatal(err)
		}
		defer sdb.Close()
		if _, err := sdb.Exec(`UPDATE automations SET next_run_at = 1 WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
	}
	due(a.ID)
	h.h.Automations.RunDue(c)
	got, err := pm.Pull(c, 10, 0)
	if err != nil || len(got) != 1 || got[0].Body != body || got[0].From != a.From || got[0].Automation != a.ID || got[0].Type != "task" {
		t.Fatalf("the scheduled task: %v %+v", err, got)
	}
	d, err := admin.Automation(c, a.ID)
	if err != nil || len(d.Runs) != 1 || d.Runs[0].Trigger != "schedule" || d.Runs[0].Outcome != "sent" || d.Runs[0].MessageID != got[0].ID ||
		d.Automation.NextRunAt == nil || !d.Automation.NextRunAt.After(d.Runs[0].At) {
		t.Fatalf("after the run: %v %+v", err, d)
	}
	// Run again by hand while that task is open: skipped.
	run, err := admin.RunAutomation(c, a.ID)
	if err != nil || run.Trigger != "manual" || run.Outcome != "skipped" || run.MessageID != "" {
		t.Fatalf("busy: %v %+v", err, run)
	}
	if _, err := pm.Reply(c, got[0].ID, "done", false); err != nil {
		t.Fatal(err)
	}
	if run, err := admin.RunAutomation(c, a.ID); err != nil || run.Outcome != "sent" {
		t.Fatalf("after the answer: %v %+v", err, run)
	}

	// A webhook: the secret once; the call's body in the message.
	hookBody := "CI 失败了，看一下：{{payload}}"
	res2, err := admin.CreateAutomation(c, api.AutomationRequest{Name: ptr("CI"), To: &to, Body: &hookBody, Webhook: ptr(true)})
	if err != nil || res2.WebhookSecret == "" || !res2.Automation.Webhook || !strings.HasSuffix(res2.Automation.WebhookURL, "/hooks/"+res2.Automation.ID) || res2.Automation.NextRunAt != nil {
		t.Fatalf("webhook: %v %+v", err, res2)
	}
	call := func(secret, payload string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, res2.Automation.WebhookURL, strings.NewReader(payload))
		if secret != "" {
			req.Header.Set("Authorization", "Bearer "+secret)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	if resp := call("whk_wrong", "x"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("a wrong secret: %d", resp.StatusCode)
	}
	if resp := call("", "x"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("no secret: %d", resp.StatusCode)
	}
	if resp := call(res2.WebhookSecret, "exit 1 in TestLogin"); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("call: %d", resp.StatusCode)
	}
	pulled, _ := pm.Pull(c, 10, 0)
	var hooked *api.Message
	for i := range pulled {
		if pulled[i].Automation == res2.Automation.ID {
			hooked = &pulled[i]
		}
	}
	if hooked == nil || hooked.Body != "CI 失败了，看一下：exit 1 in TestLogin" {
		t.Fatalf("the webhook's task: %+v", pulled)
	}
	if resp := call(res2.WebhookSecret, strings.Repeat("x", 17<<10)); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("too large: %d", resp.StatusCode)
	}
	// 30 calls an hour: the 31st is refused, with when to try again.
	for i := 1; i < app.WebhookRate; i++ {
		if resp := call(res2.WebhookSecret, "again"); resp.StatusCode != http.StatusAccepted {
			t.Fatalf("call %d: %d", i+1, resp.StatusCode)
		}
	}
	if resp := call(res2.WebhookSecret, "one too many"); resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("rate limit: %d %q", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	// A new secret: the old one stops working. Turned off: 404 too.
	rot, err := admin.UpdateAutomation(c, res2.Automation.ID, api.AutomationRequest{RotateSecret: true, Enabled: ptr(false)})
	if err != nil || rot.WebhookSecret == "" || rot.WebhookSecret == res2.WebhookSecret || rot.Automation.Enabled {
		t.Fatalf("rotate: %v %+v", err, rot)
	}
	if resp := call(rot.WebhookSecret, "off"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("turned off: %d", resp.StatusCode)
	}

	list, err := admin.Automations(c)
	if err != nil || len(list) != 2 || list[0].LastRun == nil {
		t.Fatalf("list: %v %+v", err, list)
	}
	if err := admin.DeleteAutomation(c, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Automation(c, a.ID); err == nil {
		t.Fatal("removed")
	}
	// The messages it sent stay.
	if th, err := admin.Thread(c, got[0].ThreadID); err != nil || len(th) == 0 || th[0].Automation != a.ID {
		t.Fatalf("its messages stay: %v %+v", err, th)
	}
}

func ptr[T any](v T) *T { return &v }
