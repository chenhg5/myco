package integration_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
)

// A workflow saved with the CLI, started by an agent with its own token:
// each step is a task the hub sends; replies with a structured result move
// it along; a person decides at the end. A result from an old CLI (a JSON
// block in the reply) counts too.
func TestWorkflowAcrossHubClientAndCLI(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	coder := h.client(h.register(api.RegisterRequest{Name: "coder", Binding: api.Binding{Harness: "claude", SessionID: "s1"}}))
	reviewer := h.client(h.register(api.RegisterRequest{Name: "reviewer", Binding: api.Binding{Harness: "codex", SessionID: "s2"}}))
	cli := newCLI(t, h, filepath.Join(t.TempDir(), "config.yaml"))

	file := filepath.Join(t.TempDir(), "wf.json")
	def := `{"name": "写-审-改",
	  "inputs": {"required": ["task"], "properties": {"task": {"type": "string"}}},
	  "steps": [
	    {"id": "write", "agent": "coder", "prompt": "做：{{inputs.task}}", "output": {"required": ["branch"]}},
	    {"id": "review", "agent": "reviewer", "prompt": "审 {{steps.write.output.branch}}", "output": {"required": ["approved"]}},
	    {"id": "ok", "human": "$starter", "prompt": "合并吗？", "options": ["合并", "不合并"]}],
	  "edges": [
	    {"from": "write", "to": "review"},
	    {"from": "review", "to": "ok", "when": "output.approved"},
	    {"from": "review", "to": "write", "when": "!output.approved", "maxIterations": 2},
	    {"from": "ok", "to": "$end", "when": "output.choice == '合并'"},
	    {"from": "ok", "to": "$fail", "when": "output.choice != '合并'"}]}`
	if err := os.WriteFile(file, []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}
	if out := cli.must(c, "workflow", "validate", "--file", file); !strings.Contains(out, `"valid": true`) {
		t.Fatalf("not valid: %s", out)
	}
	var applied api.WorkflowApplyResponse
	if err := json.Unmarshal([]byte(cli.must(c, "workflow", "apply", "--file", file)), &applied); err != nil || !applied.Created {
		t.Fatalf("apply: %+v %v", applied, err)
	}
	if code, _, _ := cli.run(c, "workflow", "run", "start", "--name", "写-审-改"); code == 0 {
		t.Fatal("started without its required input")
	}

	// An agent starts it with its own token.
	run, _, err := coder.StartWorkflow(c, "写-审-改", map[string]any{"task": "加导出按钮"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if run.By != "coder" || run.Trigger != "agent" || len(run.Current) != 1 || run.Current[0] != "write" {
		t.Fatalf("run: %+v", run)
	}
	ms, err := coder.Pull(c, 1, 0)
	if err != nil || len(ms) != 1 || ms[0].Workflow != run.ID || !strings.Contains(ms[0].Body, "做：加导出按钮") {
		t.Fatalf("coder's task: %+v %v", ms, err)
	}
	if _, err := coder.ReplyResult(c, ms[0].ID, "做好了", json.RawMessage(`{"branch": "feat/x", "summary": "加好了"}`), nil, false); err != nil {
		t.Fatal(err)
	}
	h.h.Workflows.Tick(c)

	ms, err = reviewer.Pull(c, 1, 0)
	if err != nil || len(ms) != 1 || !strings.Contains(ms[0].Body, "审 feat/x") {
		t.Fatalf("reviewer's task: %+v %v", ms, err)
	}
	// The way an old CLI hands a result back: a json block at the end.
	if _, err := reviewer.Reply(c, ms[0].ID, "看过了\n\n```json\n{\"approved\": true}\n```", false); err != nil {
		t.Fatal(err)
	}
	h.h.Workflows.Tick(c)

	var shown api.WorkflowRun
	if err := json.Unmarshal([]byte(cli.must(c, "workflow", "run", "show", "--id", run.ID)), &shown); err != nil {
		t.Fatal(err)
	}
	if shown.Status != "waiting" || len(shown.Steps) != 3 || shown.Steps[0].Summary != "加好了" {
		t.Fatalf("shown: %+v", shown)
	}
	if out := cli.must(c, "workflow", "decisions"); !strings.Contains(out, "合并吗？") {
		t.Fatalf("decisions: %s", out)
	}
	var done api.WorkflowRun
	if err := json.Unmarshal([]byte(cli.must(c, "workflow", "run", "decide", "--id", run.ID, "--step", "ok", "--choice", "合并")), &done); err != nil {
		t.Fatal(err)
	}
	if done.Status != "succeeded" {
		t.Fatalf("after deciding: %+v", done)
	}
	if out := cli.must(c, "workflow", "list"); !strings.Contains(out, `"name": "写-审-改"`) {
		t.Fatalf("list: %s", out)
	}

	// An automation starts it: its inputs filled from the webhook call.
	out := cli.must(c, "automation", "create", "--name", "新 issue", "--workflow", "写-审-改", "--input", "task=修：{{payload.issue.title}}", "--webhook")
	var made api.AutomationResponse
	if err := json.Unmarshal([]byte(out), &made); err != nil || made.Automation.WorkflowName != "写-审-改" || made.WebhookSecret == "" {
		t.Fatalf("automation: %s %v", out, err)
	}
	req, _ := http.NewRequest(http.MethodPost, made.Automation.WebhookURL, strings.NewReader(`{"issue": {"title": "闪退"}}`))
	req.Header.Set("Authorization", "Bearer "+made.WebhookSecret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var hooked struct {
		Run api.AutomationRun `json:"run"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&hooked)
	resp.Body.Close()
	if hooked.Run.WorkflowRun == "" {
		t.Fatalf("webhook run: %+v", hooked)
	}
	// (the coder started the first run: a note says it ended)
	ms, err = coder.PullTypes(c, 1, 0, []string{"task"})
	if err != nil || len(ms) != 1 || !strings.Contains(ms[0].Body, "做：修：闪退") {
		t.Fatalf("task from the automation's run: %+v %v", ms, err)
	}
}
