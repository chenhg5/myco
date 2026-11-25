package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

type recordFixtureComponent struct {
	policyFixtureComponent
	scope  string
	legacy bool
}

func (f *recordFixtureComponent) Call(_ context.Context, scope, method, path string, in, out any) error {
	if scope != f.scope || method != "GET" {
		return fmt.Errorf("recording scope/method changed")
	}
	var raw string
	switch path {
	case "/status":
		raw = `{"recordings":[{"id":"rec_browser","profile":"p_work","url":"https://example.com/","started_at":"2026-11-25T00:00:00Z","origin":"browser"}]}`
		if f.legacy {
			raw = `{}`
		}
	case "/record/events?id=rec_browser&after=8":
		raw = `{"steps":[],"removed":["n_removed"],"next":9,"done":true}`
	default:
		return fmt.Errorf("unexpected recording request %s", path)
	}
	return json.Unmarshal([]byte(raw), out)
}
func TestBrowserRecordingActiveAndRemovedReachPage(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/pages/pg_fixture" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(api.Page{ID: "pg_fixture", Name: "Fixture"})
	}))
	defer hub.Close()
	c, _ := client.New(hub.URL, "fixture")
	b := &Backend{}
	fixture := &recordFixtureComponent{scope: browserScope("hub_fixture", "pg_fixture", "frame")}
	b.browser.component = fixture
	g := b.grants()
	g.mu.Lock()
	g.perm("hub_fixture", "pg_fixture").once["browser"] = true
	g.mu.Unlock()
	call := func(action string, args json.RawMessage) any {
		t.Helper()
		out, err := b.browserCall(context.Background(), c, Hub{ID: "hub_fixture"}, browserCall{Page: "pg_fixture", Instance: "frame", Action: action, Args: args})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	raw, _ := json.Marshal(call("record.active", nil))
	if !strings.Contains(string(raw), `"origin":"browser"`) {
		t.Fatal("active metadata lost", string(raw))
	}
	raw, _ = json.Marshal(call("record.events", json.RawMessage(`{"id":"rec_browser","after":8}`)))
	if !strings.Contains(string(raw), `"removed":["n_removed"]`) || !strings.Contains(string(raw), `"next":9`) {
		t.Fatal("recording delta lost", string(raw))
	}
	fixture.legacy = true
	raw, _ = json.Marshal(call("record.active", nil))
	if string(raw) != "[]" {
		t.Fatal("old component discovery should be an empty list", string(raw))
	}
}
