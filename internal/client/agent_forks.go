package client

import (
	"context"
	"net/http"

	"github.com/myco-ai/myco/internal/api"
)

func forkCompatibility(err error) error {
	if e, ok := AsError(err); ok && ((e.Status == http.StatusNotFound && e.Code == "internal") || e.Status == http.StatusMethodNotAllowed) {
		return &Error{Status: e.Status, Code: "unsupported", Message: "this Hub does not support managed agent fork/retire", Suggestion: "upgrade the Hub and machine runner", Retryable: false}
	}
	if e, ok := AsError(err); ok && e.Code == "unsupported" {
		copy := *e
		copy.Retryable = false
		return &copy
	}
	return err
}
func (c *Client) ForkAgents(ctx context.Context, in api.AgentForkRequest, dry bool) (*api.AgentForkResult, error) {
	var out api.AgentForkResult
	err := c.do(ctx, http.MethodPost, "/v1/agents/fork", dryQuery(dry), in, &out)
	return &out, forkCompatibility(err)
}
func (c *Client) RetireAgents(ctx context.Context, in api.AgentRetireRequest, dry bool) (*api.AgentForkResult, error) {
	var out api.AgentForkResult
	err := c.do(ctx, http.MethodPost, "/v1/agents/retire", dryQuery(dry), in, &out)
	return &out, forkCompatibility(err)
}
func (c *Client) AgentFork(ctx context.Context, id string) (*api.AgentFork, error) {
	var out api.AgentFork
	err := c.do(ctx, http.MethodGet, "/v1/agent-forks/"+esc(id), nil, nil, &out)
	return &out, forkCompatibility(err)
}
func (c *Client) MachineForkPlan(ctx context.Context, id string) (*api.AgentFork, error) {
	var out api.AgentFork
	err := c.do(ctx, http.MethodGet, "/v1/me/agent-forks/"+esc(id), nil, nil, &out)
	return &out, forkCompatibility(err)
}
func (c *Client) CompleteAgentFork(ctx context.Context, id string, in api.AgentForkCompletion) (*api.AgentFork, error) {
	var out api.AgentFork
	err := c.do(ctx, http.MethodPost, "/v1/me/agent-forks/"+esc(id), nil, in, &out)
	return &out, forkCompatibility(err)
}
