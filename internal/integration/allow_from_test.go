package integration_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
)

func TestAtomicSenderEditsPreserveOthers(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	h.register(api.RegisterRequest{Name: "box", Binding: api.Binding{Harness: "exec"}, AllowFrom: []string{"boss", "console"}})
	admin := h.admin()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			add := []string{fmt.Sprintf("sender-%d", i)}
			r, err := admin.UpdateProfileWithChanges(c, "box", api.ProfileRequest{AllowAdd: &add})
			if err != nil {
				t.Error(err)
				return
			}
			if !slices.Contains(r.AllowFrom, "console") || !slices.Contains(r.AllowFrom, "boss") || r.AllowFromChange == nil || !slices.Equal(r.AllowFromChange.Added, add) {
				t.Errorf("lost original permissions: %+v", r)
			}
		}(i)
	}
	wg.Wait()
	a, err := admin.GetAgent(c, "box")
	if err != nil || len(a.AllowFrom) != 10 {
		t.Fatalf("concurrent appends lost: %+v %v", a, err)
	}
	remove := []string{"sender-0"}
	if _, err := admin.UpdateProfile(c, "box", api.ProfileRequest{AllowRemove: &remove}); err != nil {
		t.Fatal(err)
	}
	entries, err := admin.Audit(c, client.AuditFilter{Target: "box", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, e := range entries {
		if e.Action != "agent.update" {
			continue
		}
		var d struct {
			Actor, Node    string
			Added, Removed []string
		}
		if err := json.Unmarshal([]byte(e.Detail), &d); err != nil || d.Actor == "" || d.Node != "box" {
			t.Fatalf("audit missing actor/node/diff: %+v %v", e, err)
		}
		count++
	}
	if count != 9 {
		t.Fatalf("want 9 permission audits, got %d", count)
	}
	replace := []string{"boss"}
	replaced, err := admin.UpdateProfileWithChanges(c, "box", api.ProfileRequest{AllowFrom: &replace})
	if err != nil || !slices.Equal(replaced.AllowFrom, replace) || replaced.AllowFromChange == nil || !slices.Contains(replaced.AllowFromChange.Removed, "console") || !slices.Equal(replaced.AllowFromChange.Unchanged, replace) {
		t.Fatalf("replacement response: %+v %v", replaced, err)
	}

}

func TestSenderApprovalAppendsInsteadOfReplacing(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	console, err := admin.CreateConsoleToken(c, "owner-mac")
	if err != nil {
		t.Fatal(err)
	}
	owner := h.client(console.Token)
	shroom := h.client(h.register(api.RegisterRequest{Name: "shroom", Binding: api.Binding{Harness: "claude", SessionID: "s1"}}))
	h.register(api.RegisterRequest{Name: "box", Binding: api.Binding{Harness: "exec"}, AllowFrom: []string{"boss", "release-console"}})
	if _, err := owner.SetSuperagent(c, "shroom", false); err != nil {
		t.Fatal(err)
	}
	// A legacy grant must be normalized before being queued for approval.
	legacy := []string{"helper"}
	if _, err := shroom.UpdateProfile(c, "box", api.ProfileRequest{AllowFrom: &legacy}); err == nil {
		t.Fatal("grant must wait for approval")
	}
	pending, err := owner.Approvals(c, "pending")
	if err != nil || len(pending) != 1 || !strings.Contains(pending[0].What, "允许 helper 派任务给 box") {
		t.Fatalf("approval: %+v %v", pending, err)
	}
	concurrent := []string{"reviewer"}
	if _, err := owner.UpdateProfile(c, "box", api.ProfileRequest{AllowAdd: &concurrent}); err != nil {
		t.Fatal(err)
	}
	if done, err := owner.DecideApproval(c, pending[0].ID, true); err != nil || done.Result != 200 {
		t.Fatalf("approve: %+v %v", done, err)
	}
	a, err := owner.GetAgent(c, "box")
	if err != nil || !slices.Equal(a.AllowFrom, []string{"boss", "release-console", "reviewer", "helper"}) {
		t.Fatalf("grant erased senders: %+v %v", a, err)
	}
}
