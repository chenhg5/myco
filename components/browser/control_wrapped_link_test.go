package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNativeChromeWrappedLinksUsePaintedLineHitPoints(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with installed Chrome")
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/frame" {
			fmt.Fprint(w, `<!doctype html><style>body{font:16px/42px monospace}a{color:blue}</style><a id="link" href="#activated" onclick="window.clicked=event.isTrusted">Frame first line<br>Frame second</a>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><style>body{font:16px/42px monospace}a{color:blue}iframe{position:absolute;left:40px;top:170px;width:360px;height:150px;transform:scale(.9);transform-origin:top left}</style><a id="link" href="#activated" onclick="window.clicked=event.isTrusted">First line<br>Second</a><iframe id="frame" src="/frame"></iframe>`)
	}))
	defer fixture.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	e, err := newEngine(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	call := func(path string, body any) (int, map[string]any) {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", path, bytes.NewReader(raw)).WithContext(ctx)
		r.Header.Set("X-Myco-Scope", "demo/wrapped-link")
		w := httptest.NewRecorder()
		e.handler().ServeHTTP(w, r)
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	_, profile := call("/profiles", map[string]any{"name": "Wrapped link fixture"})
	id := profile["id"].(string)
	call("/profiles/"+id+"/agents", map[string]bool{"allowed": true})
	code, opened := call("/control/open", map[string]any{"request_id": "open", "profile": id, "url": fixture.URL, "grant_id": "demo-grant", "actor": controlActor{ID: "demo-agent"}, "expires_at": time.Now().Add(15 * time.Minute)})
	if code != 200 {
		t.Fatal(code, opened)
	}
	session := opened["session"].(string)
	e.mu.Lock()
	c := e.controls[session]
	e.mu.Unlock()
	p, _ := c.browser.page()
	p = p.Context(ctx)
	// The old union-center really is outside the inline anchor. This fixture
	// cannot pass merely because a browser happens to hit its bounding center.
	check, err := p.Eval(`()=>{const a=document.querySelector('#link'),r=a.getBoundingClientRect(),hit=document.elementFromPoint(r.x+r.width/2,r.y+r.height/2);return {lines:a.getClientRects().length,miss:hit!==a&&!a.contains(hit)}}`)
	if err != nil || check.Value.Get("lines").Int() < 2 || !check.Value.Get("miss").Bool() {
		t.Fatal("fixture center is not between lines", check, err)
	}
	find := func(name string) string {
		t.Helper()
		code, snapshot := call("/control/snapshot", map[string]any{"request_id": randID(), "session": session})
		if code != 200 {
			t.Fatal(code, snapshot)
		}
		for _, item := range snapshot["nodes"].([]any) {
			node := item.(map[string]any)
			if node["role"] == "link" && node["name"] == name {
				return node["ref"].(string)
			}
		}
		t.Fatal("missing link", name, snapshot)
		return ""
	}
	code, result := call("/control/act", map[string]any{"request_id": "wrapped-top", "session": session, "op": "click", "ref": find("First line Second")})
	if code != 200 {
		t.Fatal(code, result)
	}
	trusted, err := p.Eval(`()=>window.clicked===true&&location.hash==='#activated'`)
	if err != nil || !trusted.Value.Bool() {
		t.Fatal("wrapped link did not receive trusted input", err)
	}
	code, result = call("/control/act", map[string]any{"request_id": "wrapped-frame", "session": session, "op": "click", "ref": find("Frame first line Frame second")})
	if code != 200 {
		t.Fatal("frame-local hit point failed", code, result)
	}
	iframe, err := p.Element("#frame")
	if err != nil {
		t.Fatal(err)
	}
	frame, err := iframe.Frame()
	if err != nil {
		t.Fatal(err)
	}
	trusted, err = frame.Context(ctx).Eval(`()=>window.clicked===true&&location.hash==='#activated'`)
	if err != nil || !trusted.Value.Bool() {
		t.Fatal("wrapped frame link did not receive trusted input", err)
	}
	_, err = p.Eval(`()=>{window.clicked=false;const a=document.querySelector('#link'),r=a.getClientRects()[0],cover=document.createElement('div');cover.style.cssText='position:fixed;z-index:100;left:'+r.x+'px;top:'+r.y+'px;width:'+r.width+'px;height:'+r.height+'px;background:black';document.body.append(cover)}`)
	if err != nil {
		t.Fatal(err)
	}
	code, result = call("/control/act", map[string]any{"request_id": "occluded", "session": session, "op": "click", "ref": find("First line Second")})
	if code < 400 || result["code"] != "target_not_interactable" {
		t.Fatal("occlusion check weakened", code, result)
	}
	trusted, err = p.Eval(`()=>window.clicked===false`)
	if err != nil || !trusted.Value.Bool() {
		t.Fatal("occluded link was clicked", err)
	}
}
