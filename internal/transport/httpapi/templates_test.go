package httpapi_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
)

const weeklyManifest = `{
  "id": "alice/weekly",
  "name": "周报",
  "version": "1.0.0",
  "kind": "records",
  "data": {
    "items": {"fields": {"title": "text", "done": "bool", "due": "date"}},
    "summary": {"single": true, "fields": {"text": "markdown"}}
  },
  "permissions": ["download"],
  "export": ["xlsx"]
}`

// A template: published to the library (myco.json, the agent's guide and
// sample data in the zip), seen by everyone, used to make a page; the page
// checks records against its structure, writes are versioned (a conflict
// answers with the latest), and the change feed wakes a waiting watcher.
func TestTemplatesAndPageRecords(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", nil)
	zipHdr := map[string]string{"Content-Type": "application/zip"}
	tpl := zipOf(t, map[string]string{
		"myco.json":   weeklyManifest,
		"index.html":  "<h1>周报</h1>",
		"agent.md":    "Fill items: one per thing done this week.",
		"sample.json": `{"items": [{"id": "a", "title": "写周报", "done": false}], "summary": [{"text": "本周…"}]}`,
	})

	r := h.doReq("POST", "/v1/templates", alice, tpl, zipHdr).want(t, 201)
	id := str(r.body, "id")
	if !strings.HasPrefix(id, "tp_") || r.body["slug"] != "alice/weekly" || r.body["version"] != "1.0.0" || r.body["guide"] != "Fill items: one per thing done this week." || r.body["can_manage"] != true {
		t.Fatalf("publish: %s", r.raw)
	}
	// The same version again is refused.
	h.doReq("POST", "/v1/templates", alice, tpl, zipHdr).wantErr(t, 409, "conflict")
	// A bad manifest says what is wrong.
	h.doReq("POST", "/v1/templates", alice, zipOf(t, map[string]string{"myco.json": `{"id": "x", "name": "x", "version": "1"}`, "index.html": "x"}), zipHdr).wantErr(t, 400, "invalid_argument")
	h.doReq("POST", "/v1/templates", alice, zipOf(t, map[string]string{"index.html": "x"}), zipHdr).wantErr(t, 400, "invalid_argument")

	// Everyone sees the library; by id or by slug.
	if l := h.do("GET", "/v1/templates", bob, nil).want(t, 200); len(l.body["templates"].([]any)) != 1 {
		t.Fatalf("list: %s", l.raw)
	}
	if g := h.do("GET", "/v1/templates/alice%2Fweekly", bob, nil).want(t, 200); str(g.body, "id") != id {
		t.Fatalf("by slug: %s", g.raw)
	}
	h.do("GET", "/v1/templates/"+id, bob, nil).want(t, 200)

	// A page from it, with the sample data.
	u := h.do("POST", "/v1/templates/"+id+"/use", alice, map[string]any{"name": "第 47 周", "sample": true}).want(t, 201)
	page := str(u.body, "page", "id")
	if str(u.body, "page", "template", "slug") != "alice/weekly" || str(u.body, "page", "template", "version") != "1.0.0" || str(u.body, "template", "guide") == "" {
		t.Fatalf("use: %s", u.raw)
	}
	if f := h.doReq("GET", "/v1/pages/"+page+"/v/latest/", alice, nil, nil).want(t, 200); !strings.Contains(string(f.raw), "周报") {
		t.Fatalf("its files: %q", f.raw)
	}
	recs := h.do("GET", "/v1/pages/"+page+"/data", alice, nil).want(t, 200)
	if n := len(recs.body["records"].([]any)); n != 2 {
		t.Fatalf("sample records: %s", recs.raw)
	}
	seq := int64(recs.body["seq"].(float64))

	// Records are checked against the structure.
	h.do("POST", "/v1/pages/"+page+"/data/items", alice, map[string]any{"data": map[string]any{"title": 3}}).wantErr(t, 400, "invalid_argument")
	h.do("POST", "/v1/pages/"+page+"/data/items", alice, map[string]any{"data": map[string]any{"nope": "x"}}).wantErr(t, 400, "invalid_argument")
	h.do("POST", "/v1/pages/"+page+"/data/other", alice, map[string]any{"data": map[string]any{"x": "y"}}).wantErr(t, 400, "invalid_argument")
	h.do("POST", "/v1/pages/"+page+"/data/items", alice, map[string]any{"data": map[string]any{"due": "next week"}}).wantErr(t, 400, "invalid_argument")

	// A watcher waits; a new record wakes it.
	got := make(chan resp, 1)
	go func() {
		got <- h.do("GET", "/v1/pages/"+page+"/changes?wait=10&after="+itoa(seq), adminToken, nil)
	}()
	time.Sleep(200 * time.Millisecond)
	nr := h.do("POST", "/v1/pages/"+page+"/data/items", alice, map[string]any{"data": map[string]any{"title": "发版", "done": false, "due": "2026-11-20"}}).want(t, 201)
	rid := str(nr.body, "id")
	select {
	case c := <-got:
		ch := c.want(t, 200).body["changes"].([]any)
		if len(ch) != 1 || str(ch[0], "record") != rid || str(ch[0], "op") != "put" || str(ch[0], "by") != "alice" {
			t.Fatalf("changes: %s", c.raw)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the watcher was not woken")
	}

	// Versioned writes: the version read, or a conflict with the latest.
	h.do("PUT", "/v1/pages/"+page+"/data/items/"+rid, adminToken, map[string]any{"data": map[string]any{"title": "发版", "done": true}, "if_version": 1}).want(t, 200)
	c := h.do("PUT", "/v1/pages/"+page+"/data/items/"+rid, alice, map[string]any{"data": map[string]any{"title": "改名"}, "if_version": 1})
	if latest, _ := c.body["latest"].(map[string]any); c.status != 409 || latest == nil || latest["version"] != float64(2) {
		t.Fatalf("conflict: %d %s", c.status, c.raw)
	}
	// A single collection holds main only.
	h.do("PUT", "/v1/pages/"+page+"/data/summary/x", alice, map[string]any{"data": map[string]any{"text": "x"}}).wantErr(t, 400, "invalid_argument")
	h.do("PUT", "/v1/pages/"+page+"/data/summary/main", alice, map[string]any{"data": map[string]any{"text": "**完成**"}}).want(t, 200)
	// Deleted: gone, and in the feed.
	h.do("DELETE", "/v1/pages/"+page+"/data/items/"+rid+"?if_version=1", alice, nil).wantErr(t, 409, "conflict")
	h.do("DELETE", "/v1/pages/"+page+"/data/items/"+rid, alice, nil).want(t, 204)
	all := h.do("GET", "/v1/pages/"+page+"/changes?after="+itoa(seq), alice, nil).want(t, 200)
	if ch := all.body["changes"].([]any); len(ch) != 4 || str(ch[3], "op") != "delete" {
		t.Fatalf("feed: %s", all.raw)
	}

	// The template goes (bob is an agent of the same person: he may); the
	// page made from it stays as it is.
	h.do("DELETE", "/v1/templates/"+id, bob, nil).want(t, 204)
	h.do("POST", "/v1/pages/"+page+"/data/items", alice, map[string]any{"data": map[string]any{"nope": 1}}).wantErr(t, 400, "invalid_argument")
	h.do("POST", "/v1/pages/"+page+"/data/items", alice, map[string]any{"data": map[string]any{"title": "还在"}}).want(t, 201)

	// A page of its own (no template) takes any records.
	own := h.doReq("POST", "/v1/pages?name=看板", alice, zipOf(t, map[string]string{"index.html": "x"}), zipHdr).want(t, 201)
	h.do("POST", "/v1/pages/"+str(own.body, "id")+"/data/cards", alice, map[string]any{"data": map[string]any{"anything": []int{1, 2}}}).want(t, 201)
	h.do("POST", "/v1/pages/"+str(own.body, "id")+"/data/cards", alice, map[string]any{"data": []int{1}}).wantErr(t, 400, "invalid_argument")
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
