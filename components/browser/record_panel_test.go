package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
