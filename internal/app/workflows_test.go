package app_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/infra/notify"
	"github.com/chenhg5/myco/internal/infra/security"
	"github.com/chenhg5/myco/internal/infra/sqlite"
)

type wfEnv struct {
	t      *testing.T
	ctx    context.Context
	clock  *fakeClock
	svc    *app.WorkflowService
	msgs   *app.MessageService
	agents map[string]*domain.Agent
	glenn  app.Caller
	aut    *app.AutomationService
	reg    *app.AgentService
}

func newWF(t *testing.T, names ...string) *wfEnv {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	clock := &fakeClock{t: time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)}
	ar, mr := sqlite.NewAgentRepo(db), sqlite.NewMessageRepo(db)
	msgs := app.NewMessageService(ar, mr, notify.NewMemory(), security.IDs{}, clock, app.DefaultMessagePolicy())
	reg := app.NewAgentService(ar, security.Tokens{}, security.IDs{}, clock, time.Minute, nil)
	svc := app.NewWorkflowService(sqlite.NewWorkflowRepo(db), msgs, mr, security.IDs{}, clock, slog.New(slog.NewTextHandler(io.Discard, nil)))
	aut := app.NewAutomationService(sqlite.NewAutomationRepo(db), msgs, mr, security.Tokens{}, security.IDs{}, clock, slog.New(slog.NewTextHandler(io.Discard, nil)))
	aut.SetWorkflows(svc)
	e := &wfEnv{t: t, ctx: context.Background(), clock: clock, svc: svc, msgs: msgs, agents: map[string]*domain.Agent{},
		glenn: app.Caller{Name: "glenn", MemberID: "mem_1"}, aut: aut, reg: reg}
	for _, n := range names {
		in := app.RegisterInput{Name: n}
		if strings.HasPrefix(n, "box") {
			in.Binding = domain.Binding{Harness: domain.HarnessExec}
		}
		out, err := reg.Register(e.ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		e.agents[n] = out.Agent
	}
	return e
}

func (e *wfEnv) apply(def string) *domain.Workflow {
	e.t.Helper()
	d, err := domain.ParseWorkflowDef([]byte(def))
	if err != nil {
		e.t.Fatal(err)
	}
	out, err := e.svc.Apply(e.ctx, e.glenn, d, false)
	if err != nil {
		e.t.Fatal(err)
	}
	return out.Workflow
}

func (e *wfEnv) start(name string, inputs map[string]any) *domain.WorkflowRun {
	e.t.Helper()
	d, _, err := e.svc.Start(e.ctx, e.glenn, name, app.StartInput{Inputs: inputs})
	if err != nil {
		e.t.Fatal(err)
	}
	return d.Run
}

// task pulls agent's next task (failing when there is none).
func (e *wfEnv) task(agent string) *domain.Message {
	e.t.Helper()
	ms, err := e.msgs.Pull(e.ctx, e.agents[agent], 1, 0)
	if err != nil || len(ms) != 1 {
		e.t.Fatalf("%s: no task (%v)", agent, err)
	}
	return ms[0]
}

func (e *wfEnv) noTask(agent string) {
	e.t.Helper()
	if ms, _ := e.msgs.Pull(e.ctx, e.agents[agent], 1, 0); len(ms) != 0 {
		e.t.Fatalf("%s got an unexpected task: %s", agent, ms[0].Body)
	}
}

func (e *wfEnv) reply(agent string, m *domain.Message, body string, result any) {
	e.t.Helper()
	res := ""
	if result != nil {
		b, _ := json.Marshal(result)
		res = string(b)
	}
	if _, err := e.msgs.ReplyResult(e.ctx, e.agents[agent], m.ID, body, res, nil, false); err != nil {
		e.t.Fatal(err)
	}
	e.svc.Tick(e.ctx)
}

func (e *wfEnv) run(id string) *app.RunDetail {
	e.t.Helper()
	d, err := e.svc.GetRun(e.ctx, e.glenn, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return d
}

const writeReview = `{
  "name": "写-审-改",
  "inputs": {"type": "object", "required": ["task"], "properties": {"task": {"type": "string"}}},
  "steps": [
    {"id": "write", "agent": "coder", "prompt": "做：{{inputs.task}}{{#if steps.review}}\n意见：{{steps.review.output.comments}}{{/if}}",
     "output": {"required": ["branch"], "properties": {"branch": {"type": "string"}}}},
    {"id": "review", "agent": "reviewer", "prompt": "审 {{steps.write.output.branch}}",
     "output": {"required": ["approved"], "properties": {"approved": {"type": "boolean"}, "comments": {"type": "array"}}}},
    {"id": "approve", "human": "$starter", "prompt": "合并 {{steps.write.output.branch}} 吗？", "options": ["合并", "不合并"]}
  ],
  "edges": [
    {"from": "write", "to": "review"},
    {"from": "review", "to": "approve", "when": "output.approved"},
    {"from": "review", "to": "write", "when": "!output.approved", "maxIterations": 1, "else": "$fail"},
    {"from": "approve", "to": "$end", "when": "output.choice == '合并'"},
    {"from": "approve", "to": "$fail", "when": "output.choice != '合并'"}
  ]
}`

func TestWorkflowWriteReviewLoopAndDecision(t *testing.T) {
	e := newWF(t, "coder", "reviewer")
	e.apply(writeReview)
	run := e.start("写-审-改", map[string]any{"task": "加导出按钮"})

	m := e.task("coder")
	if m.Workflow != run.ID || m.From != app.ConsoleSender("glenn") || !strings.Contains(m.Body, "做：加导出按钮") ||
		!strings.Contains(m.Body, "--result") || strings.Contains(m.Body, "意见") {
		t.Fatalf("first task: %+v\n%s", m, m.Body)
	}
	e.noTask("reviewer")
	e.reply("coder", m, "done", map[string]any{"branch": "feat/x", "summary": "加好了"})

	m = e.task("reviewer")
	if !strings.Contains(m.Body, "审 feat/x") {
		t.Fatalf("review task: %s", m.Body)
	}
	e.reply("reviewer", m, "not yet", map[string]any{"approved": false, "comments": []any{"少了测试"}})

	m = e.task("coder") // round two, with the comments
	if !strings.Contains(m.Body, `意见：["少了测试"]`) || !strings.Contains(m.Body, "第 2 次") {
		t.Fatalf("second write: %s", m.Body)
	}
	e.reply("coder", m, "done", map[string]any{"branch": "feat/x2"})
	e.reply("reviewer", e.task("reviewer"), "ok", map[string]any{"approved": true})

	d := e.run(run.ID)
	if d.Run.Status != domain.FlowWaiting {
		t.Fatalf("waiting for the decision: %s %s", d.Run.Status, d.Run.Error)
	}
	ds, _ := e.svc.Decisions(e.ctx, e.glenn)
	if len(ds) != 1 || ds[0].Step.Input != "合并 feat/x2 吗？" || ds[0].Step.Target != "glenn" {
		t.Fatalf("decisions: %+v", ds)
	}
	if _, err := e.svc.Decide(e.ctx, e.glenn, run.ID, "approve", "随便", ""); err == nil {
		t.Fatal("a choice that is not an option was taken")
	}
	if _, err := e.svc.Decide(e.ctx, app.Caller{Name: "bob", MemberID: "mem_2"}, run.ID, "approve", "合并", ""); err == nil {
		t.Fatal("someone else decided")
	}
	r, err := e.svc.Decide(e.ctx, e.glenn, run.ID, "approve", "合并", "好")
	if err != nil || r.Status != domain.FlowSucceeded {
		t.Fatalf("after deciding: %+v %v", r, err)
	}
	d = e.run(run.ID)
	if len(d.Steps) != 5 || d.Steps[4].Summary != "合并：好" {
		t.Fatalf("steps: %d %+v", len(d.Steps), d.Steps[len(d.Steps)-1])
	}
}

func TestWorkflowLoopStopsAtItsLimit(t *testing.T) {
	e := newWF(t, "coder", "reviewer")
	e.apply(writeReview)
	run := e.start("写-审-改", map[string]any{"task": "x"})
	for i := 0; i < 2; i++ {
		e.reply("coder", e.task("coder"), "done", map[string]any{"branch": "b"})
		e.reply("reviewer", e.task("reviewer"), "no", map[string]any{"approved": false})
	}
	d := e.run(run.ID)
	if d.Run.Status != domain.FlowFailed || !strings.Contains(d.Run.Error, "最多 1 次") {
		t.Fatalf("after two rejections: %s %q", d.Run.Status, d.Run.Error)
	}
	e.noTask("coder")
}

func TestWorkflowBouncesAResultThatDoesNotFitOnce(t *testing.T) {
	e := newWF(t, "coder", "reviewer")
	e.apply(writeReview)
	run := e.start("写-审-改", map[string]any{"task": "x"})
	m := e.task("coder")
	e.reply("coder", m, "I did it", nil) // no result
	again := e.task("coder")
	if again.ThreadID != m.ThreadID || !strings.Contains(again.Body, "branch is missing") {
		t.Fatalf("bounce: %+v\n%s", again, again.Body)
	}
	// The old-CLI way: a ```json block at the end of the reply.
	e.reply("coder", again, "here\n```json\n{\"branch\": \"feat/y\", \"summary\": \"补上了\"}\n```", nil)
	m = e.task("reviewer")
	if !strings.Contains(m.Body, "审 feat/y") {
		t.Fatalf("review: %s", m.Body)
	}
	d := e.run(run.ID)
	if d.Steps[0].Summary != "补上了" || !d.Steps[0].Bounced {
		t.Fatalf("write step: %+v", d.Steps[0])
	}
	// A second bad result fails the step (and the run: no error edge).
	e.reply("reviewer", m, "hmm", nil)
	e.reply("reviewer", e.task("reviewer"), "still prose", nil)
	if d := e.run(run.ID); d.Run.Status != domain.FlowFailed || !strings.Contains(d.Run.Error, "approved is missing") {
		t.Fatalf("run: %s %q", d.Run.Status, d.Run.Error)
	}
}

func TestWorkflowDecisionThatExpiresIsANo(t *testing.T) {
	e := newWF(t)
	e.apply(`{"name": "gate", "steps": [
	  {"id": "ok", "human": "$starter", "prompt": "发吗？", "options": ["发", "不发"], "timeout": "1h"},
	  {"id": "after", "human": "$starter", "prompt": "真的？", "options": ["是", "否"]}],
	  "edges": [{"from": "ok", "to": "after", "when": "output.choice == '发'"}]}`)
	run := e.start("gate", nil)
	e.clock.t = e.clock.t.Add(2 * time.Hour)
	e.svc.Tick(e.ctx)
	d := e.run(run.ID)
	if d.Run.Status != domain.FlowFailed || !strings.Contains(d.Run.Error, "没有路可走") || len(d.Steps) != 1 {
		t.Fatalf("expired: %s %q %d", d.Run.Status, d.Run.Error, len(d.Steps))
	}
}

func TestWorkflowParallelJoinAndCommands(t *testing.T) {
	e := newWF(t, "a", "b", "boxmac", "writer")
	e.apply(`{"name": "fan", "steps": [
	  {"id": "split", "computer": "boxmac", "run": "echo go", "dispatch": "all"},
	  {"id": "left", "agent": "a", "prompt": "left"},
	  {"id": "right", "agent": "b", "prompt": "right"},
	  {"id": "merge", "agent": "writer", "prompt": "合并 {{steps.left.summary}} + {{steps.right.summary}}", "join": "all"},
	  {"id": "test", "computer": "boxmac", "run": "make test", "parse": "json"},
	  {"id": "fix", "agent": "writer", "prompt": "修：{{steps.test.error}}"}],
	  "edges": [
	    {"from": "split", "to": "left"}, {"from": "split", "to": "right"},
	    {"from": "left", "to": "merge"}, {"from": "right", "to": "merge"},
	    {"from": "merge", "to": "test"},
	    {"from": "test", "to": "fix", "onError": true},
	    {"from": "test", "to": "$end", "when": "output.json.ok"}]}`)
	run := e.start("fan", nil)
	m := e.task("boxmac")
	if m.Body != "echo go" {
		t.Fatalf("command: %q", m.Body)
	}
	e.reply("boxmac", m, `{"exit_code":0,"stdout":"go\n","stderr":"","duration_ms":3}`, nil)
	l, r := e.task("a"), e.task("b")
	e.reply("a", l, "左边好了", nil)
	e.noTask("writer") // still waiting for right
	e.reply("b", r, "右边好了", nil)
	mm := e.task("writer")
	if !strings.Contains(mm.Body, "合并 左边好了 + 右边好了") {
		t.Fatalf("merge: %s", mm.Body)
	}
	e.reply("writer", mm, "合好了", nil)
	e.reply("boxmac", e.task("boxmac"), `{"exit_code":2,"stdout":"","stderr":"FAIL x_test\n","duration_ms":3}`, nil)
	fix := e.task("writer")
	if !strings.Contains(fix.Body, "修：退出码 2：FAIL x_test") {
		t.Fatalf("fix: %s", fix.Body)
	}
	e.reply("writer", fix, "修好了", nil)
	if d := e.run(run.ID); d.Run.Status != domain.FlowSucceeded {
		t.Fatalf("run: %s %q", d.Run.Status, d.Run.Error)
	}
}

func TestWorkflowCancelStopsItsTaskAndRetryStartsAgain(t *testing.T) {
	e := newWF(t, "coder", "reviewer")
	e.apply(writeReview)
	run := e.start("写-审-改", map[string]any{"task": "x"})
	m := e.task("coder")
	if _, err := e.svc.Cancel(e.ctx, app.Caller{Name: "bob", MemberID: "mem_2"}, run.ID); err == nil {
		t.Fatal("someone else cancelled it")
	}
	r, err := e.svc.Cancel(e.ctx, e.glenn, run.ID)
	if err != nil || r.Status != domain.FlowCancelled {
		t.Fatalf("cancel: %+v %v", r, err)
	}
	if got, _ := e.msgs.Get(e.ctx, e.agents["coder"], m.ID); !got.Status.Terminal() {
		t.Fatalf("the task was left open: %s", got.Status)
	}
	r, err = e.svc.Retry(e.ctx, e.glenn, run.ID, "")
	if err != nil || r.Status != domain.FlowRunning {
		t.Fatalf("retry: %+v %v", r, err)
	}
	e.reply("coder", e.task("coder"), "done", map[string]any{"branch": "b"})
	e.task("reviewer")
}

func TestWorkflowInputsAreCheckedAndCoerced(t *testing.T) {
	e := newWF(t, "coder")
	e.apply(`{"name": "n", "inputs": {"required": ["count"], "properties": {"count": {"type": "integer"}, "dry": {"type": "boolean", "default": true}}},
	  "steps": [{"id": "s", "agent": "coder", "prompt": "{{inputs.count}} {{inputs.dry}}"}]}`)
	if _, _, err := e.svc.Start(e.ctx, e.glenn, "n", app.StartInput{Inputs: map[string]any{"count": "lots"}}); err == nil {
		t.Fatal("a word was taken for a number")
	}
	e.start("n", map[string]any{"count": "3"})
	if m := e.task("coder"); !strings.Contains(m.Body, "3 true") {
		t.Fatalf("task: %s", m.Body)
	}
}

func TestWorkflowApplyVersionsAndOwnership(t *testing.T) {
	e := newWF(t, "coder")
	def := `{"name": "v", "steps": [{"id": "s", "agent": "coder", "prompt": "PROMPT"}]}`
	w := e.apply(def)
	if w.Version != 1 {
		t.Fatalf("version %d", w.Version)
	}
	if w = e.apply(def); w.Version != 1 {
		t.Fatal("the same definition made a new version")
	}
	if w = e.apply(strings.Replace(def, "PROMPT", "P2", 1)); w.Version != 2 {
		t.Fatalf("changed: version %d", w.Version)
	}
	d, _ := domain.ParseWorkflowDef([]byte(def))
	if _, err := e.svc.Apply(e.ctx, app.Caller{Name: "bob", MemberID: "mem_2"}, d, false); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("bob changed glenn's: %v", err)
	}
	chk := e.svc.Check(e.ctx, &domain.WorkflowDef{Name: "x", Steps: []domain.StepDef{{ID: "s", Agent: "ghost", Prompt: "p"}}})
	if len(chk.Problems) != 0 || len(chk.Warnings) != 1 {
		t.Fatalf("check: %+v", chk)
	}
}

func TestAutomationStartsAWorkflowFromAWebhookCall(t *testing.T) {
	e := newWF(t, "coder", "reviewer")
	w := e.apply(writeReview)
	str := func(v string) *string { return &v }
	on := true
	out, err := e.aut.Create(e.ctx, e.glenn, app.AutomationInput{Name: str("issue → 写-审-改"), Workflow: str("写-审-改"),
		Inputs: map[string]string{"task": "修 issue：{{payload.issue.title}}"}, Webhook: &on, SkipIfBusy: &on})
	if err != nil {
		t.Fatal(err)
	}
	a := out.Automation
	if a.Workflow != w.ID || a.To != "" {
		t.Fatalf("automation: %+v", a)
	}
	secret := out.Secret
	call := &domain.WebhookRequest{Body: `{"issue": {"title": "登录闪退"}}`}
	r, err := e.aut.Webhook(e.ctx, a.ID, secret, call)
	if err != nil || r.Outcome != domain.RunSent || r.WorkflowRun == "" {
		t.Fatalf("run: %+v %v", r, err)
	}
	m := e.task("coder")
	if m.Workflow != r.WorkflowRun || !strings.Contains(m.Body, "做：修 issue：登录闪退") {
		t.Fatalf("task: %s", m.Body)
	}
	if d := e.run(r.WorkflowRun); d.Run.Trigger != domain.FlowByAutomation || d.Run.Owner != "glenn" {
		t.Fatalf("run: %+v", d.Run)
	}
	// The last run is still going: skipped.
	if r2, err := e.aut.Webhook(e.ctx, a.ID, secret, call); err != nil || r2.Outcome != domain.RunSkipped {
		t.Fatalf("second: %+v %v", r2, err)
	}
	if _, err := e.svc.Cancel(e.ctx, e.glenn, r.WorkflowRun); err != nil {
		t.Fatal(err)
	}
	if r3, err := e.aut.RunNow(e.ctx, e.glenn, a.ID); err != nil || r3.Outcome != domain.RunSent {
		t.Fatalf("after it ended: %+v %v", r3, err)
	}
	// Someone who may not see the workflow cannot point one at it.
	if _, err := e.aut.Create(e.ctx, app.Caller{Name: "bob", MemberID: "mem_2"}, app.AutomationInput{Name: str("x"), Workflow: str(w.ID), Webhook: &on}); err == nil {
		t.Fatal("bob used glenn's workflow")
	}
}

func TestWorkflowTellsTheAgentThatStartedIt(t *testing.T) {
	e := newWF(t, "lead", "coder")
	e.apply(`{"name": "one", "steps": [{"id": "do", "title": "做", "agent": "coder", "prompt": "x"}]}`)
	lead := app.Caller{Name: "glenn", MemberID: "mem_1", Agent: "lead"}
	d, _, err := e.svc.Start(e.ctx, lead, "one", app.StartInput{})
	if err != nil || d.Run.Trigger != domain.FlowByAgent || d.Run.By != "lead" {
		t.Fatalf("start: %+v %v", d, err)
	}
	e.reply("coder", e.task("coder"), "改好了", nil)
	n := e.task("lead")
	if n.Type != domain.TypeNote || n.Workflow != d.Run.ID || !strings.Contains(n.Body, "完成了") || !strings.Contains(n.Body, "做（coder）：改好了") {
		t.Fatalf("note: %+v\n%s", n, n.Body)
	}
	// Cancelled by the agent itself: nothing to tell.
	d, _, _ = e.svc.Start(e.ctx, lead, "one", app.StartInput{})
	if _, err := e.svc.Cancel(e.ctx, lead, d.Run.ID); err != nil {
		t.Fatal(err)
	}
	e.noTask("lead")
	// Started by a person: no note to anyone.
	e.start("one", nil)
	e.reply("coder", e.task("coder"), "ok", nil)
	e.noTask("lead")
}

func TestWorkflowStepByRolePicksTheLeastBusy(t *testing.T) {
	e := newWF(t)
	add := func(in app.RegisterInput) {
		in.Binding = domain.Binding{Harness: "claude", SessionID: string(in.Name) + "-s"}
		out, err := e.reg.Register(e.ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		e.agents[in.Name] = out.Agent
	}
	add(app.RegisterInput{Name: "rev-a", Description: "负责审查 Go 代码"})
	add(app.RegisterInput{Name: "rev-b", Labels: []string{"role=审查"}})
	add(app.RegisterInput{Name: "writer", Description: "写文档"})
	chk := e.svc.Check(e.ctx, &domain.WorkflowDef{Name: "x", Steps: []domain.StepDef{{ID: "s", Role: "设计", Prompt: "p"}}})
	if len(chk.Problems) != 0 || len(chk.Warnings) != 1 {
		t.Fatalf("check: %+v", chk)
	}
	e.apply(`{"name": "r", "steps": [{"id": "rv", "title": "审", "role": "审查", "prompt": "看看"}]}`)
	// rev-a is busy with something else: rev-b is free.
	if _, err := e.msgs.SendFromMember(e.ctx, "glenn", "mem_1", app.SendInput{To: "rev-a", Body: "别的事"}); err != nil {
		t.Fatal(err)
	}
	run := e.start("r", nil)
	m := e.task("rev-b")
	if m.Workflow != run.ID {
		t.Fatalf("went to the wrong one: %+v", m)
	}
	d := e.run(run.ID)
	if d.Steps[0].Target != "rev-b" || !strings.Contains(d.Events[len(d.Events)-1].Text, "按分工「审查」交给 rev-b（空闲）") {
		t.Fatalf("step: %+v events: %+v", d.Steps[0], d.Events)
	}
	// No one does it: the step fails, saying so.
	e.apply(`{"name": "none", "steps": [{"id": "x", "role": "设计", "prompt": "p"}]}`)
	if d := e.run(e.start("none", nil).ID); d.Run.Status != domain.FlowFailed || !strings.Contains(d.Run.Error, "没有分工是「设计」的 agent") {
		t.Fatalf("no one: %s %q", d.Run.Status, d.Run.Error)
	}
}
