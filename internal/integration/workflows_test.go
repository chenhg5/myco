package integration_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	clipkg "github.com/myco-ai/myco/internal/cli"
	"github.com/myco-ai/myco/internal/config"
)

// A computer step is sent by its owner's console, not by a listed agent.
// Exercise the real Hub /me and runner wiring rather than injecting senders
// directly into the exec harness.
func TestWorkflowComputerRunsOnOwnersRestrictedExecNode(t *testing.T) {
	home := isolatedHome(t)
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	if _, err := h.admin().UpdateMember(c, "owner", api.MemberUpdateRequest{Name: "alice"}); err != nil {
		t.Fatal(err)
	}
	node := newCLI(t, h, filepath.Join(home, "node.yaml"))
	// The console-owner exception does not replace explicit exec-risk setup.
	if code, _, _ := node.run(c, "agent", "register", "--name", "unconfigured-shell", "--harness", "exec"); code == 0 {
		t.Fatal("exec registration without allowed agents or risk confirmation succeeded")
	}
	node.must(c, "agent", "register", "--name", "test-shell", "--harness", "exec", "--allow-from", "lead", "--workdir", t.TempDir())
	file := filepath.Join(t.TempDir(), "workflow.json")
	def := `{"name":"owner-computer","steps":[{"id":"command","computer":"test-shell","run":"echo owner-ok"}],"edges":[{"from":"command","to":"$end"}]}`
	if err := os.WriteFile(file, []byte(def), 0o600); err != nil {
		t.Fatal(err)
	}
	owner := newCLI(t, h, filepath.Join(home, "owner.yaml"))
	owner.must(c, "workflow", "apply", "--file", file)
	run, _, err := h.admin().StartWorkflow(c, "owner-computer", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(filepath.Join(home, "node.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	me, err := h.client(cfg.Token).Me(c)
	if err != nil || me.Owner != "alice" || len(me.AllowFrom) != 1 || me.AllowFrom[0] != "lead" {
		t.Fatalf("Hub owner and restricted list: %+v %v", me, err)
	}
	node.must(c, "runner", "start", "--once")
	for {
		h.h.Workflows.Tick(c)
		done, err := h.admin().WorkflowRun(c, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if done.Status == "succeeded" {
			break
		}
		if done.Status != "running" || c.Err() != nil {
			t.Fatalf("computer step did not succeed: %+v", done)
		}
		time.Sleep(10 * time.Millisecond)
	}
	log, err := os.ReadFile(filepath.Join(home, ".myco", "logs", "test-shell.exec.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var entry struct {
		From   string `json:"from"`
		Denied bool   `json:"denied"`
	}
	if err := json.Unmarshal(log, &entry); err != nil || entry.From != "@console:alice" || entry.Denied {
		t.Fatalf("owner console audit: %s, %v", log, err)
	}
	me, err = h.client(cfg.Token).Me(c)
	if err != nil || len(me.AllowFrom) != 1 || me.AllowFrom[0] != "lead" {
		t.Fatalf("derived console was persisted in allow_from: %+v %v", me, err)
	}
}

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
	var initial api.WorkflowList
	if err := json.Unmarshal([]byte(cli.must(c, "workflow", "list")), &initial); err != nil {
		t.Fatal(err)
	}
	if len(initial.Workflows) != 1 || initial.Workflows[0].RunCounts == nil || initial.Workflows[0].RunCounts.Total != 0 {
		t.Fatalf("new workflow must report known zero counts: %+v", initial)
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

	// The list carries exact, additive counters without loading the stats route.
	var catalog api.WorkflowList
	if err := json.Unmarshal([]byte(cli.must(c, "workflow", "list")), &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Workflows) != 1 || catalog.Workflows[0].RunCounts == nil {
		t.Fatalf("catalog counters missing: %+v", catalog)
	}
	counts := catalog.Workflows[0].RunCounts
	if counts.Total != 1 || counts.Succeeded != 1 || counts.Running != 0 || counts.Waiting != 0 || counts.Failed != 0 || counts.Cancelled != 0 {
		t.Fatalf("catalog counts: %+v", counts)
	}

	// What it did and how it went: metrics, a step's log, scores (the
	// reviewer agent gives one), figures, runs by state.
	if done.Metrics == nil || done.Metrics.Steps != 3 || done.Metrics.Done != 3 {
		t.Fatalf("metrics: %+v", done.Metrics)
	}
	var log api.StepLog
	if err := json.Unmarshal([]byte(cli.must(c, "workflow", "run", "logs", "--id", run.ID, "--step", "write")), &log); err != nil {
		t.Fatal(err)
	}
	if len(log.Entries) < 3 || log.Entries[0].Kind != "task" || !strings.Contains(log.Entries[0].Text, "做：加导出按钮") {
		t.Fatalf("log: %+v", log.Entries)
	}
	if _, err := reviewer.EvaluateWorkflowRun(c, run.ID, api.RunEvaluate{Name: "quality", Value: 4, Direction: "higher"}, false); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := cli.run(c, "workflow", "run", "evaluate", "--id", run.ID, "--name", "quality", "--value", "5", "--dry-run"); code != clipkg.ExitDryRun ||
		!strings.Contains(out, `"workflow_run_evaluate"`) {
		t.Fatalf("dry run: %d %s", code, out)
	}
	if code, _, _ := cli.run(c, "workflow", "run", "evaluate", "--id", run.ID, "--name", "quality", "--value", "lots"); code == 0 {
		t.Fatal("a score that is not a number was taken")
	}
	var st api.WorkflowStats
	if err := json.Unmarshal([]byte(cli.must(c, "workflow", "stats", "--name", "写-审-改")), &st); err != nil {
		t.Fatal(err)
	}
	if st.Total != 1 || st.SuccessRate != 1 || len(st.Steps) != 3 || len(st.Evaluations) != 1 || st.Evaluations[0].Last != 4 {
		t.Fatalf("stats: %+v", st)
	}
	var cmp api.RunComparison
	if err := json.Unmarshal([]byte(cli.must(c, "workflow", "run", "compare", "--a", run.ID, "--b", run.ID)), &cmp); err != nil || !cmp.SameVersion {
		t.Fatalf("compare: %+v %v", cmp, err)
	}
	var none api.WorkflowRunList
	if err := json.Unmarshal([]byte(cli.must(c, "workflow", "run", "list", "--status", "failed")), &none); err != nil || len(none.Runs) != 0 {
		t.Fatalf("failed runs: %+v %v", none, err)
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
