package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskBoardHelpSchemaAndOldHub(t *testing.T) {
	h := &harnessT{t, map[string]string{"MYCO_CONFIG": filepath.Join(t.TempDir(), "config.yaml"), "HOME": t.TempDir()}}
	code, out, err := h.run("task", "board", "--help")
	if code != 0 || !strings.Contains(out, "--all") || !strings.Contains(out, "--stale") {
		t.Fatal(code, out, err)
	}
	code, out, err = h.run("schema", "--all")
	if code != 0 || !json.Valid([]byte(out)) || !strings.Contains(out, "task board") || !strings.Contains(out, "task alerts set") {
		t.Fatal(code, err)
	}
	old := httptest.NewServer(http.NotFoundHandler())
	defer old.Close()
	h.env["MYCO_HUB"] = old.URL
	h.env["MYCO_TOKEN"] = "fake-token"
	code, _, err = h.run("task", "board")
	if code != ExitUsage || !strings.Contains(err, `"error":"unsupported"`) {
		t.Fatal(code, err)
	}
}
