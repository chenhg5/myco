// Package client is the Go client for the myco hub HTTP API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
)

// Error is a structured API or transport error.
type Error struct {
	Status     int    // HTTP status; 0 for transport errors
	Code       string // api error code, or "unavailable"/"timeout"
	Message    string
	Suggestion string
	Retryable  bool
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Transport-level error codes (complementing the hub's domain codes).
const (
	CodeUnavailable = "unavailable"
	CodeTimeout     = "timeout"
)

// Client talks to one hub with one credential (agent or admin token).
type Client struct {
	base  *url.URL
	token string
	http  *http.Client
}

// New returns a client for hub base URL authenticated with token.
func New(base, token string) (*Client, error) {
	u, err := ValidateHubURL(base)
	if err != nil {
		return nil, err
	}
	// No global timeout: long-polls are bounded server-side and by ctx.
	return &Client{base: u, token: token, http: &http.Client{}}, nil
}

// ValidateHubURL accepts only http(s) URLs without embedded credentials.
func ValidateHubURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, &Error{Code: "invalid_argument", Message: fmt.Sprintf("invalid hub URL %q", raw),
			Suggestion: "use an http(s) URL such as https://hub.example.com"}
	}
	if u.User != nil {
		return nil, &Error{Code: "invalid_argument", Message: "hub URL must not embed credentials",
			Suggestion: "pass tokens via MYCO_TOKEN / MYCO_ADMIN_TOKEN or the config file"}
	}
	return u, nil
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, in, out any) error {
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawQuery = query.Encode()
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("User-Agent", "myco-cli")
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &Error{Code: CodeUnavailable, Message: fmt.Sprintf("cannot reach hub %s: %v", c.base, err),
			Suggestion: "check the hub URL and network; this error is retryable", Retryable: true}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return &Error{Code: CodeUnavailable, Message: err.Error(), Retryable: true}
	}
	if resp.StatusCode >= 300 {
		var ae api.Error
		if json.Unmarshal(data, &ae) != nil || ae.Code == "" {
			return &Error{Status: resp.StatusCode, Code: "internal", Message: strings.TrimSpace(string(data)),
				Retryable: resp.StatusCode >= 500}
		}
		return &Error{Status: resp.StatusCode, Code: ae.Code, Message: ae.Message, Suggestion: ae.Suggestion,
			Retryable: ae.Retryable || resp.StatusCode >= 500}
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

func dryQuery(dry bool) url.Values {
	q := url.Values{}
	if dry {
		q.Set("dry_run", "true")
	}
	return q
}

func esc(s string) string { return url.PathEscape(s) }

// Health checks hub liveness.
func (c *Client) Health(ctx context.Context) (*api.Health, error) {
	var out api.Health
	return &out, c.do(ctx, http.MethodGet, "/v1/health", nil, nil, &out)
}

// Register creates (or with RotateToken, re-keys) an agent. Admin only.
func (c *Client) Register(ctx context.Context, req api.RegisterRequest, dry bool) (*api.RegisterResponse, error) {
	var out api.RegisterResponse
	return &out, c.do(ctx, http.MethodPost, "/v1/agents", dryQuery(dry), req, &out)
}

// RemoveAgent deletes an agent. Admin only.
func (c *Client) RemoveAgent(ctx context.Context, name string, dry bool) (map[string]any, error) {
	var out map[string]any
	return out, c.do(ctx, http.MethodDelete, "/v1/agents/"+esc(name), dryQuery(dry), nil, &out)
}

// ListAgents lists agents, optionally filtered.
func (c *Client) ListAgents(ctx context.Context, capability string, onlineOnly bool) ([]api.Agent, error) {
	q := url.Values{}
	if capability != "" {
		q.Set("capability", capability)
	}
	if onlineOnly {
		q.Set("online", "true")
	}
	var out api.AgentList
	return out.Agents, c.do(ctx, http.MethodGet, "/v1/agents", q, nil, &out)
}

// GetAgent returns one agent.
func (c *Client) GetAgent(ctx context.Context, name string) (*api.Agent, error) {
	var out api.Agent
	return &out, c.do(ctx, http.MethodGet, "/v1/agents/"+esc(name), nil, nil, &out)
}

// Me returns the authenticated agent.
func (c *Client) Me(ctx context.Context) (*api.Agent, error) {
	var out api.Agent
	return &out, c.do(ctx, http.MethodGet, "/v1/me", nil, nil, &out)
}

// Bind updates the calling agent's session binding.
func (c *Client) Bind(ctx context.Context, b api.Binding) (*api.Agent, error) {
	var out api.Agent
	return &out, c.do(ctx, http.MethodPut, "/v1/me/binding", nil, b, &out)
}

// WaitPending long-polls until the inbox has deliverable messages (without
// claiming them), re-polling until total elapses. Returns nil on timeout.
func (c *Client) WaitPending(ctx context.Context, total time.Duration) ([]api.Message, error) {
	deadline := time.Now().Add(total)
	for {
		secs := max(int(time.Until(deadline).Seconds()), 0)
		q := url.Values{"wait_seconds": {strconv.Itoa(min(secs, 60))}}
		var out api.PendingResponse
		if err := c.do(ctx, http.MethodGet, "/v1/inbox/wait", q, nil, &out); err != nil {
			return nil, err
		}
		if len(out.Pending) > 0 || time.Until(deadline) <= 0 {
			return out.Pending, nil
		}
	}
}

// Send queues a message.
func (c *Client) Send(ctx context.Context, req api.SendRequest, dry bool) (*api.SendResponse, error) {
	var out api.SendResponse
	return &out, c.do(ctx, http.MethodPost, "/v1/messages", dryQuery(dry), req, &out)
}

// GetMessage returns a message visible to the caller.
func (c *Client) GetMessage(ctx context.Context, id string) (*api.Message, error) {
	var out api.Message
	return &out, c.do(ctx, http.MethodGet, "/v1/messages/"+esc(id), nil, nil, &out)
}

// Transition applies "ack", "hold" or "done".
func (c *Client) Transition(ctx context.Context, id, verb string, dry bool) (*api.Message, error) {
	var out api.MessageResult
	return &out.Message, c.do(ctx, http.MethodPost, "/v1/messages/"+esc(id)+"/"+verb, dryQuery(dry), nil, &out)
}

// Ack acknowledges a pulled message.
func (c *Client) Ack(ctx context.Context, id string) error {
	_, err := c.Transition(ctx, id, "ack", false)
	return err
}

// Complete closes a message without replying.
func (c *Client) Complete(ctx context.Context, id string) error {
	_, err := c.Transition(ctx, id, "done", false)
	return err
}

// Reply answers a message and returns the reply.
func (c *Client) Reply(ctx context.Context, id, body string, dry bool) (*api.Message, error) {
	var out api.MessageResult
	return &out.Message, c.do(ctx, http.MethodPost, "/v1/messages/"+esc(id)+"/reply", dryQuery(dry), api.ReplyRequest{Body: body}, &out)
}

// Fail closes a message as failed.
func (c *Client) Fail(ctx context.Context, id, reason string, dry bool) (*api.Message, error) {
	var out api.MessageResult
	return &out.Message, c.do(ctx, http.MethodPost, "/v1/messages/"+esc(id)+"/fail", dryQuery(dry), api.FailRequest{Reason: reason}, &out)
}

// WaitReply long-polls for the reply to a sent message, re-polling until
// total elapses (the hub caps each poll). Returns (nil, nil) on timeout.
func (c *Client) WaitReply(ctx context.Context, id string, total time.Duration) (*api.Message, error) {
	deadline := time.Now().Add(total)
	for {
		remaining := time.Until(deadline)
		secs := int(remaining.Seconds())
		if secs < 0 {
			secs = 0
		}
		q := url.Values{"wait_seconds": {strconv.Itoa(min(secs, 60))}}
		var out api.WaitResponse
		if err := c.do(ctx, http.MethodGet, "/v1/messages/"+esc(id)+"/reply", q, nil, &out); err != nil {
			return nil, err
		}
		if out.Reply != nil {
			return out.Reply, nil
		}
		if time.Until(deadline) <= 0 {
			return nil, nil
		}
	}
}

// Pull leases up to limit messages, long-polling up to wait.
func (c *Client) Pull(ctx context.Context, limit int, wait time.Duration) ([]api.Message, error) {
	var out api.MessageList
	err := c.do(ctx, http.MethodPost, "/v1/inbox/pull", nil, api.PullRequest{Limit: limit, WaitSeconds: int(wait.Seconds())}, &out)
	return out.Messages, err
}

// Thread returns all messages of a thread.
func (c *Client) Thread(ctx context.Context, id string) ([]api.Message, error) {
	var out api.MessageList
	return out.Messages, c.do(ctx, http.MethodGet, "/v1/threads/"+esc(id), nil, nil, &out)
}

// AsError extracts a *Error from err.
func AsError(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
