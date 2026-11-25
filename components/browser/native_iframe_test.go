package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
)

// A verification frame may create another frame and a worker before our
// attachment handler runs. All must make progress while recording is active.
func TestNativeChromeNestedIframeRecording(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with an installed Chrome/Edge")
	}
	var base string
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/worker.js":
			w.Header().Set("Content-Type", "text/javascript")
			fmt.Fprint(w, `self.postMessage('worker-ready')`)
		case "/child":
			fmt.Fprint(w, `<!doctype html><title>Nested frame</title><label>Nested<input id="nested-value"></label><script>new Worker('/worker.js').onmessage=()=>{document.body.dataset.worker='ready';parent.postMessage('child-ready','*')}</script>`)
		case "/frame":
			fmt.Fprintf(w, `<!doctype html><title>Cross-origin frame</title><label>Outer<input id="outer-value"></label><iframe id="nested" src="%s/child"></iframe><script>new Worker('/worker.js').onmessage=()=>document.body.dataset.worker='ready';addEventListener('message',e=>{if(e.data==='child-ready')document.body.dataset.child='ready'})</script>`, strings.Replace(base, "127.0.0.1", "deep.localhost", 1))
		default:
			fmt.Fprintf(w, `<!doctype html><title>Frame fixture</title><iframe id="outer" src="%s/frame"></iframe>`, strings.Replace(base, "127.0.0.1", "localhost", 1))
		}
	}))
	defer fixture.Close()
	base = fixture.URL
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	e, err := newEngine(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	launchLog, err := os.CreateTemp(t.TempDir(), "launch-*.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { launchLog.Close() })
	e.launchLog = launchLog
	stages := nativeDiagnostics(t, e)
	diagnose := func() {
		stages()
		raw, _ := os.ReadFile(launchLog.Name())
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "private helper failed:") {
				t.Log(line)
			}
		}
	}
	invoke := func(method, path string, in, out any) {
		t.Helper()
		raw, _ := json.Marshal(in)
		req := httptest.NewRequest(method, "http://component"+path, bytes.NewReader(raw)).WithContext(ctx)
		req.Header.Set("X-Myco-Scope", "native/iframe")
		w := httptest.NewRecorder()
		e.handler().ServeHTTP(w, req)
		if w.Code != 200 {
			diagnose()
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if out != nil {
			if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
				t.Fatal(err)
			}
		}
	}
	var p profile
	invoke("POST", "/profiles", map[string]string{"name": "Nested frame fixture"}, &p)
	var recorded struct {
		ID string `json:"id"`
	}
	invoke("POST", "/record/start", map[string]any{"profile": p.ID, "url": base, "headless": true}, &recorded)
	s, err := e.session("native/iframe", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.page()
	if err != nil {
		t.Fatal(err)
	}
	page = page.Context(ctx)
	outer, err := targetPage(ctx, page, target{Frame: []string{"#outer"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = outer.Timeout(10 * time.Second).Wait(rod.Eval(`()=>document.body?.dataset.worker==='ready'&&document.body?.dataset.child==='ready'`)); err != nil {
		diagnose()
		t.Fatal("nested iframe/worker remained paused:", err)
	}
	field, err := outer.Element("#outer-value")
	if err != nil {
		t.Fatal(err)
	}
	if err = field.Input("Outer capture"); err != nil {
		t.Fatal(err)
	}
	inner, err := targetPage(ctx, page, target{Frame: []string{"#outer", "#nested"}})
	if err != nil {
		t.Fatal(err)
	}
	field, err = inner.Element("#nested-value")
	if err != nil {
		t.Fatal(err)
	}
	if err = field.Input("Nested capture"); err != nil {
		t.Fatal(err)
	}
	invoke("POST", "/record/stop", map[string]string{"id": recorded.ID}, nil)
	e.mu.Lock()
	rec := e.recordings[recorded.ID]
	e.mu.Unlock()
	cursor := 0
	outerSeen, nestedSeen := false, false
	for {
		events, err := rec.events(ctx, cursor)
		if err != nil {
			diagnose()
			t.Fatal(err)
		}
		raw, _ := json.Marshal(events)
		var update struct {
			Steps []node `json:"steps"`
			Next  int    `json:"next"`
			Done  bool   `json:"done"`
		}
		json.Unmarshal(raw, &update)
		for _, n := range update.Steps {
			if n.Type == "type" && n.Target != nil {
				outerSeen = outerSeen || n.Value == "Outer capture" && len(n.Target.Frame) == 1
				nestedSeen = nestedSeen || n.Value == "Nested capture" && len(n.Target.Frame) == 2
			}
		}
		cursor = update.Next
		if update.Done {
			break
		}
	}
	if !outerSeen || !nestedSeen {
		t.Fatalf("recording lost iframe actions: outer=%v nested=%v", outerSeen, nestedSeen)
	}
}
