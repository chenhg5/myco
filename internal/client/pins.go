package client

import (
	"context"
	"net/http"
	"net/url"

	"github.com/chenhg5/myco/internal/api"
)

// Pins lists what the conversation with `with` keeps at hand (from
// v0.20.2). a is the caller's side ("" for the caller itself).
func (c *Client) Pins(ctx context.Context, a, with string) (*api.PinList, error) {
	q := url.Values{"with": {with}}
	if a != "" {
		q.Set("a", a)
	}
	var out api.PinList
	return &out, c.do(ctx, http.MethodGet, "/v1/pins", q, nil, &out)
}

// AddPin pins a link, a file or a message to a conversation.
func (c *Client) AddPin(ctx context.Context, in api.PinCreate) (*api.Pin, error) {
	var out api.Pin
	return &out, c.do(ctx, http.MethodPost, "/v1/pins", nil, in, &out)
}

// RenamePin gives a pin another title.
func (c *Client) RenamePin(ctx context.Context, id string, in api.PinUpdate) (*api.Pin, error) {
	var out api.Pin
	return &out, c.do(ctx, http.MethodPatch, "/v1/pins/"+esc(id), nil, in, &out)
}

// RemovePin unpins one.
func (c *Client) RemovePin(ctx context.Context, a, with, id string) error {
	q := url.Values{"with": {with}}
	if a != "" {
		q.Set("a", a)
	}
	return c.do(ctx, http.MethodDelete, "/v1/pins/"+esc(id), q, nil, nil)
}

// OrderPins puts a conversation's pins in this order.
func (c *Client) OrderPins(ctx context.Context, in api.PinOrder) (*api.PinList, error) {
	var out api.PinList
	return &out, c.do(ctx, http.MethodPut, "/v1/pins/order", nil, in, &out)
}

// FileMeta is a file on the hub the agent may read (its name and type).
func (c *Client) FileMeta(ctx context.Context, id string) (*api.File, error) {
	var out api.File
	return &out, c.do(ctx, http.MethodGet, "/v1/files/"+esc(id)+"/meta", nil, nil, &out)
}
