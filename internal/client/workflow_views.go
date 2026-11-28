package client

import (
	"context"
	"net/http"
	"net/url"

	"github.com/myco-ai/myco/internal/api"
)

func (c *Client) workflowRunView(ctx context.Context, id, view, step string) (*api.WorkflowRunView, error) {
	var out api.WorkflowRunView
	q := url.Values{"view": {view}}
	if step != "" {
		q.Set("step", step)
	}
	if err := c.do(ctx, http.MethodGet, "/v1/workflow-runs/"+esc(id), q, nil, &out); err != nil {
		return nil, err
	}
	if out.View != view {
		return nil, &Error{Code: "unsupported_by_hub", Message: "this Hub did not return the requested workflow run view", Suggestion: "upgrade the Hub before using --compact; the full run was not printed", Retryable: false}
	}
	if out.ID != id || (view == "step" && (len(out.Steps) != 1 || (out.Steps[0].ID != step && out.Steps[0].Step != step))) {
		return nil, &Error{Code: "invalid_response", Message: "Hub returned a mismatched workflow run view", Suggestion: "check the Hub version and requested run/step", Retryable: false}
	}
	if view == "compact" {
		for _, st := range out.Steps {
			if st.Output != nil {
				return nil, &Error{Code: "invalid_response", Message: "Hub included output in a compact workflow run view", Suggestion: "upgrade or repair the Hub projection", Retryable: false}
			}
		}
	}
	return &out, nil
}
func (c *Client) WorkflowRunCompact(ctx context.Context, id string) (*api.WorkflowRunView, error) {
	return c.workflowRunView(ctx, id, "compact", "")
}
func (c *Client) WorkflowRunStep(ctx context.Context, id, step string) (*api.WorkflowRunView, error) {
	return c.workflowRunView(ctx, id, "step", step)
}
