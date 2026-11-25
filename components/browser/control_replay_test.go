package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

func TestNativeChromeControlDraftReplayExtensions(t *testing.T) {
	if !*browserSmoke {
		t.Skip("enable -browser-smoke with installed Chrome")
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<!doctype html><title>%s</title><p>Public page</p><div style="height:2100px">Long page</div>`, r.URL.Path)
	}))
	defer fixture.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	e, err := newEngine(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw)).WithContext(ctx)
		req.Header.Set("X-Myco-Scope", "demo/replay")
		w := httptest.NewRecorder()
		e.handler().ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
		return w
	}
	var profile profile
	json.Unmarshal(call("POST", "/profiles", map[string]any{"name": "Draft replay"}).Body.Bytes(), &profile)
	call("POST", "/sessions", map[string]any{"profile": profile.ID, "headless": true})
	s, err := e.session("demo/replay", profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.page()
	if err != nil {
		t.Fatal(err)
	}
	p = p.Context(ctx)
	r := &playRun{session: s, scope: "demo/replay", tabOrder: []proto.TargetTargetID{p.TargetID}}
	act := func(n node) any {
		t.Helper()
		value, err := r.act(ctx, n)
		if err != nil {
			t.Fatal(n.Type, n.Tab, err)
		}
		return value
	}
	waitURL := func(suffix string) {
		t.Helper()
		for {
			info, err := p.Info()
			if err == nil && strings.HasSuffix(info.URL, suffix) {
				if err := p.WaitLoad(); err != nil {
					t.Fatal(err)
				}
				return
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
	act(node{Type: "navigate", URL: fixture.URL + "/first"})
	act(node{Type: "navigate", URL: fixture.URL + "/second"})
	act(node{Type: "back"})
	waitURL("/first")
	act(node{Type: "forward"})
	waitURL("/second")
	act(node{Type: "reload"})
	waitURL("/second")
	act(node{Type: "tab", Tab: "new", URL: fixture.URL + "/popup"})
	act(node{Type: "tab", Tab: "close"})
	current, err := s.page()
	if err != nil || current.TargetID != p.TargetID {
		t.Fatal("tab close did not restore previous tab", err)
	}
	id, ok := act(node{Type: "screenshot", Full: true}).(string)
	if !ok || id == "" {
		t.Fatal("full replay screenshot missing")
	}
	cfg, err := png.DecodeConfig(call("GET", "/artifacts/"+id, nil).Body)
	if err != nil || cfg.Height < 2100 {
		t.Fatal("full replay screenshot clipped", cfg, err)
	}
}
