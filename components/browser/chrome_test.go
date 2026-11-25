package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var browserSmoke = flag.Bool("browser-smoke", false, "exercise the installed system browser in a fresh dedicated profile")

// CI enables this on the native packaging machines. It never downloads a
// browser, uses an everyday profile, signs in, or submits a real website form.
func TestNativeChromeRecordAndReplayTenTimes(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke on a machine that can start its system Chrome/Edge")
	}
	if len(installedBrowsers()) == 0 {
		t.Fatal("system Chrome/Edge required; no browser is downloaded")
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/child" {
			w.Write([]byte(`<label>Child<input id="child-value" data-testid="child-value"></label>`))
			return
		}
		w.Write([]byte(`<!doctype html><title>Automation fixture</title><form id="form"><label>Title<input id="title" data-testid="title"></label><label>Body<textarea id="body" placeholder="Body"></textarea></label><div id="host"></div><iframe id="child" src="/child"></iframe><button id="finish" type="button">Finish editing</button><button id="submit" type="submit">Do not submit</button></form><div class="blank" style="width:48px;height:20px"></div><script>window.submitted=0;document.querySelector('form').onsubmit=e=>{e.preventDefault();window.submitted++};document.querySelector('#host').attachShadow({mode:'open'}).innerHTML='<label>Shadow<input id="shadow" aria-label="Shadow"></label>';</script>`))
	}))
	defer fixture.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	e, err := newEngine(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	diagnose := nativeDiagnostics(t, e)
	handler := e.handler()
	invoke := func(method, path string, input any, out any) {
		t.Helper()
		raw, _ := json.Marshal(input)
		req := httptest.NewRequest(method, "http://127.0.0.1"+path, bytes.NewReader(raw)).WithContext(ctx)
		req.Header.Set("X-Myco-Scope", "fixture/page")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != 200 {
			diagnose()
			t.Fatalf("%s %s: HTTP %d %s", method, path, w.Code, w.Body.String())
		}
		if out != nil {
			if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
				t.Fatal(err)
			}
		}
	}
	var p profile
	invoke("POST", "/profiles", map[string]string{"name": "Native fixture"}, &p)
	var started struct {
		ID string `json:"id"`
	}
	invoke("POST", "/record/start", map[string]any{"profile": p.ID, "url": fixture.URL, "headless": true}, &started)
	s, err := e.session("fixture/page", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.page()
	if err != nil {
		t.Fatal(err)
	}
	page = page.Context(ctx)
	hidden, err := page.Eval(`()=>typeof window.__mycoBrowserEvent`)
	if err != nil || hidden.Value.Str() != "undefined" {
		t.Fatal("collector binding leaked into the website's world", err)
	}
	for selector, value := range map[string]string{"#title": "Myco native fixture", "#body": "Recorded body"} {
		el, err := page.Element(selector)
		if err != nil {
			t.Fatal(err)
		}
		if err = el.Input(value); err != nil {
			t.Fatal(err)
		}
	}
	host, err := page.Element("#host")
	if err != nil {
		t.Fatal(err)
	}
	shadow, err := host.ShadowRoot()
	if err != nil {
		t.Fatal(err)
	}
	inside, err := shadow.Element("#shadow")
	if err != nil {
		t.Fatal(err)
	}
	if err = inside.Input("Shadow text"); err != nil {
		t.Fatal(err)
	}
	iframe, err := page.Element("#child")
	if err != nil {
		t.Fatal(err)
	}
	frame, err := iframe.Frame()
	if err != nil {
		t.Fatal(err)
	}
	child, err := frame.Element("#child-value")
	if err != nil {
		t.Fatal(err)
	}
	if err = child.Input("Frame text"); err != nil {
		t.Fatal(err)
	}
	// An unnamed document element has no semantic locator or ID: CSS is
	// its valid locator, and recording/replay must preserve that click too.
	blank, err := page.Element("body > .blank")
	if err != nil {
		t.Fatal(err)
	}
	if err = blank.Click("left", 1); err != nil {
		t.Fatal(err)
	}
	finish, err := page.Element("#finish")
	if err != nil {
		t.Fatal(err)
	}
	if err = finish.Click("left", 1); err != nil {
		t.Fatal(err)
	}
	invoke("POST", "/record/stop", map[string]string{"id": started.ID}, nil)
	e.mu.Lock()
	rec := e.recordings[started.ID]
	e.mu.Unlock()
	steps := []node{}
	after := 0
	for {
		events, err := rec.events(ctx, after)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(events)
		var out struct {
			Steps []node `json:"steps"`
			Next  int    `json:"next"`
			Done  bool   `json:"done"`
		}
		json.Unmarshal(raw, &out)
		steps = append(steps, out.Steps...)
		after = out.Next
		if out.Done {
			break
		}
	}
	if len(steps) < 6 {
		t.Fatalf("not enough captured actions: %d", len(steps))
	}
	foundFrame, foundShadow, foundCSSOnly := false, false, false
	formActions := map[string]string{"#title": "type", "#body": "type", "#shadow": "type", "#child-value": "type", "#finish": "click"}
	foundForm := map[string]bool{}
	for _, n := range steps {
		if n.Target != nil {
			foundFrame = foundFrame || len(n.Target.Frame) > 0
			foundShadow = foundShadow || len(n.Target.Shadow) > 0
			for _, l := range n.Target.Locators {
				if l.Kind != "css" {
					continue
				}
				// Named controls in this fixture have semantic/ID fallbacks.
				// Unnamed document targets may legitimately have only CSS.
				if want := formActions[l.Value]; want != "" && n.Type == want {
					if len(n.Target.Locators) < 2 {
						t.Fatalf("missing form fallback: node=%+v target=%+v", n, n.Target)
					}
					foundForm[l.Value] = true
				}
				if n.Type == "click" && l.Value == "html > body > div" && len(n.Target.Locators) == 1 && len(n.Target.Frame) == 0 && len(n.Target.Shadow) == 0 {
					foundCSSOnly = true
				}
			}
		}
	}
	for selector := range formActions {
		if !foundForm[selector] {
			t.Fatalf("required form action missing: %s; steps=%+v", selector, steps)
		}
	}
	if !foundCSSOnly {
		t.Fatalf("unnamed document click with a CSS-only target missing: %+v", steps)
	}
	if !foundFrame || !foundShadow {
		t.Fatalf("capture paths missing: frame=%v shadow=%v", foundFrame, foundShadow)
	}
	var script script
	script.Name = "Native form fixture"
	script.StartURL = fixture.URL
	script.OnError = "stop"
	script.Params = []param{}
	script.Graph.Nodes = steps
	for i := 1; i < len(steps); i++ {
		script.Graph.Edges = append(script.Graph.Edges, []string{steps[i-1].ID, steps[i].ID})
	}
	for iteration := 0; iteration < 10; iteration++ {
		var run struct {
			ID string `json:"id"`
		}
		invoke("POST", "/runs", runOptions{Script: script, ScriptID: "scr_fixture", ScriptVersion: 1, Profile: p.ID, Headless: true, Params: map[string]string{}, Secrets: map[string]string{}}, &run)
		e.mu.Lock()
		live := e.runs[run.ID]
		e.mu.Unlock()
		cursor := 0
		for {
			out, err := live.events(ctx, cursor)
			if err != nil {
				t.Fatal(err)
			}
			cursor = out.Next
			if finishedRun(out.State) {
				if out.State != "succeeded" {
					t.Fatalf("iteration %d failed at %s (%s)", iteration+1, out.ErrorNode, out.Error)
				}
				break
			}
		}
		page, err = s.page()
		if err != nil {
			t.Fatal(err)
		}
		page = page.Context(ctx)
		value, err := page.Eval(`()=>({title:document.querySelector('#title').value,body:document.querySelector('#body').value,shadow:document.querySelector('#host').shadowRoot.querySelector('#shadow').value,submitted:window.submitted})`)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(value.Value.Val())
		if !strings.Contains(string(raw), "Myco native fixture") || !strings.Contains(string(raw), "Shadow text") || !strings.Contains(string(raw), `"submitted":0`) {
			t.Fatalf("iteration %d did not reproduce the form", iteration+1)
		}
	}
	// Verify deterministic start_at with a breakpoint before touching Chrome.
	ordered, err := linearOrder(script, steps[len(steps)-1].ID)
	if err != nil || len(ordered) != 1 {
		t.Fatal("start_at", err)
	}
	var count struct {
		Count int `json:"count"`
	}
	invoke("POST", "/locate", map[string]any{"profile": p.ID, "target": target{Locators: []locator{{Kind: "testid", Value: "title"}}}}, &count)
	if count.Count != 1 {
		t.Fatal("locate count", count.Count)
	}

}
