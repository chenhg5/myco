package oscap

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"

	"github.com/myco-ai/myco/internal/api"
)

type credentialTestSecrets struct {
	sync.Mutex
	data map[string]string
}

func (s *credentialTestSecrets) Get(k string) (string, error) {
	s.Lock()
	defer s.Unlock()
	return s.data[k], nil
}
func (s *credentialTestSecrets) Set(k, v string) error {
	s.Lock()
	defer s.Unlock()
	s.data[k] = v
	return nil
}
func (s *credentialTestSecrets) Delete(k string) error {
	s.Lock()
	defer s.Unlock()
	delete(s.data, k)
	return nil
}

type credentialTestTransport func(*http.Request) (*http.Response, error)

func (f credentialTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func credentialTestEnv(t *testing.T) *Env {
	t.Helper()
	return &Env{Home: t.TempDir(), Scope: "profile\x00workspace\x00identity\x00page\x00app", Secrets: &credentialTestSecrets{data: map[string]string{}}}
}
func credentialTestStore(t *testing.T, e *Env) api.CredentialConnection {
	t.Helper()
	c, err := CredentialStore(e, api.CredentialDescriptor{Name: "Fixture", Origin: "https://api.example.com", Authentication: "bearer", Paths: []api.CredentialPath{{Path: "/repos/", Prefix: true}, {Path: "/user"}}}, "fixture-secret-value", "token", "", false)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func credentialTestCall(t *testing.T, e *Env, name string, args any) (any, *Error) {
	t.Helper()
	raw, _ := json.Marshal(args)
	return Call(context.Background(), e, name, raw)
}
func TestCredentialDestinationAndPathRules(t *testing.T) {
	e := credentialTestEnv(t)
	c := credentialTestStore(t, e)
	for _, path := range []string{"/repos/demo/one", "/user", "/repos/demo/my.repo"} {
		if _, err := CredentialRequestURL(e, c.ID, path); err != nil {
			t.Fatalf("valid %s: %v", path, err)
		}
	}
	for _, path := range []string{"/userfoo", "/repos", "/", "//evil.example/repos/a", "https://evil.example/repos/a", "/repos//a", "/repos/a/../b", "/repos/./b", "/repos/%2e%2e/a", "/repos/%2Fa", "/repos/%5ca", "/repos/%252e/a", "/repos/%252f/a", "/repos/a\\b", "/repos/a?x=y", "/repos/a#fragment", "/repos/%00a", "/repos/a/"} {
		if _, err := CredentialRequestURL(e, c.ID, path); err == nil {
			t.Errorf("accepted %q", path)
		}
	}
	for _, origin := range []string{"http://example.com", "https://u:pw@example.com", "https://example.com/path", "https://example.com?x=y", "https://example.com#f", "https://example.com.", "https://example.com/?"} {
		if _, err := CredentialOrigin(origin); err == nil {
			t.Errorf("accepted origin %s", origin)
		}
	}
	for _, row := range []struct{ cli, host, origin, base string }{{"gh", "github.com", "https://api.github.com", ""}, {"gh", "git.example.com", "https://git.example.com", "/api/v3"}, {"glab", "git.example.com", "https://git.example.com", "/api/v4"}} {
		origin, base, err := CredentialSourceTarget(row.cli, row.host)
		if err != nil || origin != row.origin || base != row.base {
			t.Fatal(row, origin, base, err)
		}
	}
	for _, host := range []string{"evil.example/path", "github.com@evil.example", "github.com?x=y", "github.com#f", "GITHUB.COM"} {
		if _, _, err := CredentialSourceTarget("gh", host); err == nil {
			t.Errorf("accepted login host %s", host)
		}
	}
}
func TestCredentialScopeColdStartAndRevocation(t *testing.T) {
	e := credentialTestEnv(t)
	c := credentialTestStore(t, e)
	// The same scope can recover a connection after a host restart, without exporting its secret.
	fresh := *e
	v, err := credentialTestCall(t, &fresh, "credentials.connections", map[string]string{"action": "list"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "fixture-secret-value") {
		t.Fatal("secret in listing")
	}
	parts := strings.Split(e.Scope, "\x00")
	for i := range parts {
		other := *e
		changed := append([]string{}, parts...)
		changed[i] = "other"
		other.Scope = strings.Join(changed, "\x00")
		if _, err := CredentialRequestURL(&other, c.ID, "/user"); err == nil {
			t.Fatalf("crossed scope component %d", i)
		}
	}
	if _, err := credentialTestCall(t, e, "credentials.connections", map[string]string{"action": "connect", "token": "fixture-secret-value"}); err == nil {
		t.Fatal("iframe created connection")
	}
	if _, err := credentialTestCall(t, e, "credentials.connections", map[string]string{"action": "remove", "id": c.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := CredentialRequestURL(e, c.ID, "/user"); err == nil {
		t.Fatal("revoked connection remained usable")
	}
}
func TestCredentialFetchSecurityAndProviderNeutralResponses(t *testing.T) {
	e := credentialTestEnv(t)
	c := credentialTestStore(t, e)
	old := credentialTransport
	t.Cleanup(func() { credentialTransport = old })
	calls := 0
	response := "{\"message\":\"not signed in\"}"
	status := 401
	credentialTransport = credentialTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "api.example.com" || r.Header.Get("Authorization") != "Bearer fixture-secret-value" || r.Header.Get("Cookie") != "" {
			t.Fatal("wrong credential destination")
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}, "Set-Cookie": {"private-cookie"}, "Authorization": {"fixture-secret-value"}}, Body: io.NopCloser(strings.NewReader(response))}, nil
	})
	audit := ""
	e.CredentialAudit = func(origin, method, path string, s int) {
		audit = origin + method + path
		if s != 401 {
			t.Fatal(s)
		}
	}
	args := map[string]any{"credential": c.ID, "path": "/repos/private/name", "query": map[string]string{"q": "private-search"}}
	v, err := credentialTestCall(t, e, "net.fetch", args)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(v)
	if !strings.Contains(string(raw), "401") || strings.Contains(string(raw), "private-cookie") || strings.Contains(string(raw), "fixture-secret-value") {
		t.Fatal("invalid HTTP result", string(raw))
	}
	if strings.Contains(audit, "private") || !strings.Contains(audit, "/repos/*") {
		t.Fatal("audit contains concrete request", audit)
	}
	e.CredentialAudit = nil
	for key, value := range map[string]any{"url": "https://evil.example", "method": "POST", "body": "mutation", "redirect": true, "headers": map[string]string{"Authorization": "override"}} {
		bad := map[string]any{"credential": c.ID, "path": "/user", key: value}
		if _, err := credentialTestCall(t, e, "net.fetch", bad); err == nil {
			t.Error("accepted override", key)
		}
	}
	if calls != 1 {
		t.Fatal("rejected calls reached transport", calls)
	}
	response = "fixture-secret-value"
	if _, err := credentialTestCall(t, e, "net.fetch", args); err == nil {
		t.Fatal("credential echo leaked")
	}
	response = strings.Repeat("x", 100)
	args["max_bytes"] = 10
	if _, err := credentialTestCall(t, e, "net.fetch", args); err == nil || err.Code != CodeTooLarge {
		t.Fatal("response limit ignored", err)
	}
	status = 302
	response = ""
	delete(args, "max_bytes")
	v, err = credentialTestCall(t, e, "net.fetch", args)
	if err != nil || v.(map[string]any)["status"] != 302 {
		t.Fatal("redirect not returned without following", err)
	}
}
func TestCredentialLimitsAndPrivateDNS(t *testing.T) {
	e := credentialTestEnv(t)
	c := credentialTestStore(t, e)
	first, err := credentialAcquire(e.Scope, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := credentialAcquire(e.Scope, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := credentialAcquire(e.Scope, c.ID); err == nil {
		t.Fatal("concurrency limit ignored")
	}
	first()
	second()
	for i := 2; i < 60; i++ {
		release, err := credentialAcquire(e.Scope, c.ID)
		if err != nil {
			t.Fatal(i, err)
		}
		release()
	}
	if _, err := credentialAcquire(e.Scope, c.ID); err == nil {
		t.Fatal("rate limit ignored")
	}
	old := credentialLookupIP
	t.Cleanup(func() { credentialLookupIP = old })
	credentialLookupIP = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}
	if _, err := credentialPinnedTransport(context.Background(), "https://api.example.com", false); err == nil {
		t.Fatal("private DNS rebinding accepted")
	}
}
