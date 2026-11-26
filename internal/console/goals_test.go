package console_test

import (
	"context"
	"testing"

	"github.com/myco-ai/myco/internal/api"
)

// A goal set on a chat (myco goal), through the App's backend: made,
// listed, read with its rounds, its interval changed, paused, resumed and
// stopped, and still listed with state=all once it ended.
func TestGoalsThroughTheApp(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if _, err := r.admin.Register(ctx, api.RegisterRequest{Name: "worker", Binding: api.Binding{Harness: "claude", SessionID: "s-worker"}}, false); err != nil {
		t.Fatal(err)
	}
	code, h, _ := r.do("POST", "/app/hubs", map[string]string{"name": "Main", "url": r.hubURL, "token": r.consoleToken("test-mac")})
	if code != 201 {
		t.Fatalf("add hub: %d %v", code, h)
	}
	base := "/app/hubs/" + h["id"].(string) + "/goals"
	code, g, raw := r.do("POST", base, map[string]any{"agent": "worker", "objective": "Get coverage to 80%", "done_when": "CI shows 80%", "every_sec": 1800})
	if code != 201 || g["id"] == nil || g["state"] != "active" {
		t.Fatalf("create: %d %s", code, raw)
	}
	id := g["id"].(string)
	if code, m, raw := r.do("GET", base+"?agent=worker", nil); code != 200 || len(m["goals"].([]any)) != 1 {
		t.Fatalf("list: %d %s", code, raw)
	}
	if code, m, raw := r.do("GET", base+"/"+id, nil); code != 200 || m["goal"] == nil || m["rounds"] == nil {
		t.Fatalf("get: %d %s", code, raw)
	}
	if code, m, raw := r.do("PATCH", base+"/"+id, map[string]int{"every_sec": 3600}); code != 200 || m["base_every_sec"] != float64(3600) {
		t.Fatalf("patch: %d %s", code, raw)
	}
	for _, step := range []struct{ action, state string }{{"pause", "paused"}, {"resume", "active"}, {"stop", "stopped"}} {
		if code, m, raw := r.do("POST", base+"/"+id+"/"+step.action, map[string]string{"reason": "test"}); code != 200 || m["state"] != step.state {
			t.Fatalf("%s: %d %s", step.action, code, raw)
		}
	}
	if code, _, _ := r.do("POST", base+"/"+id+"/nope", map[string]string{}); code != 404 {
		t.Fatalf("unknown action: %d", code)
	}
	if _, m, raw := r.do("GET", base+"?agent=worker", nil); len(m["goals"].([]any)) != 0 {
		t.Fatalf("active after stop: %s", raw)
	}
	if _, m, raw := r.do("GET", base+"?agent=worker&state=all", nil); len(m["goals"].([]any)) != 1 {
		t.Fatalf("all after stop: %s", raw)
	}
}
