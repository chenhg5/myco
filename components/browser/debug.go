package main

import (
	"context"
	"github.com/go-rod/rod/lib/proto"
	"net/http"
	"time"
)

type picked struct {
	Target target
	Err    error
}

func (e *engine) debugRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /locate", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Profile string `json:"profile"`
			Target  target `json:"target"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		s, err := e.session(scope(r), in.Profile)
		if err != nil {
			fail(w, "not_found", 404)
			return
		}
		p, err := s.page()
		if err != nil {
			fail(w, "not_found", 404)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		frame, err := targetPage(ctx, p, in.Target)
		if err != nil {
			fail(w, "not_found", 404)
			return
		}
		count, err := frame.Eval(`t=>(`+locatorSource+`)(t,true).length`, in.Target)
		if err != nil {
			fail(w, "invalid_argument", 400)
			return
		}
		if count.Value.Int() > 0 {
			if el, err := findTarget(ctx, p, in.Target); err == nil {
				if described, err := el.Describe(0, false); err == nil {
					_ = s.call(s.primarySession(frame.TargetID), "Overlay.highlightNode", map[string]any{"backendNodeId": described.BackendNodeID, "highlightConfig": map[string]any{"showInfo": true, "contentColor": map[string]any{"r": 70, "g": 130, "b": 255, "a": 0.25}}}, nil)
				}
			}
		}
		jsonReply(w, map[string]int{"count": count.Value.Int()})
	})
	m.HandleFunc("POST /pick", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Profile string `json:"profile"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		s, err := e.session(scope(r), in.Profile)
		if err != nil {
			fail(w, "not_found", 404)
			return
		}
		if s.claim("pick") != nil {
			fail(w, "busy", 409)
			return
		}
		defer s.release("pick")
		p, err := s.page()
		if err != nil {
			fail(w, "not_found", 404)
			return
		}
		events := make(chan picked, 1)
		s.mu.Lock()
		s.picking = events
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			s.picking = nil
			s.mu.Unlock()
			_ = s.call(s.primarySession(p.TargetID), "Overlay.setInspectMode", map[string]any{"mode": "none"}, nil)
		}()
		if s.call(s.primarySession(p.TargetID), "DOM.enable", map[string]any{}, nil) != nil || s.call(s.primarySession(p.TargetID), "Overlay.enable", map[string]any{}, nil) != nil {
			fail(w, "browser_failed", 409)
			return
		}
		err = s.call(s.primarySession(p.TargetID), "Overlay.setInspectMode", map[string]any{"mode": "searchForNode", "highlightConfig": map[string]any{"showInfo": true, "contentColor": map[string]any{"r": 70, "g": 130, "b": 255, "a": 0.25}}}, nil)
		if err != nil {
			fail(w, "browser_failed", 409)
			return
		}
		select {
		case event := <-events:
			if event.Err != nil {
				fail(w, "browser_failed", 409)
				return
			}
			jsonReply(w, event.Target)
		case <-r.Context().Done():
			fail(w, "cancelled", 409)
		case <-s.ctx.Done():
			fail(w, "cancelled", 409)
		case <-time.After(2 * time.Minute):
			fail(w, "timeout", 408)
		}
	})
	m.HandleFunc("POST /page/{action}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Profile string `json:"profile"`
			node
		}
		if !readJSON(w, r, &in) {
			return
		}
		s, err := e.session(scope(r), in.Profile)
		if err != nil {
			fail(w, "not_found", 404)
			return
		}
		if s.claim("debug") != nil {
			fail(w, "busy", 409)
			return
		}
		defer s.release("debug")
		n := in.node
		n.Type = r.PathValue("action")
		if n.Type == "snapshot" {
			p, err := s.page()
			if err != nil {
				fail(w, "not_found", 404)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			tree, err := proto.AccessibilityGetFullAXTree{}.Call(p.Context(ctx))
			if err != nil {
				fail(w, "browser_failed", 409)
				return
			}
			shot, _ := e.snapshot(scope(r), p, nil)
			jsonReply(w, map[string]any{"tree": tree.Nodes, "shot": shot})
			return
		}
		if n.Type == "eval" {
			n.Type = "js"
		}
		allowed := map[string]bool{"navigate": true, "click": true, "type": true, "select": true, "scroll": true, "press": true, "screenshot": true, "js": true, "upload": true}
		if !allowed[n.Type] {
			fail(w, "unsupported", 400)
			return
		}
		temp := &playRun{session: s, scope: scope(r)}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		out, err := temp.act(ctx, n)
		if err != nil {
			fail(w, publicError(err), 409)
			return
		}
		jsonReply(w, map[string]any{"value": out})
	})
}
func (s *session) pickNode(sid proto.TargetSessionID, id proto.DOMBackendNodeID) {
	s.mu.Lock()
	waiting := s.picking
	s.mu.Unlock()
	if waiting == nil {
		return
	}
	out, frame, err := s.describeBackend(sid, id)
	s.mu.Lock()
	p := s.pages[s.targets[sid]]
	if p == nil {
		p = s.pages[s.current]
	}
	s.mu.Unlock()
	if err == nil && p != nil {
		out.Frame, err = s.frameChain(p, frame)
	}
	select {
	case waiting <- picked{Target: out, Err: err}:
	default:
	}
}
