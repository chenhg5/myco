package cli

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoalOldHubValidationAndSchema(t *testing.T) {
	s := httptest.NewServer(http.NotFoundHandler())
	defer s.Close()
	h := &harnessT{t, map[string]string{"MYCO_CONFIG": filepath.Join(t.TempDir(), "config.yaml"), "MYCO_HUB": s.URL, "MYCO_TOKEN": "test-token"}}
	for _, args := range [][]string{{"goal", "list"}, {"goal", "get", "--id", "gol_demo"}, {"goal", "pause", "--id", "gol_demo"}, {"goal", "start", "--agent", "worker", "--objective", "Check", "--done-when", "Reviewed"}} {
		code, _, stderr := h.run(args...)
		if code != ExitUsage || !strings.Contains(stderr, "upgrade the Hub") {
			t.Fatalf("old Hub %v %d %s", args, code, stderr)
		}
	}
	for _, args := range [][]string{{"goal", "report", "--id", "gol_demo", "--summary", "Done"}, {"goal", "report", "--id", "gol_demo", "--summary", "Done", "--progressed", "--no-progress"}, {"goal", "set", "--id", "gol_demo"}, {"goal", "start", "--agent", "worker", "--objective", "Check", "--done-when", "Reviewed", "--for", "0s"}} {
		code, _, stderr := h.run(args...)
		if code != ExitUsage || !strings.Contains(stderr, "invalid_argument") {
			t.Fatal(args, code, stderr)
		}
	}
	code, out, stderr := h.run("schema", "--all")
	if code != ExitOK || !strings.Contains(out, "goal report") || !strings.Contains(out, "--dry-run") {
		t.Fatal(code, stderr)
	}
}
