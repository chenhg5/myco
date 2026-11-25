package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/proto"
)

func TestNativeChromeRecordsOnlyUserTopLevelNavigation(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with installed Chrome")
	}
	var base string
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/ad" {
			stage, _ := strconv.Atoi(r.URL.Query().Get("stage"))
			if stage < 3 {
				fmt.Fprintf(w, `<!doctype html><p>Ad fixture %d</p><script>history.pushState({},'',location.pathname+location.search+'#ad');setTimeout(()=>location.replace('/ad?stage=%d'),60)</script>`, stage, stage+1)
			} else {
				fmt.Fprint(w, `<!doctype html><p>Ad fixture complete</p><script>parent.postMessage({fixtureAd:true},'*')</script>`)
			}
			return
		}
		if r.URL.Path == "/" {
			fmt.Fprintf(w, `<!doctype html><title>Navigation fixture</title><a id="go" href="/landed">Open article</a><iframe src="/ad?stage=0"></iframe><iframe src="%s/ad?stage=0"></iframe><script>window.adsDone=0;addEventListener('message',e=>{if(e.data&&e.data.fixtureAd)window.adsDone++})</script>`, strings.Replace(base, "127.0.0.1", "ads.localhost", 1))
			return
		}
		fmt.Fprintf(w, `<!doctype html><title>Article fixture</title><h1>%s</h1><a id="hash" href="#section">Read section</a><button id="stay" type="button">Mark viewed</button><div id="section">Article section</div>`, r.URL.Path)
	}))
	defer fixture.Close()
	base = fixture.URL
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	e, err := newEngine(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	invoke := func(method, path string, in, out any) {
		t.Helper()
		raw, _ := json.Marshal(in)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw)).WithContext(ctx)
		req.Header.Set("X-Myco-Scope", "native/navigation")
		w := httptest.NewRecorder()
		e.handler().ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if out != nil && json.Unmarshal(w.Body.Bytes(), out) != nil {
			t.Fatal("invalid response")
		}
	}
	var p profile
	invoke("POST", "/profiles", map[string]string{"name": "Navigation fixture"}, &p)
	var started struct {
		ID string `json:"id"`
	}
	invoke("POST", "/record/start", map[string]any{"profile": p.ID, "url": base, "headless": true}, &started)
	s, err := e.session("native/navigation", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.page()
	if err != nil {
		t.Fatal(err)
	}
	page = page.Context(ctx)
	// Runtime reads issued during a document swap may target its old
	// execution context. First observe the browser's URL, then require the
	// new document itself to be ready, without retaining a JS object ID.
	waitDocument := func(want string) {
		t.Helper()
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		var lastURL string
		var lastErr error
		for {
			info, err := page.Info()
			if err != nil {
				t.Fatal(err)
			}
			lastURL = info.URL
			if info.URL == want {
				var state struct {
					Result *proto.RuntimeRemoteObject `json:"result"`
				}
				lastErr = s.call(page.SessionID, "Runtime.evaluate", map[string]any{
					"expression": "({href:location.href,ready:document.readyState})", "returnByValue": true,
				}, &state)
				if lastErr == nil && state.Result != nil && state.Result.Value.Get("href").Str() == want {
					ready := state.Result.Value.Get("ready").Str()
					if ready == "interactive" || ready == "complete" {
						return
					}
				}
				if lastErr != nil {
					var rpcErr *cdp.Error
					if !errors.As(lastErr, &rpcErr) || rpcErr.Code != -32000 {
						t.Fatal(lastErr)
					}
				}
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-deadline.C:
				t.Fatalf("document did not become ready: want=%s got=%s error=%v", want, lastURL, lastErr)
			case <-tick.C:
			}
		}
	}
	watchCtx, stopWatch := context.WithCancel(ctx)
	watch := s.browser.Context(watchCtx).Event()
	var watchMu sync.Mutex
	var observed []string
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		for msg := range watch {
			switch msg.Method {
			case "Page.frameStartedNavigating":
				var evt frameStartedNavigation
				msg.Load(&evt)
				watchMu.Lock()
				observed = append(observed, fmt.Sprintf("started frame=%s type=%s url=%s", evt.FrameID, evt.NavigationType, evt.URL))
				watchMu.Unlock()
			case "Page.frameRequestedNavigation":
				var evt proto.PageFrameRequestedNavigation
				msg.Load(&evt)
				watchMu.Lock()
				observed = append(observed, fmt.Sprintf("requested frame=%s reason=%s url=%s", evt.FrameID, evt.Reason, evt.URL))
				watchMu.Unlock()
			case "Page.frameNavigated":
				var evt proto.PageFrameNavigated
				msg.Load(&evt)
				if evt.Frame != nil {
					watchMu.Lock()
					observed = append(observed, fmt.Sprintf("committed frame=%s url=%s", evt.Frame.ID, evt.Frame.URL))
					watchMu.Unlock()
				}
			case "Runtime.bindingCalled":
				var evt proto.RuntimeBindingCalled
				msg.Load(&evt)
				var action struct{ Type string }
				if evt.Name == "__mycoBrowserEvent" && json.Unmarshal([]byte(evt.Payload), &action) == nil {
					watchMu.Lock()
					observed = append(observed, fmt.Sprintf("collector context=%d type=%s", evt.ExecutionContextID, action.Type))
					watchMu.Unlock()
				}
			}
		}
	}()
	defer func() {
		stopWatch()
		<-watchDone
		if t.Failed() {
			watchMu.Lock()
			defer watchMu.Unlock()
			for _, line := range observed {
				t.Log(line)
			}
		}
	}()

	if err = page.Wait(rod.Eval(`()=>window.adsDone===2`)); err != nil {
		t.Fatal("advertising frames did not finish", err)
	}
	e.mu.Lock()
	rec := e.recordings[started.ID]
	e.mu.Unlock()
	rec.mu.Lock()
	initial := len(rec.steps)
	rec.mu.Unlock()
	if initial != 0 {
		t.Fatalf("iframe navigation created %d steps", initial)
	}
	s.mu.Lock()
	hasOOPIF := false
	for _, installed := range s.installs {
		hasOOPIF = hasOOPIF || installed.kind == "iframe"
	}
	s.mu.Unlock()
	if !hasOOPIF {
		t.Fatal("fixture did not exercise an out-of-process iframe")
	}
	link, err := page.Element("#go")
	if err != nil {
		t.Fatal(err)
	}
	// A fast document swap may invalidate the input RPC's reply after the
	// trusted click was delivered. Verify its navigation and recorded click
	// below instead of requiring the old document to survive the reply.
	clickErr := link.Click(proto.InputMouseButtonLeft, 1)
	if clickErr != nil {
		var rpcErr *cdp.Error
		if !errors.As(clickErr, &rpcErr) || rpcErr.Code != -32000 || rpcErr.Message != "Inspected target navigated or closed" {
			t.Fatal(clickErr)
		}
	}
	waitDocument(base + "/landed")
	if clickErr != nil {
		t.Logf("click delivered before its document changed: %v", clickErr)
	}
	waitCount := func(n int) {
		t.Helper()
		until := time.Now().Add(5 * time.Second)
		for {
			rec.mu.Lock()
			got := len(rec.steps)
			rec.mu.Unlock()
			if got >= n {
				return
			}
			if time.Now().After(until) {
				rec.mu.Lock()
				steps := append([]node(nil), rec.steps...)
				rec.mu.Unlock()
				t.Fatalf("recorded %d steps, wanted %d: %+v", got, n, steps)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	waitCount(2)
	rec.mu.Lock()
	first := append([]node(nil), rec.steps...)
	rec.mu.Unlock()
	if len(first) != 2 || first[0].Type != "click" || first[1].Type != "wait" || first[1].Wait != "url" || first[1].Value != base+"/landed" {
		t.Fatalf("expected click + top wait: %+v", first)
	}
	hashLink, err := page.Element("#hash")
	if err != nil {
		t.Fatal(err)
	}
	if err = hashLink.Click(proto.InputMouseButtonLeft, 1); err != nil {
		t.Fatal(err)
	}
	waitDocument(base + "/landed#section")
	waitCount(4)
	// Timer-driven navigation outside the user's action window is not a step.
	time.Sleep(navigationActionWindow + 100*time.Millisecond)
	if _, err = page.Eval(`()=>setTimeout(()=>location.assign('/automatic'),0)`); err != nil {
		t.Fatal(err)
	}
	waitDocument(base + "/automatic")
	if _, err = page.Eval(`()=>{history.pushState({},'','/history');location.hash='automatic'}`); err != nil {
		t.Fatal(err)
	}
	waitDocument(base + "/history#automatic")
	// CDP Page.navigate models browser/address-bar initiated navigation.
	if err = page.Navigate(base + "/typed"); err != nil {
		t.Fatal(err)
	}
	waitDocument(base + "/typed")
	stay, err := page.Element("#stay")
	if err != nil {
		t.Fatal(err)
	}
	if err = stay.Click(proto.InputMouseButtonLeft, 1); err != nil {
		t.Fatal(err)
	}
	waitCount(6)
	if err = page.Navigate(base + "/typed#section"); err != nil {
		t.Fatal(err)
	}
	waitDocument(base + "/typed#section")
	invoke("POST", "/record/stop", map[string]string{"id": started.ID}, nil)
	var result struct {
		Steps []node `json:"steps"`
		Done  bool   `json:"done"`
	}
	invoke("GET", "/record/events?id="+started.ID+"&after=0", nil, &result)
	for !result.Done {
		time.Sleep(20 * time.Millisecond)
		invoke("GET", "/record/events?id="+started.ID+"&after=0", nil, &result)
	}
	if len(result.Steps) != 7 {
		t.Fatalf("unexpected automatic or child navigation: %+v", result.Steps)
	}
	for i, want := range []string{"click", "wait", "click", "wait", "navigate", "click", "navigate"} {
		if result.Steps[i].Type != want {
			t.Fatalf("step %d: %+v", i, result.Steps[i])
		}
	}
	if result.Steps[4].URL != base+"/typed" || result.Steps[6].URL != base+"/typed#section" {
		t.Fatalf("address-bar navigation lost: %+v", result.Steps)
	}
}
