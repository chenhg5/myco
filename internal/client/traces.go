package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/chenhg5/myco/internal/api"
)

// PostTrace uploads steps of the run handling task id (its recipient).
func (c *Client) PostTrace(ctx context.Context, id string, events []api.TraceEvent) error {
	return c.do(ctx, http.MethodPost, "/v1/messages/"+esc(id)+"/trace", nil, api.TraceUpload{Events: events}, nil)
}

// Trace returns the steps of task id after seq (as an agent).
func (c *Client) Trace(ctx context.Context, id string, after int) (*api.Trace, error) {
	var out api.Trace
	return &out, c.do(ctx, http.MethodGet, "/v1/messages/"+esc(id)+"/trace", afterQuery(after), nil, &out)
}

// ConsoleTrace returns the steps of task id after seq (as a person in the app).
func (c *Client) ConsoleTrace(ctx context.Context, id string, after int) (*api.Trace, error) {
	var out api.Trace
	return &out, c.do(ctx, http.MethodGet, "/v1/console/messages/"+esc(id)+"/trace", afterQuery(after), nil, &out)
}

func afterQuery(after int) url.Values {
	if after <= 0 {
		return nil
	}
	return url.Values{"after": {strconv.Itoa(after)}}
}
