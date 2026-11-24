package client

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/chenhg5/myco/internal/api"
)

// PostUsage returns nil,nil for an old Hub's 404. Never downgrade to trace.
func (c *Client) PostUsage(ctx context.Context, req api.UsageRequest) (*api.UsageRecorded, error) {
	var out api.UsageRecorded
	if err := c.do(ctx, http.MethodPost, "/v1/me/usage", nil, req, &out); err != nil {
		var ce *Error
		if errors.As(err, &ce) && ce.Status == 404 {
			return nil, nil
		}
		return nil, err
	}
	return &out, nil
}

type UsageFilter struct{ From, To, Project string }

func (c *Client) Usage(ctx context.Context, f UsageFilter) (*api.UsageList, error) {
	var out api.UsageList
	q := url.Values{}
	if f.From != "" {
		q.Set("from", f.From)
	}
	if f.To != "" {
		q.Set("to", f.To)
	}
	if f.Project != "" {
		q.Set("project", f.Project)
	}
	if err := c.do(ctx, http.MethodGet, "/v1/console/usage", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
