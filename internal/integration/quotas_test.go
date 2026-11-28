package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

func TestAccountQuotaSharedBindingOwnerPrivacyAndRestart(t *testing.T) {
	c := ctx(t)
	dbPath := filepath.Join(t.TempDir(), "hub.db")
	h := startHub(t, dbPath, app.DefaultMessagePolicy())
	admin := h.admin()
	ct, e := admin.CreateConsoleToken(c, "demo-device")
	if e != nil {
		t.Fatal(e)
	}
	owner := h.client(ct.Token)
	a := h.client(h.register(api.RegisterRequest{Name: "one", Binding: api.Binding{Harness: "codex", SessionID: "one"}}))
	bToken := h.register(api.RegisterRequest{Name: "two", Binding: api.Binding{Harness: "codex", SessionID: "two"}})
	b := h.client(bToken)
	outsider := h.client(h.register(api.RegisterRequest{Name: "other"}))
	for _, cl := range []*client.Client{a, b} {
		if e = cl.ReportMachine(c, api.MachineReport{Fingerprint: strings.Repeat("a", 64), Name: "demo-mac", OS: "darwin"}); e != nil {
			t.Fatal(e)
		}
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	profile := "qp_" + strings.Repeat("a", 64)
	q := api.Quota{Source: "codex", Provider: "codex", Plan: "chatgpt_subscription", ProfileRef: profile, MachineID: "untrusted-machine", SourceKind: "native", At: now, CheckedAt: now, Status: "ok", Windows: []api.QuotaWindow{{Name: "primary", Used: .82, DurationSec: 604800}}}
	if e = a.ReportRuntime(c, api.Runtime{CLI: "codex", Quota: &q}); e != nil {
		t.Fatal(e)
	}
	poisoned := q
	poisoned.CheckedAt = now.Add(24 * time.Hour)
	if e = a.ReportRuntime(c, api.Runtime{CLI: "codex", Quota: &poisoned}); e == nil {
		t.Fatal("future timestamp poisoned shared snapshot ordering")
	}
	// A newly bound agent must not overwrite a shared good observation with
	// its later 'pending' timestamp before it reads the local shared cache.
	pending := q
	pending.Status = "transient_error"
	pending.ErrorCode = "pending"
	pending.At = time.Time{}
	pending.Windows = nil
	pending.CheckedAt = now.Add(time.Minute)
	if e = b.ReportRuntime(c, api.Runtime{CLI: "codex", Quota: &pending}); e != nil {
		t.Fatal(e)
	}
	get := func(cl *client.Client, name string) *api.Quota {
		t.Helper()
		ag, err := cl.GetAgent(c, name)
		if err != nil {
			t.Fatal(err)
		}
		if ag.Runtime == nil {
			return nil
		}
		return ag.Runtime.Quota
	}
	for _, name := range []string{"one", "two"} {
		got := get(owner, name)
		if got == nil || got.Status != "ok" || len(got.Windows) != 1 || got.Windows[0].Used != .82 || got.MachineID == "untrusted-machine" {
			t.Fatalf("shared quota for %s: %+v", name, got)
		}
	}
	if get(a, "one") == nil {
		t.Fatal("agent cannot see its own quota")
	}
	// An agent its owner owns reads that owner's accounts (the owner's
	// coordinator plans their use); an agent with another owner does not.
	if get(a, "two") == nil || get(b, "one") == nil {
		t.Fatal("an agent could not read its own owner's account quota")
	}
	inv, e := admin.InviteMember(c, api.InviteRequest{Name: "viewer"})
	if e != nil {
		t.Fatal(e)
	}
	paired, e := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "viewer-device")
	if e != nil {
		t.Fatal(e)
	}
	viewer := h.client(paired.Token)
	if _, e = admin.UpdateMember(c, "viewer", api.MemberUpdateRequest{Role: "admin"}); e != nil {
		t.Fatal(e)
	}
	if get(viewer, "one") != nil {
		t.Fatal("admin role exposed another member's paid account")
	}
	if _, e = admin.SetAgentOwner(c, "other", "viewer"); e != nil {
		t.Fatal(e)
	}
	if get(outsider, "one") != nil {
		t.Fatal("another owner's agent read private account quota")
	}
	for _, cl := range []*client.Client{viewer, outsider} {
		list, e := cl.ListAgents(c, "", false)
		if e != nil {
			t.Fatal(e)
		}
		for _, ag := range list {
			if ag.Runtime != nil && ag.Runtime.Quota != nil {
				t.Fatal("list leaked private quota")
			}
		}
		ov, e := cl.Overview(c, 0)
		if e != nil {
			t.Fatal(e)
		}
		for _, ag := range ov.Agents {
			if ag.Agent.Runtime != nil && ag.Agent.Runtime.Quota != nil {
				t.Fatal("overview leaked private quota")
			}
		}
	}
	// Failures preserve old numbers and their observation time, not the time
	// of the failed check. Both agents resolve the same updated shared row.
	failed := q
	failed.At = time.Time{}
	failed.Windows = nil
	failed.Status = "throttled"
	failed.CheckedAt = now.Add(2 * time.Minute)
	if e = a.ReportRuntime(c, api.Runtime{CLI: "codex", Quota: &failed}); e != nil {
		t.Fatal(e)
	}
	got := get(owner, "two")
	if got.Status != "throttled" || !got.At.Equal(now) || len(got.Windows) != 1 {
		t.Fatal("failed query erased shared observation")
	}
	// The new account is a new namespace. Its missing key has no old numbers.
	switched := q
	switched.ProfileRef = "qp_" + strings.Repeat("b", 64)
	switched.Provider = "minimax"
	switched.Plan = "coding_plan"
	switched.Status = "no_credentials"
	switched.At = time.Time{}
	switched.Windows = nil
	if e = a.ReportRuntime(c, api.Runtime{CLI: "claude", Quota: &switched}); e != nil {
		t.Fatal(e)
	}
	got = get(owner, "one")
	if got.Provider != "minimax" || len(got.Windows) != 0 || !got.At.IsZero() {
		t.Fatal("switched provider inherited old quota")
	}
	if get(owner, "two").Provider != "codex" {
		t.Fatal("switch changed the other agent's binding")
	}
	if _, e = admin.SetAgentOwner(c, "two", "viewer"); e != nil {
		t.Fatal(e)
	}
	if get(viewer, "two") != nil {
		t.Fatal("transferring the agent transferred a private computer account")
	}
	if get(b, "two") != nil {
		t.Fatal("transferred agent token exposed former computer owner quota")
	}
	if get(owner, "two") == nil {
		t.Fatal("computer owner lost account visibility")
	}
	legacy := api.Quota{Source: "codex", At: now, Windows: []api.QuotaWindow{{Name: "primary", Used: .82}}}
	if e = b.ReportRuntime(c, api.Runtime{CLI: "codex", Quota: &legacy}); e != nil {
		t.Fatal(e)
	}
	if get(viewer, "two") != nil {
		t.Fatal("legacy runtime bypassed computer-owner privacy")
	}
	// The runtime write response must not resolve and expose a shared
	// snapshot through the transferred credential either.
	raw, e := json.Marshal(api.Runtime{CLI: "codex", Quota: &pending})
	if e != nil {
		t.Fatal(e)
	}
	req, e := http.NewRequestWithContext(c, http.MethodPut, h.url+"/v1/me/runtime", bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	req.Header.Set("Authorization", "Bearer "+bToken)
	req.Header.Set("Content-Type", "application/json")
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	var rt api.Runtime
	decodeErr := json.NewDecoder(resp.Body).Decode(&rt)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || decodeErr != nil {
		t.Fatalf("runtime write failed: status=%d decode=%v", resp.StatusCode, decodeErr)
	}
	if rt.Quota != nil {
		t.Fatal("runtime write response exposed the former owner's quota")
	}
	if get(b, "two") != nil {
		t.Fatal("legacy runtime bypassed transferred-token filtering")
	}
	db, e := sqlite.Open(dbPath)
	if e != nil {
		t.Fatal(e)
	}
	var rows int
	if e = db.QueryRow(`SELECT count(*) FROM account_quotas`).Scan(&rows); e != nil || rows != 2 {
		t.Fatalf("expected two shared snapshots: %d %v", rows, e)
	}
	db.Close()
	h.stop()
	h = startHub(t, dbPath, app.DefaultMessagePolicy())
	owner = h.client(ct.Token)
	if got = get(owner, "two"); got == nil || len(got.Windows) != 1 || got.Status != "throttled" {
		t.Fatal("restart lost quota or binding")
	}
}

// An agent in a cloud session (inbox wait) has no computer: its session
// reports the quota itself (myco agent runtime quota: cloud_session, no
// machine, no profile). Its owner's App (a console token) gets it in the
// overview it shows and on the agent; another member does not.
func TestSessionReportedQuotaReachesTheOwnersApp(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	ct, e := admin.CreateConsoleToken(c, "demo-device")
	if e != nil {
		t.Fatal(e)
	}
	owner := h.client(ct.Token)
	cloudy := h.client(h.register(api.RegisterRequest{Name: "cloudy", Binding: api.Binding{Harness: "claude-cloud"}}))
	now := time.Now().UTC().Truncate(time.Millisecond)
	q := api.Quota{Source: "claude", Provider: "claude", SourceKind: "cloud_session", Status: "ok", At: now, CheckedAt: now,
		Windows: []api.QuotaWindow{{Name: "five_hour", Used: .17, DurationSec: 18000}, {Name: "seven_day", Used: .81, DurationSec: 604800, Status: "warning"}}}
	if e = cloudy.ReportRuntime(c, api.Runtime{CLI: "inbox", Quota: &q}); e != nil {
		t.Fatal(e)
	}
	ag, e := owner.GetAgent(c, "cloudy")
	if e != nil {
		t.Fatal(e)
	}
	if ag.Runtime == nil || ag.Runtime.Quota == nil || len(ag.Runtime.Quota.Windows) != 2 || ag.Runtime.Quota.SourceKind != "cloud_session" {
		t.Fatalf("the owner cannot see the session's quota: %+v", ag.Runtime)
	}
	if got := ag.Runtime.Quota.Windows[1]; got.Status != "warning" || got.Used != .81 {
		t.Fatalf("warning lost before reaching the App: %+v", got)
	}
	ov, e := owner.Overview(c, 0)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, x := range ov.Agents {
		if x.Agent.Name == "cloudy" {
			found = x.Agent.Runtime != nil && x.Agent.Runtime.Quota != nil && len(x.Agent.Runtime.Quota.Windows) == 2
		}
	}
	if !found {
		t.Fatal("the owner's overview lacks the session's quota")
	}
	inv, e := admin.InviteMember(c, api.InviteRequest{Name: "someone"})
	if e != nil {
		t.Fatal(e)
	}
	paired, e := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "someone-device")
	if e != nil {
		t.Fatal(e)
	}
	if ag, e = h.client(paired.Token).GetAgent(c, "cloudy"); e != nil || (ag.Runtime != nil && ag.Runtime.Quota != nil) {
		t.Fatalf("another member read the session's quota: %v %+v", e, ag)
	}
}

func TestPlanQuotaProvidersAndBalance(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	agent := h.client(h.register(api.RegisterRequest{Name: "plan-agent"}))
	inv, e := h.admin().InviteMember(c, api.InviteRequest{Name: "plan-viewer"})
	if e != nil {
		t.Fatal(e)
	}
	paired, e := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "plan-viewer-device")
	if e != nil {
		t.Fatal(e)
	}
	outsider := h.client(paired.Token)
	if e := agent.ReportMachine(c, api.MachineReport{Fingerprint: strings.Repeat("b", 64), Name: "demo-mac", OS: "darwin"}); e != nil {
		t.Fatal(e)
	}
	limit, left := 50.0, 17.5
	now := time.Now().UTC()
	for _, provider := range []string{"glm", "kimi", "openrouter"} {
		window := api.QuotaWindow{Name: "weekly", Used: .65, DurationSec: 604800}
		if provider == "openrouter" {
			window = api.QuotaWindow{Name: "balance", Used: .65, Balance: &api.QuotaBalance{Currency: "USD", Limit: &limit, Remaining: &left}}
		}
		q := api.Quota{Source: provider, Provider: provider, ProfileRef: "qp_" + strings.Repeat("b", 64), Plan: "coding_plan", SourceKind: "official_api", Status: "ok", At: now, CheckedAt: now, Windows: []api.QuotaWindow{window}}
		if e := agent.ReportRuntime(c, api.Runtime{CLI: "claude", Quota: &q}); e != nil {
			t.Fatal(e)
		}
		got, e := agent.GetAgent(c, "plan-agent")
		if e != nil {
			t.Fatal(e)
		}
		if got.Runtime == nil || got.Runtime.Quota == nil || got.Runtime.Quota.Provider != provider {
			t.Fatalf("new provider not returned: %+v", got.Runtime)
		}
		if provider == "openrouter" {
			balance := got.Runtime.Quota.Windows[0].Balance
			if balance == nil || *balance.Remaining != 17.5 || *balance.Limit != 50 {
				t.Fatal("balance did not survive persistence/DTO")
			}
			q.CheckedAt = now.Add(time.Second)
			q.Status = "transient_error"
			q.At = time.Time{}
			q.Windows = nil
			if e = agent.ReportRuntime(c, api.Runtime{CLI: "claude", Quota: &q}); e != nil {
				t.Fatal(e)
			}
			got, e = agent.GetAgent(c, "plan-agent")
			if e != nil {
				t.Fatal(e)
			}
			if got.Runtime.Quota.Status != "transient_error" || *got.Runtime.Quota.Windows[0].Balance.Remaining != 17.5 {
				t.Fatal("failed report discarded old balance")
			}
			q.CheckedAt = now.Add(2 * time.Second)
			q.At = now.Add(2 * time.Second)
			q.Status = "ok"
			q.Windows = []api.QuotaWindow{{Name: "balance", Used: 0, Balance: &api.QuotaBalance{Currency: "USD", Unlimited: true}}}
			if e = agent.ReportRuntime(c, api.Runtime{CLI: "claude", Quota: &q}); e != nil {
				t.Fatal(e)
			}
			got, e = agent.GetAgent(c, "plan-agent")
			if e != nil {
				t.Fatal(e)
			}
			if !got.Runtime.Quota.Windows[0].Balance.Unlimited {
				t.Fatal("unlimited allowance lost")
			}
		}
		hidden, e := outsider.GetAgent(c, "plan-agent")
		if e != nil {
			t.Fatal(e)
		}
		if hidden.Runtime != nil && hidden.Runtime.Quota != nil {
			t.Fatal("private quota leaked to another owner")
		}
	}
}
