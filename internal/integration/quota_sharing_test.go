package integration_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
)

func TestAccountQuotaCloudAndCrossMachineSharing(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	now := time.Now().UTC()
	cloud := func(name string) *client.Client {
		return h.client(h.register(api.RegisterRequest{Name: name, Binding: api.Binding{Harness: "claude-cloud", SessionID: name, Environment: "claude.ai"}}))
	}
	clients := []*client.Client{}
	for i := 0; i < 3; i++ {
		cl := cloud(fmt.Sprintf("cloud-%d", i))
		clients = append(clients, cl)
		if e := cl.ReportRuntime(c, api.Runtime{CLI: "claude-cloud", Account: " Shared@Example.com "}); e != nil {
			t.Fatal(e)
		}
	}
	// No profile_ref and no machine: one old session report updates its peers.
	q := &api.Quota{Source: "claude", SourceKind: "cloud_session", Status: "ok", At: now, CheckedAt: now, Windows: []api.QuotaWindow{{Name: "five_hour", Used: .85, DurationSec: 18000}}}
	if e := clients[0].ReportRuntime(c, api.Runtime{CLI: "claude-cloud", Account: "shared@example.com", Quota: q}); e != nil {
		t.Fatal(e)
	}
	for i := 1; i < 3; i++ {
		got, e := clients[i].GetAgent(c, fmt.Sprintf("cloud-%d", i))
		if e != nil {
			t.Fatal(e)
		}
		if got.Runtime == nil || got.Runtime.Quota == nil || !got.Runtime.Quota.Shared || got.Runtime.Quota.Windows[0].Used != .85 {
			t.Fatal("cloud peer did not inherit current numbers")
		}
	}
	// A native agent on another machine joins through its confirmed stable-ID/email hint.
	native := h.client(h.register(api.RegisterRequest{Name: "native", Binding: api.Binding{Harness: "claude", SessionID: "native"}}))
	if e := native.ReportMachine(c, api.MachineReport{Fingerprint: strings.Repeat("a", 64), Name: "demo-mac", OS: "darwin"}); e != nil {
		t.Fatal(e)
	}
	q2 := *q
	q2.Provider = "claude"
	q2.Plan = "claude_subscription"
	q2.SourceKind = "cache"
	q2.ProfileRef = "qp_" + strings.Repeat("b", 64)
	q2.At = now.Add(time.Second)
	q2.CheckedAt = q2.At
	q2.Windows = []api.QuotaWindow{{Name: "five_hour", Used: .95}}
	identity := "qa_" + strings.Repeat("c", 64)
	if e := native.ReportRuntime(c, api.Runtime{CLI: "claude", Account: "shared@example.com", AccountIDHash: identity, Quota: &q2}); e != nil {
		t.Fatal(e)
	}
	got, e := clients[1].GetAgent(c, "cloud-1")
	if e != nil {
		t.Fatal(e)
	}
	if got.Runtime.Quota == nil || got.Runtime.Quota.Windows[0].Used != .95 || !got.Runtime.Quota.Shared || got.Runtime.Quota.ProfileRef != "" || got.Runtime.Quota.MachineID != "" {
		t.Fatal("cross-machine numbers or donor metadata incorrect")
	}
	raw, _ := json.Marshal(got.Runtime.Quota)
	if strings.Contains(string(raw), "account_key") || strings.Contains(string(raw), identity) || strings.Contains(string(raw), "native") {
		t.Fatal("private grouping/donor metadata leaked")
	}
	own, e := native.Me(c)
	if e != nil {
		t.Fatal(e)
	}
	if own.Runtime.AccountIDHash != "" {
		t.Fatal("identity digest echoed")
	}
	// Older report cannot replace the newer native observation.
	q.At = now.Add(-time.Minute)
	q.Windows[0].Used = .1
	if e = clients[0].ReportRuntime(c, api.Runtime{CLI: "claude-cloud", Account: "shared@example.com", Quota: q}); e != nil {
		t.Fatal(e)
	}
	got, _ = clients[1].GetAgent(c, "cloud-1")
	if got.Runtime.Quota.Windows[0].Used != .95 {
		t.Fatal("older observation won")
	}
	issue := &api.Quota{Source: "claude", Provider: "claude", Plan: "claude_subscription", SourceKind: "cloud_session", Status: "auth_required", CheckedAt: now}
	if e = clients[1].ReportRuntime(c, api.Runtime{CLI: "claude-cloud", Account: "shared@example.com", Quota: issue}); e != nil {
		t.Fatal(e)
	}
	got, _ = clients[1].Me(c)
	if got.Runtime.Quota.Status != "auth_required" || got.Runtime.Quota.Windows[0].Used != .95 {
		t.Fatal("connection issue was promoted to ok")
	}
	// A second member with the same hint sees none of the first member's quota.
	inv, e := h.admin().InviteMember(c, api.InviteRequest{Name: "viewer"})
	if e != nil {
		t.Fatal(e)
	}
	paired, e := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "viewer-device")
	if e != nil {
		t.Fatal(e)
	}
	viewer := h.client(paired.Token)
	hidden, e := viewer.GetAgent(c, "cloud-0")
	if e != nil {
		t.Fatal(e)
	}
	if hidden.Runtime != nil && hidden.Runtime.Quota != nil {
		t.Fatal("another owner saw shared quota")
	}
	outsider := cloud("other-owner")
	if _, e = h.admin().SetAgentOwner(c, "other-owner", "viewer"); e != nil {
		t.Fatal(e)
	}
	if e = outsider.ReportRuntime(c, api.Runtime{CLI: "claude-cloud", Account: "shared@example.com"}); e != nil {
		t.Fatal(e)
	}
	hidden, e = outsider.Me(c)
	if e != nil {
		t.Fatal(e)
	}
	if hidden.Runtime.Quota != nil {
		t.Fatal("same email crossed owner namespace")
	}
	// Agent picking receives the exact same effective account windows.
	q2.At = now.Add(2 * time.Second)
	q2.CheckedAt = q2.At
	q2.Windows = []api.QuotaWindow{{Name: "five_hour", Used: 1, Status: "rejected"}}
	if e = native.ReportRuntime(c, api.Runtime{CLI: "claude", Account: "shared@example.com", AccountIDHash: identity, Quota: &q2}); e != nil {
		t.Fatal(e)
	}
	picks, e := clients[0].PickAgents(c, client.AgentPickFilter{Count: 100})
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, pick := range picks.Candidates {
		if pick.Name == "cloud-2" {
			found = true
			if pick.Quota != "blocked" {
				t.Fatal("picker ignored account quota")
			}
		}
	}
	if !found {
		t.Fatal("picker omitted target")
	}
	// Exercise list/overview visibility in addition to individual reads.
	list, e := clients[0].ListAgents(c, "", false)
	if e != nil {
		t.Fatal(e)
	}
	seen := false
	for _, a := range list {
		if a.Name == "cloud-2" {
			seen = true
			if a.Runtime.Quota == nil || a.Runtime.Quota.Windows[0].Used != 1 {
				t.Fatal("list did not resolve shared quota")
			}
		}
	}
	if !seen {
		t.Fatal("target missing")
	}
}
