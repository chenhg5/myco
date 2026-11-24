package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type captured struct {
	node  node
	page  *rod.Page
	frame proto.PageFrameID
}
type recording struct {
	tabs      []proto.TargetTargetID
	id, scope string
	session   *session
	mu        sync.Mutex
	steps     []node
	done      bool
	err       string
	wake      chan struct{}
	queue     chan captured
	closing   bool
	barriers  map[string]chan struct{}
	bytes     int
}

func (r *recording) signalLocked()               { close(r.wake); r.wake = make(chan struct{}) }
func (r *recording) enqueue(n node, p *rod.Page) { r.enqueueFrame(n, p, "") }
func (r *recording) enqueueFrame(n node, p *rod.Page, frame proto.PageFrameID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done || r.closing {
		return
	}
	select {
	case r.queue <- captured{n, p, frame}:
	default:
		r.err = "recording_limit"
		r.closing = true
		close(r.queue)
	}
}
func (r *recording) finish() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closing {
		r.closing = true
		close(r.queue)
	}
}
func (r *recording) worker() {
	defer func() {
		s := r.session
		s.mu.Lock()
		if s.recorder == r {
			s.recorder = nil
		}
		s.mu.Unlock()
		s.release(r.id)
		r.mu.Lock()
		r.done = true
		r.signalLocked()
		r.mu.Unlock()
	}()

	for c := range r.queue {
		n := c.node
		n.ID = "n_" + rand.Text()
		if c.frame != "" {
			if n.Target == nil {
				n.Target = &target{Locators: []locator{}}
			}
			chain, err := r.session.frameChain(c.page, c.frame)
			if err != nil {
				r.mu.Lock()
				r.err = "frame_unavailable"
				r.mu.Unlock()
				r.finish()
				return
			}
			if len(chain) > 0 {
				n.Target.Frame = chain
			}
		}
		if c.page != nil && !n.Sensitive {
			id, err := r.session.e.snapshot(r.scope, c.page, n.Target)
			if err == nil {
				if n.Target == nil {
					n.Target = &target{Locators: []locator{}}
				}
				n.Target.Snapshot = id
			}
		}
		raw, _ := json.Marshal(n)
		r.mu.Lock()
		if len(r.steps) >= 128 || r.bytes+len(raw) > 48<<10 {
			r.err = "recording_limit"
			r.mu.Unlock()
			r.finish()
			return
		}
		r.steps = append(r.steps, n)
		r.bytes += len(raw)
		r.signalLocked()
		r.mu.Unlock()
	}
}
func (r *recording) events(ctx context.Context, after int) (map[string]any, error) {
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		r.mu.Lock()
		if after < 0 || after > len(r.steps) {
			r.mu.Unlock()
			return nil, errors.New("invalid_argument")
		}
		if len(r.steps) > after || r.done || r.err != "" {
			steps := append([]node{}, r.steps[after:]...)
			next, done, err := len(r.steps), r.done, r.err
			r.mu.Unlock()
			if err != "" {
				return nil, errors.New(err)
			}
			return map[string]any{"steps": steps, "next": next, "done": done}, nil
		}
		wake := r.wake
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-wake:
		case <-timer.C:
			return map[string]any{"steps": []node{}, "next": after, "done": false}, nil
		}
	}
}
func (e *engine) recordRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /record/start", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Profile  string `json:"profile"`
			URL      string `json:"url"`
			Headless bool   `json:"headless"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		if checkedURL(in.URL) != nil {
			fail(w, "invalid_argument", 400)
			return
		}
		s, err := e.openSession(r.Context(), scope(r), in.Profile, in.Headless)
		if err != nil {
			fail(w, publicError(err), 409)
			return
		}
		e.mu.Lock()
		full := len(e.recordings) >= 256
		e.mu.Unlock()
		if full {
			fail(w, "recording_limit", 409)
			return
		}
		rec := &recording{id: "rec_" + rand.Text(), scope: scope(r), session: s, steps: []node{}, wake: make(chan struct{}), queue: make(chan captured, 128), barriers: map[string]chan struct{}{}}
		if err = s.claim(rec.id); err != nil {
			fail(w, "busy", 409)
			return
		}
		p, err := s.page()
		if err != nil {
			s.release(rec.id)
			fail(w, "target_not_found", 409)
			return
		}
		// Navigate first, then enable capture: the script already stores start_url.
		if err = p.Context(r.Context()).Navigate(in.URL); err != nil {
			s.release(rec.id)
			fail(w, publicError(err), 409)
			return
		}
		if err = p.Context(r.Context()).WaitLoad(); err != nil {
			s.release(rec.id)
			fail(w, publicError(err), 409)
			return
		}
		rec.tabIndex(p.TargetID)
		s.mu.Lock()
		s.recorder = rec
		s.mu.Unlock()
		e.mu.Lock()
		e.recordings[rec.id] = rec
		e.mu.Unlock()
		go rec.worker()
		s.setRecording(true)
		jsonReply(w, map[string]string{"id": rec.id})
	})
	m.HandleFunc("GET /record/events", func(w http.ResponseWriter, r *http.Request) {
		after, err := strconv.Atoi(r.URL.Query().Get("after"))
		if err != nil {
			fail(w, "invalid_argument", 400)
			return
		}
		e.mu.Lock()
		rec := e.recordings[r.URL.Query().Get("id")]
		e.mu.Unlock()
		if rec == nil || rec.scope != scope(r) {
			fail(w, "not_found", 404)
			return
		}
		out, err := rec.events(r.Context(), after)
		if err != nil {
			fail(w, publicError(err), 409)
			return
		}
		jsonReply(w, out)
	})
	m.HandleFunc("POST /record/stop", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		e.mu.Lock()
		rec := e.recordings[in.ID]
		e.mu.Unlock()
		if rec == nil || rec.scope != scope(r) {
			fail(w, "not_found", 404)
			return
		}
		rec.flush(r.Context())
		rec.finish()
		jsonReply(w, true)
	})
}
func (r *recording) flush(ctx context.Context) {
	s := r.session
	s.mu.Lock()
	contexts := map[proto.TargetSessionID][]proto.RuntimeExecutionContextID{}
	for sid, ids := range s.contexts {
		for id := range ids {
			contexts[proto.TargetSessionID(sid)] = append(contexts[proto.TargetSessionID(sid)], id)
		}
	}
	s.mu.Unlock()
	for sid, ids := range contexts {
		for _, id := range ids {
			token := rand.Text()
			done := make(chan struct{})
			r.mu.Lock()
			r.barriers[token] = done
			r.mu.Unlock()
			encoded, _ := json.Marshal(token)
			expression := "if(window.__mycoRecorder){window.__mycoRecorder.flush();window.__mycoRecorder.enabled=false;window.__mycoBrowserEvent(JSON.stringify({type:'barrier',id:" + string(encoded) + "}));}"
			err := s.call(sid, "Runtime.evaluate", map[string]any{"expression": expression, "contextId": id}, nil)
			if err == nil {
				select {
				case <-done:
				case <-ctx.Done():
				case <-time.After(time.Second):
				}
			}
			r.mu.Lock()
			delete(r.barriers, token)
			r.mu.Unlock()
		}
	}
}

func (r *recording) tabIndex(id proto.TargetTargetID) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, target := range r.tabs {
		if target == id {
			return i
		}
	}
	r.tabs = append(r.tabs, id)
	return len(r.tabs) - 1
}
