package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/testutil"
)

func TestIncrementalAllowFromCLI(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	h := &harnessT{t, map[string]string{"MYCO_HUB": newHub(t), "MYCO_ADMIN_TOKEN": "admin-token-123456"}}
	h.run("agent", "register", "--name", "box", "--harness", "exec", "--allow-from", "boss,console", "--no-save")
	if code, out, e := h.run("agent", "update", "--name", "box", "--allow-add", "new", "--format", "json"); code != 0 || !strings.Contains(out, `"console"`) || !strings.Contains(out, `"new"`) {
		t.Fatalf("add: %d %s %s", code, out, e)
	}
	// The test harness is non-TTY: no prompt or write may occur.
	if code, _, e := h.run("agent", "update", "--name", "box", "--allow-from", "new"); code != ExitConflict || !strings.Contains(e, "allow_replace_required") || !strings.Contains(e, "boss, console") || !strings.Contains(e, "--replace") {
		t.Fatalf("replacement guard: %d %s", code, e)
	}
	if code, out, e := h.run("agent", "update", "--name", "box", "--allow-from", "new", "--dry-run"); code != ExitDryRun || !strings.Contains(out, `"before"`) || !strings.Contains(out, `"removed"`) {
		t.Fatalf("dry run: %d %s %s", code, out, e)
	}
	if _, out, _ := h.run("agent", "get", "--name", "box"); !strings.Contains(out, `"console"`) {
		t.Fatal("guard/dry-run changed senders")
	}
	if code, out, e := h.run("agent", "update", "--name", "box", "--allow-remove", "boss"); code != 0 || !strings.Contains(out, `"console"`) {
		t.Fatalf("remove: %d %s %s", code, out, e)
	}
	if code, out, e := h.run("agent", "update", "--name", "box", "--allow-from", "new", "--replace"); code != 0 || !strings.Contains(out, `"removed"`) {
		t.Fatalf("explicit replace: %d %s %s", code, out, e)
	}
	if code, _, _ := h.run("agent", "update", "--name", "box", "--allow-from", "new", "--allow-add", "boss"); code != ExitUsage {
		t.Fatalf("mixed modes: %d", code)
	}
}

func TestExecReplacementPreviewCannotEraseConcurrentSender(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(api.Agent{Name: "box", Binding: api.Binding{Harness: "exec"}, AllowFrom: []string{"boss"}})
			return
		}
		var req api.ProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req.AllowFrom != nil || req.AllowAdd == nil {
			t.Error("without --replace, a late sender could be erased")
		}
		// Another admin added a sender after the initial GET.
		json.NewEncoder(w).Encode(api.Agent{Name: "box", AllowFrom: []string{"boss", "concurrent", "new"}})
	}))
	defer srv.Close()
	h := &harnessT{t, map[string]string{"MYCO_HUB": srv.URL, "MYCO_ADMIN_TOKEN": "admin-token-123456"}}
	if code, out, e := h.run("agent", "update", "--name", "box", "--allow-from", "boss,new"); code != 0 || !strings.Contains(out, "concurrent") {
		t.Fatalf("%d %s %s", code, out, e)
	}
}
