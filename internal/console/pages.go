package console

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
)

// Pages (docs/design/browser.md): web pages agents publish to a hub, opened
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

const pageHelperJS = `(function(){
  function out(u){ try { var x = new URL(u, location.href); return x.origin !== location.origin && /^(https?|mailto):$/.test(x.protocol) ? x.href : ""; } catch (e) { return ""; } }
  document.addEventListener("click", function(e){
    if (e.defaultPrevented || e.button !== 0) return;
    var a = e.target && e.target.closest ? e.target.closest("a[href]") : null;
    if (!a) return;
    var u = out(a.getAttribute("href"));
    if (!u) return;
    e.preventDefault();
    parent.postMessage({ myco: "open-url", url: u }, "*");
  }, true);
  var open = window.open;
  window.open = function(u){ var x = out(String(u || "")); if (x) { parent.postMessage({ myco: "open-url", url: x }, "*"); return null; } return open.apply(window, arguments); };
})();`

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
}

const maxPageKeys = 500

func (b *Backend) pageServer() (*pageServer, error) {
	b.pagesOnce.Do(func() {
		ps := &pageServer{keys: map[string]pageKey{}}
		b.pages = ps
		ln, err := net.Listen("tcp", "127.0.0.1:0")
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
