package app

import (
	"context"

	"github.com/myco-ai/myco/internal/domain"
)

// ReadRunSteps uses the exact run-read authorization, without reading the
// definition, events, artifacts, evaluations or computing full-run metrics.
func (s *WorkflowService) ReadRunSteps(ctx context.Context, c Caller, id string) (*domain.WorkflowRun, []*domain.RunStep, error) {
	run, err := s.getRun(ctx, c, id)
	if err != nil {
		return nil, nil, err
	}
	steps, err := s.repo.Steps(ctx, run.ID)
	if err != nil {
		return nil, nil, err
	}
	return run, steps, nil
}
