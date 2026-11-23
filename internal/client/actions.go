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

// Agent actions (hubs from v0.21.21): see api/actions.go.

func waitQuery(wait time.Duration) url.Values {
	q := url.Values{}
	if wait > 0 {
		q.Set("wait_seconds", strconv.Itoa(int(wait.Round(time.Second)/time.Second)))
	}
	return q
}

// RequestActionGrant asks a target's owner to let this agent act on it.
func (c *Client) RequestActionGrant(ctx context.Context, req api.ActionGrantRequest, dry bool) (*api.ActionGrant, error) {
	if dry {
		var out struct {
			Grant api.ActionGrant `json:"grant"`
		}
		if err := c.do(ctx, http.MethodPost, "/v1/me/action-grants", dryQuery(true), req, &out); err != nil {
			return nil, sshGrantCompatibility(err, req.Target.Resource)
		}
		return &out.Grant, nil
	}
	var out api.ActionGrant
	if err := c.do(ctx, http.MethodPost, "/v1/me/action-grants", nil, req, &out); err != nil {
		return nil, sshGrantCompatibility(err, req.Target.Resource)
	}
	return &out, nil
}

// ActionGrant returns one of this agent's grants; with wait, until it is
// no longer pending.
func (c *Client) ActionGrant(ctx context.Context, id string, wait time.Duration) (*api.ActionGrant, error) {
	var out api.ActionGrant
	if err := c.do(ctx, http.MethodGet, "/v1/me/action-grants/"+esc(id), waitQuery(wait), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ActionGrants lists this agent's grants.
func (c *Client) ActionGrants(ctx context.Context) ([]api.ActionGrant, error) {
	var out api.ActionGrantList
	if err := c.do(ctx, http.MethodGet, "/v1/me/action-grants", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Grants, nil
}

// EndActionGrant ends one of this agent's grants.
func (c *Client) EndActionGrant(ctx context.Context, id string, dry bool) (*api.ActionGrant, error) {
	if dry {
		var out struct {
			Grant api.ActionGrant `json:"grant"`
		}
		if err := c.do(ctx, http.MethodPost, "/v1/me/action-grants/"+esc(id)+"/end", dryQuery(true), struct{}{}, &out); err != nil {
			return nil, err
		}
		return &out.Grant, nil
	}
	var out api.ActionGrant
	if err := c.do(ctx, http.MethodPost, "/v1/me/action-grants/"+esc(id)+"/end", nil, struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GrantActions lists what a grant lets this agent do on its target now.
func (c *Client) GrantActions(ctx context.Context, id string) ([]api.ActionDescriptor, error) {
	var out api.ActionDescriptorList
	if err := c.do(ctx, http.MethodGet, "/v1/me/action-grants/"+esc(id)+"/actions", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Actions, nil
}

// RunAction admits an action as a job.
func (c *Client) RunAction(ctx context.Context, inv api.ActionInvocation) (*api.ActionJob, error) {
	var out api.ActionJob
	if err := c.do(ctx, http.MethodPost, "/v1/me/action-jobs", nil, inv, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ActionJob returns one of this agent's jobs; with wait, until it ends.
func (c *Client) ActionJob(ctx context.Context, id string, wait time.Duration) (*api.ActionJob, error) {
	var out api.ActionJob
	if err := c.do(ctx, http.MethodGet, "/v1/me/action-jobs/"+esc(id), waitQuery(wait), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelActionJob cancels a queued job, or asks a running one to stop.
func (c *Client) CancelActionJob(ctx context.Context, id string) (*api.ActionJob, error) {
	var out api.ActionJob
	if err := c.do(ctx, http.MethodPost, "/v1/me/action-jobs/"+esc(id)+"/cancel", nil, struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ActionArtifact downloads a file one of this agent's jobs made.
func (c *Client) ActionArtifact(ctx context.Context, id string) ([]byte, string, error) {
	return c.raw(ctx, http.MethodGet, "/v1/me/action-artifacts/"+esc(id), nil, nil, "")
}

// OwnerActionGrants lists the grants the caller decides (state "" for all).
func (c *Client) OwnerActionGrants(ctx context.Context, state string) ([]api.ActionGrant, error) {
	q := url.Values{}
	if state != "" {
		q.Set("state", state)
	}
	var out api.ActionGrantList
	if err := c.do(ctx, http.MethodGet, "/v1/console/action-grants", q, nil, &out); err != nil {
		return nil, err
	}
	return out.Grants, nil
}

// RevokeActionGrant ends a grant its owner no longer allows.
func (c *Client) RevokeActionGrant(ctx context.Context, id string) (*api.ActionGrant, error) {
	var out api.ActionGrant
	if err := c.do(ctx, http.MethodPost, "/v1/console/action-grants/"+esc(id)+"/revoke", nil, struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DecideActionGrant approves an action_grant approval, allowing only
// narrow of what was asked (zero value: all of it).
func (c *Client) DecideActionGrant(ctx context.Context, approvalID string, narrow api.ActionGrantNarrow) (*api.Approval, error) {
	var out api.Approval
	if err := c.do(ctx, http.MethodPost, "/v1/console/approvals/"+esc(approvalID)+"/approve", nil, narrow, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RegisterActionHost offers a provider's actions on a computer (the
// trusted App there).
func (c *Client) RegisterActionHost(ctx context.Context, req api.ActionHostRegister) (*api.ActionHost, error) {
	var out api.ActionHost
	if err := c.do(ctx, http.MethodPost, "/v1/console/action-hosts", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ActionHostJobs takes the jobs waiting for a host, waiting up to wait.
func (c *Client) ActionHostJobs(ctx context.Context, hostID string, wait time.Duration) ([]api.ActionHostJob, error) {
	var out api.ActionHostJobs
	if err := c.do(ctx, http.MethodGet, "/v1/console/action-hosts/"+esc(hostID)+"/jobs", waitQuery(wait), nil, &out); err != nil {
		return nil, err
	}
	return out.Jobs, nil
}

// ActionHostJob says whether a host should stop a job it runs; with wait,
// it answers once it should (or after wait).
func (c *Client) ActionHostJob(ctx context.Context, jobID, hostID, generation string, wait time.Duration) (*api.ActionHostJobState, error) {
	q := waitQuery(wait)
	q.Set("host_id", hostID)
	q.Set("generation", generation)
	var out api.ActionHostJobState
	if err := c.do(ctx, http.MethodGet, "/v1/console/action-jobs/"+esc(jobID), q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UploadActionArtifact keeps a file a running job made.
func (c *Client) UploadActionArtifact(ctx context.Context, jobID, hostID, generation, mediaType string, data []byte) (*api.ActionArtifact, error) {
	q := url.Values{"host_id": {hostID}, "generation": {generation}}
	var out api.ActionArtifact
	body, _, err := c.raw(ctx, http.MethodPost, "/v1/console/action-jobs/"+esc(jobID)+"/artifacts", q, data, mediaType)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CompleteActionJob reports how a running job ended.
func (c *Client) CompleteActionJob(ctx context.Context, jobID string, done api.ActionJobComplete) (*api.ActionJob, error) {
	var out api.ActionJob
	if err := c.do(ctx, http.MethodPost, "/v1/console/action-jobs/"+esc(jobID)+"/complete", nil, done, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// raw sends body as is (contentType) and returns the answer's bytes and
// type.
func (c *Client) raw(ctx context.Context, method, path string, query url.Values, body []byte, contentType string) ([]byte, string, error) {
	u := c.urlFor(path)
	u.RawQuery = query.Encode()
	rctx, cancel := context.WithTimeout(ctx, plainBound)
	defer cancel()
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(rctx, method, u.String(), rd)
	if err != nil {
		return nil, "", err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", &Error{Code: CodeUnavailable, Message: fmt.Sprintf("cannot reach hub %s: %v", c.base, err), Retryable: true}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, "", &Error{Code: CodeUnavailable, Message: err.Error(), Retryable: true}
	}
	if resp.StatusCode >= 300 {
		return nil, "", decodeError(resp.StatusCode, data)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// ActionTargets lists enabled resources belonging to this agent's person.
func (c *Client) ActionTargets(ctx context.Context) ([]api.ActionTarget, error) {
	var out api.ActionTargetList
	if err := c.do(ctx, http.MethodGet, "/v1/me/action-targets", nil, nil, &out); err != nil {
		var e *Error
		if errors.As(err, &e) && e.Status == 404 {
			return nil, &Error{Code: "unsupported", Status: e.Status, Message: "this Hub does not support bound SSH action targets", Suggestion: "update the Hub and controller App"}
		}
		return nil, err
	}
	return out.Targets, nil
}

// Never downgrade a bound request to a machine-wide grant on an old Hub.
func sshGrantCompatibility(err error, r *api.ActionResource) error {
	var e *Error
	if r != nil && errors.As(err, &e) && e.Status == 400 && strings.Contains(e.Message, `unknown field "resource"`) {
		return &Error{Code: "unsupported", Status: e.Status, Message: "this Hub does not support resource-bound SSH grants", Suggestion: "update the Hub and controller App"}
	}
	return err
}
