package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
)

func TestRuntimeAccountPrivacyAndPersistence(t *testing.T) {
	c := ctx(t)
	path := filepath.Join(t.TempDir(), "hub.db")
	h := startHub(t, path, app.DefaultMessagePolicy())
	admin := h.admin()
	member := func(name string) *client.Client {
		t.Helper()
		inv, err := admin.InviteMember(c, api.InviteRequest{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		paired, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "test-device")
		if err != nil {
			t.Fatal(err)
		}
		return h.client(paired.Token)
	}
	owner, viewer, manager := member("alice"), member("bob"), member("manager")
	if _, err := admin.UpdateMember(c, "manager", api.MemberUpdateRequest{Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	token := h.register(api.RegisterRequest{Name: "account-agent", Binding: api.Binding{Harness: "claude-cloud"}})
	if _, err := admin.SetAgentOwner(c, "account-agent", "alice"); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"cli":"claude-cloud","account":"alice@example.com"}`)
	req, _ := http.NewRequestWithContext(c, "PUT", h.url+"/v1/me/runtime", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var echo api.Runtime
	err = json.NewDecoder(res.Body).Decode(&echo)
	res.Body.Close()
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("runtime report status=%d err=%v", res.StatusCode, err)
	}
	if echo.Account != "" {
		t.Fatal("report response exposed account to agent token")
	}
	for _, tc := range []struct {
		name    string
		cl      *client.Client
		visible bool
	}{{"owner", owner, true}, {"admin", admin, true}, {"workspace-admin", manager, true}, {"other-member", viewer, false}, {"agent", h.client(token), false}} {
		t.Run(tc.name, func(t *testing.T) {
			a, err := tc.cl.GetAgent(c, "account-agent")
			if err != nil {
				t.Fatal(err)
			}
			check := func(a api.Agent) {
				t.Helper()
				got := ""
				if a.Runtime != nil {
					got = a.Runtime.Account
				}
				if (got != "") != tc.visible {
					t.Fatal("account visibility mismatch")
				}
			}
			check(*a)
			list, err := tc.cl.ListAgents(c, "", false)
			if err != nil {
				t.Fatal(err)
			}
			for _, a := range list {
				if a.Name == "account-agent" {
					check(a)
				}
			}
			ov, err := tc.cl.Overview(c, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, a := range ov.Agents {
				if a.Agent.Name == "account-agent" {
					check(a.Agent)
				}
			}
		})
	}
	// Runtime lives in the existing JSON column: an upgraded DB needs no DDL.
	h.stop()
	h = startHub(t, path, app.DefaultMessagePolicy())
	a, err := h.admin().GetAgent(c, "account-agent")
	if err != nil || a.Runtime == nil || a.Runtime.Account != "alice@example.com" {
		t.Fatal("account did not survive restart")
	}
	// An older report with no account remains accepted.
	if err := h.client(token).ReportRuntime(c, api.Runtime{CLI: "claude-cloud"}); err != nil {
		t.Fatal(err)
	}
}
