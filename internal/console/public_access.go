package console

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
)

func validPublicURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}

// A local health check only proves the hub started. Quick tunnels report their
// public URL later; wait for that URL and a real health response through it.
func waitPublicAccess(ctx context.Context, poll time.Duration, read func(context.Context) string) (string, error) {
	var last error
	for {
		public := strings.TrimSpace(read(ctx))
		if public != "" {
			if !validPublicURL(public) {
				return "", fmt.Errorf("the hub reported an invalid public URL")
			}
			c, err := client.New(public, "")
			if err == nil {
				probe, cancel := context.WithTimeout(ctx, 3*time.Second)
				health, e := c.Health(probe)
				cancel()
				if e == nil && health.OK {
					return public, nil
				}
				if e == nil {
					e = fmt.Errorf("the public health check did not report ready")
				}
				err = e
			}
			last = err
		} else {
			last = fmt.Errorf("the tunnel has not reported a public URL yet")
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("%v: %w", last, ctx.Err())
		case <-time.After(poll):
		}
	}
}

func writePublicAccessNotReady(w http.ResponseWriter, reason error) {
	writeJSON(w, http.StatusServiceUnavailable, api.Error{
		Code: "tunnel_not_ready", Retryable: true,
		Message:    "the local workspace is running, but public access is not ready: " + reason.Error(),
		Suggestion: "retry public access or inspect hub.log; local and LAN access remain available",
	})
}
