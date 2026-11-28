package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	clipkg "github.com/myco-ai/myco/internal/cli"
)

func TestWorkflowRunCompactCLI(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	worker := h.client(h.register(api.RegisterRequest{Name: "worker"}))
	def := json.RawMessage(`{"name":"demo-view","steps":[{"id":"work","agent":"worker","prompt":"private rendered task"}],"edges":[{"from":"work","to":"$end"}]}`)
	if _, err := h.admin().ApplyWorkflow(c, def, false); err != nil {
		t.Fatal(err)
	}
	run, _, err := h.admin().StartWorkflow(c, "demo-view", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = worker.Pull(c, 1, 0); err != nil {
		t.Fatal(err)
	}
	taskID := run.Steps[0].MessageID
	if err = worker.Ack(c, taskID); err != nil {
		t.Fatal(err)
	}
	if _, err = worker.Reply(c, taskID, "example selected output", false); err != nil {
		t.Fatal(err)
	}
	h.h.Workflows.Tick(c)
	e := newCLI(t, h, filepath.Join(t.TempDir(), "config.yaml"))
	var compact api.WorkflowRunView
	if err = json.Unmarshal([]byte(e.must(c, "workflow", "run", "show", "--id", run.ID, "--compact")), &compact); err != nil {
		t.Fatal(err)
	}
	if compact.ID != run.ID || compact.View != "compact" || len(compact.Steps) != 1 {
		t.Fatalf("compact: %+v", compact)
	}
	var step api.WorkflowRunView
	if err = json.Unmarshal([]byte(e.must(c, "workflow", "run", "show", "--id", run.ID, "--compact", "--step", compact.Steps[0].ID)), &step); err != nil {
		t.Fatal(err)
	}
	if step.View != "step" || len(step.Steps) != 1 || step.Steps[0].ID != compact.Steps[0].ID || step.Steps[0].Output == nil || (*step.Steps[0].Output)["text"] != "example selected output" || step.Steps[0].OutputSHA256 == "" || step.Steps[0].OutputSHA256 != compact.Steps[0].OutputSHA256 {
		t.Fatal(step)
	}
	if code, _, stderr := e.run(c, "workflow", "run", "show", "--id", run.ID, "--step", compact.Steps[0].ID); code != 2 || !strings.Contains(stderr, "requires --compact") {
		t.Fatal(code, stderr)
	}
	// A completed run has stable duration, so the no-view path can be
	// compared byte-for-byte with the old path's ignored-query behavior.
	completed, err := h.admin().WorkflowRun(c, run.ID)
	if err != nil || completed.Status != "succeeded" {
		t.Fatal("fixture did not finish", completed, err)
	}
	read := func(query string) []byte {
		t.Helper()
		req, _ := http.NewRequestWithContext(c, "GET", h.url+"/v1/workflow-runs/"+run.ID+query, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var b bytes.Buffer
		if _, err = b.ReadFrom(resp.Body); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	if !bytes.Equal(read(""), read("?legacy_ignored=1")) {
		t.Fatal("default run representation changed")
	}
}
func TestWorkflowRunCompactOldHubCLIFailsClosed(t *testing.T) {
	for _, marker := range []string{"", "wrong", "compact"} {
		t.Run("marker_"+marker, func(t *testing.T) {
			old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "run_demo", "name": "demo", "view": marker, "definition": map[string]any{"secret": "MUST_NOT_PRINT"}, "steps": []any{map[string]any{"id": "step_one", "step": "one", "status": "done", "input": "MUST_NOT_PRINT", "payload": "MUST_NOT_PRINT", "output": map[string]any{"secret": "MUST_NOT_PRINT"}}}})
			}))
			defer old.Close()
			env := map[string]string{"MYCO_HUB": old.URL, "MYCO_ADMIN_TOKEN": "example-admin-token", "MYCO_CONFIG": filepath.Join(t.TempDir(), "missing.yaml"), "MYCO_FORMAT": "json"}
			var out, stderr bytes.Buffer
			code := clipkg.Run(ctx(t), []string{"workflow", "run", "show", "--id", "run_demo", "--compact"}, strings.NewReader(""), &out, &stderr, func(k string) string { return env[k] }, false)
			if out.Len() != 0 || strings.Contains(stderr.String(), "MUST_NOT_PRINT") {
				t.Fatal("full-run body escaped the failed compact read")
			}
			want := clipkg.ExitUnsupportedHub
			if marker == "compact" {
				want = clipkg.ExitError
			}
			if code != want {
				t.Fatal(code, stderr.String())
			}
			if marker != "compact" && (!strings.Contains(stderr.String(), `"error":"unsupported_by_hub"`) || !strings.Contains(stderr.String(), `"retryable":false`) || !strings.Contains(stderr.String(), "upgrade")) {
				t.Fatal(stderr.String())
			}
		})
	}
}
