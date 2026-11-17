package httpapi

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/hashicorp/yamux"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// Live pages (docs/design/browser.md, step 3): a server on an agent's
// computer (a dev server on localhost:5173), forwarded through the hub.
// The agent's `myco page serve` keeps a WebSocket to
// /v1/pages/{id}/tunnel; on it, yamux carries one stream per connection
// the hub opens to that server. /v1/pages/{id}/live/… is the server, for
// whoever may see the page: plain requests and WebSockets (hot reload).

type tunnel struct {
	sess  *yamux.Session
	port  int
	since time.Time
	proxy *httputil.ReverseProxy
}

type tunnels struct {
	mu sync.Mutex
	m  map[string]*tunnel
}

func (t *tunnels) get(id string) *tunnel {
	t.mu.Lock()
	defer t.mu.Unlock()
	tu := t.m[id]
	if tu != nil && tu.sess.IsClosed() {
		delete(t.m, id)
		return nil
	}
	return tu
}

// online says whether a live page is forwarded now.
func (s *Server) online(id string) bool { return s.tun.get(id) != nil }

func (s *Server) liveRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /v1/pages/live", s.pageCaller(s.publishLive))
	m.HandleFunc("GET /v1/pages/{id}/tunnel", s.pageCaller(s.pageTunnel))
	m.HandleFunc("/v1/pages/{id}/live/{path...}", s.pageCaller(s.liveRequest))
}

// publishLive: POST /v1/pages/live?name=&project=&port=&page= (an agent).
func (s *Server) publishLive(w http.ResponseWriter, r *http.Request, c pageCall) {
	q := r.URL.Query()
	port, _ := strconv.Atoi(q.Get("port"))
	in := app.LiveInput{Page: q.Get("page"), Name: q.Get("name"), Project: q.Get("project"), Port: port, By: c.name}
	v, err := s.pages.PublishLive(r.Context(), c.by, in)
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

// pageTunnel: the agent forwarding a live page connects here (WebSocket);
// a newer connection replaces an older one.
func (s *Server) pageTunnel(w http.ResponseWriter, r *http.Request, c pageCall) {
	p, err := s.pages.Serves(r.Context(), c.by, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return // (Accept answered)
	}
	ws.SetReadLimit(64 << 20)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := websocket.NetConn(ctx, ws, websocket.MessageBinary)
	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	cfg.KeepAliveInterval = 20 * time.Second
	cfg.ConnectionWriteTimeout = 30 * time.Second
	sess, err := yamux.Client(conn, cfg)
	if err != nil {
		_ = ws.Close(websocket.StatusInternalError, "tunnel")
		return
	}
	tu := &tunnel{sess: sess, port: p.Port, since: time.Now()}
	tu.proxy = s.liveProxy(tu)
	s.tun.mu.Lock()
	if s.tun.m == nil {
		s.tun.m = map[string]*tunnel{}
	}
	if old := s.tun.m[p.ID]; old != nil {
		_ = old.sess.Close()
	}
	s.tun.m[p.ID] = tu
	s.tun.mu.Unlock()
	select {
	case <-sess.CloseChan():
	case <-r.Context().Done():
	}
	_ = sess.Close()
	s.tun.mu.Lock()
	if s.tun.m[p.ID] == tu {
		delete(s.tun.m, p.ID)
	}
	s.tun.mu.Unlock()
}

// liveProxy forwards to the server on the agent's computer, over the
// tunnel's streams. The caller's credentials stay here.
func (s *Server) liveProxy(tu *tunnel) *httputil.ReverseProxy {
	host := "localhost:" + strconv.Itoa(tu.port)
	tr := &http.Transport{
		DialContext:         func(context.Context, string, string) (net.Conn, error) { return tu.sess.Open() },
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     60 * time.Second,
		DisableCompression:  true,
	}
	return &httputil.ReverseProxy{
		Transport: tr,
		Rewrite: func(pr *httputil.ProxyRequest) {
			rest, raw := livePath(pr.In)
			pr.Out.URL.Scheme, pr.Out.URL.Host = "http", host
			pr.Out.URL.Path, pr.Out.URL.RawPath = rest, raw
			pr.Out.URL.RawQuery = pr.In.URL.RawQuery
			pr.Out.Host = host
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Del("Cookie")
			// A dev server checks where a request comes from; to it, this
			// is its own computer.
			if pr.Out.Header.Get("Origin") != "" {
				pr.Out.Header.Set("Origin", "http://"+host)
			}
		},
		ModifyResponse: func(res *http.Response) error {
			res.Header.Set("X-Content-Type-Options", "nosniff")
			res.Header.Del("Set-Cookie")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			writeJSON(w, http.StatusBadGateway, api.Error{Code: "unavailable", Retryable: true,
				Message:    fmt.Sprintf("nothing answered on port %d of the agent's computer", tu.port),
				Suggestion: "start the server there (myco page serve forwards it, it does not start it)"})
		},
	}
}

// livePath is what follows /live in the request's path (decoded and as
// escaped).
func livePath(r *http.Request) (string, string) {
	esc := r.URL.EscapedPath()
	i := strings.Index(esc, "/live/")
	if i < 0 {
		return "/", ""
	}
	raw := esc[i+len("/live"):]
	dec := "/" + r.PathValue("path")
	if raw == dec {
		raw = ""
	}
	return dec, raw
}

// liveRequest: /v1/pages/{id}/live/… for whoever sees the page.
func (s *Server) liveRequest(w http.ResponseWriter, r *http.Request, c pageCall) {
	pv, err := s.pages.Get(r.Context(), c.by, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if !pv.Page.Live {
		writeError(w, domain.Errf(domain.CodeNotFound, "page %s is not a live page", pv.Page.ID))
		return
	}
	tu := s.tun.get(pv.Page.ID)
	if tu == nil {
		writeJSON(w, http.StatusServiceUnavailable, api.Error{Code: "offline", Retryable: true,
			Message:    fmt.Sprintf("%s is not forwarding this page now", pv.Page.Publisher),
			Suggestion: fmt.Sprintf("ask it to run: myco page serve --page %s --port %d", pv.Page.ID, pv.Page.Port)})
		return
	}
	tu.proxy.ServeHTTP(w, r)
}
