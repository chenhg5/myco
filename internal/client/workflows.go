package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/chenhg5/myco/internal/api"
)

// Workflows lists the workflows the caller may see.
func (c *Client) Workflows(ctx context.Context) ([]api.Workflow, error) {
	var out api.WorkflowList
	if err := c.do(ctx, http.MethodGet, "/v1/workflows", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Workflows, nil
}

// ValidateWorkflow checks a definition without saving it.
func (c *Client) ValidateWorkflow(ctx context.Context, def json.RawMessage) (*api.WorkflowCheck, error) {
	var out api.WorkflowCheck
	if err := c.do(ctx, http.MethodPost, "/v1/workflows/validate", nil, api.WorkflowApplyRequest{Definition: def}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ApplyWorkflow creates the workflow def names, or gives it a new version.
func (c *Client) ApplyWorkflow(ctx context.Context, def json.RawMessage, dry bool) (*api.WorkflowApplyResponse, error) {
	var out api.WorkflowApplyResponse
	if err := c.do(ctx, http.MethodPost, "/v1/workflows", dryQuery(dry), api.WorkflowApplyRequest{Definition: def}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Workflow returns one (by id or name) with its definition.
func (c *Client) Workflow(ctx context.Context, ref string) (*api.Workflow, error) {
	var out api.Workflow
	if err := c.do(ctx, http.MethodGet, "/v1/workflows/"+esc(ref), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetWorkflowEnabled turns one on or off.
func (c *Client) SetWorkflowEnabled(ctx context.Context, ref string, on bool) (*api.Workflow, error) {
	var out api.Workflow
	if err := c.do(ctx, http.MethodPatch, "/v1/workflows/"+esc(ref), nil, api.WorkflowUpdate{Enabled: &on}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteWorkflow removes one and its runs (stopping the ones going).
func (c *Client) DeleteWorkflow(ctx context.Context, ref string, dry bool) error {
	return c.do(ctx, http.MethodDelete, "/v1/workflows/"+esc(ref), dryQuery(dry), nil, nil)
}

// StartWorkflow starts a run. With dry, it only shows the first step.
func (c *Client) StartWorkflow(ctx context.Context, ref string, inputs map[string]any, dry bool) (*api.WorkflowRun, *api.WorkflowPreview, error) {
	if dry {
		var pre api.WorkflowPreview
		err := c.do(ctx, http.MethodPost, "/v1/workflows/"+esc(ref)+"/runs", dryQuery(true), api.WorkflowStart{Inputs: inputs}, &pre)
		return nil, &pre, err
	}
	var out api.WorkflowRun
	if err := c.do(ctx, http.MethodPost, "/v1/workflows/"+esc(ref)+"/runs", nil, api.WorkflowStart{Inputs: inputs}, &out); err != nil {
		return nil, nil, err
	}
	return &out, nil, nil
}

// WorkflowRuns lists runs, newest first (of one workflow when ref is set).
func (c *Client) WorkflowRuns(ctx context.Context, ref string, active bool, limit int) ([]api.WorkflowRun, error) {
	q := url.Values{}
	if ref != "" {
		q.Set("workflow", ref)
	}
	if active {
		q.Set("active", "true")
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var out api.WorkflowRunList
	if err := c.do(ctx, http.MethodGet, "/v1/workflow-runs", q, nil, &out); err != nil {
		return nil, err
	}
	return out.Runs, nil
}

// WorkflowRun returns a run with its steps and log.
func (c *Client) WorkflowRun(ctx context.Context, id string) (*api.WorkflowRun, error) {
	var out api.WorkflowRun
	if err := c.do(ctx, http.MethodGet, "/v1/workflow-runs/"+esc(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) runAction(ctx context.Context, id, action string, body any) (*api.WorkflowRun, error) {
	var out api.WorkflowRun
	if err := c.do(ctx, http.MethodPost, "/v1/workflow-runs/"+esc(id)+"/"+action, nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelWorkflowRun stops a run.
func (c *Client) CancelWorkflowRun(ctx context.Context, id string) (*api.WorkflowRun, error) {
	return c.runAction(ctx, id, "cancel", struct{}{})
}

// RetryWorkflowRun restarts a failed or cancelled run (at from, or the
// step that failed).
func (c *Client) RetryWorkflowRun(ctx context.Context, id, from string) (*api.WorkflowRun, error) {
	return c.runAction(ctx, id, "retry", api.WorkflowRetry{From: from})
}

// DecideWorkflowRun answers a human step.
func (c *Client) DecideWorkflowRun(ctx context.Context, id string, in api.WorkflowDecide) (*api.WorkflowRun, error) {
	return c.runAction(ctx, id, "decide", in)
}

// WorkflowDecisions lists the decisions waiting for the caller.
func (c *Client) WorkflowDecisions(ctx context.Context) ([]api.WorkflowDecision, error) {
	var out api.WorkflowDecisions
	if err := c.do(ctx, http.MethodGet, "/v1/workflow-decisions", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Decisions, nil
}
