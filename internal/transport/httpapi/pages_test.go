package httpapi_test

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

func zipOf(t *testing.T, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// A page: an agent publishes a folder, people (and its owner's agents) see
// it, its files are served from the version asked for, sandboxed; a new
// version replaces "latest"; others do not see it; it can be deleted.
func TestPages(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	zipHdr := map[string]string{"Content-Type": "application/zip"}
	site := zipOf(t, map[string]string{"index.html": "<h1>v1</h1><script src=app.js></script>", "app.js": "console.log(1)", "img/a.svg": "<svg/>"})

	r := h.doReq("POST", "/v1/pages?name="+"周报", alice, site, zipHdr).want(t, 201)
	id := str(r.body, "id")
	if r.body["version"] != float64(1) || r.body["entry"] != "index.html" || r.body["publisher"] != "alice" ||
		!strings.HasSuffix(str(r.body, "url"), "/pages/"+id) || r.body["can_manage"] != true {
		t.Fatalf("publish: %s", r.raw)
	}
	// Its files, from a version or the latest; the entry for an empty path.
	f := h.doReq("GET", "/v1/pages/"+id+"/v/1/app.js", adminToken, nil, nil).want(t, 200)
	if string(f.raw) != "console.log(1)" || !strings.Contains(f.header.Get("Content-Type"), "javascript") ||
		!strings.HasPrefix(f.header.Get("Content-Security-Policy"), "sandbox") || !strings.Contains(f.header.Get("Cache-Control"), "immutable") {
		t.Fatalf("file: %v %q", f.header, f.raw)
	}
	if f := h.doReq("GET", "/v1/pages/"+id+"/v/latest/", alice, nil, nil).want(t, 200); !strings.Contains(string(f.raw), "v1") {
		t.Fatalf("entry: %q", f.raw)
	}
	h.doReq("GET", "/v1/pages/"+id+"/v/1/missing.js", alice, nil, nil).wantErr(t, 404, "not_found")
	// (a path out of the version is cleaned away by the router)
	h.doReq("GET", "/v1/pages/"+id+"/v/1/../x", alice, nil, nil).wantErr(t, 400, "invalid_argument")

	// A new version: latest moves, the old one stays.
	r = h.doReq("POST", "/v1/pages/"+id+"/versions", alice, zipOf(t, map[string]string{"index.html": "<h1>v2</h1>"}), zipHdr).want(t, 200)
	if r.body["version"] != float64(2) || len(r.body["versions"].([]any)) != 2 {
		t.Fatalf("version 2: %s", r.raw)
	}
	if f := h.doReq("GET", "/v1/pages/"+id+"/v/latest/index.html", alice, nil, nil).want(t, 200); string(f.raw) != "<h1>v2</h1>" {
		t.Fatalf("latest: %q", f.raw)
	}
	if f := h.doReq("GET", "/v1/pages/"+id+"/v/1/index.html", alice, nil, nil).want(t, 200); !strings.Contains(string(f.raw), "v1") {
		t.Fatalf("v1 kept: %q", f.raw)
	}

	// Bad uploads are refused, saying why.
	h.doReq("POST", "/v1/pages?name=x", alice, "not a zip", zipHdr).wantErr(t, 400, "invalid_argument")
	h.doReq("POST", "/v1/pages?name=x", alice, zipOf(t, map[string]string{"a.html": "a", "b.html": "b"}), zipHdr).wantErr(t, 400, "invalid_argument")
	h.doReq("POST", "/v1/pages?name=x", alice, zipOf(t, map[string]string{"../evil.html": "a"}), zipHdr).wantErr(t, 400, "invalid_argument")
	h.doReq("POST", "/v1/pages?name=", alice, site, zipHdr).wantErr(t, 400, "invalid_argument")
	if r := h.doReq("POST", "/v1/pages?name=x&entry=b.html", alice, zipOf(t, map[string]string{"a.html": "a", "b.html": "b"}), zipHdr).want(t, 201); r.body["entry"] != "b.html" {
		t.Fatalf("entry given: %s", r.raw)
	}

	// Its shared state: nothing yet; written with the version read; a stale
	// write is refused with the latest; kept across versions.
	if st := h.do("GET", "/v1/pages/"+id+"/state", alice, nil).want(t, 200); st.body["state"] != nil || st.body["version"] != float64(0) {
		t.Fatalf("no state yet: %s", st.raw)
	}
	if st := h.do("PUT", "/v1/pages/"+id+"/state", adminToken, map[string]any{"state": map[string]any{"done": []int{1}}, "if_version": 0}).want(t, 200); st.body["version"] != float64(1) {
		t.Fatalf("first write: %s", st.raw)
	}
	c := h.do("PUT", "/v1/pages/"+id+"/state", alice, map[string]any{"state": map[string]any{"done": []int{2}}, "if_version": 0}).want(t, 409)
	if latest, _ := c.body["latest"].(map[string]any); latest == nil || latest["version"] != float64(1) || c.body["error"] != "conflict" {
		t.Fatalf("stale write: %s", c.raw)
	}
	if st := h.do("PUT", "/v1/pages/"+id+"/state", alice, map[string]any{"state": map[string]any{"done": []int{1, 2}}, "if_version": 1}).want(t, 200); st.body["updated_by"] != "alice" {
		t.Fatalf("second write: %s", st.raw)
	}
	h.doReq("PUT", "/v1/pages/"+id+"/state", alice, `{"state": nope}`, nil).wantErr(t, 400, "invalid_argument")
	if st := h.do("GET", "/v1/pages/"+id+"/state", alice, nil).want(t, 200); st.body["version"] != float64(2) || !strings.Contains(string(st.raw), `"done":[1,2]`) {
		t.Fatalf("read back: %s", st.raw)
	}

	// Someone else's: a member who is not its owner does not see it.
	link := str(h.do("POST", "/v1/members", adminToken, map[string]any{"name": "dan"}).want(t, 201).body, "link")
	code := link[strings.LastIndex(link, "/")+1:]
	dan := str(h.do("POST", "/v1/console/pair", "", map[string]any{"code": code, "device": "test"}).want(t, 201).body, "token")
	h.do("GET", "/v1/pages/"+id, dan, nil).wantErr(t, 404, "not_found")
	h.doReq("GET", "/v1/pages/"+id+"/v/1/index.html", dan, nil, nil).wantErr(t, 404, "not_found")
	h.doReq("POST", "/v1/pages/"+id+"/versions", dan, site, zipHdr).wantErr(t, 404, "not_found")
	h.do("PUT", "/v1/pages/"+id+"/state", dan, map[string]any{"state": 1, "if_version": 2}).wantErr(t, 404, "not_found")
	if l := h.do("GET", "/v1/pages", dan, nil).want(t, 200); len(l.body["pages"].([]any)) != 0 {
		t.Fatalf("dan lists: %s", l.raw)
	}
	// Their own: they see and manage it; the hub's owner sees it too.
	mine := str(h.doReq("POST", "/v1/pages?name=mine", dan, site, zipHdr).want(t, 201).body, "id")
	if l := h.do("GET", "/v1/pages", adminToken, nil).want(t, 200); len(l.body["pages"].([]any)) != 3 {
		t.Fatalf("owner lists: %s", l.raw)
	}
	h.do("DELETE", "/v1/pages/"+mine, dan, nil).want(t, 204)

	// Deleted: gone.
	h.do("DELETE", "/v1/pages/"+id, alice, nil).want(t, 204)
	h.do("GET", "/v1/pages/"+id, alice, nil).wantErr(t, 404, "not_found")
	// Its address in a browser says where it opens.
	if p := h.doReq("GET", "/pages/"+id, "", nil, nil).want(t, 200); !strings.Contains(string(p.raw), "myco App") {
		t.Fatalf("link page: %q", p.raw)
	}
}

// A page or template bigger than a message (a bundled app is often a few
// MB) is taken, as a new page, a new version and a template.
func TestBigPageUploads(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	raw := make([]byte, 600<<10)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	big := base64.StdEncoding.EncodeToString(raw) // (does not compress)
	zipHdr := map[string]string{"Content-Type": "application/zip"}
	site := zipOf(t, map[string]string{"index.html": "<script src=app.js></script>", "app.js": big})
	if len(site) <= domain.MaxBodyBytes+64*1024 {
		t.Fatalf("the zip should be bigger than a message: %d", len(site))
	}
	r := h.doReq("POST", "/v1/pages?name=big", alice, site, zipHdr).want(t, 201)
	h.doReq("POST", "/v1/pages/"+str(r.body, "id")+"/versions", alice, site, zipHdr).want(t, 200)
	tpl := zipOf(t, map[string]string{"myco.json": weeklyManifest, "index.html": "<script src=app.js></script>", "app.js": big})
	h.doReq("POST", "/v1/templates", alice, tpl, zipHdr).want(t, 201)
}
