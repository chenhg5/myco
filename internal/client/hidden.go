package client

import (
	"context"
	"net/http"

	"github.com/chenhg5/myco/internal/api"
)

// Hidden lists the messages the caller (a person, by console token) took
// out of their own view.
func (c *Client) Hidden(ctx context.Context) (*api.HiddenList, error) {
	var out api.HiddenList
	return &out, c.do(ctx, http.MethodGet, "/v1/console/hidden", nil, nil, &out)
}

// UpdateHidden hides and shows messages in the caller's own view.
func (c *Client) UpdateHidden(ctx context.Context, in api.HiddenUpdate) (*api.HiddenList, error) {
	var out api.HiddenList
	return &out, c.do(ctx, http.MethodPost, "/v1/console/hidden", nil, in, &out)
}
