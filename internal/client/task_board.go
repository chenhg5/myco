package client

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

func taskBoardError(err error) error {
	if e, ok := AsError(err); ok && (e.Status == http.StatusNotFound || e.Status == http.StatusMethodNotAllowed) && e.Code == "internal" {
		return &Error{Status: http.StatusNotImplemented, Code: "unsupported", Message: "this Hub does not provide task board and alert settings", Suggestion: "upgrade the workspace Hub; old inbox APIs cannot enumerate sent tasks safely"}
	}
	return err
}
func (c *Client) TaskBoard(ctx context.Context, project string, all bool, stale time.Duration) (*api.TaskBoard, error) {
	q := url.Values{}
	if project != "" {
		q.Set("project", project)
	}
	if all {
		q.Set("all", "true")
	}
	if stale != 0 {
		q.Set("stale", stale.String())
	}
	var out api.TaskBoard
	err := c.do(ctx, http.MethodGet, "/v1/tasks/board", q, nil, &out)
	return &out, taskBoardError(err)
}
func (c *Client) TaskAlertPolicy(ctx context.Context, project string, set *api.TaskAlertPolicyRequest, dry bool) (*api.TaskAlertPolicy, error) {
	method := http.MethodGet
	var in any
	if set != nil {
		method = http.MethodPut
		in = set
	}
	q := url.Values{}
	if dry {
		q.Set("dry_run", "true")
	}
	var out api.TaskAlertPolicy
	err := c.do(ctx, method, "/v1/projects/"+esc(project)+"/task-alerts", q, in, &out)
	return &out, taskBoardError(err)
}
