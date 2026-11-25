package main

import (
	"strings"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

const navigationActionWindow = 2 * time.Second

type recordedNavigationRequest struct {
	RecordID     string
	User         bool
	Page         proto.TargetTargetID
	Contexts     []proto.RuntimeExecutionContextID
	Started      time.Time
	CommittedURL string
}

func navigationAction(n node) bool {
	return (n.Type == "click" && (n.Button == "left" || n.Button == "middle" || n.Button == "")) || (n.Type == "press" && (n.Key == "Enter" || strings.HasSuffix(n.Key, "+Enter")))
}

// Called only for already-validated collector events from the isolated world.
// Typing, focus, panel actions, and activity in another tab are not navigation intent.
func (r *recording) noteNavigationAction(page proto.TargetTargetID, n node, now time.Time) {
	if !navigationAction(n) {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done || r.closing {
		return
	}
	if r.navActions == nil {
		r.navActions = map[proto.TargetTargetID]time.Time{}
	}
	r.navActions[page] = now
}

// The browser may commit before the old renderer's binding is delivered.
// Match only a trusted action from a context that existed at the request,
// never a subsequent click in the newly loaded document.
func (s *session) lateNavigation(r *recording, sid proto.TargetSessionID, contextID proto.RuntimeExecutionContextID, n node, now time.Time) string {
	if !navigationAction(n) {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	page := s.targets[sid]
	request := s.lateNavigations[page]
	if request.RecordID != r.id || request.CommittedURL == "" || now.Sub(request.Started) < 0 || now.Sub(request.Started) > navigationActionWindow {
		return ""
	}
	for _, id := range request.Contexts {
		if id == contextID {
			delete(s.lateNavigations, page)
			return request.CommittedURL
		}
	}
	return ""
}

func (s *session) keepLateNavigation(request recordedNavigationRequest, url string) {
	request.CommittedURL = url
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lateNavigations == nil {
		s.lateNavigations = map[proto.TargetTargetID]recordedNavigationRequest{}
	}
	s.lateNavigations[request.Page] = request
}
func (r *recording) recentNavigationAction(page proto.TargetTargetID, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	at, ok := r.navActions[page]
	return ok && !r.done && !r.closing && now.Sub(at) >= 0 && now.Sub(at) <= navigationActionWindow
}

// The caller holds s.mu. An OOPIF root may have no parent in its own target;
// only a page target's root counts as the document the user is recording.
func (s *session) topNavigationLocked(sid proto.TargetSessionID, frame proto.PageFrameID) bool {
	installed := s.installs[s.targets[sid]]
	return installed != nil && installed.sid == sid && installed.kind == "page" && installed.rootFrame == frame && frame != ""
}

// Newer Chrome emits this for browser-side navigation starts, including an
// address-bar fragment. Script hash/history updates need not emit it.
type frameStartedNavigation struct {
	FrameID        proto.PageFrameID `json:"frameId"`
	URL            string            `json:"url"`
	NavigationType string            `json:"navigationType"`
}

func (frameStartedNavigation) ProtoEvent() string { return "Page.frameStartedNavigating" }

type recordedBrowserNavigation struct{ RecordID, URL string }
