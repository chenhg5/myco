package client

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/myco-ai/myco/internal/api"
)

func (c *Client) pageAttachmentsSupported(ctx context.Context, page string) error {
	p, err := c.Page(ctx, page)
	if err != nil {
		return err
	}
	if p.AttachmentVersion < 1 {
		return &Error{Status: http.StatusNotImplemented, Code: "unsupported", Message: "this Hub does not support page-owned files", Suggestion: "upgrade the Hub; do not upload application media to project files"}
	}
	return nil
}
func (c *Client) PageAttachments(ctx context.Context, page, after string, limit int) (*api.PageAttachmentList, error) {
	if err := c.pageAttachmentsSupported(ctx, page); err != nil {
		return nil, err
	}
	q := url.Values{}
	if after != "" {
		q.Set("after", after)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var out api.PageAttachmentList
	return &out, c.do(ctx, http.MethodGet, "/v1/pages/"+esc(page)+"/attachments", q, nil, &out)
}
func (c *Client) ResolvePageAttachment(ctx context.Context, page, name string) (*api.PageAttachment, error) {
	if err := c.pageAttachmentsSupported(ctx, page); err != nil {
		return nil, err
	}
	var out api.PageAttachmentList
	if err := c.do(ctx, http.MethodGet, "/v1/pages/"+esc(page)+"/attachments", url.Values{"name": {name}}, nil, &out); err != nil {
		return nil, err
	}
	if len(out.Attachments) != 1 {
		return nil, &Error{Code: "not_found", Message: "page attachment not found"}
	}
	return out.Attachments[0], nil
}
func (c *Client) PageAttachment(ctx context.Context, page, id string) (*api.PageAttachment, error) {
	if err := c.pageAttachmentsSupported(ctx, page); err != nil {
		return nil, err
	}
	var out api.PageAttachment
	return &out, c.do(ctx, http.MethodGet, "/v1/pages/"+esc(page)+"/attachments/"+esc(id), nil, nil, &out)
}

type PageAttachmentUpload struct{ Page, Collection, Name, ContentType, Key, LegacyName string }

func (c *Client) PutPageAttachment(ctx context.Context, in PageAttachmentUpload, r io.Reader, size int64) (*api.PageAttachment, error) {
	if err := c.pageAttachmentsSupported(ctx, in.Page); err != nil {
		return nil, err
	}
	u := c.urlFor("/v1/pages/" + esc(in.Page) + "/attachments")
	q := url.Values{"name": {in.Name}, "collection": {in.Collection}}
	if in.LegacyName != "" {
		q.Set("legacy_name", in.LegacyName)
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), r)
	if err != nil {
		return nil, err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", in.ContentType)
	req.Header.Set("Idempotency-Key", in.Key)
	var out api.PageAttachment
	return &out, c.send(ctx, req, &out)
}
func (c *Client) DeletePageAttachment(ctx context.Context, page, id string) error {
	if err := c.pageAttachmentsSupported(ctx, page); err != nil {
		return err
	}
	return c.do(ctx, http.MethodDelete, "/v1/pages/"+esc(page)+"/attachments/"+esc(id), nil, nil, nil)
}

// OpenPageAttachment preserves Range/status headers for native streaming proxies.
func (c *Client) OpenPageAttachment(ctx context.Context, page, id, byteRange string) (*http.Response, error) {
	if err := c.pageAttachmentsSupported(ctx, page); err != nil {
		return nil, err
	}
	u := c.urlFor("/v1/pages/" + esc(page) + "/attachments/" + esc(id) + "/content")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, decodeError(resp.StatusCode, b)
	}
	return resp, nil
}
func (c *Client) MigratePageAttachments(ctx context.Context, in api.PageAttachmentMigrationRequest) (*api.PageAttachmentMigrationReport, error) {
	var out api.PageAttachmentMigrationReport
	err := c.do(ctx, http.MethodPost, "/v1/page-attachments/migrate", nil, in, &out)
	if e, ok := err.(*Error); ok && e.Status == 404 {
		return nil, &Error{Status: http.StatusNotImplemented, Code: "unsupported", Message: "this Hub does not support page file migration", Suggestion: "upgrade the Hub"}
	}
	return &out, err
}
