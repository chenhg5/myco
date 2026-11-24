package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/console"
	"github.com/zalando/go-keyring"
)

func TestHandlerServesAPIAndUI(t *testing.T) {
	keyring.MockInit()
	h := handler(&console.Backend{Store: console.NewStore(filepath.Join(t.TempDir(), "hubs.json")), Secrets: console.Keychain{}})
	for path, want := range map[string]string{"/app/hubs": "[]", "/": "<!doctype html", "/app/nope": "404"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		got := strings.ToLower(rec.Body.String())
		if want == "404" {
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s: %d, want 404 (unknown API paths must not fall back to the UI)", path, rec.Code)
			}
			continue
		}
		if rec.Code != 200 || !strings.Contains(got, want) {
			t.Errorf("%s: %d %.80q, want %q", path, rec.Code, got, want)
		}
	}
}

func TestServeGuard(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	g := guarded(inner, 4242, "k1")
	req := func(host, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://"+host+path, nil)
		r.Host = host
		if cookie != nil {
			r.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		g.ServeHTTP(rec, r)
		return rec
	}
	if rec := req("evil.example:4242", "/?key=k1", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("foreign Host (DNS rebinding): %d", rec.Code)
	}
	if rec := req("127.0.0.1:4242", "/app/hubs", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no cookie: %d", rec.Code)
	}
	if rec := req("127.0.0.1:4242", "/?key=wrong", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong key: %d", rec.Code)
	}
	rec := req("127.0.0.1:4242", "/?key=k1", nil)
	if rec.Code != http.StatusFound || len(rec.Result().Cookies()) != 1 || !rec.Result().Cookies()[0].HttpOnly {
		t.Fatalf("key exchange: %d %v", rec.Code, rec.Result().Cookies())
	}
	if rec := req("localhost:4242", "/app/hubs", rec.Result().Cookies()[0]); rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatalf("with the cookie: %d %s", rec.Code, rec.Body.String())
	}
}

func TestServeGuardRejectsRequestsFromOtherPageOrigins(t *testing.T) {
	g := guarded(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), 4242, "test-key")
	for _, tc := range []struct {
		site, origin string
		status       int
	}{
		{"same-origin", "http://127.0.0.1:4242", http.StatusNoContent},
		{"same-site", "http://127.0.0.1:4243", http.StatusForbidden},
		{"", "http://127.0.0.1:4243", http.StatusForbidden},
		{"same-site", "", http.StatusForbidden},
	} {
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4242/app/page-storage-recovery", nil)
		r.AddCookie(&http.Cookie{Name: serveCookie, Value: "test-key"})
		r.Header.Set("Sec-Fetch-Site", tc.site)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("site %q origin %q: got %d want %d", tc.site, tc.origin, w.Code, tc.status)
		}
	}
}
