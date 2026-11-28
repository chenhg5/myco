package oscap

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Tests inject a fixture transport. Production always verifies TLS and DNS policy.
var credentialTransport http.RoundTripper
var credentialRates = struct {
	sync.Mutex
	rows map[string]*credentialRate
}{rows: map[string]*credentialRate{}}

type credentialRate struct {
	start         time.Time
	count, active int
}

func credentialAcquire(scope, id string) (func(), error) {
	credentialRates.Lock()
	defer credentialRates.Unlock()
	now := time.Now()
	key := scope + "\x00" + id
	r := credentialRates.rows[key]
	if r == nil {
		for k, v := range credentialRates.rows {
			if v.active == 0 && now.Sub(v.start) >= time.Minute {
				delete(credentialRates.rows, k)
			}
		}
		if len(credentialRates.rows) >= 1024 {
			return nil, errf("busy", "too many active connections")
		}
		r = &credentialRate{start: now}
		credentialRates.rows[key] = r
	}
	if now.Sub(r.start) >= time.Minute {
		r.start = now
		r.count = 0
	}
	if r.active >= 2 {
		return nil, errf("busy", "this connection has two requests in progress")
	}
	if r.count >= 60 {
		return nil, errf("rate_limited", "this connection permits 60 requests per minute; try again later")
	}
	r.active++
	r.count++
	return func() { credentialRates.Lock(); r.active--; credentialRates.Unlock() }, nil
}
func credentialFetch(ctx context.Context, e *Env, id, path string, query map[string]string, method string, headers map[string]string, rawURL, body string, redirect *bool, max, timeoutMS int64, response string) (any, error) {
	target, err := CredentialRequestURL(e, id, path)
	if err != nil {
		return nil, err
	}
	s, err := credentialLookup(e, id)
	if err != nil {
		return nil, err
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "GET"
	}
	if (method != "GET" && method != "HEAD") || rawURL != "" || body != "" || (redirect != nil && *redirect) {
		return nil, errf(CodePermission, "credential requests are read-only and cannot override the destination or follow redirects")
	}
	// An allowlist avoids alternate spelling, whitespace and transport header tricks.
	for k, v := range headers {
		switch strings.ToLower(k) {
		case "accept", "accept-language", "if-none-match", "if-modified-since":
			if strings.ContainsAny(v, "\r\n\x00") {
				return nil, errf(CodeInvalid, "invalid request header")
			}
		default:
			return nil, errf(CodePermission, "request header is not allowed")
		}
	}
	q := url.Values{}
	if len(query) > 64 {
		return nil, errf(CodeInvalid, "too many query parameters")
	}
	for k, v := range query {
		if len(k) > 256 || len(v) > 8192 || strings.ContainsAny(k+v, "\x00\r\n") {
			return nil, errf(CodeInvalid, "invalid query parameter")
		}
		q.Set(k, v)
	}
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	release, err := credentialAcquire(e.Scope, id)
	if err != nil {
		return nil, err
	}
	defer release()
	timeout := 30 * time.Second
	if timeoutMS > 0 && timeoutMS < 30000 {
		timeout = time.Duration(timeoutMS) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, errf(CodeInvalid, "invalid request")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("User-Agent", "myco-app")
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	switch s.Connection.Authentication {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+s.Token)
	case "token":
		req.Header.Set("Authorization", "token "+s.Token)
	case "private-token":
		req.Header.Set("PRIVATE-TOKEN", s.Token)
	default:
		return nil, errf(CodePermission, "invalid saved authentication scheme")
	}
	transport := credentialTransport
	if transport == nil {
		transport, err = credentialPinnedTransport(ctx, s.Connection.Origin, s.AllowPrivate)
		if err != nil {
			return nil, err
		}
		defer transport.(*http.Transport).CloseIdleConnections()
	}
	rule := ""
	decoded, _ := url.PathUnescape(path)
	for _, p := range s.Connection.Paths {
		if (!p.Prefix && decoded == p.Path) || (p.Prefix && strings.HasPrefix(decoded, p.Path)) {
			rule = p.Path
			if p.Prefix {
				rule += "*"
			}
			break
		}
	}
	status := 0
	if e.CredentialAudit != nil {
		defer func() { e.CredentialAudit(s.Connection.Origin, method, rule, status) }()
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, AsError(ctx.Err())
		}
		return nil, errf("network_error", "could not reach the authorized destination")
	}
	defer res.Body.Close()
	status = res.StatusCode
	if max <= 0 || max > 4<<20 {
		max = 4 << 20
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, max+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, AsError(ctx.Err())
		}
		return nil, errf("network_error", "could not read response")
	}
	if int64(len(raw)) > max {
		return nil, errf(CodeTooLarge, "response exceeds the connection limit")
	}
	if strings.Contains(string(raw), s.Token) {
		return nil, errf("invalid_response", "response contained a credential and was discarded")
	}
	outHeaders := map[string]string{}
	for _, k := range []string{"Content-Type", "ETag", "Last-Modified", "Link", "X-Next-Page", "X-Total-Pages", "X-RateLimit-Remaining", "Retry-After"} {
		v := res.Header.Get(k)
		if v != "" && !strings.Contains(v, s.Token) {
			outHeaders[strings.ToLower(k)] = v
		}
	}
	encoding := "utf8"
	text := string(raw)
	if !(response == "text" || response != "base64" && isText(res.Header.Get("Content-Type")) && utf8.Valid(raw)) {
		encoding = "base64"
		text = base64.StdEncoding.EncodeToString(raw)
	}
	// Deliberately omit the query-bearing URL and provider error interpretation.
	return map[string]any{"status": status, "status_text": http.StatusText(status), "headers": outHeaders, "body": text, "encoding": encoding, "url": s.Connection.Origin + path}, nil
}
