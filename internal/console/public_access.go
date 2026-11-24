package console

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
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
			last = probePublicHealth(ctx, public)
			if last == nil {
				return public, nil
			}
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

func probePublicHealth(ctx context.Context, public string) error {
	c, err := client.New(public, "")
	if err != nil {
		return err
	}
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	h, err := c.Health(probe)
	if err != nil {
		return err
	}
	if !h.OK {
		return fmt.Errorf("the public health check did not report ready")
	}
	return nil
}

func writePublicAccessNotReady(w http.ResponseWriter, reason error) {
	writeJSON(w, http.StatusServiceUnavailable, api.Error{
		Code: "tunnel_not_ready", Retryable: true,
		Message:    "the local workspace is running, but public access is not ready: " + reason.Error(),
		Suggestion: "retry public access or inspect hub.log; local access remains available",
	})
}
