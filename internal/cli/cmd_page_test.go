package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPagePublish(t *testing.T) {
	hub := newHub(t)
	dir := t.TempDir()
	h := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "agent.yaml")}}
	if code, _, e := h.run("agent", "register", "--name", "maker"); code != 0 {
		t.Fatalf("register: %s", e)
	}
	site := filepath.Join(dir, "site")
	for name, body := range map[string]string{
		"index.html": "<h1>hi</h1>", "app.js": "1", "img/a.png": "png",
		"node_modules/x/y.js": "no", ".git/HEAD": "no", ".env": "SECRET",
	} {
		p := filepath.Join(site, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	// A dry run says what would go up, and leaves out what a page never needs.
	code, out, e := h.run("page", "publish", "--dir", site, "--name", "报告", "--dry-run", "--format", "json")
	if code != ExitDryRun || strings.Contains(out, "node_modules") || strings.Contains(out, ".env") || !strings.Contains(out, "img/a.png") {
		t.Fatalf("dry run: %d %s %s", code, out, e)
	}
	code, out, e = h.run("page", "publish", "--dir", site, "--name", "报告", "--format", "json")
	if code != 0 {
		t.Fatalf("publish: %s", e)
	}
	var p struct {
		ID      string
		Version int
		URL     string
	}
	json.Unmarshal([]byte(out), &p)
	if p.Version != 1 || !strings.HasSuffix(p.URL, "/pages/"+p.ID) {
		t.Fatalf("published: %s", out)
	}
	// Text output says where it opens.
	if code, out, _ := h.run("page", "publish", "--dir", site, "--page", p.ID, "--format", "text"); code != 0 || !strings.Contains(out, "version 2") || !strings.Contains(out, p.URL) {
		t.Fatalf("new version: %s", out)
	}
	// One html file is a page too.
	one := filepath.Join(dir, "chart.html")
	os.WriteFile(one, []byte("<canvas></canvas>"), 0o644)
	if code, out, e := h.run("page", "publish", "--dir", one, "--name", "图表", "--format", "json"); code != 0 || !strings.Contains(out, `"entry": "chart.html"`) {
		t.Fatalf("one file: %d %s %s", code, out, e)
	}
	if code, out, _ := h.run("page", "list", "--format", "json"); code != 0 || strings.Count(out, `"id": "pg_`) != 2 {
		t.Fatalf("list: %s", out)
	}
	if code, out, _ := h.run("page", "get", "--page", p.ID, "--format", "json"); code != 0 || strings.Count(out, `"n":`) != 2 {
		t.Fatalf("get: %s", out)
	}
	// A new page needs a name; a folder without html is refused.
	if code, _, e := h.run("page", "publish", "--dir", site); code == 0 || !strings.Contains(e, "--name") {
		t.Fatalf("no name: %d %s", code, e)
	}
	empty := filepath.Join(dir, "empty")
	os.MkdirAll(empty, 0o755)
	os.WriteFile(filepath.Join(empty, "data.json"), []byte("{}"), 0o644)
	if code, _, e := h.run("page", "publish", "--dir", empty, "--name", "x"); code == 0 || !strings.Contains(e, "html") {
		t.Fatalf("no html: %d %s", code, e)
	}
	if code, _, e := h.run("page", "rm", "--page", p.ID); code != 0 {
		t.Fatalf("rm: %s", e)
	}
	if code, _, _ := h.run("page", "get", "--page", p.ID); code == 0 {
		t.Fatal("removed page still there")
	}
}
