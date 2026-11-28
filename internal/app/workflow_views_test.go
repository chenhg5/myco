package app

import (
	"context"
	"testing"

	"github.com/myco-ai/myco/internal/domain"
)

type runViewRepo struct {
	domain.WorkflowRepository
	run   *domain.WorkflowRun
	steps []*domain.RunStep
	reads int
}

func (r *runViewRepo) GetRun(context.Context, string) (*domain.WorkflowRun, error) { return r.run, nil }
func (r *runViewRepo) Steps(context.Context, string) ([]*domain.RunStep, error) {
	r.reads++
	return r.steps, nil
}
func TestWorkflowViewReadsOnlyAuthorizedRunAndSteps(t *testing.T) {
	repo := &runViewRepo{run: &domain.WorkflowRun{ID: "run_demo", Owner: "alice", OwnerID: "member_alice"}}
	svc := &WorkflowService{repo: repo}
	if _, _, err := svc.ReadRunSteps(context.Background(), Caller{Name: "bob", MemberID: "member_bob"}, repo.run.ID); !domain.IsCode(err, domain.CodeNotFound) || repo.reads != 0 {
		t.Fatal("unauthorized caller read steps", err)
	}
	run, steps, err := svc.ReadRunSteps(context.Background(), Caller{Name: "alice", MemberID: "member_alice"}, repo.run.ID)
	if err != nil || run != repo.run || len(steps) != 0 || repo.reads != 1 {
		t.Fatal(run, steps, err)
	}
	// Unimplemented definition/events methods panic through the embedded nil
	// interface if this path accidentally starts reading full-run data.
}
