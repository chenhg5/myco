package client

import (
	"context"
	"net/http"
	"net/url"

	"github.com/myco-ai/myco/internal/api"
)

func (c *Client) InviteProjectAgent(ctx context.Context, name string, in api.ProjectInviteRequest, dry bool) (*api.ProjectInviteResponse, error) {
	var out api.ProjectInviteResponse
	var q url.Values
	if dry {
		q = url.Values{"dry_run": {"true"}}
	}
	err := c.do(ctx, http.MethodPost, "/v1/projects/"+url.PathEscape(name)+"/invite-agent", q, in, &out)
	return &out, err
}
func (c *Client) ProjectInvitation(ctx context.Context, name, id string, claim bool) (*api.ProjectInviteResponse, error) {
	var out api.ProjectInviteResponse
	method, path := http.MethodGet, "/v1/projects/"+url.PathEscape(name)+"/invite-agent/"+url.PathEscape(id)
	if claim {
		method = http.MethodPost
		path += "/claim"
	}
	err := c.do(ctx, method, path, nil, nil, &out)
	return &out, err
}
func (c *Client) SetProjectCoordinatorInvites(ctx context.Context, name string, allow bool) (*api.Project, error) {
	var out api.Project
	err := c.do(ctx, http.MethodPut, "/v1/projects/"+url.PathEscape(name)+"/agent-invites", nil, map[string]bool{"allowed": allow}, &out)
	return &out, err
}
