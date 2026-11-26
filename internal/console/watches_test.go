package console_test

import (
	"context"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

// What an agent waits for in the background (myco watch), through the
// App's backend: listed (with ended ones on state=all), read, extended and
// stopped, as the hub has them.
func TestWatchesThroughTheApp(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	reg := func(name string) *client.Client {
		res, err := r.admin.Register(ctx, api.RegisterRequest{Name: name, Binding: api.Binding{Harness: "claude", SessionID: "s-" + name}}, false)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := client.New(r.hubURL, res.Token)
		return c
	}
	watcher := reg("watcher")
	reg("peer")
	sent, err := watcher.Send(ctx, api.SendRequest{To: "peer", Body: "please review"}, false)
	if err != nil {
		t.Fatal(err)
	}
	w, err := watcher.StartWatch(ctx, api.WatchStartRequest{What: "peer's review", Kind: "reply", Target: sent.Message.ID, ForSec: 600}, false)
	if err != nil {
		t.Fatal(err)
	}

	code, h, _ := r.do("POST", "/app/hubs", map[string]string{"name": "Main", "url": r.hubURL, "token": r.consoleToken("test-mac")})
	if code != 201 {
		t.Fatalf("add hub: %d %v", code, h)
	}
	base := "/app/hubs/" + h["id"].(string) + "/watches"
	code, m, raw := r.do("GET", base+"?agent=watcher&state=all", nil)
	list, _ := m["watches"].([]any)
	if code != 200 || len(list) != 1 || list[0].(map[string]any)["id"] != w.ID || list[0].(map[string]any)["state"] != "watching" {
		t.Fatalf("list: %d %s", code, raw)
	}
	if code, m, raw := r.do("GET", base+"/"+w.ID, nil); code != 200 || m["what"] != "peer's review" {
		t.Fatalf("get: %d %s", code, raw)
	}
	if code, m, raw := r.do("POST", base+"/"+w.ID+"/extend", map[string]int{"for_sec": 3600}); code != 200 || m["deadline"] == w.Deadline.Format("2006-01-02T15:04:05Z07:00") {
		t.Fatalf("extend: %d %s", code, raw)
	}
	if code, m, raw := r.do("POST", base+"/"+w.ID+"/stop", map[string]string{"reason": "not needed"}); code != 200 || m["state"] != "stopped" {
		t.Fatalf("stop: %d %s", code, raw)
	}
	// Ended: gone from the active list, still there with state=all.
	if _, m, raw := r.do("GET", base+"?agent=watcher", nil); len(m["watches"].([]any)) != 0 {
		t.Fatalf("active after stop: %s", raw)
	}
	if _, m, raw := r.do("GET", base+"?agent=watcher&state=all", nil); len(m["watches"].([]any)) != 1 {
		t.Fatalf("all after stop: %s", raw)
	}
}
