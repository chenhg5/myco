package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/myco-ai/myco/internal/api"
)

type AgentStatsFilter struct{ Name, Project, Since string }
type AgentPickFilter struct {
	Skill, CLI, Project, Machine string
	Count                        int
	Strict                       bool
}

func coordinationError(err error, version int) error {
	if err != nil {
		if e, ok := AsError(err); ok && e.Code == "unsupported" {
			copy := *e
			copy.Retryable = false
			return &copy
		}
		e, ok := AsError(err)
		missingRoute := ok && (e.Status == 404 || e.Status == 405) && (e.Code == "internal" || (e.Code == "not_found" && strings.HasPrefix(e.Message, "no route for GET /v1/stats/")))
		if !missingRoute {
			return err
		}
	}
	if err != nil || version != 1 {
		return &Error{Status: 501, Code: "unsupported", Message: "this Hub does not support coordination statistics version 1", Suggestion: "upgrade the Hub to use agent pick/stats and stats coordinator"}
	}
	return nil
}
func (c *Client) AgentStats(ctx context.Context, f AgentStatsFilter) (*api.AgentStatsList, error) {
	q := url.Values{"name": {f.Name}, "project": {f.Project}, "since": {f.Since}}
	var out api.AgentStatsList
	err := c.do(ctx, http.MethodGet, "/v1/stats/agents", q, nil, &out)
	return &out, coordinationError(err, out.Version)
}
func (c *Client) PickAgents(ctx context.Context, f AgentPickFilter) (*api.AgentPickList, error) {
	q := url.Values{"skill": {f.Skill}, "cli": {f.CLI}, "project": {f.Project}, "machine": {f.Machine}, "strict": {strconv.FormatBool(f.Strict)}}
	if f.Count != 0 {
		q.Set("count", strconv.Itoa(f.Count))
	}
	var out api.AgentPickList
	err := c.do(ctx, http.MethodGet, "/v1/stats/agent-pick", q, nil, &out)
	return &out, coordinationError(err, out.Version)
}
func (c *Client) CoordinatorStats(ctx context.Context, name, since string) (*api.CoordinatorStats, error) {
	var out api.CoordinatorStats
	err := c.do(ctx, http.MethodGet, "/v1/stats/coordinator", url.Values{"name": {name}, "since": {since}}, nil, &out)
	return &out, coordinationError(err, out.Version)
}
