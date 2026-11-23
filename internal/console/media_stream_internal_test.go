package console

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPublicIP(t *testing.T) {
	for ip, want := range map[string]bool{
		"8.8.8.8": true, "1.1.1.1": true, "2606:4700:4700::1111": true,
		"127.0.0.1": false, "10.1.2.3": false, "192.168.1.9": false, "172.16.0.1": false,
		"169.254.169.254": false, "100.100.1.1": false, "::1": false, "fe80::1": false, "0.0.0.0": false,
	} {
		if got := publicIP(net.ParseIP(ip)); got != want {
			t.Errorf("publicIP(%s) = %v", ip, got)
		}
	}
	for _, h := range []string{"127.0.0.1", "10.0.0.1", "localhost", "printer.local", "x.localhost"} {
		if checkHost(context.Background(), h) == nil {
			t.Errorf("checkHost(%s) allowed", h)
		}
	}
	for _, raw := range []string{"file:///etc/passwd", "ftp://x/y", "https://user:pw@example.com/a.mp3", "https:///a.mp3", "not a url"} {
		if _, ok := streamTarget(raw); ok {
			t.Errorf("streamTarget(%q) allowed", raw)
		}
	}
}

func stream(t *testing.T, target string, hdr http.Header) *http.Response {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/p/k/__myco/stream?url="+url.QueryEscape(target), nil)
	for k, v := range hdr {
		r.Header[k] = v
	}
	w := httptest.NewRecorder()
	(&Backend{}).serveStream(w, r)
	return w.Result()
}

func TestStreamPassesMediaWithRange(t *testing.T) {
	audio := strings.Repeat("0123456789", 100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ep.mp3":
			w.Header().Set("Content-Type", "audio/mpeg")
			http.ServeContent(w, r, "ep.mp3", time.Unix(1700000000, 0), strings.NewReader(audio))
		case "/moved":
			http.Redirect(w, r, "/ep.mp3", http.StatusFound)
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "<p>hi</p>")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// A local server is what a page must not reach.
	if res := stream(t, srv.URL+"/ep.mp3", nil); res.StatusCode != http.StatusForbidden {
		t.Fatalf("a local address streamed: %d", res.StatusCode)
	}

	streamPublicOnly = false
	defer func() { streamPublicOnly = true }()
	res := stream(t, srv.URL+"/moved", http.Header{"Range": {"bytes=10-19"}})
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusPartialContent || string(b) != "0123456789" || res.Header.Get("Content-Range") != "bytes 10-19/1000" ||
		res.Header.Get("Content-Type") != "audio/mpeg" || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("range: %d %q %v", res.StatusCode, b, res.Header)
	}
	if res := stream(t, srv.URL+"/page", nil); res.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("a web page was passed on: %d", res.StatusCode)
	}
	if res := stream(t, srv.URL+"/gone", nil); res.StatusCode != http.StatusBadGateway {
		t.Fatalf("a missing file: %d", res.StatusCode)
	}
}
