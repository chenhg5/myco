package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// SetPages enables /v1/pages (web pages agents publish; docs/design/browser.md).
func (s *Server) SetPages(p *app.PageService) { s.pages = p }

// pageSandbox is the CSP every file of a page is served with: whoever opens
// one straight from the hub gets it in a sandbox of its own (an opaque
// origin), so it cannot act as the hub's own pages.
const pageSandbox = "sandbox allow-scripts allow-forms allow-popups allow-modals allow-downloads"

func (s *Server) pageRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/pages", s.pageCaller(s.listPages))
	m.HandleFunc("POST /v1/pages", s.pageCaller(s.publishPage))
	m.HandleFunc("GET /v1/pages/{id}", s.pageCaller(s.getPage))
	m.HandleFunc("POST /v1/pages/{id}/versions", s.pageCaller(s.publishPage))
	m.HandleFunc("DELETE /v1/pages/{id}", s.pageCaller(s.deletePage))
	m.HandleFunc("GET /v1/pages/{id}/v/{n}/{path...}", s.pageCaller(s.pageFile))
	m.HandleFunc("GET /v1/pages/{id}/state", s.pageCaller(s.getPageState))
	m.HandleFunc("PUT /v1/pages/{id}/state", s.pageCaller(s.putPageState))
	// A page's address, opened in a browser: say where it opens.
	m.HandleFunc("GET /pages/{id}", s.pageLink)
}

type pageCall struct {
	by   app.Actor
	name string // who acts, for display
}

// pageCaller lets both agents and people (console tokens) use pages.
func (s *Server) pageCaller(h func(http.ResponseWriter, *http.Request, pageCall)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.pages == nil {
			writeError(w, domain.Errf(domain.CodeNotFound, "pages are not enabled on this hub").
				WithSuggestion("upgrade the hub to v0.19.24 or later"))
			return
		}
		if s.isAdmin(r) {
			name, _ := s.consoleIdentity(r)
			h(w, r, pageCall{by: s.actorOf(r), name: name})
			return
		}
		s.agent(func(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
			h(w, r, pageCall{by: app.Actor{Agent: a}, name: string(a.Name)})
		})(w, r)
	}
}

func (s *Server) pageDTO(r *http.Request, by app.Actor, v *app.PageView) api.Page {
	p := v.Page
	out := api.Page{ID: p.ID, Name: p.Name, Project: p.Project, Publisher: p.Publisher, Version: p.Latest,
		URL: s.publicBase(r) + "/pages/" + p.ID, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
		CanManage: s.pages.Manages(by, p), Versions: []api.PageVersion{}, Live: p.Live, Port: p.Port, Template: s.pageTemplate(r.Context(), p)}
	if p.Live {
		out.Online = s.online(p.ID)
	}
	for i, x := range v.Versions {
		if i == 0 {
			out.Entry = x.Entry
		}
		out.Versions = append(out.Versions, api.PageVersion{N: x.N, Entry: x.Entry, Files: x.Files, Size: x.Size, SHA256: x.SHA256, By: x.By, At: x.At})
	}
	return out
}

func (s *Server) listPages(w http.ResponseWriter, r *http.Request, c pageCall) {
	vs, err := s.pages.List(r.Context(), c.by, r.URL.Query().Get("project"))
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.PageList{Pages: []api.Page{}}
	for _, v := range vs {
		out.Pages = append(out.Pages, s.pageDTO(r, c.by, v))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getPage(w http.ResponseWriter, r *http.Request, c pageCall) {
	v, err := s.pages.Get(r.Context(), c.by, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.pageDTO(r, c.by, v))
}

// publishPage: POST /v1/pages?name=&project=&entry= (a new page) or POST
// /v1/pages/{id}/versions?entry=&name= (a new version); the body is a zip.
func (s *Server) publishPage(w http.ResponseWriter, r *http.Request, c pageCall) {
	q := r.URL.Query()
	in := app.PublishInput{Page: r.PathValue("id"), Name: q.Get("name"), Project: q.Get("project"), Entry: q.Get("entry"), By: c.name}
	body := http.MaxBytesReader(w, r.Body, domain.MaxPageSize+1)
	v, err := s.pages.Publish(r.Context(), c.by, in, body)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusCreated
	if in.Page != "" {
		status = http.StatusOK
	}
	writeJSON(w, status, s.pageDTO(r, c.by, v))
}

func (s *Server) deletePage(w http.ResponseWriter, r *http.Request, c pageCall) {
	if _, err := s.pages.Delete(r.Context(), c.by, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pageFile: GET /v1/pages/{id}/v/{n}/{path} (n a number or "latest"; an
// empty path is the entry file).
func (s *Server) pageFile(w http.ResponseWriter, r *http.Request, c pageCall) {
	n := 0
	if raw := r.PathValue("n"); raw != "latest" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "version is a number or latest"))
			return
		}
		n = v
	}
	f, err := s.pages.Open(r.Context(), c.by, r.PathValue("id"), n, r.PathValue("path"))
	if err != nil {
		writeError(w, err)
		return
	}
	defer f.Close()
	h := w.Header()
	h.Set("Content-Type", f.ContentType)
	h.Set("Content-Length", strconv.FormatInt(f.Size, 10))
	h.Set("Content-Security-Policy", pageSandbox)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Myco-Page-Version", strconv.Itoa(f.Version.N))
	h.Set("X-Myco-Page-Path", f.Path)
	etag := fmt.Sprintf(`"%s-%s"`, f.Version.SHA256[:min(16, len(f.Version.SHA256))], strconv.Quote(f.Path))
	h.Set("ETag", etag)
	if n == 0 {
		h.Set("Cache-Control", "private, no-cache")
	} else {
		h.Set("Cache-Control", "private, max-age=31536000, immutable")
	}
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = io.Copy(w, f)
}

// pageLink is what a page's address shows in a browser: it opens in the
// myco App (whether it exists is not said: that needs a token).
func (s *Server) pageLink(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>myco 网页</title>
<style>body{font:15px/1.6 -apple-system,system-ui,sans-serif;max-width:520px;margin:12vh auto;padding:0 20px;color:#1c1c21}code{background:#f1f1f4;padding:1px 5px;border-radius:4px}</style>
<h2>这是 myco 里的一个网页</h2>
<p>在 myco App 里点开这个链接就能看。也可以用命令行把它下载下来：</p>
<p><code>myco page get --page %s</code></p>
<p style="color:#85858d">A page published in myco: open this link in the myco App.</p>`, html.EscapeString(strings.TrimSpace(id)))
}

func pageStateDTO(st *domain.PageState) api.PageState {
	out := api.PageState{State: json.RawMessage("null"), Version: st.Version, UpdatedBy: st.UpdatedBy}
	if st.State != "" {
		out.State = json.RawMessage(st.State)
	}
	if !st.UpdatedAt.IsZero() {
		t := st.UpdatedAt
		out.UpdatedAt = &t
	}
	return out
}

func (s *Server) getPageState(w http.ResponseWriter, r *http.Request, c pageCall) {
	st, err := s.pages.State(r.Context(), c.by, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pageStateDTO(st))
}

// putPageState: {state, if_version} → the new state; 409 with the latest
// when someone wrote first.
func (s *Server) putPageState(w http.ResponseWriter, r *http.Request, c pageCall) {
	var req api.PageStateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, domain.MaxPageState+4096)).Decode(&req); err != nil {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "the body is {state, if_version} (the state at most %d KB)", domain.MaxPageState>>10))
		return
	}
	if len(req.State) == 0 {
		req.State = json.RawMessage("null")
	}
	st, err := s.pages.PutState(r.Context(), c.by, r.PathValue("id"), string(req.State), req.IfVersion, c.name)
	if err != nil {
		var de *domain.Error
		if st != nil && errors.As(err, &de) && de.Code == domain.CodeConflict {
			writeJSON(w, http.StatusConflict, api.PageStateConflict{
				Error:  api.Error{Code: string(de.Code), Message: de.Message, Suggestion: de.Suggestion},
				Latest: pageStateDTO(st),
			})
			return
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pageStateDTO(st))
}
