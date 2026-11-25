package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/proto"
)

func navigationFixture(t *testing.T) (*session, *recording, func(proto.Event)) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	wire := &pausedPopupClient{events: make(chan *cdp.Event, 8)}
	t.Cleanup(func() { cancel(); close(wire.events) })
	b := rod.New().Context(ctx).ControlURL("").Client(wire).NoDefaultDevice()
	if err := b.Connect(); err != nil {
		t.Fatal(err)
	}
	events := b.Event()
	s := &session{ctx: ctx, browser: b, current: "page", frames: map[proto.PageFrameID]frameInfo{}, targets: map[proto.TargetSessionID]proto.TargetTargetID{"sid": "page", "oopif-sid": "oopif"}, installs: map[proto.TargetTargetID]*targetInstall{"page": {sid: "sid", kind: "page", rootFrame: "top"}, "oopif": {sid: "oopif-sid", kind: "iframe", rootFrame: "oopif-root"}}, pages: map[proto.TargetTargetID]*rod.Page{"page": {TargetID: "page"}}, contexts: map[string]map[proto.RuntimeExecutionContextID]proto.PageFrameID{"sid": {1: "top"}, "oopif-sid": {2: "oopif-root"}}}
	rec := &recording{id: "rec_fixture", queue: make(chan captured, 32)}
	s.recorder = rec
	send := func(event proto.Event) {
		t.Helper()
		raw, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		sid := "sid"
		switch evt := event.(type) {
		case *proto.PageFrameNavigated:
			if evt.Frame.ID == "oopif-root" {
				sid = "oopif-sid"
			}
		case *proto.PageNavigatedWithinDocument:
			if evt.FrameID == "oopif-root" {
				sid = "oopif-sid"
			}
		}
		wire.events <- &cdp.Event{SessionID: string(sid), Method: event.ProtoEvent(), Params: raw}
		select {
		case msg := <-events:
			s.event(msg)
		case <-time.After(time.Second):
			t.Fatal("event was not delivered")
		}
	}
	return s, rec, send
}
func queuedNavigation(t *testing.T, r *recording, types ...string) {
	t.Helper()
	if len(r.queue) != len(types) {
		t.Fatalf("unexpected queued steps: got=%d want=%v", len(r.queue), types)
	}
	for _, kind := range types {
		if c := <-r.queue; c.node.Type != kind {
			t.Fatalf("got %s want %s", c.node.Type, kind)
		}
	}
}
func TestRecordingIgnoresAllIframeNavigation(t *testing.T) {
	_, r, send := navigationFixture(t)
	r.noteNavigationAction("page", node{Type: "click", Button: "left"}, time.Now())
	for _, frame := range []*proto.PageFrame{{ID: "child", ParentID: "top", URL: "https://ads.example/1"}, {ID: "oopif-root", URL: "https://ads.example/2"}} {
		send(&proto.PageFrameNavigated{Frame: frame})
		send(&proto.PageNavigatedWithinDocument{FrameID: frame.ID, URL: frame.URL + "#next"})
	}
	queuedNavigation(t, r)
}
func TestRecordingNavigationRequiresRecentUserAction(t *testing.T) {
	_, r, send := navigationFixture(t)
	for _, n := range []node{{Type: "type"}, {Type: "press", Key: "Tab"}, {Type: "click", Button: "right"}} {
		r.noteNavigationAction("page", n, time.Now())
	}
	send(&proto.PageFrameRequestedNavigation{FrameID: "top", URL: "https://example.com/timer"})
	send(&proto.PageFrameNavigated{Frame: &proto.PageFrame{ID: "top", URL: "https://example.com/timer"}})
	send(&proto.PageNavigatedWithinDocument{FrameID: "top", URL: "https://example.com/history"})
	queuedNavigation(t, r)
	r.noteNavigationAction("page", node{Type: "click", Button: "left"}, time.Now())
	send(&proto.PageFrameRequestedNavigation{FrameID: "top", URL: "https://example.com/click"})
	// A slow network response remains associated with the original click.
	r.mu.Lock()
	r.navActions["page"] = time.Now().Add(-3 * time.Second)
	r.mu.Unlock()
	send(&proto.PageFrameNavigated{Frame: &proto.PageFrame{ID: "top", URL: "https://example.com/click"}})
	queuedNavigation(t, r, "wait")
	send(&proto.PageFrameRequestedNavigation{FrameID: "top", URL: "https://example.com/later"})
	send(&proto.PageFrameNavigated{Frame: &proto.PageFrame{ID: "top", URL: "https://example.com/later"}})
	queuedNavigation(t, r)
	// Address-bar/browser navigation has no renderer request.
	send(&proto.PageFrameNavigated{Frame: &proto.PageFrame{ID: "top", URL: "https://example.com/typed"}})
	queuedNavigation(t, r, "navigate")
}
func TestRecordingNavigationIntentIsPerTabAndRecording(t *testing.T) {
	s, r, send := navigationFixture(t)
	r.noteNavigationAction("another-page", node{Type: "press", Key: "Enter"}, time.Now())
	send(&proto.PageFrameRequestedNavigation{FrameID: "top", URL: "https://example.com/other-tab"})
	send(&proto.PageFrameNavigated{Frame: &proto.PageFrame{ID: "top", URL: "https://example.com/other-tab"}})
	queuedNavigation(t, r)
	r.noteNavigationAction("page", node{Type: "press", Key: "Enter"}, time.Now())
	send(&proto.PageNavigatedWithinDocument{FrameID: "top", URL: "https://example.com/#clicked"})
	queuedNavigation(t, r, "wait")
	send(&proto.PageFrameRequestedNavigation{FrameID: "top", URL: "https://example.com/old"})
	next := &recording{id: "rec_new", queue: make(chan captured, 8)}
	s.recorder = next
	send(&proto.PageFrameNavigated{Frame: &proto.PageFrame{ID: "top", URL: "https://example.com/old"}})
	queuedNavigation(t, next)
}

func TestRecordingDistinguishesBrowserFragmentFromScriptHash(t *testing.T) {
	_, r, send := navigationFixture(t)
	send(&proto.PageNavigatedWithinDocument{FrameID: "top", URL: "https://example.com/#automatic"})
	queuedNavigation(t, r)
	send(&frameStartedNavigation{FrameID: "top", URL: "https://example.com/#typed", NavigationType: "sameDocument"})
	send(&proto.PageNavigatedWithinDocument{FrameID: "top", URL: "https://example.com/#typed"})
	queuedNavigation(t, r, "navigate")
	r.noteNavigationAction("page", node{Type: "click", Button: "left"}, time.Now())
	send(&frameStartedNavigation{FrameID: "top", URL: "https://example.com/#typed-after-click", NavigationType: "sameDocument"})
	send(&proto.PageNavigatedWithinDocument{FrameID: "top", URL: "https://example.com/#typed-after-click"})
	queuedNavigation(t, r, "navigate")
	r.mu.Lock()
	r.navActions["page"] = time.Now().Add(-3 * time.Second)
	r.mu.Unlock()
	// A stale browser marker cannot authorize a different hash change.
	send(&frameStartedNavigation{FrameID: "top", URL: "https://example.com/#aborted", NavigationType: "sameDocument"})
	send(&proto.PageNavigatedWithinDocument{FrameID: "top", URL: "https://example.com/#unrelated"})
	queuedNavigation(t, r)
}

func TestRecordingNavigationRequestCanPrecedeCollectorClick(t *testing.T) {
	for _, within := range []bool{false, true} {
		name := "new document"
		if within {
			name = "fragment"
		}
		t.Run(name, func(t *testing.T) {
			_, r, send := navigationFixture(t)
			url := "https://example.com/click"
			if within {
				url += "#section"
			}
			// Page navigation is reported by the browser process; the trusted
			// click binding can arrive later from the renderer process.
			send(&proto.PageFrameRequestedNavigation{FrameID: "top", URL: url, Reason: proto.PageClientNavigationReasonAnchorClick})
			send(&proto.RuntimeBindingCalled{Name: "__mycoBrowserEvent", ExecutionContextID: 1,
				Payload: `{"type":"click","button":"left","target":{"locators":[{"kind":"css","value":"#go"}]}}`})
			if within {
				send(&proto.PageNavigatedWithinDocument{FrameID: "top", URL: url})
			} else {
				send(&proto.PageFrameNavigated{Frame: &proto.PageFrame{ID: "top", URL: url}})
			}
			queuedNavigation(t, r, "click", "wait")
		})
	}
}

func TestRecordingCommitCanPrecedeCollectorClick(t *testing.T) {
	for _, within := range []bool{false, true} {
		name := "new document"
		if within {
			name = "fragment"
		}
		t.Run(name, func(t *testing.T) {
			_, r, send := navigationFixture(t)
			url := "https://example.com/landed"
			send(&proto.PageFrameRequestedNavigation{FrameID: "top", URL: url, Reason: proto.PageClientNavigationReasonAnchorClick})
			if within {
				send(&proto.PageNavigatedWithinDocument{FrameID: "top", URL: url})
			} else {
				send(&proto.RuntimeExecutionContextsCleared{})
				send(&proto.PageFrameNavigated{Frame: &proto.PageFrame{ID: "top", URL: url}})
			}
			// An iframe commit must not discard the top-level correlation.
			send(&proto.PageFrameNavigated{Frame: &proto.PageFrame{ID: "child", ParentID: "top", URL: "https://ads.example/next"}})
			send(&proto.RuntimeBindingCalled{Name: "__mycoBrowserEvent", ExecutionContextID: 1,
				Payload: `{"type":"click","button":"left","target":{"locators":[{"kind":"css","value":"#go"}]}}`})
			queuedNavigation(t, r, "click", "wait")
		})
	}
}

func TestRecordingLateClickIsBoundToOriginalRequest(t *testing.T) {
	for _, phase := range []string{"old recording", "expired request", "non-navigation key", "other tab"} {
		t.Run(phase, func(t *testing.T) {
			s, r, send := navigationFixture(t)
			send(&proto.PageFrameRequestedNavigation{FrameID: "top", URL: "https://example.com/automatic"})
			send(&proto.PageFrameNavigated{Frame: &proto.PageFrame{ID: "top", URL: "https://example.com/automatic"}})
			payload := `{"type":"click","button":"left","target":{"locators":[{"kind":"css","value":"#stay"}]}}`
			switch phase {
			case "old recording":
				r.id = "rec_next"
			case "expired request":
				request := s.lateNavigations["page"]
				request.Started = time.Now().Add(-navigationActionWindow - time.Second)
				s.lateNavigations["page"] = request
			case "non-navigation key":
				payload = `{"type":"press","key":"Tab","target":{"locators":[{"kind":"css","value":"#stay"}]}}`
			case "other tab":
				s.targets["sid"] = "another-page"
				s.pages["another-page"] = &rod.Page{TargetID: "another-page"}
				s.current = "another-page"
			}
			send(&proto.RuntimeBindingCalled{Name: "__mycoBrowserEvent", ExecutionContextID: 1, Payload: payload})
			want := "click"
			if phase == "non-navigation key" {
				want = "press"
			}
			queuedNavigation(t, r, want)
		})
	}
}

func TestRecordingLateClickCannotAuthorizeAnotherDocument(t *testing.T) {
	s, r, send := navigationFixture(t)
	send(&proto.PageFrameRequestedNavigation{FrameID: "top", URL: "https://example.com/automatic"})
	send(&proto.RuntimeExecutionContextsCleared{})
	send(&proto.PageFrameNavigated{Frame: &proto.PageFrame{ID: "top", URL: "https://example.com/automatic"}})
	s.contexts["sid"] = map[proto.RuntimeExecutionContextID]proto.PageFrameID{3: "top"}
	send(&proto.RuntimeBindingCalled{Name: "__mycoBrowserEvent", ExecutionContextID: 3,
		Payload: `{"type":"click","button":"left","target":{"locators":[{"kind":"css","value":"#stay"}]}}`})
	queuedNavigation(t, r, "click")
}
