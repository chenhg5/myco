package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// A hub on this computer, joined at a LAN address it no longer has: once
// the address fails and the hub on 127.0.0.1 knows the token, requests go
// there (bodies sent again), and stay there.
func TestLANHubFallsBackToThisComputer(t *testing.T) {
	var got atomic.Value
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		b, _ := io.ReadAll(r.Body)
		got.Store(r.Method + " " + r.URL.Path + " " + string(b))
		io.WriteString(w, `{"name":"mac-machine"}`)
	}))
	defer hub.Close()
	port := hub.URL[strings.LastIndex(hub.URL, ":")+1:]
	u, _ := url.Parse("http://192.168.66.160:" + port + "/w/team")

	var direct atomic.Int32
	base := rtFunc(func(r *http.Request) (*http.Response, error) {
		direct.Add(1)
		if r.URL.Host != u.Host {
			t.Errorf("direct request to %s", r.URL.Host)
		}
		if r.Body != nil {
			io.ReadAll(r.Body) // a proxy reads it, then says 503
		}
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("proxy")), Request: r}, nil
	})
	c := &Client{base: u, token: "good", http: &http.Client{Transport: forLANHub(u, base)}}
	if err := c.do(t.Context(), http.MethodPost, "/v1/runtime", nil, map[string]string{"cli": "codex"}, nil); err != nil {
		t.Fatal(err)
	}
	if g := got.Load(); g != `POST /w/team/v1/runtime {"cli":"codex"}`+"\n" && g != `POST /w/team/v1/runtime {"cli":"codex"}` {
		t.Fatalf("hub got %q", g)
	}
	if _, err := c.Me(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n := direct.Load(); n != 1 {
		t.Fatalf("%d requests went to the old address, want 1", n)
	}

	// Another hub's token: the failure stands, nothing is redirected.
	other := &Client{base: u, token: "other", http: &http.Client{Transport: forLANHub(u, base)}}
	if _, err := other.Me(t.Context()); err == nil {
		t.Fatal("a token this computer's hub does not know reached it")
	}
}

func TestLANHubOnlyForPrivateAddresses(t *testing.T) {
	for _, raw := range []string{"https://hub.example.com", "http://127.0.0.1:7777", "http://8.8.8.8:7777", "http://192.168.1.2"} {
		u, _ := url.Parse(raw)
		if _, ok := forLANHub(u, hubTransport).(*lanHub); ok {
			t.Errorf("%s: wrapped", raw)
		}
	}
	u, _ := url.Parse("http://10.0.0.5:7777")
	if _, ok := forLANHub(u, hubTransport).(*lanHub); !ok {
		t.Error("10.0.0.5:7777 not wrapped")
	}
}
