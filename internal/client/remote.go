package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/myco-ai/myco/internal/remotedesktop"
)

func (c *Client) RemoteDevices(ctx context.Context) ([]remotedesktop.Device, error) {
	var out struct {
		Machines []remotedesktop.Device `json:"machines"`
	}
	e := c.do(ctx, "GET", "/v1/remote/machines", nil, nil, &out)
	return out.Machines, e
}

// RemoteRequest uses the usual authenticated transport but never puts the host
// capability in a URL. Bodies and headers are deliberately not logged.
func (c *Client) RemoteRequest(ctx context.Context, method, path, hostKey string, in, out any) error {
	var body []byte
	var e error
	if in != nil {
		body, e = json.Marshal(in)
		if e != nil {
			return e
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	u := c.urlFor("/v1/remote/" + path)
	req, e := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	if hostKey != "" {
		req.Header.Set("X-Myco-Remote-Host", hostKey)
	}
	return c.send(ctx, req, out)
}
