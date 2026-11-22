package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/chenhg5/myco/internal/api"
)

// Notices returns the caller's notices after after (a console credential:
// they are a person's). With wait > 0 and nothing new, the hub holds the
// request up to wait (55 seconds at most) for the next one. Messages the
// notices carry are opened like any other (end-to-end encryption).
func (c *Client) Notices(ctx context.Context, after int64, limit int, wait time.Duration) (*api.NoticeFeed, error) {
	q := url.Values{}
	if after > 0 {
		q.Set("after", strconv.FormatInt(after, 10))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if s := int(wait / time.Second); s > 0 {
		q.Set("wait_seconds", strconv.Itoa(s))
	}
	var out api.NoticeFeed
	if err := c.do(ctx, http.MethodGet, "/v1/console/notices", q, nil, &out); err != nil {
		return nil, err
	}
	for i := range out.Notices {
		c.open(ctx, out.Notices[i].Message)
	}
	return &out, nil
}

// ReadNotices marks the caller's notices as seen: those leading to target
// ("" for all) up to through (0 for all).
func (c *Client) ReadNotices(ctx context.Context, target string, through int64) (int, error) {
	var out api.NoticeReadResponse
	err := c.do(ctx, http.MethodPost, "/v1/console/notices/read", nil, api.NoticeReadRequest{Target: target, Through: through}, &out)
	return out.Changed, err
}

// NoticePrefs returns what the caller chose about being told.
func (c *Client) NoticePrefs(ctx context.Context) (*api.NoticePrefs, error) {
	var out api.NoticePrefs
	if err := c.do(ctx, http.MethodGet, "/v1/console/notice-prefs", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetNoticePrefs replaces what the caller chose about being told.
func (c *Client) SetNoticePrefs(ctx context.Context, p api.NoticePrefs) (*api.NoticePrefs, error) {
	var out api.NoticePrefs
	if err := c.do(ctx, http.MethodPut, "/v1/console/notice-prefs", nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
