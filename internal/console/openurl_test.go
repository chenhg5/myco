package console

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenable(t *testing.T) {
	for in, want := range map[string]bool{
		"https://github.com/chenhg5/myco/pull/1": true,
		"http://localhost:5173/":                 true,
		"mailto:someone@example.com":             true,
		"file:///etc/passwd":                     false,
		"javascript:alert(1)":                    false,
		"myco://add-hub?link=x":                  false,
		"vscode://file/x":                        false,
		"https://":                               false,
		"/app/info":                              false,
		"":                                       false,
		"https://a.example/\nx":                  false,
	} {
		if got := openable(in); got != want {
			t.Errorf("openable(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestOpenURLRoute(t *testing.T) {
	var opened []string
	b := &Backend{OpenURL: func(u string) error { opened = append(opened, u); return nil }}
	h := b.Handler()
	post := func(body string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/app/open-url", strings.NewReader(body)))
		return rec.Code
	}
	if c := post(`{"url":"https://example.com/a"}`); c != http.StatusOK {
		t.Fatalf("https: %d", c)
	}
	if c := post(`{"url":"file:///etc/hosts"}`); c != http.StatusBadRequest {
		t.Fatalf("file: %d", c)
	}
	if len(opened) != 1 || opened[0] != "https://example.com/a" {
		t.Fatalf("opened %v", opened)
	}
}

func TestFullscreenRoute(t *testing.T) {
	put := func(b *Backend, body string) int {
		rec := httptest.NewRecorder()
		b.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/app/window/fullscreen", strings.NewReader(body)))
		return rec.Code
	}
	// A browser: the page uses the browser's own full screen.
	if c := put(&Backend{}, `{"on":true}`); c != http.StatusNotFound {
		t.Fatalf("no window: %d", c)
	}
	var got []bool
	b := &Backend{Fullscreen: func(on bool) error { got = append(got, on); return nil }}
	if c := put(b, `{"on":true}`); c != http.StatusNoContent {
		t.Fatalf("on: %d", c)
	}
	if c := put(b, `{"on":false}`); c != http.StatusNoContent {
		t.Fatalf("off: %d", c)
	}
	if c := put(b, `not json`); c != http.StatusBadRequest {
		t.Fatalf("bad body: %d", c)
	}
	if len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("calls %v", got)
	}
}
