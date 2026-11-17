package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/chenhg5/myco/internal/api"
)

// PageInput says what a publish is: a new page (Name, Project) or a new
// version of one (Page).
type PageInput struct {
	Page    string
	Name    string
	Project string
	Entry   string
}

// PublishPage uploads a zip of a page's files (from v0.19.24).
func (c *Client) PublishPage(ctx context.Context, in PageInput, zip io.Reader, size int64) (*api.Page, error) {
	path := "/v1/pages"
	if in.Page != "" {
		path = "/v1/pages/" + esc(in.Page) + "/versions"
	}
	u := c.urlFor(path)
	q := url.Values{}
	for k, v := range map[string]string{"name": in.Name, "project": in.Project, "entry": in.Entry} {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), zip)
	if err != nil {
		return nil, err
	}
	if size >= 0 {
		req.ContentLength = size
	}
	req.Header.Set("Content-Type", "application/zip")
	var out api.Page
	return &out, c.send(ctx, req, &out)
}

// Pages lists the pages the caller may see (of one project, if given).
func (c *Client) Pages(ctx context.Context, project string) ([]api.Page, error) {
	q := url.Values{}
	if project != "" {
		q.Set("project", project)
	}
	var out api.PageList
	return out.Pages, c.do(ctx, http.MethodGet, "/v1/pages", q, nil, &out)
}

// Page returns a page and its versions.
func (c *Client) Page(ctx context.Context, id string) (*api.Page, error) {
	var out api.Page
	return &out, c.do(ctx, http.MethodGet, "/v1/pages/"+esc(id), nil, nil, &out)
}

// DeletePage removes a page and its versions.
func (c *Client) DeletePage(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/pages/"+esc(id), nil, nil, nil)
}

// OpenPageFile streams one file of a page's version ("latest" or a number;
// an empty path is its entry file); the caller closes it.
func (c *Client) OpenPageFile(ctx context.Context, id, version, path string, hdr http.Header) (*http.Response, error) {
	if version == "" {
		version = "latest"
	}
	u := c.urlFor("/v1/pages/" + esc(id) + "/v/" + esc(version) + "/" + escPath(path))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &Error{Code: CodeUnavailable, Message: err.Error(), Retryable: true}
	}
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotModified {
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, decodeError(resp.StatusCode, data)
	}
	return resp, nil
}

// escPath escapes each part of a slash-separated path.
func escPath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

// PageState reads a page's shared state.
func (c *Client) PageState(ctx context.Context, id string) (*api.PageState, error) {
	var out api.PageState
	return &out, c.do(ctx, http.MethodGet, "/v1/pages/"+esc(id)+"/state", nil, nil, &out)
}

// PutPageState writes a page's shared state, based on version ifVersion.
// When someone wrote first it returns the latest with a conflict error.
func (c *Client) PutPageState(ctx context.Context, id string, state json.RawMessage, ifVersion int) (*api.PageState, error) {
	b, err := json.Marshal(api.PageStateRequest{State: state, IfVersion: ifVersion})
	if err != nil {
		return nil, err
	}
	u := c.urlFor("/v1/pages/" + esc(id) + "/state")
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), bytes.NewReader(b))
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
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode == http.StatusConflict {
		var cf api.PageStateConflict
		if json.Unmarshal(data, &cf) == nil {
			return &cf.Latest, decodeError(resp.StatusCode, data)
		}
	}
	if resp.StatusCode >= 300 {
		return nil, decodeError(resp.StatusCode, data)
	}
	var out api.PageState
	return &out, json.Unmarshal(data, &out)
}
