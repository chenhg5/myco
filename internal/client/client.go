// Package client is the Go client for the myco hub HTTP API.
package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	return c.send(ctx, req, out)
}

func (c *Client) auth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("User-Agent", "myco-cli")
}

// send executes req and decodes a JSON response into out.
func (c *Client) send(ctx context.Context, req *http.Request, out any) error {
	c.auth(req)
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
		return decodeError(resp.StatusCode, data)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// decodeError turns a non-2xx response into *Error.
func decodeError(status int, data []byte) error {
	var ae api.Error
	if json.Unmarshal(data, &ae) != nil || ae.Code == "" {
		// Not a hub error body: typically a reverse proxy answering while
		// the hub restarts (502/503/504).
		code := "internal"
		switch status {
		case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			code = CodeUnavailable
		}
		msg := fmt.Sprintf("hub returned HTTP %d", status)
		if body := strings.TrimSpace(string(data)); body != "" {
			msg += ": " + truncate(body, 200)
		}
		return &Error{Status: status, Code: code, Message: msg, Retryable: status >= 500,
			Suggestion: "the hub or its proxy may be restarting; retry shortly"}
	}
	return &Error{Status: status, Code: ae.Code, Message: ae.Message, Suggestion: ae.Suggestion,
		Retryable: ae.Retryable || status >= 500}
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
func (c *Client) ListAgents(ctx context.Context, capability string, onlineOnly bool, selector ...string) ([]api.Agent, error) {
	q := url.Values{}
	if len(selector) > 0 && selector[0] != "" {
		q.Set("selector", selector[0])
	}
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
			// Ride out hub restarts and network blips until the deadline.
			if e, ok := AsError(err); ok && e.Retryable && time.Until(deadline) > 5*time.Second {
				select {
				case <-time.After(5 * time.Second):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return nil, err
		}
		if len(out.Pending) > 0 || time.Until(deadline) <= 0 {
			return out.Pending, nil
		}
	}
}

// CreateEnrollment issues an enrollment token (admin).
func (c *Client) CreateEnrollment(ctx context.Context, req api.CreateEnrollmentRequest, dry bool) (*api.CreateEnrollmentResponse, error) {
	var out api.CreateEnrollmentResponse
	return &out, c.do(ctx, http.MethodPost, "/v1/enrollments", dryQuery(dry), req, &out)
}

// ListEnrollments lists enrollments (admin).
func (c *Client) ListEnrollments(ctx context.Context) ([]api.Enrollment, error) {
	var out api.EnrollmentList
	return out.Enrollments, c.do(ctx, http.MethodGet, "/v1/enrollments", nil, nil, &out)
}

// RevokeEnrollment disables an enrollment (admin).
func (c *Client) RevokeEnrollment(ctx context.Context, id string, dry bool) (map[string]any, error) {
	var out map[string]any
	return out, c.do(ctx, http.MethodDelete, "/v1/enrollments/"+esc(id), dryQuery(dry), nil, &out)
}

// Enroll joins (or rejoins) the network as the calling session; the client
// must be built with an enrollment token.
func (c *Client) Enroll(ctx context.Context, req api.EnrollRequest, dry bool) (*api.RegisterResponse, error) {
	var out api.RegisterResponse
	return &out, c.do(ctx, http.MethodPost, "/v1/enroll", dryQuery(dry), req, &out)
}

// SetLabels changes labels of the caller (name == "") or, with an admin
// token, of agent name.
func (c *Client) SetLabels(ctx context.Context, name string, req api.LabelsRequest) (*api.Agent, error) {
	path := "/v1/me/labels"
	if name != "" {
		path = "/v1/agents/" + esc(name) + "/labels"
	}
	var out api.Agent
	return &out, c.do(ctx, http.MethodPut, path, nil, req, &out)
}

// Broadcast sends one message per agent matching req.Selector.
func (c *Client) Broadcast(ctx context.Context, req api.BroadcastRequest, dry bool) (*api.BroadcastResponse, error) {
	var out api.BroadcastResponse
	return &out, c.do(ctx, http.MethodPost, "/v1/broadcasts", dryQuery(dry), req, &out)
}

// BroadcastStatus returns deliveries and replies of a broadcast; consume
// marks the returned replies done so they leave the caller's inbox.
func (c *Client) BroadcastStatus(ctx context.Context, id string, consume bool) (*api.BroadcastStatus, error) {
	q := url.Values{}
	if consume {
		q.Set("consume", "true")
	}
	var out api.BroadcastStatus
	return &out, c.do(ctx, http.MethodGet, "/v1/broadcasts/"+esc(id), q, nil, &out)
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
	return c.ReplyWith(ctx, id, body, nil, dry)
}

// ReplyWith answers a message with attachments (file ids).
func (c *Client) ReplyWith(ctx context.Context, id, body string, attachments []string, dry bool) (*api.Message, error) {
	var out api.MessageResult
	return &out.Message, c.do(ctx, http.MethodPost, "/v1/messages/"+esc(id)+"/reply", dryQuery(dry),
		api.ReplyRequest{Body: body, Attachments: attachments}, &out)
}

// Release defers a message for redelivery after delay (no failed attempt).
func (c *Client) Release(ctx context.Context, id string, delay time.Duration, dry bool) (*api.Message, error) {
	var out api.MessageResult
	return &out.Message, c.do(ctx, http.MethodPost, "/v1/messages/"+esc(id)+"/release", dryQuery(dry),
		api.ReleaseRequest{DelaySeconds: int(delay.Seconds())}, &out)
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

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// UploadFile streams a local file to the hub and returns its metadata.
func (c *Client) UploadFile(ctx context.Context, path string) (*api.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return nil, &Error{Code: "invalid_argument", Message: path + " is a directory", Suggestion: "attach files (tar/zip directories first)"}
	}
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/files"
	u.RawQuery = url.Values{"name": {filepath.Base(path)}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), f)
	if err != nil {
		return nil, err
	}
	req.ContentLength = st.Size()
	ct := mime.TypeByExtension(filepath.Ext(path))
	if ct == "" {
		ct = "application/octet-stream"
	}
	req.Header.Set("Content-Type", ct)
	var out api.File
	return &out, c.send(ctx, req, &out)
}

// DownloadFile saves file id to dest (a directory or a file path) and
// verifies its SHA-256. It returns the metadata and the written path.
func (c *Client) DownloadFile(ctx context.Context, id, dest string) (*api.File, string, error) {
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/files/" + esc(id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", &Error{Code: CodeUnavailable, Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, "", decodeError(resp.StatusCode, data)
	}
	name, _ := url.PathUnescape(resp.Header.Get("X-Myco-File-Name"))
	name = filepath.Base(name)
	if name == "" || name == "." || name == "/" {
		name = id
	}
	path := dest
	if st, err := os.Stat(dest); err == nil && st.IsDir() {
		path = filepath.Join(dest, name)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".myco-download-*")
	if err != nil {
		return nil, "", err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), resp.Body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, "", err
	}
	want := resp.Header.Get("X-Myco-SHA256")
	got := hex.EncodeToString(h.Sum(nil))
	if want != "" && want != got {
		return nil, "", &Error{Code: "integrity_error", Message: fmt.Sprintf("checksum mismatch for %s", id), Retryable: true}
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return nil, "", err
	}
	return &api.File{ID: id, Name: name, Size: n, SHA256: got, ContentType: resp.Header.Get("Content-Type")}, path, nil
}
