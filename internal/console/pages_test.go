package console_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/yamux"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
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
	// the port is remembered, so the next start keeps the pages' origin
	if raw, err := os.ReadFile(filepath.Join(filepath.Dir(r.store), "page-port")); err != nil || !strings.HasPrefix(u, "http://127.0.0.1:"+strings.TrimSpace(string(raw))+"/") {
		t.Fatalf("page port remembered: %q %v (url %s)", raw, err, u)
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

// A live page opened in the App: its own loopback port, the whole root
// forwarded to the server on the agent's computer (through the hub), with
// the helper put in; when nothing forwards it, a page that says so.
// The App serves pages on the port they had before (when it is free), so
// what pages keep in the browser stays theirs across restarts.
func TestPagesKeepTheirPortAcrossRestarts(t *testing.T) {
	r := newRig(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	os.MkdirAll(filepath.Dir(r.store), 0o700)
	if err := os.WriteFile(filepath.Join(filepath.Dir(r.store), "page-port"), []byte(strconv.Itoa(port)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	reg, err := r.admin.Register(ctx, api.RegisterRequest{Name: "maker"}, false)
	if err != nil {
		t.Fatal(err)
	}
	agent, _ := client.New(r.hubURL, reg.Token)
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, _ := z.Create("index.html")
	w.Write([]byte("<!doctype html><title>t</title>"))
	z.Close()
	p, err := agent.PublishPage(ctx, client.PageInput{Name: "p"}, bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	_, h, _ := r.do("POST", "/app/hubs", map[string]string{"name": "Main", "url": r.hubURL, "token": r.consoleToken("t")})
	code, o, raw := r.do("POST", "/app/hubs/"+h["id"].(string)+"/pages/"+p.ID+"/open", nil)
	if code != 200 || !strings.HasPrefix(o["url"].(string), "http://127.0.0.1:"+strconv.Itoa(port)+"/") {
		t.Fatalf("open on port %d: %d %s", port, code, raw)
	}
}

func TestLivePageOpensOnAPortOfItsOwn(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	reg, err := r.admin.Register(ctx, api.RegisterRequest{Name: "front"}, false)
	if err != nil {
		t.Fatal(err)
	}
	agent, _ := client.New(r.hubURL, reg.Token)
	dev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("X-Frame-Options", "DENY")
		io.WriteString(w, "<html><head></head><body>dev "+r.URL.Path+"</body></html>")
	}))
	defer dev.Close()
	port, _ := strconv.Atoi(dev.URL[strings.LastIndex(dev.URL, ":")+1:])
	p, err := agent.PublishLivePage(ctx, client.LiveInput{Name: "前端预览", Port: port})
	if err != nil {
		t.Fatal(err)
	}
	code, h, _ := r.do("POST", "/app/hubs", map[string]string{"name": "Main", "url": r.hubURL, "token": r.consoleToken("t")})
	if code != 201 {
		t.Fatalf("add hub: %d", code)
	}
	hub := h["id"].(string)
	get := func(u string) (*http.Response, string) {
		res, err := http.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	// Not forwarded yet: it says so.
	code, o, raw := r.do("POST", "/app/hubs/"+hub+"/pages/"+p.ID+"/open", nil)
	if code != 200 || o["live"] != true || o["online"] == true {
		t.Fatalf("open: %d %s", code, raw)
	}
	u := o["url"].(string)
	if res, body := get(u); res.StatusCode != 503 || !strings.Contains(body, "没有在转发") || !strings.Contains(body, "page serve --page "+p.ID) {
		t.Fatalf("offline: %d %s", res.StatusCode, body)
	}
	// Forwarded (what myco page serve does).
	fctx, stop := context.WithCancel(ctx)
	defer stop()
	conn, err := agent.DialPageTunnel(fctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	sess, err := yamux.Server(conn, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	go func() {
		for {
			st, err := sess.Accept()
			if err != nil {
				return
			}
			go func() {
				defer st.Close()
				local, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port))
				if err != nil {
					return
				}
				defer local.Close()
				go io.Copy(local, st)
				io.Copy(st, local)
			}()
		}
	}()
	// (the hub takes the tunnel up a moment after it connects)
	for i := 0; i < 100; i++ {
		if pg, err := agent.Page(ctx, p.ID); err == nil && pg.Online {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	res, body := get(u + "src/main.ts")
	if res.StatusCode != 200 || body != `<html><head><script src="/__myco/page.js"></script></head><body>dev /src/main.ts</body></html>` {
		t.Fatalf("forwarded: %d %s", res.StatusCode, body)
	}
	if res.Header.Get("X-Frame-Options") != "" || !strings.Contains(res.Header.Get("Content-Security-Policy"), "ws://127.0.0.1:") {
		t.Fatalf("headers: %v", res.Header)
	}
	// Opened again: the same port.
	if _, o2, _ := r.do("POST", "/app/hubs/"+hub+"/pages/"+p.ID+"/open", nil); o2["url"] != u || o2["online"] != true {
		t.Fatalf("again: %v", o2)
	}
}

// The App shows an app's screenshots in its new-page dialog: they come
// through the App's backend, pictures in shots/ only.
func TestTemplateShotsThroughTheApp(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	reg, err := r.admin.Register(ctx, api.RegisterRequest{Name: "maker"}, false)
	if err != nil {
		t.Fatal(err)
	}
	agent, _ := client.New(r.hubURL, reg.Token)
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, body := range map[string]string{"myco.json": `{"id": "maker/board", "name": "看板", "version": "1.0.0", "kind": "board", "data": {"cards": {"fields": {"title": "text"}}}}`,
		"index.html": "<h1>看板</h1>", "shots/1.png": "\x89PNG one"} {
		w, _ := z.Create(name)
		w.Write([]byte(body))
	}
	z.Close()
	tp, err := agent.PublishTemplate(ctx, bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	_, h, _ := r.do("POST", "/app/hubs", map[string]string{"name": "Main", "url": r.hubURL, "token": r.consoleToken("t")})
	hub := h["id"].(string)
	if code, o, raw := r.do("GET", "/app/hubs/"+hub+"/templates/"+tp.ID, nil); code != 200 || len(o["shots"].([]any)) != 1 {
		t.Fatalf("template: %d %s", code, raw)
	}
	res, err := http.Get(r.app.URL + "/app/hubs/" + hub + "/templates/" + tp.ID + "/files/shots/1.png")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || string(b) != "\x89PNG one" || res.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("shot: %d %q %v", res.StatusCode, b, res.Header)
	}
	if code, _, _ := r.do("GET", "/app/hubs/"+hub+"/templates/"+tp.ID+"/files/index.html", nil); code != 404 {
		t.Fatalf("not a shot: %d", code)
	}
}
