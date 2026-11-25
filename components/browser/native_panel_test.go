package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

var panelShots = flag.String("browser-panel-shots", "", "optional directory for native recorder panel screenshots")

func TestNativeChromeRecorderPanel(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with installed Chrome/Edge")
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><title>Recording fixture</title><style>body{font:16px system-ui;padding:32px;background:#fafafa;color:#222}button,input{padding:10px;margin:10px}iframe{width:300px;height:120px}</style><button id="corner" style="position:fixed;right:16px;top:16px;width:270px;height:40px;margin:0" onclick="window.cornerClicked=(window.cornerClicked||0)+1">Sign in fixture</button><h1>Recording fixture</h1><p id="text" data-kind="fixture">A public fixture for recording controls</p><button id="site" onclick="window.siteClicks=(window.siteClicks||0)+1">Save draft</button><label>Title<input id="title" placeholder="Draft title"></label>`)
		if r.URL.Path != "/child" {
			fmt.Fprint(w, `<iframe id="child" src="/child"></iframe>`)
		}
	}))
	defer fixture.Close()
	for _, lang := range []string{"en-US", "zh-CN"} {
		t.Run(lang, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
				req.Header.Set("X-Myco-Scope", "native/panel")
				w := httptest.NewRecorder()
				e.handler().ServeHTTP(w, req)
				if w.Code != 200 {
					t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
				}
				if out != nil {
					if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
						t.Fatal(err)
					}
				}
			}
			var profile profile
			invoke("POST", "/profiles", map[string]string{"name": "Panel fixture"}, &profile)
			invoke("PATCH", "/profiles/"+profile.ID, map[string]any{"settings": profileSettingsInput{Lang: lang, Args: []string{}}}, nil)
			invoke("POST", "/sessions", map[string]any{"profile": profile.ID, "headless": false}, nil)
			s, err := e.session("native/panel", profile.ID)
			if err != nil {
				t.Fatal(err)
			}
			page, err := s.page()
			if err != nil {
				t.Fatal(err)
			}
			page = page.Context(ctx)
			if err = page.Navigate(fixture.URL); err != nil {
				t.Fatal(err)
			}
			if err = page.WaitLoad(); err != nil {
				t.Fatal(err)
			}
			sid := s.primarySession(page.TargetID)
			wait := func(check func() bool) {
				t.Helper()
				for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
					if check() {
						return
					}
					select {
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					case <-time.After(20 * time.Millisecond):
					}
				}
				t.Fatal("native panel condition timed out")
			}
			var root proto.DOMNodeID
			refreshRoot := func() {
				root = 0
				wait(func() bool {
					var doc struct {
						Root *proto.DOMNode `json:"root"`
					}
					if s.call(sid, "DOM.getDocument", map[string]any{"depth": -1, "pierce": true}, &doc) != nil {
						return false
					}
					var visit func(*proto.DOMNode)
					visit = func(n *proto.DOMNode) {
						if n == nil {
							return
						}
						for i := 0; i+1 < len(n.Attributes); i += 2 {
							if n.Attributes[i] == "id" && n.Attributes[i+1] == "__myco_recording_panel__" && len(n.ShadowRoots) > 0 {
								root = n.ShadowRoots[0].NodeID
							}
						}
						for _, c := range n.Children {
							visit(c)
						}
					}
					visit(doc.Root)
					return root != 0
				})
			}
			refreshRoot()
			query := func(selector string) proto.DOMNodeID {
				t.Helper()
				var id proto.DOMNodeID
				wait(func() bool {
					var out struct {
						ID proto.DOMNodeID `json:"nodeId"`
					}
					if s.call(sid, "DOM.querySelector", map[string]any{"nodeId": root, "selector": selector}, &out) != nil {
						return false
					}
					id = out.ID
					return id != 0
				})
				return id
			}
			click := func(selector string) {
				t.Helper()
				wait(func() bool {
					var attrs struct {
						Attributes []string `json:"attributes"`
					}
					if s.call(sid, "DOM.getAttributes", map[string]any{"nodeId": query(selector)}, &attrs) != nil {
						return false
					}
					for i := 0; i+1 < len(attrs.Attributes); i += 2 {
						if attrs.Attributes[i] == "disabled" {
							return false
						}
					}
					return true
				})

				// DOM insertion is not a presented frame. In headed Chrome a
				// first input can otherwise focus the window or hit the old
				// compositor frame behind the freshly mounted controls.
				focusBeforeActivation, focusErr := page.Eval(`()=>document.hasFocus()`)
				if focusErr != nil {
					t.Fatal(focusErr)
				}
				if err := s.call(sid, "Page.bringToFront", map[string]any{}, nil); err != nil {
					t.Fatal(err)
				}
				var object struct {
					Object *proto.RuntimeRemoteObject `json:"object"`
				}
				if err := s.call(sid, "DOM.resolveNode", map[string]any{"nodeId": query(selector)}, &object); err != nil || object.Object == nil {
					t.Fatal("resolve click target", err)
				}
				defer s.call(sid, "Runtime.releaseObject", map[string]any{"objectId": object.Object.ObjectID}, nil)
				var ready struct {
					Result *proto.RuntimeRemoteObject `json:"result"`
				}
				err := s.call(sid, "Runtime.callFunctionOn", map[string]any{
					"objectId": object.Object.ObjectID, "awaitPromise": true, "returnByValue": true,
					"functionDeclaration": `async function(){
                    const before=document.hasFocus();
                    await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
                    const r=this.getBoundingClientRect(),x=r.left+r.width/2,y=r.top+r.height/2;
                    const hit=this.getRootNode().elementFromPoint(x,y);
                    this.__nativePanelClick=null;
                    this.addEventListener('click',e=>{this.__nativePanelClick={trusted:e.isTrusted,focused:document.hasFocus()}},{once:true});
                    return {x,y,hit:hit===this||this.contains(hit),focused:document.hasFocus(),before};
                  }`,
				}, &ready)
				if err != nil || ready.Result == nil {
					t.Fatal("panel presentation", err)
				}
				raw, _ := json.Marshal(ready.Result.Value.Val())
				var point struct {
					X, Y                 float64
					Hit, Focused, Before bool
				}
				if json.Unmarshal(raw, &point) != nil || !point.Hit {
					t.Fatalf("panel button not hit-testable: %s %s", selector, raw)
				}
				if selector == `[data-action="start"]` {
					t.Logf("presented start button: focused=%v before-frame=%v before-activation=%v", point.Focused, point.Before, focusBeforeActivation.Value.Bool())
				}
				for _, kind := range []string{"mouseMoved", "mousePressed", "mouseReleased"} {
					if err := s.call(sid, "Input.dispatchMouseEvent", map[string]any{"type": kind, "x": point.X, "y": point.Y, "button": "left", "clickCount": 1}, nil); err != nil {
						t.Fatal(err)
					}
				}
				var received struct {
					Result *proto.RuntimeRemoteObject `json:"result"`
				}
				if err := s.call(sid, "Runtime.callFunctionOn", map[string]any{"objectId": object.Object.ObjectID, "functionDeclaration": "function(){return this.__nativePanelClick}", "returnByValue": true}, &received); err != nil || received.Result == nil {
					t.Fatal("panel click receipt", err)
				}
				if !received.Result.Value.Get("trusted").Bool() {
					t.Fatalf("panel button did not receive trusted input: %s %s", selector, received.Result.Value.JSON("", ""))
				}

			}
			nodeEval := func(selector, fn string) {
				t.Helper()
				var obj struct {
					Object *proto.RuntimeRemoteObject `json:"object"`
				}
				if err := s.call(sid, "DOM.resolveNode", map[string]any{"nodeId": query(selector)}, &obj); err != nil || obj.Object == nil {
					t.Fatal("resolve panel node", err)
				}
				defer s.call(sid, "Runtime.releaseObject", map[string]any{"objectId": obj.Object.ObjectID}, nil)
				if err := s.call(sid, "Runtime.callFunctionOn", map[string]any{"objectId": obj.Object.ObjectID, "functionDeclaration": fn}, nil); err != nil {
					t.Fatal(err)
				}
			}
			if out, err := page.Eval(`()=>({closed:document.querySelector('#__myco_recording_panel__').shadowRoot===null,binding:typeof window.__mycoBrowserEvent})`); err != nil || !strings.Contains(out.Value.JSON("", ""), `"closed":true`) || !strings.Contains(out.Value.JSON("", ""), `"binding":"undefined"`) {
				t.Fatal("panel root/binding exposed to site", err, out)
			}
			iframe, err := page.Element("#child")
			if err != nil {
				t.Fatal(err)
			}
			child, err := iframe.Frame()
			if err != nil {
				t.Fatal(err)
			}
			child = child.Context(ctx)
			if out, err := child.Eval(`()=>!!document.querySelector('#__myco_recording_panel__')`); err != nil || out.Value.Bool() {
				t.Fatal("panel injected into iframe", err)
			}
			click(`[data-action="start"]`)
			var rec *recording
			wait(func() bool { s.mu.Lock(); rec = s.recorder; s.mu.Unlock(); return rec != nil })
			var status struct {
				Recordings []recordingInfo `json:"recordings"`
			}
			invoke("GET", "/status", nil, &status)
			if len(status.Recordings) != 1 || status.Recordings[0].Origin != "browser" || strings.TrimRight(status.Recordings[0].URL, "/") != fixture.URL {
				t.Fatal("browser recording not discoverable", status)
			}
			// The panel's own collapse/start actions must not produce any steps.
			click(`[data-action="collapse"]`)
			click(`[data-action="collapse"]`)
			dragPanel := func(dx, dy float64) {
				t.Helper()
				var box struct {
					Model struct {
						Border []float64 `json:"border"`
					} `json:"model"`
				}
				if err := s.call(sid, "DOM.getBoxModel", map[string]any{"nodeId": query("header")}, &box); err != nil {
					t.Fatal(err)
				}
				q := box.Model.Border
				x, y := q[0]+20, q[1]+12
				for _, event := range []struct {
					kind    string
					x, y    float64
					buttons int
				}{{"mousePressed", x, y, 1}, {"mouseMoved", x + dx, y + dy, 1}, {"mouseReleased", x + dx, y + dy, 0}} {
					if err := s.call(sid, "Input.dispatchMouseEvent", map[string]any{"type": event.kind, "x": event.x, "y": event.y, "button": "left", "buttons": event.buttons, "clickCount": 1}, nil); err != nil {
						t.Fatal(err)
					}
				}
				var after struct {
					Model struct {
						Border []float64 `json:"border"`
					} `json:"model"`
				}
				if err := s.call(sid, "DOM.getBoxModel", map[string]any{"nodeId": query("header")}, &after); err != nil {
					t.Fatal(err)
				}
				if delta := after.Model.Border[0] - q[0] - dx; delta > 2 || delta < -2 {
					t.Fatal("panel header did not drag", delta)
				}
			}
			dragPanel(-40, 60)
			dragPanel(40, -60)

			time.Sleep(100 * time.Millisecond)
			rec.mu.Lock()
			count := len(rec.steps)
			rec.mu.Unlock()
			if count != 0 {
				t.Fatal("panel recorded itself", count)
			}
			button, err := page.Element("#site")
			if err != nil {
				t.Fatal(err)
			}
			if err = button.Click(proto.InputMouseButtonLeft, 1); err != nil {
				t.Fatal(err)
			}
			nodesOfType := func(kind string) []node {
				rec.mu.Lock()
				defer rec.mu.Unlock()
				out := []node{}
				for _, n := range rec.steps {
					if n.Type == kind {
						out = append(out, n)
					}
				}
				return out
			}
			wait(func() bool { return len(nodesOfType("click")) == 1 })
			var first struct {
				Steps   []node   `json:"steps"`
				Next    int      `json:"next"`
				Removed []string `json:"removed"`
			}
			invoke("GET", "/record/events?id="+rec.id+"&after=0", nil, &first)
			firstClick := nodesOfType("click")[0]
			if firstClick.Target == nil {
				t.Fatal("recorded site click missing")
			}
			invoke("GET", "/status", nil, &status)
			if len(status.Recordings) != 0 {
				t.Fatal("claimed browser recording still active")
			}
			click(`[data-step-id="` + firstClick.ID + `"]`)
			wait(func() bool { rec.mu.Lock(); defer rec.mu.Unlock(); return rec.removed[firstClick.ID] })
			var deleted struct {
				Removed []string `json:"removed"`
			}
			invoke("GET", fmt.Sprintf("/record/events?id=%s&after=%d", rec.id, first.Next), nil, &deleted)
			if len(deleted.Removed) != 1 || deleted.Removed[0] != firstClick.ID {
				t.Fatal("sent deletion missing", deleted)
			}
			for i, what := range []string{"text", "attribute", "html"} {
				click(`[data-action="pick"]`)
				wait(func() bool { rec.mu.Lock(); defer rec.mu.Unlock(); return rec.picking })
				target, err := page.Element("#text")
				if err != nil {
					t.Fatal(err)
				}
				if err = target.Click(proto.InputMouseButtonLeft, 1); err != nil {
					t.Fatal(err)
				}
				wait(func() bool { rec.mu.Lock(); defer rec.mu.Unlock(); return rec.selected != nil })
				// Changing capture choices inside the closed root never becomes a recorded input.
				nodeEval("select", `function(){this.value='`+what+`';this.dispatchEvent(new Event('change',{bubbles:true}))}`)
				if what == "attribute" {
					nodeEval(".capture label:nth-of-type(2) input", `function(){this.value='data-kind'}`)
				}
				nodeEval(".capture label:last-of-type input", `function(){this.focus();this.value=''}`)
				if err := s.call(sid, "Input.insertText", map[string]any{"text": fmt.Sprintf("data%d", i+1)}, nil); err != nil {
					t.Fatal(err)
				}
				click(`[data-action="extract"]`)
				wait(func() bool { return len(nodesOfType("extract")) == i+1 })
				n := nodesOfType("extract")[i]
				if n.Type != "extract" || n.What != what || n.Into != fmt.Sprintf("data%d", i+1) {
					t.Fatalf("wrong capture node: %+v", n)
				}
			}
			click(`[data-action="pick"]`)
			wait(func() bool { rec.mu.Lock(); defer rec.mu.Unlock(); return rec.picking })
			childText, err := child.Element("#text")
			if err != nil {
				t.Fatal(err)
			}
			if err = childText.Click(proto.InputMouseButtonLeft, 1); err != nil {
				t.Fatal(err)
			}
			wait(func() bool { rec.mu.Lock(); defer rec.mu.Unlock(); return rec.selected != nil })
			click(`[data-action="extract"]`)
			wait(func() bool { return len(nodesOfType("extract")) == 4 })
			childStep := nodesOfType("extract")[3]
			if childStep.Type != "extract" || childStep.Target == nil || len(childStep.Target.Frame) != 1 {
				t.Fatalf("child selection lost frame: %+v", childStep)
			}
			if err = button.Click(proto.InputMouseButtonLeft, 1); err != nil {
				t.Fatal(err)
			}
			input, err := page.Element("#title")
			if err != nil {
				t.Fatal(err)
			}
			if err = input.Input("Public draft fixture"); err != nil {
				t.Fatal(err)
			}
			wait(func() bool { return len(nodesOfType("type")) == 1 })
			// Neither panel scrolling nor programmatic focus/layout scrolling is user intent.
			nodeEval("ol", `function(){this.scrollTop=this.scrollHeight}`)
			if _, err = child.Eval(`()=>{scrollTo(0,200);document.body.tabIndex=-1;document.body.focus();scrollTo(0,0)}`); err != nil {
				t.Fatal(err)
			}
			// Allow the collector's 400ms quiet window and 180ms debounce to elapse.
			time.Sleep(650 * time.Millisecond)
			if got := nodesOfType("scroll"); len(got) != 0 {
				t.Fatalf("automatic scrolling was recorded: %+v", got)
			}
			point, err := iframe.Eval(`function(){const r=this.getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}}`)
			if err != nil {
				t.Fatal(err)
			}
			x, y := point.Value.Get("x").Num(), point.Value.Get("y").Num()
			if err = s.call(sid, "Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": x, "y": y}, nil); err != nil {
				t.Fatal(err)
			}
			if err = s.call(sid, "Input.dispatchMouseEvent", map[string]any{"type": "mouseWheel", "x": x, "y": y, "deltaX": 0, "deltaY": 200}, nil); err != nil {
				t.Fatal(err)
			}
			wait(func() bool {
				for _, n := range nodesOfType("scroll") {
					if n.Target != nil && len(n.Target.Frame) == 1 && n.DY > 0 {
						return true
					}
				}
				return false
			})
			beforeKeys := len(nodesOfType("scroll"))
			if _, err = child.Eval(`()=>document.body.focus()`); err != nil {
				t.Fatal(err)
			}
			time.Sleep(450 * time.Millisecond) // Focus is an action; wait for the quiet window.
			for _, kind := range []string{"keyDown", "keyUp"} {
				if err = s.call(sid, "Input.dispatchKeyEvent", map[string]any{"type": kind, "key": "Home", "code": "Home", "windowsVirtualKeyCode": 36}, nil); err != nil {
					t.Fatal(err)
				}
			}
			wait(func() bool { return len(nodesOfType("scroll")) > beforeKeys })
			if got := nodesOfType("press"); len(got) != 0 {
				t.Fatalf("page navigation recorded both key and scroll: %+v", got)
			}
			for _, scheme := range []string{"light", "dark"} {
				if err := s.call(sid, "Emulation.setEmulatedMedia", map[string]any{"features": []map[string]string{{"name": "prefers-color-scheme", "value": scheme}}}, nil); err != nil {
					t.Fatal(err)
				}
				if *panelShots != "" {
					if err := os.MkdirAll(*panelShots, 0700); err != nil {
						t.Fatal(err)
					}
					data, err := page.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
					if err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(filepath.Join(*panelShots, "panel-"+lang+"-"+scheme+".png"), data, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			click(`[data-action="stop"]`)
			wait(func() bool { rec.mu.Lock(); defer rec.mu.Unlock(); return rec.done })
			var all struct {
				Steps []node `json:"steps"`
				Done  bool   `json:"done"`
			}
			invoke("GET", "/record/events?id="+rec.id+"&after=0", nil, &all)
			if !all.Done {
				t.Fatal("stop did not finish recording")
			}
			counts := map[string]int{}
			for _, n := range all.Steps {
				counts[n.Type]++
				if n.Target != nil {
					encoded, _ := json.Marshal(n.Target)
					if strings.Contains(string(encoded), "__myco_recording_panel__") {
						t.Fatalf("recorded own control: %s", encoded)
					}
				}
				switch n.Type {
				case "extract", "click", "type":
				case "scroll":
					if n.Target == nil || len(n.Target.Shadow) != 0 {
						t.Fatalf("not a page scroll: %+v", n)
					}
					pageScroller := false
					for _, l := range n.Target.Locators {
						if l.Kind == "css" && (l.Value == "html" || l.Value == "html > body") {
							pageScroller = true
						}
					}
					if !pageScroller {
						raw, _ := json.Marshal(n)
						t.Fatalf("unexpected scroll target: %s", raw)
					}
					raw, _ := json.Marshal(n)
					t.Logf("retained document scroll: %s", raw)
				default:
					t.Fatalf("unexpected recorded action: %+v", n)
				}
			}
			if counts["extract"] != 4 || counts["click"] != 1 || counts["type"] != 1 || counts["scroll"] < 1 {
				t.Fatalf("wrong recorded actions: %v", counts)
			}
			var fromPage struct {
				ID string `json:"id"`
			}
			invoke("POST", "/record/start", map[string]any{"profile": profile.ID, "url": fixture.URL, "headless": false}, &fromPage)
			refreshRoot()
			s.mu.Lock()
			pageRec := s.recorder
			s.mu.Unlock()
			if pageRec == nil || pageRec.info.Origin != "page" {
				t.Fatal("page recording did not use same session")
			}
			// A page-started recording is visible and controllable in Chrome too.
			query(`[data-action="stop"]`)
			click(`[data-action="stop"]`)
			wait(func() bool { pageRec.mu.Lock(); defer pageRec.mu.Unlock(); return pageRec.done })
			runFixture := func(profileID string, headless bool) {
				t.Helper()
				graph := script{Name: "Panel overlap fixture", OnError: "stop", Params: []param{}}
				if headless {
					graph.StartURL = fixture.URL
				}
				graph.Graph.Nodes = []node{
					{ID: "before", Type: "js", Code: "if(document.querySelector('#__myco_recording_panel__'))throw Error('panel present during run');"},
					{ID: "click", Type: "click", Button: "left", Target: &target{Locators: []locator{{Kind: "css", Value: "#corner"}}}},
					{ID: "after", Type: "js", Code: "if(document.querySelector('#__myco_recording_panel__')||window.cornerClicked!==1)throw Error('panel blocked corner click');"},
				}
				graph.Graph.Edges = [][]string{{"before", "click"}, {"click", "after"}}
				var running struct {
					ID string `json:"id"`
				}
				invoke("POST", "/runs", runOptions{Profile: profileID, Headless: headless, Script: graph, Params: map[string]string{}, Secrets: map[string]string{}}, &running)
				e.mu.Lock()
				run := e.runs[running.ID]
				e.mu.Unlock()
				wait(func() bool { run.mu.Lock(); defer run.mu.Unlock(); return finishedRun(run.update.State) })
				run.mu.Lock()
				result := run.update
				run.mu.Unlock()
				if result.State != "succeeded" {
					t.Fatalf("headless=%v corner replay: %+v", headless, result)
				}
			}
			runFixture(profile.ID, false)
			wait(func() bool {
				v, err := page.Eval(`()=>!!document.querySelector('#__myco_recording_panel__')`)
				return err == nil && v.Value.Bool()
			})
			headlessProfile := profile
			invoke("POST", "/profiles", map[string]string{"name": "Headless fixture"}, &headlessProfile)
			runFixture(headlessProfile.ID, true)

		})
	}
}
