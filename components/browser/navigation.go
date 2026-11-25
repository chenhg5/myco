package main

import (
	"strings"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

const navigationActionWindow = 2 * time.Second

type recordedNavigationRequest struct {
	RecordID string
	User     bool
}

// Called only for already-validated collector events from the isolated world.
// Typing, focus, panel actions, and activity in another tab are not navigation intent.
func (r *recording) noteNavigationAction(page proto.TargetTargetID, n node, now time.Time) {
	if !(n.Type == "click" && (n.Button == "left" || n.Button == "middle" || n.Button == "")) && !(n.Type == "press" && (n.Key == "Enter" || strings.HasSuffix(n.Key, "+Enter"))) {
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
