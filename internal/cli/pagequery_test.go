package cli

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestPageQueryCLILegacyHubTableAndWarning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"records":[{"collection":"issues","id":"i1","version":2,"data":{"n":1,"title":"One","status":"todo"}},{"collection":"issues","id":"i2","version":3,"data":{"n":2,"title":"Two","status":"progress"}}],"seq":5}`))
	}))
	defer srv.Close()
	h := &harnessT{t, map[string]string{"MYCO_HUB": srv.URL, "MYCO_ADMIN_TOKEN": "test-admin-token", "MYCO_CONFIG": filepath.Join(t.TempDir(), "config.yaml")}}
	args := []string{"page", "data", "list", "--page", "pg_test", "--collection", "issues", "--fields", "n,title", "--where", "status:in:todo,progress", "--sort", "-n", "--limit", "1", "--format", "text"}
	code, out, errb := h.run(args...)
	if code != 0 || !strings.Contains(out, "COLLECTION") || !strings.Contains(out, "VERSION") || !strings.Contains(out, "i2") || !strings.Contains(out, "Two") || strings.Contains(out, "i1") || !strings.Contains(strings.ToLower(errb), "upgrade") {
		t.Fatalf("table/fallback: %d %s %s", code, out, errb)
	}
	code, out, errb = h.run("page", "data", "list", "--page", "pg_test", "--where", "status:progress", "--count", "--format", "text")
	if code != 0 || strings.TrimSpace(out) != "1" || !strings.Contains(strings.ToLower(errb), "upgrade") {
		t.Fatalf("count: %d %s %s", code, out, errb)
	}
}
