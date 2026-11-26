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
	if get(a, "two") != nil || get(outsider, "one") != nil {
		t.Fatal("another agent read private account quota")
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
