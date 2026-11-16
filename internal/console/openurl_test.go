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
