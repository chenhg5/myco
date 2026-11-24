package httpapi_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/app"
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
	l := h.do("GET", "/v1/templates", bob, nil).want(t, 200)
	ts := l.body["templates"].([]any)
	if len(ts) < 5 || str(ts[0], "slug") != "myco/table" || ts[0].(map[string]any)["official"] != true || str(ts[len(ts)-1], "slug") != "alice/weekly" {
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

// myco's own templates are in every library: they make pages (with their
// sample data), cannot be deleted, and their ids are not anyone else's.
func TestOfficialTemplates(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	g := h.do("GET", "/v1/templates/myco%2Fbase", alice, nil).want(t, 200)
	if g.body["official"] != true || g.body["can_manage"] != false || !strings.Contains(str(g.body, "guide"), "myco page data") {
		t.Fatalf("get: %s", g.raw)
	}
	u := h.do("POST", "/v1/templates/myco%2Fbase/use", alice, map[string]any{"name": "招聘进度", "sample": true}).want(t, 201)
	page := str(u.body, "page", "id")
	// (its page and the script it loads, bundled with it)
	idx := string(h.doReq("GET", "/v1/pages/"+page+"/v/latest/index.html", alice, nil, nil).want(t, 200).raw)
	m := regexp.MustCompile(`src="\./(assets/[^"]+\.js)"`).FindStringSubmatch(idx)
	if m == nil {
		t.Fatalf("its page loads no script of its own: %q", idx[:min(200, len(idx))])
	}
	h.doReq("GET", "/v1/pages/"+page+"/v/latest/"+m[1], alice, nil, nil).want(t, 200)
	recs := h.do("GET", "/v1/pages/"+page+"/data?collection=records", alice, nil).want(t, 200)
	if len(recs.body["records"].([]any)) != 4 {
		t.Fatalf("sample: %s", recs.raw)
	}
	h.do("DELETE", "/v1/templates/myco%2Fbase", adminToken, nil).wantErr(t, 403, "permission_denied")
	h.doReq("POST", "/v1/templates", alice, zipOf(t, map[string]string{"myco.json": strings.Replace(weeklyManifest, "alice/weekly", "myco/weekly", 1), "index.html": "x"}),
		map[string]string{"Content-Type": "application/zip"}).wantErr(t, 400, "invalid_argument")
	for _, slug := range []string{"myco%2Ftable", "myco%2Fdoc", "myco%2Fslides"} {
		h.do("POST", "/v1/templates/"+slug+"/use", alice, map[string]any{"sample": true}).want(t, 201)
	}
}

// A template brings its own icon (shown by the App), an SVG in it.
func TestTemplateIconAndRequiredHub(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	zipHdr := map[string]string{"Content-Type": "application/zip"}
	icon := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/></svg>`
	withIcon := strings.Replace(weeklyManifest, `"kind": "records",`, `"kind": "records", "icon": "icon.svg", "category": "tools",`, 1)
	r := h.doReq("POST", "/v1/templates", alice, zipOf(t, map[string]string{"myco.json": withIcon, "index.html": "x", "icon.svg": icon}), zipHdr).want(t, 201)
	if str(r.body, "icon") != icon || str(r.body, "category") != "tools" {
		t.Fatalf("icon and category: %s", r.raw)
	}
	l := h.do("GET", "/v1/templates", alice, nil).want(t, 200)
	if !strings.Contains(string(l.raw), "circle cx") {
		t.Fatalf("the list should carry the icon: %s", l.raw)
	}
	noIcon := strings.Replace(withIcon, `"version": "1.0.0"`, `"version": "1.0.1"`, 1)
	h.doReq("POST", "/v1/templates", alice, zipOf(t, map[string]string{"myco.json": noIcon, "index.html": "x"}), zipHdr).wantErr(t, 400, "invalid_argument")
}

// A page made from an app sees the app's newer version and takes it: new
// files and data structure, its records as they were.
func TestPageUpgradesToItsAppsNewVersion(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	zipHdr := map[string]string{"Content-Type": "application/zip"}
	h.doReq("POST", "/v1/templates", alice, zipOf(t, map[string]string{"myco.json": weeklyManifest, "index.html": "v1"}), zipHdr).want(t, 201)
	u := h.do("POST", "/v1/templates/alice%2Fweekly/use", alice, map[string]any{"name": "第 47 周"}).want(t, 201)
	page := str(u.body, "page", "id")
	h.do("POST", "/v1/pages/"+page+"/data/items", alice, map[string]any{"data": map[string]any{"title": "发版"}}).want(t, 201)
	if g := h.do("GET", "/v1/pages/"+page, alice, nil).want(t, 200); str(g.body, "template", "latest") != "" {
		t.Fatalf("no newer version yet: %s", g.raw)
	}
	h.do("POST", "/v1/pages/"+page+"/upgrade", alice, nil).wantErr(t, 409, "conflict")
	v2 := strings.Replace(strings.Replace(weeklyManifest, `"version": "1.0.0"`, `"version": "1.1.0"`, 1), `"due": "date"}`, `"due": "date", "owner": "person"}`, 1)
	h.doReq("POST", "/v1/templates", alice, zipOf(t, map[string]string{"myco.json": v2, "index.html": "v2"}), zipHdr).want(t, 200)
	g := h.do("GET", "/v1/pages/"+page, alice, nil).want(t, 200)
	if str(g.body, "template", "latest") != "1.1.0" || str(g.body, "template", "version") != "1.0.0" {
		t.Fatalf("the page should see the newer version: %s", g.raw)
	}
	// (the new field is refused before the upgrade, taken after)
	h.do("POST", "/v1/pages/"+page+"/data/items", alice, map[string]any{"data": map[string]any{"title": "x", "owner": "Ann"}}).wantErr(t, 400, "invalid_argument")
	up := h.do("POST", "/v1/pages/"+page+"/upgrade", alice, nil).want(t, 200)
	if str(up.body, "template", "version") != "1.1.0" || str(up.body, "template", "latest") != "" {
		t.Fatalf("upgraded: %s", up.raw)
	}
	if f := h.doReq("GET", "/v1/pages/"+page+"/v/latest/index.html", alice, nil, nil).want(t, 200); string(f.raw) != "v2" {
		t.Fatalf("its files should be the new version's: %q", f.raw)
	}
	recs := h.do("GET", "/v1/pages/"+page+"/data?collection=items", alice, nil).want(t, 200)
	if len(recs.body["records"].([]any)) != 1 {
		t.Fatalf("its records should stay: %s", recs.raw)
	}
	h.do("POST", "/v1/pages/"+page+"/data/items", alice, map[string]any{"data": map[string]any{"title": "x", "owner": "Ann"}}).want(t, 201)
	h.do("POST", "/v1/pages/"+page+"/upgrade", alice, nil).wantErr(t, 409, "conflict")
}

// A field the new version drops: records written before keep it, and
// writing such a record back whole (as pages do) still works; a field the
// template has never had, or a changed value of a dropped one, is refused.
func TestUpgradeKeepsDroppedFieldsWritable(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	zipHdr := map[string]string{"Content-Type": "application/zip"}
	h.doReq("POST", "/v1/templates", alice, zipOf(t, map[string]string{"myco.json": weeklyManifest, "index.html": "v1"}), zipHdr).want(t, 201)
	page := str(h.do("POST", "/v1/templates/alice%2Fweekly/use", alice, map[string]any{"name": "第 48 周"}).want(t, 201).body, "page", "id")
	h.do("PUT", "/v1/pages/"+page+"/data/items/r1", alice, map[string]any{"data": map[string]any{"title": "发版", "done": true}}).want(t, 201)
	v2 := strings.Replace(strings.Replace(weeklyManifest, `"version": "1.0.0"`, `"version": "1.1.0"`, 1), `"done": "bool", `, ``, 1)
	h.doReq("POST", "/v1/templates", alice, zipOf(t, map[string]string{"myco.json": v2, "index.html": "v2"}), zipHdr).want(t, 200)
	h.do("POST", "/v1/pages/"+page+"/upgrade", alice, nil).want(t, 200)
	// the old record, written back whole with a new title: fine
	h.do("PUT", "/v1/pages/"+page+"/data/items/r1", alice, map[string]any{"data": map[string]any{"title": "发版 v2", "done": true}}).want(t, 200)
	// a new record with the dropped field, a changed dropped value, a field never there: refused
	h.do("PUT", "/v1/pages/"+page+"/data/items/r2", alice, map[string]any{"data": map[string]any{"title": "x", "done": true}}).wantErr(t, 400, "invalid_argument")
	h.do("PUT", "/v1/pages/"+page+"/data/items/r1", alice, map[string]any{"data": map[string]any{"title": "y", "done": false}}).wantErr(t, 400, "invalid_argument")
	h.do("PUT", "/v1/pages/"+page+"/data/items/r1", alice, map[string]any{"data": map[string]any{"title": "y", "done": true, "colour": "red"}}).wantErr(t, 400, "invalid_argument")
	// and without the dropped field: fine (it goes)
	h.do("PUT", "/v1/pages/"+page+"/data/items/r1", alice, map[string]any{"data": map[string]any{"title": "z"}}).want(t, 200)
}

// A template's screenshots (pictures in shots/, x.webp with x.dark.webp)
// are listed with it and fetched one by one; nothing else of its folder is.
func TestTemplateScreenshots(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", nil)
	zipHdr := map[string]string{"Content-Type": "application/zip"}
	r := h.doReq("POST", "/v1/templates", alice, zipOf(t, map[string]string{
		"myco.json": weeklyManifest, "index.html": "<h1>周报</h1>",
		"shots/2.png": "\x89PNG two", "shots/1.webp": "RIFF one", "shots/1.dark.webp": "RIFF one dark",
		"shots/notes.txt": "not a picture", "shots/deep/3.png": "too deep",
	}), zipHdr).want(t, 201)
	id := str(r.body, "id")
	g := h.do("GET", "/v1/templates/"+id, bob, nil).want(t, 200)
	shots, _ := g.body["shots"].([]any)
	if len(shots) != 2 || str(shots[0], "src") != "shots/1.webp" || str(shots[0], "dark") != "shots/1.dark.webp" || str(shots[1], "src") != "shots/2.png" || str(shots[1], "dark") != "" {
		t.Fatalf("shots: %s", g.raw)
	}
	f := h.do("GET", "/v1/templates/"+id+"/files/shots/1.dark.webp", bob, nil).want(t, 200)
	if string(f.raw) != "RIFF one dark" || f.header.Get("Content-Type") != "image/webp" || f.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("shot: %q %v", f.raw, f.header)
	}
	h.do("GET", "/v1/templates/"+id+"/files/index.html", bob, nil).wantErr(t, 404, "not_found")
	h.do("GET", "/v1/templates/"+id+"/files/shots/notes.txt", bob, nil).wantErr(t, 404, "not_found")
	h.do("GET", "/v1/templates/"+id+"/files/shots/9.png", bob, nil).wantErr(t, 404, "not_found")
}

// What an app declares of the computer (os) and the network (net) comes
// back with the template and with each page made from it, for the App to
// ask the person and refuse the rest.
func TestTemplateDeclaresOSAndNet(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	zipHdr := map[string]string{"Content-Type": "application/zip"}
	m := strings.Replace(weeklyManifest, `"export": ["xlsx"]`, `"export": ["xlsx"], "os": ["files", "network"], "net": ["api.example.com"]`, 1)
	r := h.doReq("POST", "/v1/templates", alice, zipOf(t, map[string]string{"myco.json": m, "index.html": "v1"}), zipHdr).want(t, 201)
	if os, _ := r.body["os"].([]any); len(os) != 2 || os[1] != "network" {
		t.Fatalf("template os: %s", r.raw)
	}
	page := str(h.do("POST", "/v1/templates/alice%2Fweekly/use", alice, map[string]any{"name": "x"}).want(t, 201).body, "page", "id")
	g := h.do("GET", "/v1/pages/"+page, alice, nil).want(t, 200)
	tpl, _ := g.body["template"].(map[string]any)
	if net, _ := tpl["net"].([]any); len(net) != 1 || net[0] != "api.example.com" {
		t.Fatalf("page template net: %s", g.raw)
	}
	bad := strings.Replace(weeklyManifest, `"export": ["xlsx"]`, `"export": ["xlsx"], "os": ["everything"]`, 1)
	h.doReq("POST", "/v1/templates", alice, zipOf(t, map[string]string{"myco.json": bad, "index.html": "v1"}), zipHdr).wantErr(t, 400, "invalid_argument")
}

// Who may change a page's records is set by those who manage it
// (PATCH /v1/pages/{id} {editors}); the page says so, and to whom it is read-only.
func TestPageEditorsSetting(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	zipHdr := map[string]string{"Content-Type": "application/zip"}
	h.doReq("POST", "/v1/templates", alice, zipOf(t, map[string]string{"myco.json": weeklyManifest, "index.html": "v1"}), zipHdr).want(t, 201)
	page := str(h.do("POST", "/v1/templates/alice%2Fweekly/use", alice, map[string]any{"name": "第 49 周"}).want(t, 201).body, "page", "id")
	if g := h.do("GET", "/v1/pages/"+page, alice, nil).want(t, 200); g.body["editors"] != nil || g.body["read_only"] != nil {
		t.Fatalf("everyone by default, writable: %s", g.raw)
	}
	p := h.do("PATCH", "/v1/pages/"+page, alice, map[string]any{"editors": "managers"}).want(t, 200)
	if p.body["editors"] != "managers" || p.body["read_only"] != nil {
		t.Fatalf("managers; alice manages it: %s", p.raw)
	}
	// she still writes it
	h.do("POST", "/v1/pages/"+page+"/data/items", alice, map[string]any{"data": map[string]any{"title": "发版"}}).want(t, 201)
	h.do("PATCH", "/v1/pages/"+page, alice, map[string]any{"editors": "friends"}).wantErr(t, 400, "invalid_argument")
	h.do("PATCH", "/v1/pages/"+page, alice, map[string]any{}).wantErr(t, 400, "invalid_argument")
	if p := h.do("PATCH", "/v1/pages/"+page, alice, map[string]any{"editors": "everyone"}).want(t, 200); p.body["editors"] != nil {
		t.Fatalf("back to everyone: %s", p.raw)
	}
}
