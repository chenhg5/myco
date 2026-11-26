package client

import (
	"context"
	"github.com/myco-ai/myco/internal/api"
	"net/http"
	"net/url"
)

func (c *Client) CreateGoal(ctx context.Context, in api.GoalCreateRequest, dry bool) (*api.Goal, error) {
	var o api.Goal
	e := c.do(ctx, http.MethodPost, "/v1/goals", dryQuery(dry), in, &o)
	return &o, e
}
func (c *Client) Goals(ctx context.Context, agent, project string, all bool) ([]api.Goal, error) {
	q := url.Values{}
	if agent != "" {
		q.Set("agent", agent)
	}
	if project != "" {
		q.Set("project", project)
	}
	if all {
		q.Set("state", "all")
	}
	var o api.GoalList
	e := c.do(ctx, http.MethodGet, "/v1/goals", q, nil, &o)
	return o.Goals, e
}
func (c *Client) Goal(ctx context.Context, id string) (*api.GoalDetail, error) {
	var o api.GoalDetail
	e := c.do(ctx, http.MethodGet, "/v1/goals/"+esc(id), nil, nil, &o)
	return &o, e
}
func (c *Client) goalControl(ctx context.Context, id, action, reason string, dry bool) (*api.Goal, error) {
	var o api.Goal
	e := c.do(ctx, http.MethodPost, "/v1/goals/"+esc(id)+"/"+action, dryQuery(dry), api.GoalControlRequest{Reason: reason}, &o)
	return &o, e
}
func (c *Client) PauseGoal(ctx context.Context, id, reason string, dry bool) (*api.Goal, error) {
	return c.goalControl(ctx, id, "pause", reason, dry)
}
func (c *Client) ResumeGoal(ctx context.Context, id, reason string, dry bool) (*api.Goal, error) {
	return c.goalControl(ctx, id, "resume", reason, dry)
}
func (c *Client) StopGoal(ctx context.Context, id, reason string, dry bool) (*api.Goal, error) {
	return c.goalControl(ctx, id, "stop", reason, dry)
}
func (c *Client) UpdateGoal(ctx context.Context, id string, in api.GoalUpdateRequest, dry bool) (*api.Goal, error) {
	var o api.Goal
	e := c.do(ctx, http.MethodPatch, "/v1/goals/"+esc(id), dryQuery(dry), in, &o)
	return &o, e
}
func (c *Client) ReportGoal(ctx context.Context, id string, in api.GoalReportRequest, dry bool) (*api.Goal, error) {
	var o api.Goal
	e := c.do(ctx, http.MethodPost, "/v1/goals/"+esc(id)+"/rounds", dryQuery(dry), in, &o)
	return &o, e
}
