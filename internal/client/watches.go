package client

import (
	"context"
	"github.com/myco-ai/myco/internal/api"
	"net/http"
	"net/url"
)

func (c *Client) StartWatch(ctx context.Context, in api.WatchStartRequest, dry bool) (*api.Watch, error) {
	var out api.Watch
	err := c.do(ctx, http.MethodPost, "/v1/watches", dryQuery(dry), in, &out)
	return &out, err
}
func (c *Client) Watches(ctx context.Context, agent, project string, all bool) ([]api.Watch, error) {
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
	var out api.WatchList
	err := c.do(ctx, http.MethodGet, "/v1/watches", q, nil, &out)
	return out.Watches, err
}
func (c *Client) Watch(ctx context.Context, id string) (*api.Watch, error) {
	var out api.Watch
	err := c.do(ctx, http.MethodGet, "/v1/watches/"+esc(id), nil, nil, &out)
	return &out, err
}
func (c *Client) StopWatch(ctx context.Context, id, reason string, dry bool) (*api.Watch, error) {
	var out api.Watch
	err := c.do(ctx, http.MethodPost, "/v1/watches/"+esc(id)+"/stop", dryQuery(dry), api.WatchStopRequest{Reason: reason}, &out)
	return &out, err
}
func (c *Client) ExtendWatch(ctx context.Context, id string, seconds int, dry bool) (*api.Watch, error) {
	var out api.Watch
	err := c.do(ctx, http.MethodPost, "/v1/watches/"+esc(id)+"/extend", dryQuery(dry), api.WatchExtendRequest{ForSec: seconds}, &out)
	return &out, err
}

func (c *Client) ClaimWatch(ctx context.Context, id, runner string) (*api.Watch, error) {
	var out api.Watch
	err := c.do(ctx, http.MethodPost, "/v1/watches/"+esc(id)+"/claim", nil, api.WatchClaimRequest{Runner: runner}, &out)
	return &out, err
}
func (c *Client) CheckWatch(ctx context.Context, id string, in api.WatchCheckRequest) (*api.Watch, error) {
	var out api.Watch
	err := c.do(ctx, http.MethodPost, "/v1/watches/"+esc(id)+"/checks", nil, in, &out)
	return &out, err
}
