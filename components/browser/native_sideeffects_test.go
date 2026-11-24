package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNativeChromeSideEffectsAndDebugger(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke on a machine allowed to start system Chrome/Edge")
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/file" {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", `attachment; filename="fixture.txt"`)
			w.Write([]byte("fixture download"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/popup" {
			w.Write([]byte("<!doctype html><title>Popup</title><p>Owned popup</p>"))
			return
		}
		w.Write([]byte(`<!doctype html><title>Side effects</title><button id="dialog" onclick="document.body.dataset.choice=confirm('Fixture confirmation')?'yes':'no'">Dialog</button><button id="popup" onclick="window.open('/popup')">Popup</button><a id="download" href="/file" download="fixture.txt">Download</a><button id="submit" onclick="document.body.dataset.submitted='yes'">Do not submit</button>`))
	}))
	defer fixture.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	e, err := newEngine(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	handler := e.handler()
	invoke := func(method, path string, in, out any) {
		t.Helper()
		raw, _ := json.Marshal(in)
		req := httptest.NewRequest(method, "http://127.0.0.1"+path, bytes.NewReader(raw)).WithContext(ctx)
		req.Header.Set("X-Myco-Scope", "native/effects")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		if out != nil {
			if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
				t.Fatal(err)
			}
		}
	}
	var profile profile
	invoke("POST", "/profiles", map[string]string{"name": "Side effects"}, &profile)
	targetAt := func(css string) *target { return &target{Locators: []locator{{Kind: "css", Value: css}}} }
	flow := script{Name: "Side effects", StartURL: fixture.URL, OnError: "stop"}
	flow.Graph.Nodes = []node{
		{ID: "trigger", Type: "click", Target: targetAt("#dialog")},
		{ID: "dialog", Type: "dialog", Dialog: "accept"},
		{ID: "accepted", Type: "assert", Check: "expr", Code: `document.body.dataset.choice==='yes'`},
		{ID: "popup", Type: "click", Target: targetAt("#popup")},
		{ID: "select", Type: "tab", Tab: "popup"},
		{ID: "destination", Type: "wait", Wait: "url", Value: "/popup"},
		{ID: "opener", Type: "tab", Tab: "opener"},
		{ID: "download", Type: "click", Target: targetAt("#download")},
		{ID: "file", Type: "wait", Wait: "download", Into: "file"},
		{ID: "confirm", Type: "confirm", Message: "Stop before submission"},
		{ID: "submit", Type: "click", Target: targetAt("#submit")},
	}
	for i := 1; i < len(flow.Graph.Nodes); i++ {
		flow.Graph.Edges = append(flow.Graph.Edges, []string{flow.Graph.Nodes[i-1].ID, flow.Graph.Nodes[i].ID})
	}
	var started struct {
		ID string `json:"id"`
	}
	invoke("POST", "/runs", runOptions{Script: flow, Profile: profile.ID, Headless: true, Breakpoints: []string{"trigger"}}, &started)
	e.mu.Lock()
	run := e.runs[started.ID]
	e.mu.Unlock()
	cursor := 0
	await := func(match func(runUpdate) bool) runUpdate {
		t.Helper()
		for {
			out, err := run.events(ctx, cursor)
			if err != nil {
				t.Fatal(err)
			}
			cursor = out.Next
			if match(out) {
				return out
			}
			if finishedRun(out.State) {
				t.Fatalf("unexpected finish: %s %s %s", out.State, out.ErrorNode, out.Error)
			}
		}
	}
	await(func(out runUpdate) bool { return out.State == "paused" && out.Node == "trigger" })
	if err := run.control("step", ""); err != nil {
		t.Fatal(err)
	}
	// The click's expected dialog is pre-armed and visible as the next step.
	await(func(out runUpdate) bool { return out.State == "paused" && out.Node == "dialog" })
	if err := run.control("resume", ""); err != nil {
		t.Fatal(err)
	}
	out := await(func(out runUpdate) bool { return out.State == "paused" && out.Node == "confirm" })
	if out.Output["file"] != "fixture.txt" {
		t.Fatalf("download output is not the filename: %#v", out.Output)
	}
	if err := run.control("cancel", ""); err != nil {
		t.Fatal(err)
	}
	await(func(out runUpdate) bool { return out.State == "cancelled" })
	s, err := e.session("native/effects", profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.page()
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := p.Context(ctx).Eval(`()=>document.body.dataset.submitted||''`)
	if err != nil || submitted.Value.Str() != "" {
		t.Fatal("fixture submitted", err)
	}
	// Start-at must preserve this DOM instead of navigating start_url again.
	short := script{Name: "Start here", StartURL: fixture.URL, OnError: "stop"}
	short.Graph.Nodes = []node{{ID: "before", Type: "navigate", URL: fixture.URL}, {ID: "here", Type: "assert", Check: "expr", Code: `document.body.dataset.choice==='yes'`}}
	short.Graph.Edges = [][]string{{"before", "here"}}
	invoke("POST", "/runs", runOptions{Script: short, Profile: profile.ID, Headless: true, StartAt: "here"}, &started)
	e.mu.Lock()
	run = e.runs[started.ID]
	e.mu.Unlock()
	cursor = 0
	await(func(out runUpdate) bool { return out.State == "succeeded" })
}
