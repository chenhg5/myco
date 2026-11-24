package console

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

// The App renders shared pages for agents (page.export, api/actions.go):
// this backend registers as each workspace's page renderer while the App's
// window has its renderer running, takes the jobs, and hands each to that
// renderer: it loads the page in a sandboxed frame with that page's
// read-only data and calls the app's own export handler (pagebridge.js
// myco.actions). The file goes back to the agent as a private artifact.
//
//	GET  /app/page-render/next?wait=25   the next export to render (204: none)
//	POST /app/page-render/{id}/result    {media_type, data (base64), width?, height?, pages?} or {error: {code, message}}
//	GET  /app/page-render/{id}/state     the page's shared state   } only while the job may run:
//	GET  /app/page-render/{id}/data      ?collection=&id=          } then 410
//	GET  /app/page-render/{id}/wait      {stop}: true as soon as it may no longer run

const (
	pageRenderTimeout = 30 * time.Second
	pageRenderMax     = 4 << 20          // the Hub's artifact limit
	pageRendererIdle  = 60 * time.Second // a renderer not polling this long is gone
)

var pageExportAction = api.ActionDescriptor{Name: "page.export", Version: 1, Title: "Export the page as a file", Permission: "page.export"}

type pageRender struct {
	mu       sync.Mutex
	queue    []*renderJob
	byID     map[string]*renderJob
	wake     chan struct{}
	lastPoll time.Time
}

type renderJob struct {
	ID      string          `json:"id"`
	HubID   string          `json:"hub_id"`
	PageID  string          `json:"page_id"`
	Action  string          `json:"action"`
	Version int             `json:"version"`
	Args    json.RawMessage `json:"args,omitempty"`
	done    chan renderResult
	taken   bool
	// While it renders: the page's data is read through it, for as long as
	// the Hub lets the job run (ended closes when it no longer may).
	ctx   context.Context
	c     *client.Client
	ended chan struct{}
}

type renderResult struct {
	MediaType string           `json:"media_type"`
	Data      string           `json:"data"` // base64
	Width     int              `json:"width,omitempty"`
	Height    int              `json:"height,omitempty"`
	Pages     int              `json:"pages,omitempty"`
	Error     *api.ActionError `json:"error,omitempty"`
}

// active reports whether the App's window runs a renderer now.
func (p *pageRender) active() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.lastPoll.IsZero() && time.Since(p.lastPoll) < pageRendererIdle
}

func (p *pageRender) signal() {
	if p.wake != nil {
		close(p.wake)
	}
	p.wake = make(chan struct{})
}

// render hands a job to the window's renderer and waits for its file.
func (p *pageRender) render(ctx context.Context, c *client.Client, j *renderJob) renderResult {
	j.done = make(chan renderResult, 1)
	j.ctx, j.c, j.ended = ctx, c, make(chan struct{})
	defer close(j.ended)
	p.mu.Lock()
	if p.byID == nil {
		p.byID = map[string]*renderJob{}
	}
	p.byID[j.ID] = j
	p.queue = append(p.queue, j)
	p.signal()
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.byID, j.ID)
		for i, q := range p.queue {
			if q == j {
				p.queue = append(p.queue[:i], p.queue[i+1:]...)
				break
			}
		}
		p.mu.Unlock()
	}()
	t := time.NewTimer(pageRenderTimeout)
	defer t.Stop()
	select {
	case r := <-j.done:
		return r
	case <-t.C:
		return renderResult{Error: &api.ActionError{Code: "timeout", Message: "the page took over 30 seconds to export", Retryable: true}}
	case <-ctx.Done():
		return renderResult{Error: &api.ActionError{Code: "cancelled", Message: "stopped"}}
	}
}

// live returns the job id while it may still run (its data readable).
func (p *pageRender) live(id string) *renderJob {
	p.mu.Lock()
	j := p.byID[id]
	p.mu.Unlock()
	if j == nil || !j.taken || j.ended == nil {
		return nil
	}
	select {
	case <-j.ended:
		return nil
	case <-j.ctx.Done():
		return nil
	default:
		return j
	}
}

func (b *Backend) pageRenderRoutes(m *http.ServeMux) {
	p := &b.pageRender
	gone := func(w http.ResponseWriter) {
		writeErr(w, &client.Error{Status: http.StatusGone, Code: "stopped", Message: "this export was stopped (revoked, cancelled or timed out)"})
	}
	m.HandleFunc("GET /app/page-render/{id}/state", func(w http.ResponseWriter, r *http.Request) {
		j := p.live(r.PathValue("id"))
		if j == nil {
			gone(w)
			return
		}
		st, err := j.c.PageState(j.ctx, j.PageID)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	m.HandleFunc("GET /app/page-render/{id}/data", func(w http.ResponseWriter, r *http.Request) {
		j := p.live(r.PathValue("id"))
		if j == nil {
			gone(w)
			return
		}
		q := r.URL.Query()
		var v any
		var err error
		if rid := q.Get("id"); rid != "" {
			v, err = j.c.PageRecord(j.ctx, j.PageID, q.Get("collection"), rid)
		} else {
			if api.HasPageQuery(q) {
				var in api.PageQuery
				in, err = api.ParsePageQuery(q)
				if err != nil {
					writeErr(w, &client.Error{Status: 400, Code: "invalid_argument", Message: err.Error()})
					return
				}
				var result *api.PageRecords
				result, err = j.c.QueryPageRecords(j.ctx, j.PageID, q.Get("collection"), in)
				if result != nil && result.Count != nil {
					v = api.PageRecordCount{Count: *result.Count, Seq: result.Seq, QueryVersion: result.QueryVersion}
				} else {
					v = result
				}
			} else {
				v, err = j.c.PageRecords(j.ctx, j.PageID, q.Get("collection"))
			}
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	})
	m.HandleFunc("GET /app/page-render/{id}/wait", func(w http.ResponseWriter, r *http.Request) {
		j := p.live(r.PathValue("id"))
		if j == nil {
			writeJSON(w, http.StatusOK, map[string]bool{"stop": true})
			return
		}
		select {
		case <-j.ended:
		case <-j.ctx.Done():
		case <-time.After(25 * time.Second):
			writeJSON(w, http.StatusOK, map[string]bool{"stop": false})
			return
		case <-r.Context().Done():
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"stop": true})
	})
	m.HandleFunc("GET /app/page-render/next", func(w http.ResponseWriter, r *http.Request) {
		deadline := time.After(25 * time.Second)
		if r.URL.Query().Get("wait") == "0" {
			deadline = time.After(0)
		}
		for {
			p.mu.Lock()
			p.lastPoll = time.Now()
			if p.wake == nil {
				p.wake = make(chan struct{})
			}
			var next *renderJob
			for _, j := range p.queue {
				if !j.taken {
					next = j
					break
				}
			}
			if next != nil {
				next.taken = true
			}
			ch := p.wake
			p.mu.Unlock()
			if next != nil {
				writeJSON(w, http.StatusOK, next)
				return
			}
			select {
			case <-ch:
			case <-deadline:
				p.mu.Lock()
				p.lastPoll = time.Now()
				p.mu.Unlock()
				w.WriteHeader(http.StatusNoContent)
				return
			case <-r.Context().Done():
				return
			}
		}
	})
	m.HandleFunc("POST /app/page-render/{id}/result", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, pageRenderMax*4/3+64<<10)
		var in renderResult
		if !decode(w, r, &in) {
			return
		}
		p.mu.Lock()
		j := p.byID[r.PathValue("id")]
		p.mu.Unlock()
		if j == nil || !j.taken {
			writeErr(w, &client.Error{Status: 404, Code: "not_found", Message: "no such export is waiting (it timed out or was stopped)"})
			return
		}
		select {
		case j.done <- in:
		default:
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

// hostPages is hub h's page renderer until ctx ends or it cannot go on; it
// returns how long to wait before trying again.
func (b *Backend) hostPages(ctx context.Context, h Hub) time.Duration {
	c, err := b.client(h)
	if err != nil {
		return 10 * time.Minute
	}
	gen := "app-" + rand.Text()
	register := func() (*api.ActionHost, error) {
		call, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		return c.RegisterActionHost(call, api.ActionHostRegister{Provider: "page", Generation: gen, Actions: []api.ActionDescriptor{pageExportAction}})
	}
	host, err := register()
	if err != nil {
		var ce *client.Error
		if errors.As(err, &ce) && (ce.Code == "unsupported" || ce.Code == "invalid_argument" || ce.Status == 404) {
			return 5 * time.Minute // an older hub (no page targets; it may be upgraded soon), or an end-to-end workspace
		}
		return time.Minute
	}
	for ctx.Err() == nil {
		if !b.pageRender.active() {
			return 0 // the window's renderer is gone: offered again when it is back
		}
		jobs, err := c.ActionHostJobs(ctx, host.ID, actionHostPoll)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			var ce *client.Error
			if errors.As(err, &ce) && (ce.Code == "not_found" || ce.Code == "conflict") {
				return actionHostRetake // offered again after a pause, as computers' hosts are
			}
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
			}
			continue
		}
		for _, j := range jobs {
			done := b.runPageJob(ctx, c, h, host, j)
			done.HostID, done.Generation = host.ID, host.Generation
			end, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			_, _ = c.CompleteActionJob(end, j.Job.ID, done)
			cancel()
		}
	}
	return 0
}

// runPageJob renders one export: grant checked here and with the Hub,
// stopped as soon as the Hub says to, the file uploaded privately.
func (b *Backend) runPageJob(ctx context.Context, c *client.Client, h Hub, host *api.ActionHost, hj api.ActionHostJob) api.ActionJobComplete {
	j := hj.Job
	if j.Action != pageExportAction.Name || j.Version != pageExportAction.Version {
		return actionFailure("unsupported", "this App exports pages with page.export v1 only", false, nil)
	}
	g := hj.Grant
	if g.State != api.GrantActive || g.Target.Kind != "page" || g.Target.ID == "" || !strings.HasPrefix(g.Target.ID, "pg_") ||
		(g.ExpiresAt != nil && !time.Now().Before(*g.ExpiresAt)) || !containsString(g.Permissions, "page.export") {
		return actionFailure("permission_denied", "the grant does not allow exporting this page now", false, nil)
	}
	run, done, wasStopped, early := watchHostJob(ctx, c, host, j.ID)
	if early != nil {
		return *early
	}
	defer done()
	var id [12]byte
	_, _ = rand.Read(id[:])
	res := b.pageRender.render(run, c, &renderJob{ID: "rnd_" + base64.RawURLEncoding.EncodeToString(id[:]), HubID: h.ID, PageID: g.Target.ID,
		Action: j.Action, Version: j.Version, Args: hj.Args})
	if wasStopped() {
		return api.ActionJobComplete{State: api.JobCancelled, Error: &api.ActionError{Code: "cancelled", Message: "stopped while it rendered"}}
	}
	if res.Error != nil {
		code := res.Error.Code
		if code == "" {
			code = "export_failed"
		}
		return actionFailure(code, res.Error.Message, res.Error.Retryable, nil)
	}
	data, err := base64.StdEncoding.DecodeString(res.Data)
	if err != nil || len(data) == 0 {
		return actionFailure("export_failed", "the page's app returned no file", false, nil)
	}
	if len(data) > pageRenderMax {
		return actionFailure("too_large", "the file is over 4 MB: ask for a smaller format or size", false, nil)
	}
	mt := strings.TrimSpace(strings.Split(res.MediaType, ";")[0])
	if mt == "" {
		mt = "application/octet-stream"
	}
	up, cancel := context.WithTimeout(run, 30*time.Second)
	art, err := c.UploadActionArtifact(up, j.ID, host.ID, host.Generation, mt, data)
	cancel()
	if err != nil {
		if wasStopped() {
			return api.ActionJobComplete{State: api.JobCancelled, Error: &api.ActionError{Code: "cancelled", Message: "stopped"}}
		}
		var ce *client.Error
		if errors.As(err, &ce) && ce.Code == "invalid_argument" {
			return actionFailure("invalid_type", ce.Message, false, nil)
		}
		return actionFailure("unavailable", "could not upload the file", true, nil)
	}
	out := map[string]any{"media_type": mt, "artifact": art}
	if res.Width > 0 {
		out["width"] = res.Width
	}
	if res.Height > 0 {
		out["height"] = res.Height
	}
	if res.Pages > 0 {
		out["pages"] = res.Pages
	}
	result, _ := json.Marshal(out)
	return api.ActionJobComplete{State: api.JobSucceeded, Result: result, ArtifactIDs: []string{art.ID}}
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
