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
	// Via marks this client's pulls (e.g. "runner"; see api.PullRequest.Via).
	Via string
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
			if err := RideOut(ctx, err, deadline); err != nil {
				return nil, err
			}
			continue
		}
		if len(out.Pending) > 0 || time.Until(deadline) <= 0 {
			return out.Pending, nil
		}
	}
}

// retryPause is how long long-poll loops back off after a retryable error.
var retryPause = 5 * time.Second

// RideOut lets long-poll loops survive hub restarts and network blips
// (proxy resets, 502s) until their deadline: it returns nil after a short
// pause when err is retryable and time remains, otherwise the error.
func RideOut(ctx context.Context, err error, deadline time.Time) error {
	if e, ok := AsError(err); !ok || !e.Retryable || time.Until(deadline) <= retryPause {
		return err
	}
	select {
	case <-time.After(retryPause):
		return nil
	case <-ctx.Done():
		return ctx.Err()
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

// Overview returns the network snapshot visible to this client's token.
func (c *Client) Overview(ctx context.Context, recent int) (*api.Overview, error) {
	var out api.Overview
	return &out, c.do(ctx, http.MethodGet, "/v1/overview", url.Values{"recent": {strconv.Itoa(recent)}}, nil, &out)
}

// UpdateProfile changes display name/description of the caller (name "")
// or, with an admin token, of agent name.
func (c *Client) UpdateProfile(ctx context.Context, name string, req api.ProfileRequest) (*api.Agent, error) {
	path := "/v1/me/profile"
	if name != "" {
		path = "/v1/agents/" + esc(name) + "/profile"
	}
	var out api.Agent
	return &out, c.do(ctx, http.MethodPut, path, nil, req, &out)
}

// ListAliases lists aliases.
func (c *Client) ListAliases(ctx context.Context) ([]api.Alias, error) {
	var out api.AliasList
	return out.Aliases, c.do(ctx, http.MethodGet, "/v1/aliases", nil, nil, &out)
}

// SetAlias creates or re-points an alias (admin).
func (c *Client) SetAlias(ctx context.Context, alias, target string, dry bool) (map[string]any, error) {
	var out map[string]any
	return out, c.do(ctx, http.MethodPut, "/v1/aliases/"+esc(alias), dryQuery(dry), api.AliasRequest{Target: target}, &out)
}

// RemoveAlias deletes an alias (admin).
func (c *Client) RemoveAlias(ctx context.Context, alias string, dry bool) (map[string]any, error) {
	var out map[string]any
	return out, c.do(ctx, http.MethodDelete, "/v1/aliases/"+esc(alias), dryQuery(dry), nil, &out)
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

// ReleaseInUse is Release for a message the agent's session could not take
// because someone else is using it; the hub shows this in the console.
func (c *Client) ReleaseInUse(ctx context.Context, id string, delay time.Duration) (*api.Message, error) {
	var out api.MessageResult
	return &out.Message, c.do(ctx, http.MethodPost, "/v1/messages/"+esc(id)+"/release", nil,
		api.ReleaseRequest{DelaySeconds: int(delay.Seconds()), Reason: "in_use"}, &out)
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
			if err := RideOut(ctx, err, deadline); err != nil {
				return nil, err
			}
			continue
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
	return c.PullTypes(ctx, limit, wait, nil)
}

// PullTypes is Pull restricted to message types (nil = all).
func (c *Client) PullTypes(ctx context.Context, limit int, wait time.Duration, types []string) ([]api.Message, error) {
	var out api.MessageList
	err := c.do(ctx, http.MethodPost, "/v1/inbox/pull", nil, api.PullRequest{Limit: limit, WaitSeconds: int(wait.Seconds()), Types: types, Via: c.Via}, &out)
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

// Join registers this machine through a join link (c must carry the link
// token) and returns the machine's own enrollment token.
func (c *Client) Join(ctx context.Context, machine string, dry bool) (*api.JoinResponse, error) {
	var out api.JoinResponse
	return &out, c.do(ctx, http.MethodPost, "/v1/join", dryQuery(dry), api.JoinRequest{Machine: machine}, &out)
}

// SetStatus publishes the caller's public status line ("" clears it).
func (c *Client) SetStatus(ctx context.Context, text string) (*api.Agent, error) {
	var out api.Agent
	return &out, c.do(ctx, http.MethodPut, "/v1/me/status", nil, api.StatusRequest{Text: text}, &out)
}

// Progress posts an interim update on task id (as its recipient).
func (c *Client) Progress(ctx context.Context, id, body string, dry bool) (*api.Message, error) {
	var out api.MessageResult
	return &out.Message, c.do(ctx, http.MethodPost, "/v1/messages/"+esc(id)+"/progress", dryQuery(dry), api.ReplyRequest{Body: body}, &out)
}

// Steer sends a correction for open task id (as its sender).
func (c *Client) Steer(ctx context.Context, id, body string, dry bool) (*api.Message, error) {
	var out api.MessageResult
	return &out.Message, c.do(ctx, http.MethodPost, "/v1/messages/"+esc(id)+"/steer", dryQuery(dry), api.ReplyRequest{Body: body}, &out)
}

// WaitReplyProgress waits like WaitReply and calls onProgress for each of the
// task's progress updates (including ones posted before the wait) as they
// arrive. It returns the reply, or nil at the deadline.
func (c *Client) WaitReplyProgress(ctx context.Context, id string, total time.Duration, onProgress func(api.Message)) (*api.Message, error) {
	deadline := time.Now().Add(total)
	since := "0"
	for {
		secs := max(int(time.Until(deadline).Seconds()), 0)
		q := url.Values{"wait_seconds": {strconv.Itoa(min(secs, 60))}, "progress_since": {since}}
		var out api.WaitResponse
		if err := c.do(ctx, http.MethodGet, "/v1/messages/"+esc(id)+"/reply", q, nil, &out); err != nil {
			if err := RideOut(ctx, err, deadline); err != nil {
				return nil, err
			}
			continue
		}
		for _, p := range out.Progress {
			if onProgress != nil {
				onProgress(p)
			}
			since = p.CreatedAt.UTC().Format(time.RFC3339Nano)
		}
		if out.Reply != nil {
			return out.Reply, nil
		}
		if time.Until(deadline) <= 0 {
			return nil, nil
		}
	}
}

// KickAgent signs an agent out: its token stops working; its record stays
// (admin or console token).
func (c *Client) KickAgent(ctx context.Context, name string, dry bool) (map[string]any, error) {
	var out map[string]any
	return out, c.do(ctx, http.MethodPost, "/v1/agents/"+esc(name)+"/kick", dryQuery(dry), nil, &out)
}

// CreateConsoleToken issues a console token (hub admin token only).
func (c *Client) CreateConsoleToken(ctx context.Context, name string) (*api.CreateConsoleTokenResponse, error) {
	var out api.CreateConsoleTokenResponse
	return &out, c.do(ctx, http.MethodPost, "/v1/console/tokens", nil, api.CreateConsoleTokenRequest{Name: name}, &out)
}

// ListConsoleTokens lists console tokens (hub admin token only).
func (c *Client) ListConsoleTokens(ctx context.Context) ([]api.ConsoleToken, error) {
	var out api.ConsoleTokenList
	return out.Tokens, c.do(ctx, http.MethodGet, "/v1/console/tokens", nil, nil, &out)
}

// RevokeConsoleToken disables a console token (hub admin token only).
func (c *Client) RevokeConsoleToken(ctx context.Context, id string) (*api.ConsoleToken, error) {
	var out api.ConsoleToken
	return &out, c.do(ctx, http.MethodDelete, "/v1/console/tokens/"+esc(id), nil, nil, &out)
}

// ReportMachine tells the hub which machine the caller runs on.
func (c *Client) ReportMachine(ctx context.Context, r api.MachineReport) error {
	return c.do(ctx, http.MethodPut, "/v1/me/machine", nil, r, nil)
}

// ReportRuntime tells the hub the caller's CLI and model.
func (c *Client) ReportRuntime(ctx context.Context, r api.Runtime) error {
	return c.do(ctx, http.MethodPut, "/v1/me/runtime", nil, r, nil)
}

// Machines lists machines (admin).
func (c *Client) Machines(ctx context.Context) ([]api.Machine, error) {
	var out api.MachineList
	return out.Machines, c.do(ctx, http.MethodGet, "/v1/machines", nil, nil, &out)
}

// RenameMachine changes a machine's display name (admin).
func (c *Client) RenameMachine(ctx context.Context, id, name string) error {
	return c.do(ctx, http.MethodPatch, "/v1/machines/"+esc(id), nil, api.RenameMachineRequest{Name: name}, nil)
}

// SpawnOnMachine starts a session on machine id (admin) and waits for it.
func (c *Client) SpawnOnMachine(ctx context.Context, id string, req api.SpawnRequest, wait time.Duration) (*api.SpawnResult, error) {
	var out api.SpawnResult
	q := url.Values{"wait_seconds": {strconv.Itoa(int(wait.Seconds()))}}
	return &out, c.do(ctx, http.MethodPost, "/v1/machines/"+esc(id)+"/spawn", q, req, &out)
}

// ConsoleSend sends a message as the administrator from the console app
// (admin or console token); the sender is "@console:<device>".
func (c *Client) ConsoleSend(ctx context.Context, req api.SendRequest) (*api.SendResponse, error) {
	var out api.SendResponse
	return &out, c.do(ctx, http.MethodPost, "/v1/console/messages", nil, req, &out)
}

// ConsoleSteer corrects a task sent from the console.
func (c *Client) ConsoleSteer(ctx context.Context, id, body string) (*api.Message, error) {
	var out api.Message
	return &out, c.do(ctx, http.MethodPost, "/v1/console/messages/"+esc(id)+"/steer", nil, api.SteerRequest{Body: body}, &out)
}

// Conversations lists the console's threads (admin or console token).
func (c *Client) Conversations(ctx context.Context, agent, query string, before int64, limit int) ([]api.Conversation, error) {
	q := url.Values{}
	if agent != "" {
		q.Set("agent", agent)
	}
	if query != "" {
		q.Set("q", query)
	}
	if before > 0 {
		q.Set("before", strconv.FormatInt(before, 10))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var out api.ConversationList
	return out.Conversations, c.do(ctx, http.MethodGet, "/v1/console/conversations", q, nil, &out)
}

// Whoami says which member a console token belongs to.
func (c *Client) Whoami(ctx context.Context) (*api.Whoami, error) {
	var out api.Whoami
	return &out, c.do(ctx, http.MethodGet, "/v1/console/me", nil, nil, &out)
}

// Members lists the hub's members (all=true includes removed ones).
func (c *Client) Members(ctx context.Context, all bool) ([]api.Member, error) {
	var out api.MemberList
	var q url.Values
	if all {
		q = url.Values{"all": {"1"}}
	}
	return out.Members, c.do(ctx, http.MethodGet, "/v1/members", q, nil, &out)
}

// InviteMember adds a person and returns their app link (once).
func (c *Client) InviteMember(ctx context.Context, req api.InviteRequest) (*api.InviteResponse, error) {
	var out api.InviteResponse
	return &out, c.do(ctx, http.MethodPost, "/v1/members", nil, req, &out)
}

// UpdateMember renames a member or changes their role.
func (c *Client) UpdateMember(ctx context.Context, member string, req api.MemberUpdateRequest) (*api.Member, error) {
	var out api.Member
	return &out, c.do(ctx, http.MethodPatch, "/v1/members/"+esc(member), nil, req, &out)
}

// RemoveMember takes a person off the hub and revokes their app access.
func (c *Client) RemoveMember(ctx context.Context, member string) (*api.Member, error) {
	var out api.Member
	return &out, c.do(ctx, http.MethodDelete, "/v1/members/"+esc(member), nil, nil, &out)
}

// SetAgentOwner gives an agent to a member (owner or admin).
func (c *Client) SetAgentOwner(ctx context.Context, agent, member string) (*api.Agent, error) {
	var out api.Agent
	return &out, c.do(ctx, http.MethodPut, "/v1/agents/"+esc(agent)+"/owner", nil, api.OwnerRequest{Owner: member}, &out)
}

// SetMachineOwner gives a machine to a member (owner or admin).
func (c *Client) SetMachineOwner(ctx context.Context, id, member string) error {
	return c.do(ctx, http.MethodPut, "/v1/machines/"+esc(id)+"/owner", nil, api.OwnerRequest{Owner: member}, nil)
}

// AuditFilter narrows Audit.
type AuditFilter struct {
	Actor, Target string
	Before        time.Time
	Limit         int
}

// Audit reads the audit log, newest first (owner or admin).
func (c *Client) Audit(ctx context.Context, f AuditFilter) ([]api.AuditEntry, error) {
	q := url.Values{}
	if f.Actor != "" {
		q.Set("actor", f.Actor)
	}
	if f.Target != "" {
		q.Set("target", f.Target)
	}
	if !f.Before.IsZero() {
		q.Set("before", strconv.FormatInt(f.Before.UnixMilli(), 10))
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	var out api.AuditList
	return out.Entries, c.do(ctx, http.MethodGet, "/v1/audit", q, nil, &out)
}
