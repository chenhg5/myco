package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestReplayRecordingBudgetAndIdleIsolation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &controlSession{ctx: ctx, cancel: cancel, replaying: true, last: time.Now().Add(-6 * time.Minute), expires: time.Now().Add(time.Hour), steps: make([]node, controlStepLimit), wake: make(chan struct{})}
	if c.check(false) != nil {
		t.Fatal("active replay counted as idle")
	}
	if err := c.appendStep(node{ID: "ignored", Type: "click"}); err != nil || len(c.steps) != controlStepLimit {
		t.Fatal("replay changed manual draft", err)
	}
	c.mu.Lock()
	c.navigationTab = "tab"
	c.captureNavigationLocked("tab", "https://example.com/?tab=tech", false)
	c.mu.Unlock()
	if len(c.steps) != controlStepLimit {
		t.Fatal("derived navigation entered the manual draft")
	}
	c.replaying = false
	if c.budget(node{Type: "click"}) == nil {
		t.Fatal("ordinary recording budget was weakened")
	}
}

func TestNativeChromeReplayUsesUnchangedCloseGraph(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with installed Chrome")
	}
	var techVisits atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		title := "Documentation"
		if r.URL.Query().Get("tab") == "tech" {
			title = "Technical discussions"
			techVisits.Add(1)
		}
		fmt.Fprintf(w, `<!doctype html><title>Replay fixture</title><a id="tech" href="/?tab=tech">Technology</a><h1 id="title">%s</h1>`, title)
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
	call := func(path string, body any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", path, bytes.NewReader(raw)).WithContext(ctx)
		r.Header.Set("X-Myco-Scope", "demo/replay")
		w := httptest.NewRecorder()
		e.handler().ServeHTTP(w, r)
		var result map[string]any
		json.Unmarshal(w.Body.Bytes(), &result)
		if w.Code >= 500 {
			diagnose()
		}
		return w.Code, result
	}
	_, profile := call("/profiles", map[string]any{"name": "Replay fixture"})
	id := profile["id"].(string)
	call("/profiles/"+id+"/agents", map[string]bool{"allowed": true})
	open := func(request string) map[string]any {
		t.Helper()
		code, result := call("/control/open", map[string]any{"request_id": request + "-open", "profile": id, "url": fixture.URL, "grant_id": "demo-grant", "actor": controlActor{ID: "demo-agent"}, "hosts": []string{"127.0.0.1"}, "expires_at": time.Now().Add(15 * time.Minute)})
		if code != 200 {
			t.Fatal(code, result)
		}
		return result
	}
	find := func(snapshot map[string]any, role, name string) string {
		t.Helper()
		for _, item := range snapshot["nodes"].([]any) {
			n := item.(map[string]any)
			if n["role"] == role && n["name"] == name {
				return n["ref"].(string)
			}
		}
		t.Fatal("missing fixture ref", role, name)
		return ""
	}
	first := open("first")
	session := first["session"].(string)
	code, clicked := call("/control/act", map[string]any{"request_id": "manual-click", "session": session, "op": "click", "ref": find(first["snapshot"].(map[string]any), "link", "Technology")})
	if code != 200 {
		t.Fatal(code, clicked)
	}
	_, snapshot := call("/control/snapshot", map[string]any{"request_id": "manual-snapshot", "session": session})
	code, extracted := call("/control/extract", map[string]any{"request_id": "manual-extract", "session": session, "ref": find(snapshot, "heading", "Technical discussions"), "what": "text"})
	if code != 200 {
		t.Fatal(code, extracted)
	}
	code, closed := call("/control/close", map[string]any{"request_id": "first-close", "session": session})
	if code != 200 {
		t.Fatal(code, closed)
	}
	rawSteps := closed["steps"].([]any)
	if len(rawSteps) != 3 {
		t.Fatal("expected click, derived wait and extract", closed)
	}
	// Preserve every returned node field, including its locator choices.
	nodes := append([]any{map[string]any{"id": "start", "type": "navigate", "url": closed["start_url"], "title": "Open start", "pos": []int{0, 0}, "on_error": "stop"}}, rawSteps...)
	edges := [][]string{}
	previous := "start"
	for _, raw := range rawSteps {
		id := raw.(map[string]any)["id"].(string)
		edges = append(edges, []string{previous, id})
		previous = id
	}
	second := open("second")
	session = second["session"].(string)
	oldRef := find(second["snapshot"].(map[string]any), "heading", "Documentation")
	input := map[string]any{"request_id": "replay", "session": session, "graph": map[string]any{"nodes": nodes, "edges": edges}, "timeout_ms": 30000}
	code, result := call("/control/run", input)
	if code != 200 || result["ok"] != true || result["state"] != "succeeded" {
		t.Fatal(code, result)
	}
	if result["output"].(map[string]any)[extracted["into"].(string)] != "Technical discussions" {
		t.Fatal("recorded graph lost output", result)
	}
	before := techVisits.Load()
	code, retried := call("/control/run", input)
	if code != 200 || techVisits.Load() != before || retried["ok"] != true {
		t.Fatal("same request repeated website effects", code, retried)
	}
	code, stale := call("/control/extract", map[string]any{"request_id": "stale", "session": session, "ref": oldRef, "what": "text"})
	if code != 409 || stale["code"] != "stale_ref" {
		t.Fatal("pre-run ref survived", code, stale)
	}
	// The graph contains a click, but a forbidden later node blocks the whole run.
	bad := map[string]any{"nodes": []any{map[string]any{"id": "click", "type": "click", "target": map[string]any{"locators": []any{map[string]string{"kind": "css", "value": "#tech"}}}}, map[string]any{"id": "forbidden", "type": "js", "code": "return document.title"}}, "edges": [][]string{{"click", "forbidden"}}}
	code, rejected := call("/control/run", map[string]any{"request_id": "invalid-graph", "session": session, "graph": bad})
	if code != 400 || rejected["code"] != "invalid_argument" || rejected["error_node"] != "forbidden" || techVisits.Load() != before {
		t.Fatal("graph was not preflighted", code, rejected)
	}
	code, closed = call("/control/close", map[string]any{"request_id": "second-close", "session": session})
	if code != 200 || len(closed["steps"].([]any)) != 0 {
		t.Fatal("replay was recorded", code, closed)
	}
}

func TestNativeChromeReplayRetainsSensitiveAndHostBoundaries(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with installed Chrome")
	}
	var outside atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if strings.HasPrefix(r.Host, "ads.localhost") {
			outside.Add(1)
		}
		if r.URL.Path == "/redirect" && !strings.HasPrefix(r.Host, "ads.localhost") {
			http.Redirect(w, r, "http://"+strings.Replace(r.Host, "127.0.0.1", "ads.localhost", 1)+"/private", http.StatusFound)
			return
		}
		fmt.Fprint(w, `<!doctype html><title>Safety fixture</title><h1>Public heading</h1><input type="password" id="password"><table id="table"><tr><td>Public cell</td><td data-sensitive>protected-fixture-value</td></tr></table><button id="change" onclick="this.textContent='Changed'">Change</button>`)
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
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", path, bytes.NewReader(raw)).WithContext(ctx)
		r.Header.Set("X-Myco-Scope", "demo/replay-safety")
		w := httptest.NewRecorder()
		e.handler().ServeHTTP(w, r)
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	_, profile := call("/profiles", map[string]any{"name": "Replay safety"})
	id := profile["id"].(string)
	call("/profiles/"+id+"/agents", map[string]bool{"allowed": true})
	code, opened := call("/control/open", map[string]any{"request_id": "open", "profile": id, "url": fixture.URL, "grant_id": "demo-grant", "actor": controlActor{ID: "demo-agent"}, "hosts": []string{"127.0.0.1"}, "expires_at": time.Now().Add(15 * time.Minute)})
	if code != 200 {
		t.Fatal(code, opened)
	}
	session := opened["session"].(string)
	target := func(css string) map[string]any {
		return map[string]any{"locators": []map[string]string{{"kind": "css", "value": css}}}
	}
	run := func(request string, nodes []any) map[string]any {
		t.Helper()
		edges := [][]string{}
		for i := 1; i < len(nodes); i++ {
			edges = append(edges, []string{nodes[i-1].(map[string]any)["id"].(string), nodes[i].(map[string]any)["id"].(string)})
		}
		code, out := call("/control/run", map[string]any{"request_id": request, "session": session, "graph": map[string]any{"nodes": nodes, "edges": edges}, "timeout_ms": 10000})
		if code != 200 {
			t.Fatal(code, out)
		}
		return out
	}
	typed := run("sensitive", []any{map[string]any{"id": "password", "type": "type", "target": target("#password"), "value": "public-fixture"}})
	if typed["ok"] != false || typed["error_node"] != "password" || typed["error"].(map[string]any)["code"] != "sensitive_field" {
		t.Fatal(typed)
	}
	table := run("table", []any{map[string]any{"id": "table", "type": "extract", "target": target("#table"), "what": "table", "into": "rows"}})
	raw, _ := json.Marshal(table)
	if table["ok"] != true || bytes.Contains(raw, []byte("protected-fixture-value")) || !bytes.Contains(raw, []byte("[redacted]")) {
		t.Fatal("table did not preserve redacted cells", string(raw))
	}
	screenshot := run("screenshot", []any{map[string]any{"id": "shot", "type": "screenshot", "full": true, "into": "image"}})
	if screenshot["ok"] != false || screenshot["error"].(map[string]any)["code"] != "sensitive_snapshot" {
		t.Fatal(screenshot)
	}
	skipped := run("skip", []any{map[string]any{"id": "missing", "type": "wait", "wait": "element", "target": target("#missing"), "timeout_ms": 40, "on_error": "skip"}, map[string]any{"id": "heading", "type": "extract", "what": "text", "target": target("h1"), "into": "heading"}})
	if skipped["ok"] != true || skipped["steps"].([]any)[0].(map[string]any)["state"] != "skipped" || skipped["output"].(map[string]any)["heading"] != "Public heading" {
		t.Fatal(skipped)
	}
	denied := run("hosts", []any{map[string]any{"id": "outside", "type": "navigate", "url": strings.Replace(fixture.URL, "127.0.0.1", "ads.localhost", 1)}, map[string]any{"id": "following", "type": "click", "target": target("#change")}})
	if denied["ok"] != false || denied["host"] != "ads.localhost" || denied["error_node"] != "outside" || denied["error"].(map[string]any)["code"] != "host_not_allowed" || outside.Load() != 0 || len(denied["steps"].([]any)) != 1 {
		t.Fatal("host scope did not stop the replay", denied, outside.Load())
	}
	redirected := run("redirect", []any{map[string]any{"id": "redirect", "type": "navigate", "url": fixture.URL + "/redirect"}, map[string]any{"id": "following", "type": "extract", "what": "text", "target": target("h1"), "into": "private"}})
	if redirected["ok"] != false || redirected["error_node"] != "redirect" || redirected["error"].(map[string]any)["code"] != "host_not_allowed" || redirected["host"] != "ads.localhost" || len(redirected["steps"].([]any)) != 1 || len(redirected["output"].(map[string]any)) != 0 {
		t.Fatal("redirect did not stop replay", redirected)
	}
	code, closed := call("/control/close", map[string]any{"request_id": "close", "session": session})
	if code != 200 || len(closed["steps"].([]any)) != 0 {
		t.Fatal("failed replay changed the draft", code, closed)
	}
}

func TestNativeChromeReplayCancellationAndPrivateOutput(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with installed Chrome")
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><title>Replay lifecycle</title><h1>Public report</h1><button id="change" onclick="this.textContent='Changed'">Change</button><p id="long">`+strings.Repeat("public ", 500)+`</p>`)
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
		r.Header.Set("X-Myco-Scope", "demo/replay-lifecycle")
		w := httptest.NewRecorder()
		e.handler().ServeHTTP(w, r)
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	_, profile := call("/profiles", map[string]any{"name": "Replay lifecycle"})
	id := profile["id"].(string)
	call("/profiles/"+id+"/agents", map[string]bool{"allowed": true})
	open := func(request string) string {
		t.Helper()
		code, out := call("/control/open", map[string]any{"request_id": request + "-open", "profile": id, "url": fixture.URL, "grant_id": "demo-grant", "actor": controlActor{ID: "demo-agent"}, "hosts": []string{"127.0.0.1"}, "expires_at": time.Now().Add(15 * time.Minute)})
		if code != 200 {
			t.Fatal(code, out)
		}
		return out["session"].(string)
	}
	target := func(css string) map[string]any {
		return map[string]any{"locators": []map[string]string{{"kind": "css", "value": css}}}
	}
	session := open("outputs")
	nodes := []any{map[string]any{"id": "image", "type": "screenshot", "full": true, "into": "image"}}
	edges := [][]string{}
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("text%d", i)
		previous := "image"
		if i > 0 {
			previous = fmt.Sprintf("text%d", i-1)
		}
		nodes = append(nodes, map[string]any{"id": name, "type": "extract", "what": "text", "target": target("#long"), "into": name})
		edges = append(edges, []string{previous, name})
	}
	code, out := call("/control/run", map[string]any{"request_id": "outputs", "session": session, "graph": map[string]any{"nodes": nodes, "edges": edges}})
	if code != 200 || out["ok"] != true {
		t.Fatal(code, out)
	}
	output := out["output"].(map[string]any)
	raw, _ := json.Marshal(output)
	if len(raw) > 16<<10 || output["truncated"] != true {
		t.Fatal("output budget not enforced", len(raw))
	}
	image := output["image"].(map[string]any)
	screenshot := image["screenshot"].(map[string]any)
	if !strings.HasPrefix(screenshot["id"].(string), "a_") || image["url"] != nil {
		t.Fatal("screenshot was not private", image)
	}
	call("/control/close", map[string]any{"request_id": "close-outputs", "session": session})
	for _, mode := range []string{"owner", "expiry", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			session := open(mode)
			e.mu.Lock()
			control := e.controls[session]
			e.mu.Unlock()
			if mode == "expiry" {
				control.mu.Lock()
				control.expires = time.Now().Add(350 * time.Millisecond)
				control.mu.Unlock()
			}
			graph := map[string]any{"nodes": []any{map[string]any{"id": "wait", "type": "wait", "wait": "time", "ms": 60000}, map[string]any{"id": "following", "type": "click", "target": target("#change")}}, "edges": [][]string{{"wait", "following"}}}
			input := map[string]any{"request_id": mode + "-run", "session": session, "graph": graph, "timeout_ms": 300000}
			if mode == "timeout" {
				input["timeout_ms"] = 80
			}
			type response struct {
				code  int
				value map[string]any
			}
			finished := make(chan response, 1)
			go func() { code, out := call("/control/run", input); finished <- response{code, out} }()
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			for {
				control.mu.Lock()
				running := control.replaying
				control.mu.Unlock()
				if running {
					break
				}
				select {
				case <-deadline.C:
					t.Fatal("replay did not start")
				case <-time.After(time.Millisecond):
				}
			}
			// One session cannot interleave a manual operation into its replay.
			code, busy := call("/control/snapshot", map[string]any{"request_id": mode + "-busy", "session": session})
			if code != 409 || busy["code"] != "busy" {
				t.Fatal("concurrent control admitted", code, busy)
			}
			if mode == "owner" {
				control.terminate("session_closed", "stopped_by_owner")
			}
			select {
			case result := <-finished:
				if mode == "timeout" {
					if result.code != 200 || result.value["ok"] != false || result.value["error_node"] != "wait" || len(result.value["steps"].([]any)) != 1 {
						t.Fatal(result)
					}
					code, closed := call("/control/close", map[string]any{"request_id": mode + "-close", "session": session})
					if code != 200 || len(closed["steps"].([]any)) != 0 {
						t.Fatal(code, closed)
					}
				} else {
					want := "session_closed"
					if mode == "expiry" {
						want = "session_expired"
					}
					if result.value["code"] != want {
						t.Fatal(result)
					}
					control.mu.Lock()
					count := len(control.steps)
					control.mu.Unlock()
					if count != 0 {
						t.Fatal("forced stop kept a draft")
					}
					_ = control.browser.close()
				}
			case <-deadline.C:
				t.Fatal("replay ignored cancellation or expiry")
			}
		})
	}
}
