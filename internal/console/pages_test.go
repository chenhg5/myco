package console_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
)

// A page opened in the App: served on a loopback port of its own, under a
// key made when it was opened, with the helper that sends outside links to
// the browser, a CSP, and nothing for a wrong key or host.
func TestPagesOpenOnTheirOwnOrigin(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	reg, err := r.admin.Register(ctx, api.RegisterRequest{Name: "maker"}, false)
	if err != nil {
		t.Fatal(err)
	}
	agent, _ := client.New(r.hubURL, reg.Token)
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, body := range map[string]string{"index.html": "<!doctype html><html><head><title>t</title></head><body>v1</body></html>", "app.js": "var x=1"} {
		w, _ := z.Create(name)
		w.Write([]byte(body))
	}
	z.Close()
	p, err := agent.PublishPage(ctx, client.PageInput{Name: "报告"}, bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	code, h, _ := r.do("POST", "/app/hubs", map[string]string{"name": "Main", "url": r.hubURL, "token": r.consoleToken("t")})
	if code != 201 {
		t.Fatalf("add hub: %d %v", code, h)
	}
	hub := h["id"].(string)
	if code, l, _ := r.do("GET", "/app/hubs/"+hub+"/pages", nil); code != 200 || len(l["pages"].([]any)) != 1 {
		t.Fatalf("list: %d %v", code, l)
	}
	code, o, raw := r.do("POST", "/app/hubs/"+hub+"/pages/"+p.ID+"/open", nil)
	if code != 200 || o["version"] != float64(1) || o["name"] != "报告" {
		t.Fatalf("open: %d %s", code, raw)
	}
	u := o["url"].(string)
	if !strings.HasPrefix(u, "http://127.0.0.1:") || !strings.HasSuffix(u, "/index.html") || strings.HasPrefix(u, r.app.URL) {
		t.Fatalf("its own origin: %s (app %s)", u, r.app.URL)
	}
	get := func(url, host string) (*http.Response, string) {
		req, _ := http.NewRequest("GET", url, nil)
		if host != "" {
			req.Host = host
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	res, body := get(u, "")
	if res.StatusCode != 200 || !strings.Contains(body, "<head><script src=\"/__myco/page.js\"></script><title>") ||
		!strings.Contains(res.Header.Get("Content-Security-Policy"), "connect-src 'self'") || res.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("entry: %d %v %s", res.StatusCode, res.Header, body)
	}
	base := u[:strings.LastIndex(u, "/")+1]
	if res, body := get(base+"app.js", ""); res.StatusCode != 200 || body != "var x=1" {
		t.Fatalf("a file: %d %s", res.StatusCode, body)
	}
	if res, body := get(u[:strings.Index(u, "/p/")]+"/__myco/page.js", ""); res.StatusCode != 200 || !strings.Contains(body, "open-url") {
		t.Fatalf("helper: %d", res.StatusCode)
	}
	// A wrong key, a missing file, another host name: nothing.
	if res, _ := get(strings.Replace(u, "/p/", "/p/x", 1), ""); res.StatusCode != 404 {
		t.Fatalf("wrong key: %d", res.StatusCode)
	}
	if res, _ := get(base+"nope.js", ""); res.StatusCode != 404 {
		t.Fatalf("missing: %d", res.StatusCode)
	}
	if res, _ := get(u, "evil.example:80"); res.StatusCode != 404 {
		t.Fatalf("other host: %d", res.StatusCode)
	}
	// A new version: an open key stays on its version; opening again gets it.
	buf.Reset()
	z = zip.NewWriter(&buf)
	w, _ := z.Create("index.html")
	w.Write([]byte("<p>v2</p>"))
	z.Close()
	if _, err := agent.PublishPage(ctx, client.PageInput{Page: p.ID}, bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
		t.Fatal(err)
	}
	if _, body := get(u, ""); !strings.Contains(body, "v1") {
		t.Fatalf("the open version stays: %s", body)
	}
	_, o, _ = r.do("POST", "/app/hubs/"+hub+"/pages/"+p.ID+"/open", nil)
	if _, body := get(o["url"].(string), ""); !strings.HasPrefix(body, `<script src="/__myco/page.js"></script><p>v2`) {
		t.Fatalf("reopened: %s", body)
	}
	// Its shared state, through the App: written, and a stale write answers
	// 409 with the latest.
	if code, st, raw := r.do("PUT", "/app/hubs/"+hub+"/pages/"+p.ID+"/state", map[string]any{"state": map[string]int{"n": 1}, "if_version": 0}); code != 200 || st["version"] != float64(1) {
		t.Fatalf("state: %d %s", code, raw)
	}
	if code, st, raw := r.do("PUT", "/app/hubs/"+hub+"/pages/"+p.ID+"/state", map[string]any{"state": map[string]int{"n": 2}, "if_version": 0}); code != 409 || st["latest"].(map[string]any)["version"] != float64(1) {
		t.Fatalf("stale: %d %s", code, raw)
	}
	if code, st, _ := r.do("GET", "/app/hubs/"+hub+"/pages/"+p.ID+"/state", nil); code != 200 || st["state"].(map[string]any)["n"] != float64(1) {
		t.Fatalf("read: %d %v", code, st)
	}
	if code, _, _ := r.do("POST", "/app/hubs/"+hub+"/pages/"+p.ID+"/open", map[string]int{"version": 9}); code != 404 {
		t.Fatalf("no such version: %d", code)
	}
}
