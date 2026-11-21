package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
)

// An agent starts a template (myco template init), publishes it, makes a
// page from it with the sample data, and fills and watches its records with
// myco page data; the template's fields are checked; a stale write is
// refused with what to do.
func TestTemplateFromInitToRecords(t *testing.T) {
	home := isolatedHome(t)
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	coder := newCLI(t, h, filepath.Join(home, "coder.yaml"))
	coder.must(c, "agent", "register", "--name", "coder")

	dir := filepath.Join(t.TempDir(), "weekly")
	coder.must(c, "template", "init", "--dir", dir, "--name", "周报", "--id", "coder/weekly")
	for _, f := range []string{"myco.json", "index.html", "agent.md", "sample.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("init wrote no %s", f)
		}
	}
	if code, _, _ := coder.run(c, "template", "init", "--dir", dir, "--name", "again"); code == 0 {
		t.Fatal("init into a folder that is not empty should be refused")
	}

	var tpl api.Template
	if err := json.Unmarshal([]byte(coder.must(c, "template", "publish", "--dir", dir)), &tpl); err != nil || tpl.Slug != "coder/weekly" || tpl.Version != "1.0.0" || tpl.Data["rows"].Fields["title"] != "text" {
		t.Fatalf("publish: %v %+v", err, tpl)
	}
	if !strings.Contains(coder.must(c, "template", "get", "--template", "coder/weekly"), "myco page data put") {
		t.Fatal("get should carry the agent's guide")
	}

	var used api.TemplateUsed
	if err := json.Unmarshal([]byte(coder.must(c, "template", "use", "--template", "coder/weekly", "--name", "第 47 周", "--sample")), &used); err != nil || used.Page.Template == nil {
		t.Fatalf("use: %v %+v", err, used)
	}
	page := used.Page.ID

	var l api.PageRecords
	if err := json.Unmarshal([]byte(coder.must(c, "page", "data", "list", "--page", page)), &l); err != nil || len(l.Records) != 2 {
		t.Fatalf("sample rows: %v %+v", err, l)
	}
	var r api.PageRecord
	if err := json.Unmarshal([]byte(coder.must(c, "page", "data", "put", "--page", page, "--collection", "rows", "--json", `{"title": "发版", "status": "待办"}`)), &r); err != nil || r.Version != 1 {
		t.Fatalf("put: %v %+v", err, r)
	}
	// A field it does not have is refused; so is a stale write.
	if code, _, errb := coder.run(c, "page", "data", "put", "--page", page, "--collection", "rows", "--json", `{"nope": 1}`); code == 0 || !strings.Contains(errb, "nope") {
		t.Fatalf("unknown field: %d %s", code, errb)
	}
	coder.must(c, "page", "data", "put", "--page", page, "--collection", "rows", "--id", r.ID, "--if-version", "1", "--json", `{"title": "发版", "done": true}`)
	if code, _, errb := coder.run(c, "page", "data", "put", "--page", page, "--collection", "rows", "--id", r.ID, "--if-version", "1", "--json", `{"title": "x"}`); code == 0 || !strings.Contains(errb, "conflict") {
		t.Fatalf("stale write: %d %s", code, errb)
	}
	// What changed since the list, one JSON line each.
	out := coder.must(c, "page", "data", "watch", "--page", page, "--after", itoa64(l.Seq), "--once", "--timeout", "5s")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], `"done":true`) {
		t.Fatalf("watch: %q", out)
	}
	coder.must(c, "page", "data", "rm", "--page", page, "--collection", "rows", "--id", r.ID)
	if !strings.Contains(coder.must(c, "page", "get", "--page", page), "coder/weekly") {
		t.Fatal("page get should say which template it is from")
	}
}

func itoa64(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestTemplateLocalizedMetadataRoundTrip(t *testing.T) {
	home := isolatedHome(t)
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	coder := newCLI(t, h, filepath.Join(home, "coder.yaml"))
	coder.must(c, "agent", "register", "--name", "coder")
	dir := filepath.Join(t.TempDir(), "localized")
	coder.must(c, "template", "init", "--dir", dir, "--name", "笔记", "--id", "coder/localized")
	if err := os.WriteFile(filepath.Join(dir, "locales.json"), []byte(`{"en":{"name":"Notes","description":"Shared notes"},"zh":{"name":"笔记"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	var tpl api.Template
	if err := json.Unmarshal([]byte(coder.must(c, "template", "publish", "--dir", dir)), &tpl); err != nil || tpl.Locales["en"].Name != "Notes" || tpl.Name != "笔记" {
		t.Fatal("localized publish", tpl, err)
	}
	var list api.TemplateList
	if err := json.Unmarshal([]byte(coder.must(c, "template", "list", "--full")), &list); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range list.Templates {
		if v.Slug == "coder/localized" {
			found = true
			if v.Locales["en"].Name != "Notes" {
				t.Fatal("list dropped locales", v)
			}
		}
	}
	if !found {
		t.Fatal("template not listed")
	}
	var used api.TemplateUsed
	if err := json.Unmarshal([]byte(coder.must(c, "template", "use", "--template", "coder/localized", "--name", "My user title 用户标题")), &used); err != nil || used.Page.Name != "My user title 用户标题" {
		t.Fatal("changed user's page name", used, err)
	}
	// A new sidecar version invalidates the metadata cache.
	manifestFile := filepath.Join(dir, "myco.json")
	raw, err := os.ReadFile(manifestFile)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	json.Unmarshal(raw, &manifest)
	manifest["version"] = "1.0.1"
	raw, _ = json.Marshal(manifest)
	os.WriteFile(manifestFile, raw, 0600)
	os.WriteFile(filepath.Join(dir, "locales.json"), []byte(`{"en":{"name":"Notebook"}}`), 0600)
	if err := json.Unmarshal([]byte(coder.must(c, "template", "publish", "--dir", dir)), &tpl); err != nil || tpl.Locales["en"].Name != "Notebook" {
		t.Fatal("stale locale cache", tpl, err)
	}
	// Broken translations are rejected, while ordinary manifests remain valid.
	manifest["version"] = "1.0.2"
	raw, _ = json.Marshal(manifest)
	os.WriteFile(manifestFile, raw, 0600)
	os.WriteFile(filepath.Join(dir, "locales.json"), []byte(`{"../en":{"name":"bad"}}`), 0600)
	if code, _, _ := coder.run(c, "template", "publish", "--dir", dir); code == 0 {
		t.Fatal("published invalid locales")
	}
}
