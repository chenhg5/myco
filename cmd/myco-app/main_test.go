package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/console"
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
