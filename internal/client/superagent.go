package client

import (
	"context"
	"net/http"
	"net/url"

	"github.com/chenhg5/myco/internal/api"
)

// Superagents (hubs from v0.21.1).

// Superagent returns the caller's superagent (Agent nil when none).
func (c *Client) Superagent(ctx context.Context) (*api.Superagent, error) {
	var out api.Superagent
	if err := c.do(ctx, http.MethodGet, "/v1/console/superagent", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetSuperagent names one of the caller's agents their superagent.
func (c *Client) SetSuperagent(ctx context.Context, agent string, dry bool) (*api.Superagent, error) {
	var out api.Superagent
	if err := c.do(ctx, http.MethodPut, "/v1/console/superagent", dryQuery(dry), api.SuperagentSet{Agent: agent}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ClearSuperagent makes the caller's superagent an ordinary agent again.
func (c *Client) ClearSuperagent(ctx context.Context) error {
	return c.do(ctx, http.MethodDelete, "/v1/console/superagent", nil, nil, nil)
}

// Approvals lists the caller's approvals (status "pending", or "" for all).
func (c *Client) Approvals(ctx context.Context, status string) ([]api.Approval, error) {
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	var out api.ApprovalList
	if err := c.do(ctx, http.MethodGet, "/v1/console/approvals", q, nil, &out); err != nil {
		return nil, err
	}
	return out.Approvals, nil
}

// DecideApproval approves (carrying it out) or denies a superagent's request.
func (c *Client) DecideApproval(ctx context.Context, id string, approve bool) (*api.Approval, error) {
	action := "deny"
	if approve {
		action = "approve"
	}
	var out api.Approval
	if err := c.do(ctx, http.MethodPost, "/v1/console/approvals/"+esc(id)+"/"+action, nil, struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MeSuperagent says whether the calling agent is a superagent, and whose.
func (c *Client) MeSuperagent(ctx context.Context) (*api.MeSuperagent, error) {
	var out api.MeSuperagent
	if err := c.do(ctx, http.MethodGet, "/v1/me/superagent", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MyApprovals lists what the calling agent asked its owner for.
func (c *Client) MyApprovals(ctx context.Context) ([]api.Approval, error) {
	var out api.ApprovalList
	if err := c.do(ctx, http.MethodGet, "/v1/me/approvals", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Approvals, nil
}
