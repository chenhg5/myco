package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestControlDecodeRejectsForeignFields(t *testing.T) {
	for _, raw := range []string{`{"request_id":"j","session":"s","op":"click","ref":"r","code":"alert(1)"}`, `{"request_id":"j","session":"s","op":"upload"}`, `{"request_id":"j","session":"s","op":"wait","wait":"url","url":"https://example.com"}`, `{"request_id":"j","session":"s","op":"click","grant_id":"forged","ref":"r"}`, `{"request_id":"j","session":"s","op":"click","ref":null}`} {
		r := httptest.NewRequest("POST", "/control/act", strings.NewReader(raw))
		if _, _, err := controlDecode(r, "act"); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}
func TestControlBudgetAndForcedDiscard(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &controlSession{ctx: ctx, cancel: cancel, wake: make(chan struct{}), expires: time.Now().Add(time.Hour), last: time.Now()}
	for i := 0; i < controlStepLimit; i++ {
		if err := c.appendStep(node{Type: "click"}); err != nil {
			t.Fatal(err)
		}
	}
	if c.budget(node{Type: "click"}) == nil {
		t.Fatal("step overflow accepted")
	}
	c.steps = []node{{Type: "type", Value: strings.Repeat("x", controlByteLimit)}}
	if c.budget(node{Type: "click"}) == nil {
		t.Fatal("byte overflow accepted")
	}
	c.terminate("session_closed", "stopped_by_owner")
	if len(c.steps) != 0 {
		t.Fatal("forced close retained draft")
	}
	if x := c.check(true); x == nil || x.Reason != "stopped_by_owner" {
		t.Fatal(x)
	}
}
func TestNativeChromeControlRefAndDraft(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with installed Chrome")
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><title>Control fixture</title><button onclick="this.dataset.count=String(+(this.dataset.count||0)+1)">Count clicks</button><label>Public title<input id="public"></label><a href="/next">Next article</a><p>Public content</p><div style="height:1900px">Long page</div>`)
	}))
	defer fixture.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	e, err := newEngine(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	diagnose := nativeDiagnostics(t, e)
	call := func(path string, in any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(in)
		req := httptest.NewRequest("POST", path, bytes.NewReader(raw)).WithContext(ctx)
		req.Header.Set("X-Myco-Scope", "demo/control")
		w := httptest.NewRecorder()
		e.handler().ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		if w.Code != 200 {
			t.Log(path, w.Code, w.Body.String())
		}
		return w.Code, out
	}
	_, p := call("/profiles", map[string]any{"name": "Demo agent"})
	id := p["id"].(string)
	call("/profiles/"+id+"/agents", map[string]any{"allowed": true})
	code, opened := call("/control/open", map[string]any{"request_id": "open", "profile": id, "url": fixture.URL, "grant_id": "demo-grant", "actor": map[string]any{"id": "demo-agent", "name": "Demo agent"}, "expires_at": time.Now().Add(15 * time.Minute)})
	if code != 200 {
		diagnose()
		t.Fatal(opened)
	}
	session := opened["session"].(string)
	snapshot := opened["snapshot"].(map[string]any)
	t.Log("nodes", snapshot["node_count"])
	find := func(role, name string) string {
		t.Helper()
		for _, item := range snapshot["nodes"].([]any) {
			n := item.(map[string]any)
			if n["role"] == role && n["name"] == name {
				return n["ref"].(string)
			}
		}
		t.Fatalf("missing %s %s: %v", role, name, snapshot)
		return ""
	}
	ref := find("button", "Count clicks")
	e.mu.Lock()
	control := e.controls[session]
	e.mu.Unlock()
	originalPage, _ := control.browser.page()
	if _, err := originalPage.Context(ctx).Eval(`()=>{const button=document.querySelector('button');button.remove();document.body.prepend(button);}`); err != nil {
		t.Fatal(err)
	}
	if code, out := call("/control/act", map[string]any{"request_id": "detached", "session": session, "op": "click", "ref": ref}); code != 409 || out["code"] != "stale_ref" {
		t.Fatal("reattached node reused old ref", code, out)
	}
	_, snapshot = call("/control/snapshot", map[string]any{"request_id": "refresh", "session": session})
	newRef := find("button", "Count clicks")
	if newRef == ref {
		t.Fatal("ref reused after detach")
	}
	ref = newRef
	input := map[string]any{"request_id": "click", "session": session, "op": "click", "ref": ref}
	if code, out := call("/control/act", input); code != 200 {
		diagnose()
		t.Fatal(out)
	}
	if code, out := call("/control/act", input); code != 200 {
		t.Fatal(out)
	}
	e.mu.Lock()
	c := e.controls[session]
	e.mu.Unlock()
	page, _ := c.browser.page()
	result, err := page.Context(ctx).Eval(`()=>document.querySelector('button').dataset.count`)
	if err != nil || result.Value.Str() != "1" {
		t.Fatal("retry repeated input", result, err)
	}
	if code, out := call("/control/act", map[string]any{"request_id": "click", "session": session, "op": "hover", "ref": ref}); code != 409 || out["code"] != "conflict" {
		t.Fatal("changed request reused cached outcome", code, out)
	}
	c.mu.Lock()
	saved := append([]node{}, c.steps...)
	c.steps = make([]node, controlStepLimit)
	for i := range c.steps {
		c.steps[i] = node{ID: fmt.Sprint("reserved", i), Type: "click"}
	}
	c.mu.Unlock()
	if code, out := call("/control/act", map[string]any{"request_id": "over-budget", "session": session, "op": "click", "ref": ref}); code != 409 || out["code"] != "recording_limit" {
		t.Fatal("budget not checked before dispatch", code, out)
	}
	result, err = page.Context(ctx).Eval(`()=>document.querySelector('button').dataset.count`)
	if err != nil || result.Value.Str() != "1" {
		t.Fatal("over-budget click dispatched", result, err)
	}
	c.mu.Lock()
	c.steps = saved
	c.mu.Unlock()
	shotCode, shot := call("/control/screenshot", map[string]any{"request_id": "shot", "session": session, "full": true})
	if shotCode != 200 {
		t.Fatal(shot)
	}
	if shot["image"].(map[string]any)["height"].(float64) < 1900 {
		t.Fatal("full screenshot clipped", shot)
	}
	if code, out := call("/control/close", map[string]any{"request_id": "close", "session": session}); code != 200 {
		t.Fatal(out)
	} else {
		if len(out["steps"].([]any)) != 2 {
			t.Fatal(out)
		}
	}
	if code, _ := call("/control/snapshot", map[string]any{"request_id": "after", "session": session}); code == 200 {
		t.Fatal("closed session still readable")
	}
}

func TestNativeChromeControlFramesSecretsAndStop(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with installed Chrome")
	}
	var base string
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/child":
			fmt.Fprint(w, `<!doctype html><button onclick="this.textContent='Frame clicked'">Frame action</button><p data-sensitive>demo-secret-text</p><label>One time<input autocomplete="one-time-code" value="123456"></label>`)
		case "/next":
			fmt.Fprint(w, `<!doctype html><h1>Article</h1>`)
		default:
			fmt.Fprintf(w, `<!doctype html><button id="root">Root action</button><a href="/next">Next article</a><label>Public title<input id="public"></label><label>Password<input type="password" value="demo-secret-value"></label><main><p>Visible text</p><p data-sensitive>demo-hidden-data</p></main><a href="#private">Your code <b data-sensitive>protected-947261</b></a><button>Continue <span data-sensitive>protected-836152</span></button><h2>Review <b data-sensitive>protected-725043</b></h2><table><tr><td>Status <b data-sensitive>protected-614932</b></td></tr></table><label>Shown password<input type="text" autocomplete="current-password" value="shown-password-secret"></label><label>OTP alias<input name="otp_code" value="alias-otp-secret"></label><label>SMS alias<input name="sms_otp" value="sms-otp-secret"></label><iframe id="same" src="/child"></iframe><iframe id="cross" src="%s/child"></iframe>`, strings.Replace(base, "127.0.0.1", "ads.localhost", 1))
		}
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
	call := func(path string, in any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(in)
		req := httptest.NewRequest("POST", path, bytes.NewReader(raw)).WithContext(ctx)
		req.Header.Set("X-Myco-Scope", "demo/frames")
		w := httptest.NewRecorder()
		e.handler().ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	_, profile := call("/profiles", map[string]any{"name": "Demo frames"})
	id := profile["id"].(string)
	call("/profiles/"+id+"/agents", map[string]any{"allowed": true})
	code, out := call("/control/open", map[string]any{"request_id": "open", "profile": id, "url": base, "grant_id": "demo-grant", "actor": map[string]any{"id": "demo-agent", "name": "Demo agent"}, "expires_at": time.Now().Add(15 * time.Minute)})
	if code != 200 {
		t.Fatal(code, out)
	}
	session := out["session"].(string)
	snapshot := out["snapshot"].(map[string]any)
	// Open returns at the top document's interactive state. Cross-site
	// renderers may still be attaching; require both real frame controls.
	deadline := time.Now().Add(5 * time.Second)
	for {
		count := 0
		for _, item := range snapshot["nodes"].([]any) {
			n := item.(map[string]any)
			if n["role"] == "button" && n["name"] == "Frame action" {
				count++
			}
		}
		if count == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("both frame controls did not become ready")
		}
		time.Sleep(20 * time.Millisecond)
		code, snapshot = call("/control/snapshot", map[string]any{"request_id": randID(), "session": session})
		if code != 200 {
			t.Fatal(code, snapshot)
		}
	}
	raw, _ := json.Marshal(snapshot)
	for _, secret := range []string{"demo-secret-text", "123456", "demo-secret-value", "demo-hidden-data", "protected-947261", "protected-836152", "protected-725043", "protected-614932", "shown-password-secret", "alias-otp-secret", "sms-otp-secret"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("snapshot leaked sensitive data", secret)
		}
	}
	textCode, textSnapshot := call("/control/snapshot", map[string]any{"request_id": "text-mode", "session": session, "mode": "text"})
	if textCode != 200 {
		t.Fatal(textCode, textSnapshot)
	}
	encodedText, _ := json.Marshal(textSnapshot)
	for _, secret := range []string{"protected-947261", "protected-836152", "protected-725043", "protected-614932", "shown-password-secret", "alias-otp-secret", "sms-otp-secret"} {
		if strings.Contains(string(encodedText), secret) {
			t.Fatal("text snapshot leaked descendant or alias", secret)
		}
	}
	frameRefs := []string{}
	var password, public, article, main, privateButton string
	for _, item := range snapshot["nodes"].([]any) {
		n := item.(map[string]any)
		name, _ := n["name"].(string)
		if name == "Frame action" && n["role"] == "button" {
			frameRefs = append(frameRefs, n["ref"].(string))
		}
		if n["role"] == "textbox" && n["redacted"] == true {
			password = n["ref"].(string)
		}
		if name == "Public title" && n["role"] == "textbox" {
			public = n["ref"].(string)
		}
		if name == "Next article" && n["role"] == "link" {
			article = n["ref"].(string)
		}
		if n["role"] == "button" && n["redacted"] == true {
			privateButton = n["ref"].(string)
		}
		if n["role"] == "main" {
			main = n["ref"].(string)
		}
	}
	if len(frameRefs) != 2 {
		t.Fatalf("need same-process and OOPIF nodes: refs=%v snapshot=%s", frameRefs, raw)
	}
	for i, ref := range frameRefs {
		code, out = call("/control/act", map[string]any{"request_id": fmt.Sprint("frame", i), "session": session, "op": "click", "ref": ref})
		if code != 200 {
			t.Fatal("frame click", code, out)
		}
	}
	code, out = call("/control/act", map[string]any{"request_id": "private-button", "session": session, "op": "click", "ref": privateButton})
	if code != 200 {
		t.Fatal("redacted button click", code, out)
	}
	e.mu.Lock()
	privacyControl := e.controls[session]
	e.mu.Unlock()
	privacyControl.mu.Lock()
	draftJSON, _ := json.Marshal(privacyControl.steps)
	privacyControl.mu.Unlock()
	if strings.Contains(string(draftJSON), "protected-") {
		t.Fatal("draft locator leaked a sensitive descendant")
	}
	privacyPage, _ := privacyControl.browser.page()
	if _, err := privacyPage.Context(ctx).Eval(`()=>document.querySelectorAll('iframe').forEach(frame=>frame.setAttribute('data-sensitive',''))`); err != nil {
		t.Fatal(err)
	}
	for _, ref := range frameRefs {
		code, out = call("/control/extract", map[string]any{"request_id": randID(), "session": session, "ref": ref, "what": "text"})
		if code != 200 || out["text"] != "[redacted]" {
			t.Fatal("sensitive iframe owner not inherited", code, out)
		}
	}
	code, protectedSnapshot := call("/control/snapshot", map[string]any{"request_id": "protected-frames", "session": session})
	protectedJSON, _ := json.Marshal(protectedSnapshot)
	if code != 200 || strings.Contains(string(protectedJSON), "Frame clicked") {
		t.Fatal("sensitive iframe snapshot not redacted", code)
	}
	code, out = call("/control/act", map[string]any{"request_id": "secret", "session": session, "op": "type", "ref": password, "value": "must-not-type"})
	if code != 403 || out["code"] != "sensitive_field" {
		t.Fatal("sensitive field input accepted", code, out)
	}
	code, out = call("/control/extract", map[string]any{"request_id": "extract", "session": session, "ref": main, "what": "html"})
	if code != 200 || strings.Contains(out["text"].(string), "demo-hidden-data") {
		t.Fatal("ancestor HTML not redacted", code, out)
	}
	code, out = call("/control/screenshot", map[string]any{"request_id": "shot", "session": session})
	if code != 403 || out["code"] != "sensitive_snapshot" {
		t.Fatal("sensitive screenshot accepted", code, out)
	}
	code, out = call("/control/act", map[string]any{"request_id": "type", "session": session, "op": "type", "ref": public, "value": "Demo title"})
	if code != 200 {
		t.Fatal(code, out)
	}
	e.mu.Lock()
	c := e.controls[session]
	e.mu.Unlock()
	page, _ := c.browser.page()
	_, err = page.Context(ctx).Eval(`()=>{document.querySelector('#root').remove();document.querySelector('label input').type='password'}`)
	if err != nil {
		t.Fatal(err)
	}
	code, out = call("/control/act", map[string]any{"request_id": "changed", "session": session, "op": "type", "ref": public, "value": "must-not-type"})
	if code != 403 || out["code"] != "sensitive_field" {
		t.Fatal("changed input type accepted", code, out)
	}
	code, out = call("/control/act", map[string]any{"request_id": "navigate", "session": session, "op": "click", "ref": article})
	if code != 200 {
		t.Fatal("top navigation", code, out)
	}
	code, out = call("/control/act", map[string]any{"request_id": "stale", "session": session, "op": "click", "ref": article})
	if code != 409 || out["code"] != "stale_ref" {
		t.Fatal("navigation ref still valid", code, out)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		call("/control/act", map[string]any{"request_id": "wait", "session": session, "op": "wait", "wait": "url", "value": "never-matches", "timeout_ms": 30000})
	}()
	time.Sleep(100 * time.Millisecond)
	started := time.Now()
	nativeControlStop(t, c, page)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("owner stop did not interrupt wait")
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("stop queued behind operation")
	}
	code, out = call("/control/close", map[string]any{"request_id": "close", "session": session})
	if code != 409 || out["reason"] != "stopped_by_owner" {
		t.Fatal("forced stop exported draft", code, out)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.steps) != 0 {
		t.Fatal("forced stop retained steps")
	}
}

// Exercise the same trusted pointer path as an owner. The page cannot call
// the binding or access the closed panel's shadow tree.
func nativeControlStop(t *testing.T, c *controlSession, p *rod.Page) {
	t.Helper()
	s := c.browser
	sid := s.primarySession(p.TargetID)
	if err := s.call(sid, "Page.bringToFront", map[string]any{}, nil); err != nil {
		t.Fatal(err)
	}
	var doc proto.DOMGetDocumentResult
	if err := s.call(sid, "DOM.getDocument", map[string]any{"depth": -1, "pierce": true}, &doc); err != nil {
		t.Fatal(err)
	}
	var root proto.DOMNodeID
	var walk func(*proto.DOMNode)
	walk = func(n *proto.DOMNode) {
		if n == nil {
			return
		}
		for i := 0; i+1 < len(n.Attributes); i += 2 {
			if n.Attributes[i] == "id" && n.Attributes[i+1] == "__myco_recording_panel__" && len(n.ShadowRoots) > 0 {
				root = n.ShadowRoots[0].NodeID
			}
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(doc.Root)
	if root == 0 {
		t.Fatal("agent panel missing")
	}
	var selected proto.DOMQuerySelectorResult
	if err := s.call(sid, "DOM.querySelector", map[string]any{"nodeId": root, "selector": "[data-action=stop]"}, &selected); err != nil || selected.NodeID == 0 {
		t.Fatal("owner stop button missing", err)
	}
	var object proto.DOMResolveNodeResult
	if err := s.call(sid, "DOM.resolveNode", map[string]any{"nodeId": selected.NodeID}, &object); err != nil {
		t.Fatal(err)
	}
	var ready proto.RuntimeCallFunctionOnResult
	if err := s.call(sid, "Runtime.callFunctionOn", map[string]any{"objectId": object.Object.ObjectID, "functionDeclaration": "async function(){this.scrollIntoView();await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return true}", "awaitPromise": true, "returnByValue": true}, &ready); err != nil {
		t.Fatal(err)
	}
	var quad proto.DOMGetContentQuadsResult
	if err := s.call(sid, "DOM.getContentQuads", map[string]any{"nodeId": selected.NodeID}, &quad); err != nil {
		t.Fatal(err)
	}
	box := quad.Box()
	x, y := box.X+box.Width/2, box.Y+box.Height/2
	for _, kind := range []string{"mouseMoved", "mousePressed", "mouseReleased"} {
		if err := s.call(sid, "Input.dispatchMouseEvent", map[string]any{"type": kind, "button": "left", "clickCount": 1, "x": x, "y": y}, nil); err != nil { // Closing the browser may disconnect the release response.
			c.mu.Lock()
			closed := c.closed
			c.mu.Unlock()
			if !closed {
				t.Fatal(err)
			}
		}
	}
}

func TestControlHostMatchingAndLiteralBoundary(t *testing.T) {
	hosts, err := normalizeControlHosts([]string{"Example.COM.", "*.example.org"})
	if err != nil {
		t.Fatal(err)
	}
	for raw, allowed := range map[string]bool{"https://example.com/a": true, "http://example.com:8080/a": true, "https://sub.example.org": true, "https://example.org": false, "https://badexample.com": false, "https://example.com.evil.invalid": false, "https://user@example.com": false} {
		if controlHostAllowed(hosts, raw) != allowed {
			t.Error(raw)
		}
	}
	for _, invalid := range []string{"https://example.com", "*.127.0.0.1", "example.com/path", "*example.com", "a..com"} {
		if _, err := normalizeControlHosts([]string{invalid}); err == nil {
			t.Error("accepted host", invalid)
		}
	}
	req := httptest.NewRequest("POST", "/control/act", strings.NewReader(`{"request_id":"j","session":"s","op":"type","ref":"r","value":"${name}"}`))
	if _, _, err := controlDecode(req, "act"); err == nil || err.Error() != "literal_required" {
		t.Fatal("template expression accepted", err)
	}
}
func TestNativeChromeControlHostsAndForcedEnd(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with installed Chrome")
	}
	var base string
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/slow-redirect":
			select {
			case <-r.Context().Done():
				return
			case <-time.After(2200 * time.Millisecond):
			}
			http.Redirect(w, r, strings.Replace(base, "127.0.0.1", "ads.localhost", 1)+"/blocked", 302)
		case "/redirect":
			http.Redirect(w, r, strings.Replace(base, "127.0.0.1", "ads.localhost", 1)+"/blocked", 302)
		default:
			fmt.Fprint(w, `<!doctype html><title>Domain fixture</title><a href="/redirect">Redirect outside</a><a href="/slow-redirect">Slow redirect</a><a target="_blank" href="/slow-redirect">Slow popup</a><a href="#waiting" onclick="setTimeout(()=>location.href='/redirect',600)">Delayed script</a><a target="_blank" href="http://ads.localhost:`+strings.Split(base, ":")[2]+`/blocked">Popup outside</a><button id="count" onclick="this.dataset.count=String(+(this.dataset.count||0)+1)">Count</button>`)
		}
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
	diagnose := nativeDiagnostics(t, e)
	call := func(path string, in any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(in)
		req := httptest.NewRequest("POST", path, bytes.NewReader(raw)).WithContext(ctx)
		req.Header.Set("X-Myco-Scope", "demo/hosts")
		w := httptest.NewRecorder()
		e.handler().ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	_, profile := call("/profiles", map[string]any{"name": "Demo agent", "kind": "agent"})
	if profile["kind"] != "agent" {
		t.Fatal("profile kind not saved", profile)
	}
	id := profile["id"].(string)
	call("/profiles/"+id+"/agents", map[string]any{"allowed": true})
	code, out := call("/control/open", map[string]any{"request_id": "open", "profile": id, "url": base, "hosts": []string{"127.0.0.1"}, "grant_id": "demo-grant", "actor": map[string]any{"id": "demo-agent", "name": "Demo agent"}, "expires_at": time.Now().Add(15 * time.Minute)})
	if code != 200 {
		t.Fatal(code, out)
	}
	session := out["session"].(string)
	e.mu.Lock()
	c := e.controls[session]
	e.mu.Unlock()
	code, out = call("/control/act", map[string]any{"request_id": "direct", "session": session, "op": "navigate", "url": "https://outside.example.com"})
	if code != 403 || out["code"] != "host_not_allowed" || out["host"] != "outside.example.com" {
		t.Fatal("navigate preflight", code, out)
	}
	find := func(name string) string {
		t.Helper()
		code, snapshot := call("/control/snapshot", map[string]any{"request_id": randID(), "session": session})
		if code != 200 {
			t.Fatal(code, snapshot)
		}
		for _, item := range snapshot["nodes"].([]any) {
			n := item.(map[string]any)
			if n["name"] == name && n["role"] == "link" {
				return n["ref"].(string)
			}
		}
		t.Fatal("missing link", name, snapshot)
		return ""
	}
	code, out = call("/control/act", map[string]any{"request_id": "redirect", "session": session, "op": "click", "ref": find("Redirect outside")})
	if code != 403 || out["code"] != "host_not_allowed" {
		t.Fatal("redirect not rejected", code, out)
	}
	page, _ := c.browser.page()
	if !c.waitAllowed(ctx, page) {
		t.Fatal("outside tab not restored")
	}
	for _, name := range []string{"Slow redirect", "Delayed script", "Slow popup"} {
		c.mu.Lock()
		before := len(c.steps)
		c.mu.Unlock()
		code, out = call("/control/act", map[string]any{"request_id": randID(), "session": session, "op": "click", "ref": find(name)})
		if code != 403 || out["code"] != "host_not_allowed" {
			t.Fatal("late navigation escaped originating act", name, code, out)
		}
		c.mu.Lock()
		after := len(c.steps)
		c.mu.Unlock()
		if after != before {
			t.Fatal("failed late navigation entered draft", name)
		}
		if !c.waitAllowed(ctx, page) {
			t.Fatal("late navigation not restored", name)
		}
	}
	code, out = call("/control/act", map[string]any{"request_id": "popup", "session": session, "op": "click", "ref": find("Popup outside")})
	if code != 403 || out["code"] != "host_not_allowed" {
		c.browser.mu.Lock()
		t.Logf("pages=%d installs=%d observations=%+v", len(c.browser.pages), len(c.browser.installs), c.browser.observations)
		c.browser.mu.Unlock()
		diagnose()
		t.Fatal("outside popup not rejected", code, out)
	}
	c.browser.mu.Lock()
	pages := len(c.browser.pages)
	c.browser.mu.Unlock()
	if pages != 1 {
		t.Fatal("outside popup survived", pages)
	}
	// Force an in-flight wait to end via the trusted App route, not voluntary close.
	done := make(chan struct{})
	go func() {
		defer close(done)
		call("/control/act", map[string]any{"request_id": "wait", "session": session, "op": "wait", "wait": "url", "value": "never-matches", "timeout_ms": 30000})
	}()
	time.Sleep(100 * time.Millisecond)
	code, out = call("/control/close", map[string]any{"request_id": "force", "session": session, "reason": "grant_ended"})
	if code != 200 || out["steps"] != nil {
		t.Fatal("forced end leaked draft", code, out)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("trusted force close queued behind wait")
	}
}

func TestNativeChromeControlOperationsAndReplay(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with installed Chrome")
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><title>Operations fixture</title><label>Title<input id="title"></label><select aria-label="Priority"><option value="low">Low</option><option value="high">High</option></select><div style="cursor:pointer" id="custom" onclick="this.dataset.clicked='yes'">Custom control</div><button onclick="window.clicks=(window.clicks||0)+1">Count</button><button onclick="confirm('Demo confirmation')">Ask</button><a target="_blank" href="/popup">Open tab</a><p>Public text</p><div style="height:1800px">Long page</div>`)
	}))
	defer fixture.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	e, err := newEngine(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	diagnose := nativeDiagnostics(t, e)
	call := func(path string, in any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(in)
		req := httptest.NewRequest("POST", path, bytes.NewReader(raw)).WithContext(ctx)
		req.Header.Set("X-Myco-Scope", "demo/operations")
		w := httptest.NewRecorder()
		e.handler().ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	_, profile := call("/profiles", map[string]any{"name": "Demo operations"})
	id := profile["id"].(string)
	call("/profiles/"+id+"/agents", map[string]any{"allowed": true})
	code, out := call("/control/open", map[string]any{"request_id": "open", "profile": id, "url": fixture.URL, "grant_id": "demo-grant", "actor": map[string]any{"id": "demo-agent", "name": "Demo agent"}, "expires_at": time.Now().Add(15 * time.Minute)})
	if code != 200 {
		t.Fatal(code, out)
	}
	session := out["session"].(string)
	find := func(role, name string) string {
		t.Helper()
		code, snap := call("/control/snapshot", map[string]any{"request_id": randID(), "session": session})
		if code != 200 {
			t.Fatal(code, snap)
		}
		for _, item := range snap["nodes"].([]any) {
			n := item.(map[string]any)
			if n["name"] == name && n["role"] == role && n["ref"] != nil {
				return n["ref"].(string)
			}
		}
		t.Fatal("missing node", role, name, snap)
		return ""
	}
	act := func(op string, fields map[string]any) map[string]any {
		t.Helper()
		if fields == nil {
			fields = map[string]any{}
		}
		fields["session"] = session
		fields["request_id"] = randID()
		fields["op"] = op
		code, out := call("/control/act", fields)
		if code != 200 {
			diagnose()
			t.Fatal(op, code, out)
		}
		return out
	}
	public := find("textbox", "Title")
	act("type", map[string]any{"ref": public, "value": "Demo"})
	act("press", map[string]any{"ref": public, "key": "End"})
	act("select", map[string]any{"ref": find("combobox", "Priority"), "value": "high"})
	act("click", map[string]any{"ref": find("generic", "Custom control")})
	act("hover", map[string]any{"ref": find("button", "Count")})
	act("dblclick", map[string]any{"ref": find("button", "Count")})
	act("wait", map[string]any{"wait": "element", "ref": public})
	act("click", map[string]any{"ref": find("button", "Ask")})
	act("dialog", map[string]any{"choice": "dismiss"})
	popup := act("click", map[string]any{"ref": find("link", "Open tab")})
	if popup["new_tab"] != true {
		t.Fatal("popup missing", popup)
	}
	act("close_tab", map[string]any{"tab": popup["active_tab"]})
	act("navigate", map[string]any{"url": fixture.URL + "/next"})
	act("wait", map[string]any{"wait": "url", "value": "/next"})
	act("back", nil)
	act("wait", map[string]any{"wait": "url", "value": fixture.URL + "/"})
	act("forward", nil)
	act("wait", map[string]any{"wait": "url", "value": "/next"})
	act("reload", nil)
	act("scroll", map[string]any{"dy": 500})
	act("wait", map[string]any{"wait": "network"})
	code, out = call("/control/close", map[string]any{"request_id": "close", "session": session})
	if code != 200 {
		t.Fatal(code, out)
	}
	raw, _ := json.Marshal(out["steps"])
	var steps []node
	if json.Unmarshal(raw, &steps) != nil {
		t.Fatal("invalid draft")
	}
	types := map[string]bool{}
	for _, n := range steps {
		types[n.Type] = true
		if n.Type == "tab" && n.Tab == "close" {
			types["close_tab"] = true
		}
	}
	for _, kind := range []string{"back", "forward", "reload", "close_tab", "type", "select", "dialog"} {
		if !types[kind] {
			t.Fatal("missing replay node", kind)
		}
	}
	// The same draft format must be accepted by replay validation.
	script := script{}
	script.Graph.Nodes = steps
	for i := 1; i < len(steps); i++ {
		script.Graph.Edges = append(script.Graph.Edges, []string{steps[i-1].ID, steps[i].ID})
	}
	if _, err = linearOrder(script, ""); err != nil {
		t.Fatal("draft not replayable", err)
	}
}

func TestControlRetryDigestIgnoresObjectKeyOrder(t *testing.T) {
	samples := []string{`{"request_id":"j","profile":"p","actor":{"id":"demo","name":"Demo"},"grant_id":"g","expires_at":"2026-01-01T00:00:00Z"}`, `{"expires_at":"2026-01-01T00:00:00Z","profile":"p","request_id":"j","grant_id":"g","actor":{"name":"Demo","id":"demo"}}`}
	var previous []byte
	for _, sample := range samples {
		r := httptest.NewRequest("POST", "/control/open", strings.NewReader(sample))
		_, canonical, err := controlDecode(r, "open")
		if err != nil {
			t.Fatal(err)
		}
		if previous != nil && !bytes.Equal(previous, canonical) {
			t.Fatal("nested object order changed retry digest")
		}
		previous = canonical
	}
}

func TestControlClosedSessionsExpireWithoutLifetimeQuota(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e, err := newEngine(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	e.controls = map[string]*controlSession{}
	e.controlRequests = map[string]*controlRequest{}
	requests := 0
	for round := 0; round < 30; round++ {
		life, stop := context.WithCancel(ctx)
		id := fmt.Sprint("demo", round)
		c := &controlSession{e: e, id: id, scope: "demo/cache", ctx: life, cancel: stop, expires: time.Now().Add(15 * time.Minute), last: time.Now(), wake: make(chan struct{}), browser: &session{e: e, id: "p_demo", closeComplete: true}, steps: []node{}}
		e.mu.Lock()
		e.controls[id] = c
		e.mu.Unlock()
		for n := 0; n < 170; n++ {
			input, _ := json.Marshal(map[string]any{"request_id": fmt.Sprint("request", requests), "session": id})
			r := httptest.NewRequest("POST", "/control/close", bytes.NewReader(input))
			r.Header.Set("X-Myco-Scope", c.scope)
			w := httptest.NewRecorder()
			e.handler().ServeHTTP(w, r)
			if w.Code != 200 {
				t.Fatalf("round %d request %d: %d %s", round, requests, w.Code, w.Body.String())
			}
			requests++
		}
		e.pruneControls(time.Now())
		if e.controls[id] == nil {
			t.Fatal("retry window was not retained")
		}
		e.pruneControls(time.Now().Add(controlRetryWindow + time.Second))
		if len(e.controls) != 0 || len(e.controlRequests) != 0 || e.controlCacheBytes != 0 {
			t.Fatalf("expired cache retained resources: %d %d %d", len(e.controls), len(e.controlRequests), e.controlCacheBytes)
		}
	}
	if requests < 5000 {
		t.Fatal("insufficient lifetime requests")
	}
}
func TestControlEmptyHostScopeIsRejected(t *testing.T) {
	r := httptest.NewRequest("POST", "/control/open", strings.NewReader(`{"request_id":"j","profile":"p","hosts":[]}`))
	if _, _, err := controlDecode(r, "open"); err == nil {
		t.Fatal("empty hosts became unrestricted")
	}
	r = httptest.NewRequest("POST", "/control/open", strings.NewReader(`{"request_id":"j","profile":"p"}`))
	if _, _, err := controlDecode(r, "open"); err != nil {
		t.Fatal("omitted hosts must remain unrestricted", err)
	}
}

func TestControlRecordedURLNeverIntroducesParameters(t *testing.T) {
	for _, raw := range []string{"https://example.com/path#${x}", "https://example.com/path#%24%7Bx%7D", "https://example.com/%24%7Bx%7D#section"} {
		value := recordedControlURL(raw)
		if strings.Contains(value, "${") {
			t.Fatalf("recorded parameter: %s", value)
		}
	}
	if value := recordedControlURL("https://example.com/path#section"); value != "#section" {
		t.Fatal("ordinary hash navigation changed", value)
	}
}

func TestSensitiveFieldPolicyAliases(t *testing.T) {
	for _, attrs := range []map[string]string{{"type": "password"}, {"type": "text", "autocomplete": "current-password"}, {"type": "text", "autocomplete": "new-password"}, {"name": "otp_code"}, {"name": "sms_otp"}, {"name": "verification_code"}, {"data-sensitive": ""}} {
		if !sensitiveAttributes(attrs) {
			t.Fatal("sensitive alias accepted", attrs)
		}
	}
	if sensitiveAttributes(map[string]string{"name": "title", "type": "text"}) {
		t.Fatal("ordinary text marked sensitive")
	}
}

func TestControlNavigationReserveUsesEncodedSubstringBound(t *testing.T) {
	raw := "https://example.com/path/" + strings.Repeat("&", 5000)
	value := recordedControlURL(raw)
	encoded, _ := json.Marshal(value)
	if len(encoded) > navigationWaitValueLimit || !strings.Contains(raw, value) {
		t.Fatal("invalid bounded URL wait", len(encoded))
	}
	reserve, _ := json.Marshal(reservedNavigationValue())
	if len(encoded) > len(reserve) {
		t.Fatal("URL wait exceeded reserved bytes")
	}
	if recordedControlURL("https://example.com/path#a%20b") != "#a%20b" {
		t.Fatal("escaped fragment changed")
	}
}

func TestControlRejectsOverlongGrantBeforeOpening(t *testing.T) {
	e := &engine{}
	_, err := e.controlOpen(context.Background(), "demo/scope", controlInput{Profile: "demo", GrantID: "grant", Actor: controlActor{ID: "demo-agent"}, ExpiresAt: time.Now().Add(31 * time.Minute)})
	if err == nil || err.Code != "invalid_argument" {
		t.Fatal("overlong grant accepted", err)
	}
}
