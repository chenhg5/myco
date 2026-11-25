package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func panelRecording() *recording {
	return &recording{scope: "fixture", info: recordingInfo{ID: "rec_fixture", Profile: "p_fixture", Origin: "browser", URL: "https://example.com", StartedAt: time.Now()}, steps: []node{{ID: "one", Type: "click"}, {ID: "two", Type: "type"}}, changes: []recordChange{{added: 0}, {added: 1}}, removed: map[string]bool{}, wake: make(chan struct{})}
}
func TestRecordingRemovedCursorAndReplay(t *testing.T) {
	r := panelRecording()
	if !r.remove("one") || r.remove("one") || r.remove("unknown") {
		t.Fatal("deletion must be idempotent and scoped to recorded IDs")
	}
	r.done = true
	out, err := r.events(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), `"one"`) || !strings.Contains(string(raw), `"two"`) || !strings.Contains(string(raw), `"removed":[]`) {
		t.Fatal("unsent deleted step leaked", string(raw))
	}
	out, err = r.events(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(out)
	if !strings.Contains(string(raw), `"removed":["one"]`) || out["next"] != 3 || out["done"] != true {
		t.Fatal("sent deletion/cursor missing", string(raw))
	}
	// Retries use the same immutable cursor and yield the same tombstone.
	again, _ := r.events(context.Background(), 1)
	a, _ := json.Marshal(again)
	if string(a) != string(raw) {
		t.Fatal("cursor replay changed")
	}
}
func TestRecordingDiscoveryIsScopedAndClaimedOnEvents(t *testing.T) {
	e, err := newEngine(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	r := panelRecording()
	r.done = true
	e.recordings[r.info.ID] = r
	if len(e.activeRecordings("other")) != 0 || len(e.activeRecordings("fixture")) != 1 {
		t.Fatal("unclaimed completed recording missing or scope leaked")
	}
	invoke := func(scope, path string) int {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("X-Myco-Scope", scope)
		w := httptest.NewRecorder()
		e.handler().ServeHTTP(w, req)
		return w.Code
	}
	if invoke("other", "/record/events?id=rec_fixture&after=0") != 404 {
		t.Fatal("other scope read recording")
	}
	if invoke("fixture", "/record/events?id=rec_fixture&after=999") != 400 || r.claimed {
		t.Fatal("invalid cursor claimed recording")
	}
	if invoke("fixture", "/record/events?id=rec_fixture&after=0") != 200 || !r.claimed || len(e.activeRecordings("fixture")) != 0 {
		t.Fatal("first read did not claim")
	}
	if invoke("fixture", "/record/events?id=rec_fixture&after=0") != 200 {
		t.Fatal("claimed recording cannot be replayed")
	}
}

func TestHeadlessCollectorOmitsPanelSource(t *testing.T) {
	s := &session{headless: true}
	if strings.Contains(s.collectorScript(), "__myco_recording_panel__") || strings.Contains(s.collectorScript(), "attachShadow") {
		t.Fatal("headless session injected panel source")
	}
	s.headless = false
	if !strings.Contains(s.collectorScript(), "__myco_recording_panel__") {
		t.Fatal("headed collector omitted panel source")
	}
}

func TestPanelContextExtractionIsBoundAndValidated(t *testing.T) {
	r := panelRecording()
	r.id = "rec_fixture"
	r.queue = make(chan captured, 8)
	s := &session{ctx: context.Background(), recorder: r,
		targets: map[proto.TargetSessionID]proto.TargetTargetID{"sid": "page", "child-sid": "iframe"},
		pages:   map[proto.TargetTargetID]*rod.Page{"page": {TargetID: "page"}},
		frames:  map[proto.PageFrameID]frameInfo{"top": {}, "child": {Parent: "top"}},
	}
	elementTarget := &target{Locators: []locator{{Kind: "css", Value: "#article"}}}
	call := func(sid proto.TargetSessionID, frame proto.PageFrameID, recordID, what, attr, into string, elementTarget *target) {
		t.Helper()
		raw, err := json.Marshal(map[string]any{"action": "extract-target", "recording_id": recordID, "what": what, "attr": attr, "into": into, "target": elementTarget})
		if err != nil {
			t.Fatal(err)
		}
		s.panelCommand(sid, frame, string(raw))
	}
	call("sid", "top", "old", "text", "", "", elementTarget)
	call("child-sid", "child", r.id, "text", "", "", elementTarget)
	call("sid", "top", r.id, "js", "", "", elementTarget)
	call("sid", "top", r.id, "attribute", "", "", elementTarget)
	call("sid", "top", r.id, "text", "", "bad variable", elementTarget)
	call("sid", "top", r.id, "text", "", "", nil)
	if len(r.queue) != 0 {
		t.Fatal("invalid or unbound context extraction accepted")
	}
	call("sid", "top", r.id, "text", "", "", elementTarget)
	first := <-r.queue
	if first.node.Type != "extract" || first.node.What != "text" || first.node.Into != "data1" || first.frame != "top" {
		t.Fatalf("wrong context node: %+v", first)
	}
	call("sid", "top", r.id, "attribute", "href", "link", elementTarget)
	if n := (<-r.queue).node; n.Attr != "href" || n.Into != "link" {
		t.Fatalf("wrong attribute: %+v", n)
	}
	r.picking = true
	call("sid", "top", r.id, "html", "", "", elementTarget)
	r.picking = false
	r.done = true
	call("sid", "top", r.id, "html", "", "", elementTarget)
	if len(r.queue) != 0 {
		t.Fatal("selection or stopped recording accepted a context extraction")
	}
}
