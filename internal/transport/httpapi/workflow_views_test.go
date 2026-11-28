package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
)

func TestWorkflowRunViewProjection(t *testing.T) {
	now := time.Now().UTC()
	run := &domain.WorkflowRun{ID: "run_demo", Name: "demo", Version: 2, Owner: "alice", Project: "demo", Status: "succeeded", Inputs: map[string]any{"source": "example"}, CreatedAt: now, UpdatedAt: now}
	steps := []*domain.RunStep{
		{ID: "step_one", RunID: run.ID, StepID: "write", Attempt: 1, Status: "failed", Input: "rendered-task-secret", Payload: map[string]any{"secret": "payload-secret"}, Output: map[string]any{"z": 2, "a": 1}, StartedAt: now},
		{ID: "step_two", RunID: run.ID, StepID: "write", Attempt: 2, Status: "succeeded", Output: map[string]any{}, StartedAt: now},
		{ID: "step_empty", RunID: run.ID, StepID: "finish", Status: "waiting", StartedAt: now},
	}
	compact, err := projectWorkflowRun(run, steps, "compact", "")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(compact)
	var obj map[string]any
	if err = json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"definition", "events", "artifacts", "evaluations"} {
		if _, ok := obj[key]; ok {
			t.Fatal("leaked " + key)
		}
	}
	for _, row := range obj["steps"].([]any) {
		for _, key := range []string{"input", "payload", "output"} {
			if _, ok := row.(map[string]any)[key]; ok {
				t.Fatal("compact leaked " + key)
			}
		}
	}
	sum := sha256.Sum256([]byte(`{"a":1,"z":2}`))
	if compact.Steps[0].OutputSHA256 != hex.EncodeToString(sum[:]) || compact.Steps[2].OutputSHA256 != "" {
		t.Fatal("wrong canonical hash")
	}
	for selector, want := range map[string]string{"step_one": "step_one", "write": "step_two"} {
		view, e := projectWorkflowRun(run, steps, "step", selector)
		if e != nil || len(view.Steps) != 1 || view.Steps[0].ID != want || view.Steps[0].Output == nil {
			t.Fatal(view, e)
		}
		b, _ := json.Marshal(view)
		if strings.Contains(string(b), `"input":`) || strings.Contains(string(b), `"payload":`) {
			t.Fatal("step leaked rendered task or payload")
		}
	}
	foreign := &domain.RunStep{ID: "step_foreign", RunID: "run_other", StepID: "foreign", Output: map[string]any{"secret": "other-run"}}
	if _, err = projectWorkflowRun(run, append(steps, foreign), "step", "step_foreign"); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatal("foreign instance accepted", err)
	}
	for _, view := range []string{"", "full", "other"} {
		if _, err = projectWorkflowRun(run, steps, view, ""); !domain.IsCode(err, domain.CodeInvalidArgument) {
			t.Fatal(view, err)
		}
	}
	if _, err = projectWorkflowRun(run, steps, "step", ""); !domain.IsCode(err, domain.CodeInvalidArgument) {
		t.Fatal("empty selector accepted")
	}
}
func TestWorkflowRunViewLargeRunSizes(t *testing.T) {
	now := time.Now().UTC()
	run := &domain.WorkflowRun{ID: "run_demo", Name: "large", Version: 1, Owner: "alice", Project: "demo", Status: "succeeded", CreatedAt: now, UpdatedAt: now}
	def := &domain.WorkflowDef{}
	steps := []*domain.RunStep{}
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("step_%02d", i)
		def.Steps = append(def.Steps, domain.StepDef{ID: id})
		steps = append(steps, &domain.RunStep{ID: id, RunID: run.ID, StepID: id, Attempt: 1, Status: "succeeded", Input: strings.Repeat("task", 8192), Payload: map[string]any{"input": strings.Repeat("p", 20000)}, Output: map[string]any{"answer": strings.Repeat("o", 16000)}, StartedAt: now, EndedAt: now})
	}
	full, _ := json.Marshal(runDTO(run, &app.RunDetail{Run: run, Def: def, Steps: steps, Events: []*domain.RunEvent{{Text: strings.Repeat("log", 16000)}}}))
	compact, err := projectWorkflowRun(run, steps, "compact", "")
	if err != nil {
		t.Fatal(err)
	}
	small, _ := json.Marshal(compact)
	selected, err := projectWorkflowRun(run, steps, "step", steps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	one, _ := json.Marshal(selected)
	if len(full) < 800000 || len(small) >= 64*1024 || len(one) >= 128*1024 {
		t.Fatalf("unexpected sizes full=%d compact=%d step=%d", len(full), len(small), len(one))
	}
	t.Logf("response bytes: full=%d compact=%d step=%d", len(full), len(small), len(one))
}
