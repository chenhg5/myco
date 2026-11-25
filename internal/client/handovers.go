package client

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

// CreateHandoverLink makes a one-time take-over link for agent name (its
// owner or a hub manager).
func (c *Client) CreateHandoverLink(ctx context.Context, name string, req api.HandoverLinkRequest, dry bool) (*api.HandoverLink, error) {
	var out api.HandoverLink
	return &out, c.do(ctx, http.MethodPost, "/v1/agents/"+esc(name)+"/handover-link", dryQuery(dry), req, &out)
}

// HandOver has a machine start a session that takes agent name over.
func (c *Client) HandOver(ctx context.Context, name string, req api.HandoverRequest, wait time.Duration, dry bool) (*api.HandoverResult, error) {
	q := dryQuery(dry)
	if wait > 0 {
		q.Set("wait_seconds", strconv.Itoa(int(wait.Seconds())))
	}
	var out api.HandoverResult
	return &out, c.do(ctx, http.MethodPost, "/v1/agents/"+esc(name)+"/handover", q, req, &out)
}

// Handovers lists agent name's handovers, newest first.
func (c *Client) Handovers(ctx context.Context, name string) ([]api.Handover, error) {
	var out api.HandoverList
	if err := c.do(ctx, http.MethodGet, "/v1/agents/"+esc(name)+"/handovers", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Handovers, nil
}

// MyHandover returns the caller's latest handover and its note.
func (c *Client) MyHandover(ctx context.Context) (*api.HandoverBrief, error) {
	var out api.HandoverBrief
	return &out, c.do(ctx, http.MethodGet, "/v1/me/handover", nil, nil, &out)
}

// TakeOver binds this session to the agent of a take-over link (no
// credential needed: the link token is one).
func (c *Client) TakeOver(ctx context.Context, req api.TakeOverRequest) (*api.TakeOverResponse, error) {
	var out api.TakeOverResponse
	return &out, c.do(ctx, http.MethodPost, "/v1/takeover", nil, req, &out)
}
