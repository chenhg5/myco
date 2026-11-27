package client

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/myco-ai/myco/internal/api"
)

func parentFieldUnsupported(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == http.StatusBadRequest && e.Code == "invalid_argument" && strings.Contains(e.Message, `unknown field "parent_id"`)
}
func messageCoordinationError(err error) error {
	var e *Error
	if errors.As(err, &e) && e.Code == "unsupported" {
		copy := *e
		copy.Retryable = false
		return &copy
	}
	if errors.As(err, &e) && (e.Status == http.StatusMethodNotAllowed || e.Status == http.StatusNotFound && e.Code != "not_found") {
		return &Error{Status: e.Status, Code: "unsupported", Message: "this Hub does not support message coordination", Suggestion: "upgrade the Hub to use message tree and reassign"}
	}
	return err
}
func (c *Client) MessageTree(ctx context.Context, id string, limit int) (*api.TaskTreeResponse, error) {
	var out api.TaskTreeResponse
	err := c.do(ctx, http.MethodGet, "/v1/messages/"+esc(id)+"/tree", url.Values{"limit": {strconv.Itoa(limit)}}, nil, &out)
	return &out, messageCoordinationError(err)
}
func (c *Client) ReassignMessage(ctx context.Context, id string, in api.ReassignRequest, dry bool) (*api.ReassignResponse, error) {
	var out api.ReassignResponse
	err := c.do(ctx, http.MethodPost, "/v1/messages/"+esc(id)+"/reassign", dryQuery(dry), in, &out)
	return &out, messageCoordinationError(err)
}
