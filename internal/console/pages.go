package console

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
)

// Pages: web pages agents publish to a hub, opened
// in the App's page panel.
//
//	GET  /app/hubs/{id}/pages?project=          {pages}
//	GET  /app/hubs/{id}/pages/{pid}             page
//	POST /app/hubs/{id}/pages/{pid}/open        {version?} → {url, version, name, entry}
//	DELETE /app/hubs/{id}/pages/{pid}
//
// A page's files are not served from the App's own origin (they are code an
// agent wrote): this backend serves them on a loopback port of its own, at
// /p/<key>/<path>, where the key is made when the page is opened, names one
// version of one page, and lives as long as the App. So a page cannot read
// the App's storage or call /app/, nor guess another page's address.

// pageCSP: a page loads only its own files (and data:, blob:) and cannot
// send anything to the internet.
const pageCSP = "default-src 'self' data: blob:; script-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; " +
	"style-src 'self' 'unsafe-inline' data: blob:; img-src 'self' data: blob:; font-src 'self' data:; media-src 'self' data: blob:; " +
	"connect-src 'self' data: blob:; worker-src 'self' blob:; frame-src 'self' data: blob:; form-action 'self'; base-uri 'self'; object-src 'none'"

// pageHelper is put at the top of every html file of a page: a link to
// elsewhere opens in the system's browser (the App is asked), not in the
// panel.
const pageHelper = `<script src="/__myco/page.js"></script>`

// pageHelperJS is the helper and the myco bridge (pagebridge.js).
//
//go:embed pagebridge.js
var pageHelperJS string

type pageKey struct {
	hub, page, name, entry string
	version                int
}

type pageServer struct {
	mu    sync.Mutex
	port  int
	err   error
	keys  map[string]pageKey
	order []string
	// live pages opened: a loopback port each (hub/page → port)
	live map[string]int
}

const maxPageKeys = 500

func (b *Backend) pageServer() (*pageServer, error) {
	b.pagesOnce.Do(func() {
		ps := &pageServer{keys: map[string]pageKey{}}
		b.pages = ps
		ln, err := b.listenPages()
		if err != nil {
			ps.err = err
			return
		}
		ps.port = ln.Addr().(*net.TCPAddr).Port
		srv := &http.Server{Handler: b.pageHandler(ps), ReadHeaderTimeout: 10 * time.Second}
		go func() { _ = srv.Serve(ln) }()
	})
	return b.pages, b.pages.err
}

// listenPages listens on the port the pages had the last time, when it is
// free. What a page keeps in the browser (IndexedDB, localStorage) belongs
// to its origin, port included: the same port keeps it across restarts of
// the App (视频剪辑 keeps the videos there). The port is remembered next to
// the App's hubs.json; without a store it is any free port.
func (b *Backend) listenPages() (net.Listener, error) {
	file := ""
	if b.Store != nil && b.Store.path != "" {
		file = filepath.Join(filepath.Dir(b.Store.path), "page-port")
	}
	if file != "" {
		if raw, err := os.ReadFile(file); err == nil {
			if p, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && p > 1024 && p < 65536 {
				if ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p)); err == nil {
					return ln, nil
				}
			}
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	if file != "" && os.MkdirAll(filepath.Dir(file), 0o700) == nil {
		_ = os.WriteFile(file, []byte(strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)+"\n"), 0o600)
	}
	return ln, nil
}

func (ps *pageServer) add(k pageKey) string {
	buf := make([]byte, 20)
	_, _ = rand.Read(buf)
	key := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf))
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.keys[key] = k
	ps.order = append(ps.order, key)
	for len(ps.order) > maxPageKeys {
		delete(ps.keys, ps.order[0])
		ps.order = ps.order[1:]
	}
	return key
}

func (ps *pageServer) get(key string) (pageKey, bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	k, ok := ps.keys[key]
	return k, ok
}

func (b *Backend) pageRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/hubs/{id}/pages", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		ps, err := c.Pages(r.Context(), r.URL.Query().Get("project"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if ps == nil {
			ps = []api.Page{}
		}
		writeJSON(w, http.StatusOK, api.PageList{Pages: ps})
	}))
	m.HandleFunc("GET /app/hubs/{id}/pages/{pid}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		p, err := c.Page(r.Context(), r.PathValue("pid"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	}))
	m.HandleFunc("DELETE /app/hubs/{id}/pages/{pid}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		if err := c.DeletePage(r.Context(), r.PathValue("pid")); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	m.HandleFunc("GET /app/hubs/{id}/pages/{pid}/state", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		st, err := c.PageState(r.Context(), r.PathValue("pid"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	}))
	m.HandleFunc("PUT /app/hubs/{id}/pages/{pid}/state", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.PageStateRequest
		if !decode(w, r, &in) {
			return
		}
		st, err := c.PutPageState(r.Context(), r.PathValue("pid"), in.State, in.IfVersion)
		if err != nil {
			if st != nil {
				e, _ := client.AsError(err)
				msg := "the page's state changed"
				if e != nil {
					msg = e.Message
				}
				writeJSON(w, http.StatusConflict, map[string]any{"error": "conflict", "message": msg, "latest": st})
				return
			}
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	}))
	m.HandleFunc("POST /app/hubs/{id}/pages/{pid}/open", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in struct {
			Version int `json:"version"`
		}
		if r.ContentLength != 0 && !decode(w, r, &in) {
			return
		}
		p, err := c.Page(r.Context(), r.PathValue("pid"))
		if err != nil {
			writeErr(w, err)
			return
		}
		v := api.PageVersion{N: p.Version, Entry: p.Entry}
		if in.Version > 0 {
			found := false
			for _, x := range p.Versions {
				if x.N == in.Version {
					v, found = x, true
				}
			}
			if !found {
				writeErr(w, &client.Error{Code: "not_found", Message: fmt.Sprintf("page %s has no version %d", p.ID, in.Version), Status: http.StatusNotFound})
				return
			}
		}
		ps, err := b.pageServer()
		if err != nil {
			writeErr(w, err)
			return
		}
		if p.Live {
			port, err := b.livePort(ps, r.PathValue("id"), p.ID)
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"url": fmt.Sprintf("http://127.0.0.1:%d/", port), "version": 0, "latest": 0,
				"name": p.Name, "entry": "", "live": true, "online": p.Online, "port": p.Port})
			return
		}
		key := ps.add(pageKey{hub: r.PathValue("id"), page: p.ID, version: v.N, name: p.Name, entry: v.Entry})
		writeJSON(w, http.StatusOK, map[string]any{
			"url":     fmt.Sprintf("http://127.0.0.1:%d/p/%s/%s", ps.port, key, escPath(v.Entry)),
			"version": v.N, "latest": p.Version, "name": p.Name, "entry": v.Entry,
		})
	}))
}

func escPath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

// pageHandler serves the files of opened pages on the page server's port.
func (b *Backend) pageHandler(ps *pageServer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only this computer, by this address (not a name that could be
		// pointed here from elsewhere).
		if r.Host != "127.0.0.1:"+strconv.Itoa(ps.port) && r.Host != "localhost:"+strconv.Itoa(ps.port) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "read only", http.StatusMethodNotAllowed)
			return
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		if r.URL.Path == "/__myco/page.js" {
			h.Set("Content-Type", "text/javascript; charset=utf-8")
			h.Set("Cache-Control", "no-cache")
			_, _ = io.WriteString(w, pageHelperJS)
			return
		}
		rest, ok := strings.CutPrefix(r.URL.Path, "/p/")
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		key, file, slash := strings.Cut(rest, "/")
		k, ok := ps.get(key)
		if !ok {
			h.Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `<!doctype html><meta charset="utf-8"><p style="font:14px system-ui;color:#85858d;margin:40px;text-align:center">这个网页的地址已经失效了，请在 myco App 里重新打开。</p>`)
			return
		}
		if !slash {
			http.Redirect(w, r, "/p/"+key+"/", http.StatusFound)
			return
		}
		if file == "__myco/media" {
			b.serveMedia(w, r)
			return
		}
		if file == "__myco/stream" {
			b.serveStream(w, r)
			return
		}
		hub, err := b.find(k.hub)
		if err != nil {
			writeErr(w, err)
			return
		}
		c, err := b.client(hub)
		if err != nil {
			writeErr(w, err)
			return
		}
		fwd := http.Header{}
		if v := r.Header.Get("If-None-Match"); v != "" {
			fwd.Set("If-None-Match", v)
		}
		res, err := c.OpenPageFile(r.Context(), k.page, strconv.Itoa(k.version), file, fwd)
		if err != nil {
			if e, ok := client.AsError(err); ok && e.Status == http.StatusNotFound {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			writeErr(w, err)
			return
		}
		defer res.Body.Close()
		h.Set("Content-Security-Policy", pageCSP)
		h.Set("Cache-Control", "private, max-age=31536000, immutable")
		if et := res.Header.Get("ETag"); et != "" {
			h.Set("ETag", et)
		}
		if res.StatusCode == http.StatusNotModified {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		ct := res.Header.Get("Content-Type")
		h.Set("Content-Type", ct)
		if !strings.HasPrefix(ct, "text/html") {
			if cl := res.Header.Get("Content-Length"); cl != "" {
				h.Set("Content-Length", cl)
			}
			if r.Method == http.MethodGet {
				_, _ = io.Copy(w, res.Body)
			}
			return
		}
		// An html file: the helper goes in first.
		page, err := io.ReadAll(io.LimitReader(res.Body, 20<<20))
		if err != nil {
			writeErr(w, err)
			return
		}
		out := withHelper(page)
		h.Set("Content-Length", strconv.Itoa(len(out)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(out)
		}
	})
}

// withHelper puts the page helper after <head> (or at the very top).
func withHelper(page []byte) []byte {
	lower := bytes.ToLower(page[:min(len(page), 4096)])
	at := 0
	if i := bytes.Index(lower, []byte("<head")); i >= 0 {
		if j := bytes.IndexByte(lower[i:], '>'); j >= 0 {
			at = i + j + 1
		}
	} else if i := bytes.Index(lower, []byte("<html")); i >= 0 {
		if j := bytes.IndexByte(lower[i:], '>'); j >= 0 {
			at = i + j + 1
		}
	} else if bytes.HasPrefix(bytes.TrimSpace(lower), []byte("<!doctype")) {
		if j := bytes.IndexByte(lower, '>'); j >= 0 {
			at = j + 1
		}
	}
	out := make([]byte, 0, len(page)+len(pageHelper))
	out = append(out, page[:at]...)
	out = append(out, pageHelper...)
	return append(out, page[at:]...)
}

// livePort is the loopback port a live page is opened on: its own, since
// a dev server's paths start at the root (/src/main.ts, /@vite/client).
// Everything on it goes to the hub's /v1/pages/{id}/live/…, as the person
// signed in there, and on to the server on the agent's computer.
func (b *Backend) livePort(ps *pageServer, hubID, pageID string) (int, error) {
	k := hubID + "/" + pageID
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if port, ok := ps.live[k]; ok {
		return port, nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	srv := &http.Server{Handler: b.liveHandler(hubID, pageID, port), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	if ps.live == nil {
		ps.live = map[string]int{}
	}
	ps.live[k] = port
	return port, nil
}

func liveCSP(port int) string {
	ws := fmt.Sprintf("ws://127.0.0.1:%d ws://localhost:%d", port, port)
	return strings.Replace(pageCSP, "connect-src 'self' data: blob:", "connect-src 'self' "+ws+" data: blob:", 1)
}

const liveOffline = `<!doctype html><meta charset="utf-8"><div style="font:14px/1.7 system-ui;color:#85858d;margin:56px auto;max-width:420px;text-align:center">` +
	`<p style="font-size:15px;color:#4b4b53">这个实时网页现在没有在转发</p><p>%s</p></div>`

func (b *Backend) liveHandler(hubID, pageID string, port int) http.Handler {
	rp := &httputil.ReverseProxy{
		Transport: client.TunnelTransport(),
		Rewrite: func(pr *httputil.ProxyRequest) {
			hub, err := b.find(hubID)
			if err != nil {
				return
			}
			base, err := url.Parse(strings.TrimRight(hub.URL, "/"))
			if err != nil {
				return
			}
			tok, _ := b.Secrets.Get(hub.ID)
			pr.Out.URL.Scheme, pr.Out.URL.Host = base.Scheme, base.Host
			prefix := base.EscapedPath() + "/v1/pages/" + url.PathEscape(pageID) + "/live"
			pr.Out.URL.Path = base.Path + "/v1/pages/" + pageID + "/live" + pr.In.URL.Path
			pr.Out.URL.RawPath = prefix + pr.In.URL.EscapedPath()
			pr.Out.URL.RawQuery = pr.In.URL.RawQuery
			pr.Out.Host = base.Host
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("Origin")
			pr.Out.Header.Set("Authorization", "Bearer "+tok)
		},
		ModifyResponse: func(res *http.Response) error {
			h := res.Header
			if res.StatusCode == http.StatusSwitchingProtocols {
				return nil
			}
			h.Del("X-Frame-Options")
			h.Del("Set-Cookie")
			h.Set("Content-Security-Policy", liveCSP(port))
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "no-referrer")
			if res.StatusCode == http.StatusServiceUnavailable || res.StatusCode == http.StatusBadGateway {
				var e struct {
					Message    string `json:"message"`
					Suggestion string `json:"suggestion"`
				}
				raw, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
				res.Body.Close()
				_ = json.Unmarshal(raw, &e)
				why := "agent 那边的 myco page serve 没在运行，或者开发服务器没起来。"
				if e.Suggestion != "" {
					why += "<br><code>" + html.EscapeString(e.Suggestion) + "</code>"
				}
				out := []byte(fmt.Sprintf(liveOffline, why))
				h.Set("Content-Type", "text/html; charset=utf-8")
				h.Set("Cache-Control", "no-store")
				h.Del("Content-Encoding")
				res.Body = io.NopCloser(bytes.NewReader(out))
				res.ContentLength = int64(len(out))
				h.Set("Content-Length", strconv.Itoa(len(out)))
				return nil
			}
			if !strings.HasPrefix(h.Get("Content-Type"), "text/html") || h.Get("Content-Encoding") != "" {
				return nil
			}
			page, err := io.ReadAll(io.LimitReader(res.Body, 20<<20))
			res.Body.Close()
			if err != nil {
				return err
			}
			out := withHelper(page)
			res.Body = io.NopCloser(bytes.NewReader(out))
			res.ContentLength = int64(len(out))
			h.Set("Content-Length", strconv.Itoa(len(out)))
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, liveOffline, "连不上工作区的 Hub。")
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "127.0.0.1:"+strconv.Itoa(port) && r.Host != "localhost:"+strconv.Itoa(port) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.URL.Path == "/__myco/page.js" {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = io.WriteString(w, pageHelperJS)
			return
		}
		rp.ServeHTTP(w, r)
	})
}

// Recordings an app made (myco.os audio.record) play in the page as they
// are, by an address on the page's own origin (myco.os.mediaURL):
// <audio> seeks in them (ranges) and changes speed without changing the
// voice, which reading them through the bridge could not do. Only files in
// ~/.myco/recordings, and only in the App on this computer.
var mediaTypes = map[string]string{".wav": "audio/wav", ".m4a": "audio/mp4", ".mp3": "audio/mpeg", ".ogg": "audio/ogg", ".opus": "audio/ogg", ".webm": "video/webm", ".mp4": "video/mp4", ".mov": "video/quicktime"}

// mediaPath is the recording a page asks for, when it may have it.
func mediaPath(home, p string) (string, bool) {
	if home == "" || p == "" {
		return "", false
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		p = filepath.Join(home, p[2:])
	}
	if !filepath.IsAbs(p) {
		return "", false
	}
	p = filepath.Clean(p)
	root := filepath.Join(home, ".myco", "recordings") + string(filepath.Separator)
	if !strings.HasPrefix(p, root) || mediaTypes[strings.ToLower(filepath.Ext(p))] == "" {
		return "", false
	}
	if real, err := filepath.EvalSymlinks(p); err != nil || !strings.HasPrefix(real, root) && !strings.HasPrefix(real, evalOr(root)) {
		return "", false
	}
	return p, true
}

func evalOr(p string) string {
	if r, err := filepath.EvalSymlinks(strings.TrimSuffix(p, string(filepath.Separator))); err == nil {
		return r + string(filepath.Separator)
	}
	return p
}

func (b *Backend) serveMedia(w http.ResponseWriter, r *http.Request) {
	if b.Local == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	p, ok := mediaPath(b.Local.Home, r.URL.Query().Get("path"))
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	f, err := os.Open(p)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", mediaTypes[strings.ToLower(filepath.Ext(p))])
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, filepath.Base(p), fi.ModTime(), f)
}
