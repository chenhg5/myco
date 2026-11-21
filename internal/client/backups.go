package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/chenhg5/myco/internal/api"
)

// Backups lists the hub's backups (its owner; hubs from v0.21.12).
func (c *Client) Backups(ctx context.Context) (*api.Backups, error) {
	var out api.Backups
	return &out, c.do(ctx, http.MethodGet, "/v1/admin/backups", nil, nil, &out)
}

// CreateBackup makes a backup of the hub's data now.
func (c *Client) CreateBackup(ctx context.Context) (*api.Backup, error) {
	var out api.Backup
	// A large hub takes a while: allow 30 minutes (the hub ignores the
	// parameter; it only raises this call's own time limit).
	return &out, c.do(ctx, http.MethodPost, "/v1/admin/backups", url.Values{"wait_seconds": {"1800"}}, nil, &out)
}

// DownloadBackup saves backup name into dir (as its own name, readable
// only by this user) and returns the path. A partial download never takes
// the name.
func (c *Client) DownloadBackup(ctx context.Context, name, dir string) (string, error) {
	u := c.urlFor("/v1/admin/backups/" + esc(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	c.auth(req)
	// A backup can be large: no overall time limit beyond the caller's.
	hc := *c.http
	hc.Timeout = 0
	resp, err := hc.Do(req)
	if err != nil {
		return "", &Error{Code: CodeUnavailable, Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return "", decodeError(resp.StatusCode, data)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".myco-backup-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	n, err := io.Copy(tmp, resp.Body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	if resp.ContentLength > 0 && n != resp.ContentLength {
		return "", &Error{Code: "integrity_error", Message: fmt.Sprintf("the backup download stopped at %d of %d bytes", n, resp.ContentLength), Retryable: true}
	}
	path := filepath.Join(dir, filepath.Base(name))
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	_ = os.Chmod(path, 0o600)
	return path, nil
}
