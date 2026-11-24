package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/myco-ai/myco/internal/api"
)

// PageRecords lists a page's records (of one collection, or all) and the
// page's latest change (hubs from v0.20.5).
func (c *Client) PageRecords(ctx context.Context, page, collection string) (*api.PageRecords, error) {
	var q url.Values
	if collection != "" {
		q = url.Values{"collection": {collection}}
	}
	var out api.PageRecords
	return &out, c.do(ctx, http.MethodGet, "/v1/pages/"+esc(page)+"/data", q, nil, &out)
}

// PageRecord reads one record.
func (c *Client) PageRecord(ctx context.Context, page, collection, id string) (*api.PageRecord, error) {
	var out api.PageRecord
	return &out, c.do(ctx, http.MethodGet, "/v1/pages/"+esc(page)+"/data/"+esc(collection)+"/"+esc(id), nil, nil, &out)
}

// PutPageRecord writes a record (id "" for a new one). ifVersion: the
// version read (0: it must be new), nil to write whatever it is. On a
// conflict it returns the latest with the error.
func (c *Client) PutPageRecord(ctx context.Context, page, collection, id string, data json.RawMessage, ifVersion *int) (*api.PageRecord, error) {
	b, err := json.Marshal(api.PageRecordWrite{Data: data, IfVersion: ifVersion})
	if err != nil {
		return nil, err
	}
	method, path := http.MethodPut, "/v1/pages/"+esc(page)+"/data/"+esc(collection)+"/"+esc(id)
	if id == "" {
		method, path = http.MethodPost, "/v1/pages/"+esc(page)+"/data/"+esc(collection)
	}
	u := c.urlFor(path)
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &Error{Code: CodeUnavailable, Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode == http.StatusConflict {
		var cf api.PageRecordConflict
		if json.Unmarshal(raw, &cf) == nil && cf.Latest.ID != "" {
			return &cf.Latest, decodeError(resp.StatusCode, raw)
		}
	}
	if resp.StatusCode >= 300 {
		return nil, decodeError(resp.StatusCode, raw)
	}
	var out api.PageRecord
	return &out, json.Unmarshal(raw, &out)
}

// DeletePageRecord deletes one (ifVersion 0: whatever it is).
func (c *Client) DeletePageRecord(ctx context.Context, page, collection, id string, ifVersion int) error {
	var q url.Values
	if ifVersion > 0 {
		q = url.Values{"if_version": {strconv.Itoa(ifVersion)}}
	}
	return c.do(ctx, http.MethodDelete, "/v1/pages/"+esc(page)+"/data/"+esc(collection)+"/"+esc(id), q, nil, nil)
}

// PageChanges lists what changed after seq, waiting up to wait seconds
// (at most 60) for something when nothing has yet.
func (c *Client) PageChanges(ctx context.Context, page string, after int64, wait int) (*api.PageChanges, error) {
	q := url.Values{"after": {strconv.FormatInt(after, 10)}}
	if wait > 0 {
		q.Set("wait", strconv.Itoa(wait))
		q.Set("wait_seconds", strconv.Itoa(wait))
	}
	var out api.PageChanges
	return &out, c.do(ctx, http.MethodGet, "/v1/pages/"+esc(page)+"/changes", q, nil, &out)
}

// Templates lists the workspace's library.
func (c *Client) Templates(ctx context.Context) ([]api.Template, error) {
	var out api.TemplateList
	return out.Templates, c.do(ctx, http.MethodGet, "/v1/templates", nil, nil, &out)
}

// Template returns one (by id, or by its manifest's id such as
// alice/weekly).
func (c *Client) Template(ctx context.Context, id string) (*api.Template, error) {
	var out api.Template
	return &out, c.do(ctx, http.MethodGet, "/v1/templates/"+esc(id), nil, nil, &out)
}

// TemplateShot opens one of a template's screenshots (hubs from v0.21.5;
// older ones answer 404). The caller closes the body.
func (c *Client) TemplateShot(ctx context.Context, id, path string) (*http.Response, error) {
	u := c.urlFor("/v1/templates/" + esc(id) + "/files/" + escPath(path))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &Error{Code: CodeUnavailable, Message: err.Error(), Retryable: true}
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, decodeError(resp.StatusCode, data)
	}
	return resp, nil
}

// PublishTemplate uploads a zip of a template's folder.
func (c *Client) PublishTemplate(ctx context.Context, zip io.Reader, size int64) (*api.Template, error) {
	u := c.urlFor("/v1/templates")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), zip)
	if err != nil {
		return nil, err
	}
	if size >= 0 {
		req.ContentLength = size
	}
	req.Header.Set("Content-Type", "application/zip")
	var out api.Template
	return &out, c.send(ctx, req, &out)
}

// SyncApps asks the hub to fetch the official apps now (hubs from v0.20.7).
func (c *Client) SyncApps(ctx context.Context) (*api.AppSync, error) {
	var out api.AppSync
	return &out, c.do(ctx, http.MethodPost, "/v1/templates/sync", nil, nil, &out)
}

// DeleteTemplate takes one out of the library.
func (c *Client) DeleteTemplate(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/templates/"+esc(id), nil, nil, nil)
}

// UseTemplate makes a page from a template.
func (c *Client) UseTemplate(ctx context.Context, id string, in api.TemplateUse) (*api.TemplateUsed, error) {
	var out api.TemplateUsed
	return &out, c.do(ctx, http.MethodPost, "/v1/templates/"+esc(id)+"/use", nil, in, &out)
}
