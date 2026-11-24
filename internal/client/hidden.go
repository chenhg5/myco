package client

import (
	"context"
	"net/http"

	"github.com/myco-ai/myco/internal/api"
)

// Hidden lists the messages the caller (a person, by console token) took
// out of their own view.
func (c *Client) Hidden(ctx context.Context) (*api.HiddenList, error) {
	var out api.HiddenList
	return &out, c.do(ctx, http.MethodGet, "/v1/console/hidden", nil, nil, &out)
}

// VoiceSettings reads what the caller (a person, by console token) set
// for voice chats, keys included. A hub from before this answers 404.
func (c *Client) VoiceSettings(ctx context.Context) (*api.VoiceSettings, error) {
	var out api.VoiceSettings
	return &out, c.do(ctx, http.MethodGet, "/v1/console/voice", nil, nil, &out)
}

// PutVoiceSettings keeps the caller's voice settings on the hub, unless
// it holds newer ones; the answer is what it holds then.
func (c *Client) PutVoiceSettings(ctx context.Context, in api.VoiceSettings) (*api.VoiceSettings, error) {
	var out api.VoiceSettings
	return &out, c.do(ctx, http.MethodPut, "/v1/console/voice", nil, in, &out)
}

// UpdateHidden hides and shows messages in the caller's own view.
func (c *Client) UpdateHidden(ctx context.Context, in api.HiddenUpdate) (*api.HiddenList, error) {
	var out api.HiddenList
	return &out, c.do(ctx, http.MethodPost, "/v1/console/hidden", nil, in, &out)
}
