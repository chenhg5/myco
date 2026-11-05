// Package wake implements app.Waker adapters: generic signed webhooks,
// Claude Code routines (direct or relayed to a bound session) and GitHub.
package wake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// Doer is the HTTP dependency of all wakers (swap for tests).
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// DefaultHTTP is a client with a sane timeout.
var DefaultHTTP Doer = &http.Client{Timeout: 30 * time.Second}

func postJSON(ctx context.Context, h Doer, u string, headers map[string]string, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return do(ctx, h, http.MethodPost, u, headers, b)
}

func do(ctx context.Context, h Doer, method, u string, headers map[string]string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "myco-hub")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := h.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return data, fmt.Errorf("%s %s: HTTP %d: %s", method, redact(u), resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func redact(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return "<url>"
	}
	p.RawQuery = ""
	return p.String()
}

// requireURL validates a wake URL: https, or http only for loopback.
func requireURL(a *domain.Agent) error {
	raw := a.Delivery.URL
	u, err := url.Parse(raw)
	if raw == "" || err != nil || u.Host == "" {
		return domain.Errf(domain.CodeInvalidArgument, "delivery %s needs a wake URL", a.Delivery.Mode).
			WithSuggestion("pass --wake-url <https url>")
	}
	host := u.Hostname()
	loopback := host == "localhost" || host == "127.0.0.1" || host == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return domain.Errf(domain.CodeInvalidArgument, "wake URL must use https (got %q)", u.Scheme)
	}
	if u.User != nil {
		return domain.Errf(domain.CodeInvalidArgument, "wake URL must not embed credentials").
			WithSuggestion("pass the secret with --wake-secret-file")
	}
	return nil
}

func requireSecret(a *domain.Agent, what string) error {
	if a.Delivery.Secret == "" {
		return domain.Errf(domain.CodeInvalidArgument, "delivery %s needs %s", a.Delivery.Mode, what).
			WithSuggestion("pass --wake-secret-file <file>")
	}
	return nil
}

func ids(msgs []*domain.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.ID
	}
	return out
}
