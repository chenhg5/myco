package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/hubserver"
	"github.com/myco-ai/myco/internal/testutil"
)

func TestPageAttachmentCLIAPIAndRange(t *testing.T) {
	home := isolatedHome(t)
	ctx := ctx(t)
	h := startAttachmentHub(t)
	cli := newCLI(t, h, filepath.Join(home, "writer.yaml"))
	cli.must(ctx, "agent", "register", "--name", "writer")
	dir := filepath.Join(t.TempDir(), "assets")
	cli.must(ctx, "template", "init", "--dir", dir, "--name", "Assets", "--id", "demo/page-assets")
	raw, err := os.ReadFile(filepath.Join(dir, "myco.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(raw, &m)
	m["data"] = map[string]any{"assets": map[string]any{"fields": map[string]string{"attachment": "file", "file": "text"}}}
	raw, _ = json.Marshal(m)
	os.WriteFile(filepath.Join(dir, "myco.json"), raw, 0600)
	cli.must(ctx, "template", "publish", "--dir", dir)
	var used api.TemplateUsed
	json.Unmarshal([]byte(cli.must(ctx, "template", "use", "--template", "demo/page-assets")), &used)
	page := used.Page.ID
	// Exceeds the generic JSON request limit; the upload route must allow it.
	content := bytes.Repeat([]byte{0, 1, 2, 3}, (1<<20)/4+9)
	path := filepath.Join(t.TempDir(), "sample.bin")
	os.WriteFile(path, content, 0600)
	if code, _, errout := cli.run(ctx, "page", "file", "put", "--page", page, "--collection", "assets", "--path", path, "--dry-run"); code != 10 {
		t.Fatal("dry run", code, errout)
	}
	var a api.PageAttachment
	if err = json.Unmarshal([]byte(cli.must(ctx, "page", "file", "put", "--page", page, "--collection", "assets", "--path", path, "--key", "upload-1")), &a); err != nil {
		t.Fatal(err)
	}
	if a.Size != int64(len(content)) || a.State != "pending" {
		t.Fatal(a)
	}
	var again api.PageAttachment
	json.Unmarshal([]byte(cli.must(ctx, "page", "file", "put", "--page", page, "--collection", "assets", "--path", path, "--key", "upload-1")), &again)
	if again.ID != a.ID {
		t.Fatal("duplicate retry")
	}
	payload, _ := json.Marshal(map[string]any{"attachment": a})
	cli.must(ctx, "page", "data", "put", "--page", page, "--collection", "assets", "--id", "sample", "--json", string(payload))
	admin := h.admin()
	response, err := admin.OpenPageAttachment(ctx, page, a.ID, "bytes=9-17")
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusPartialContent || !bytes.Equal(b, content[9:18]) || response.Header.Get("Content-Range") == "" {
		t.Fatal("range response", response.StatusCode, err)
	}
	response, err = admin.OpenPageAttachment(ctx, page, a.ID, "bytes=999999999-")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Fatal("invalid range", response.StatusCode)
	}
	dest := filepath.Join(t.TempDir(), "download.bin")
	cli.must(ctx, "page", "file", "get", "--page", page, "--file", a.ID, "--out", dest)
	b, _ = os.ReadFile(dest)
	if !bytes.Equal(b, content) {
		t.Fatal("download changed bytes")
	}
	var listed api.PageAttachmentList
	json.Unmarshal([]byte(cli.must(ctx, "page", "file", "list", "--page", page)), &listed)
	if listed.Usage.FilesUsed != 1 || listed.Usage.BytesUsed != a.Size || listed.Attachments[0].State != "ready" {
		t.Fatal(listed)
	}
	if code, _, _ := cli.run(ctx, "page", "file", "remove", "--page", page, "--file", a.ID); code == 0 {
		t.Fatal("removed referenced attachment")
	}
	cli.must(ctx, "page", "data", "rm", "--page", page, "--collection", "assets", "--id", "sample")
	cli.must(ctx, "page", "file", "remove", "--page", page, "--file", a.ID)
	out, err := admin.PageAttachments(ctx, page, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if out.Usage.FilesUsed != 0 {
		t.Fatal("usage did not decrease")
	}
}

func TestPageAttachmentMigrationHTTPCLI(t *testing.T) {
	home := isolatedHome(t)
	ctx := ctx(t)
	h := startAttachmentHub(t)
	cli := newCLI(t, h, filepath.Join(home, "writer.yaml"))
	cli.must(ctx, "agent", "register", "--name", "writer")
	admin := h.admin()
	if _, err := admin.CreateProject(ctx, api.CreateProjectRequest{Name: "demo", Visibility: "team"}); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.PutProjectMember(ctx, "demo", "writer", api.ProjectMemberRequest{}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "assets")
	cli.must(ctx, "template", "init", "--dir", dir, "--name", "Assets", "--id", "demo/legacy-assets")
	raw, _ := os.ReadFile(filepath.Join(dir, "myco.json"))
	var m map[string]any
	json.Unmarshal(raw, &m)
	m["data"] = map[string]any{"assets": map[string]any{"fields": map[string]string{"file": "text"}}}
	raw, _ = json.Marshal(m)
	os.WriteFile(filepath.Join(dir, "myco.json"), raw, 0600)
	cli.must(ctx, "template", "publish", "--dir", dir)
	var used api.TemplateUsed
	json.Unmarshal([]byte(cli.must(ctx, "template", "use", "--template", "demo/legacy-assets", "--project", "demo")), &used)
	page := used.Page.ID
	content := []byte{0, 2, 4, 6}
	if _, err := admin.PutProjectFile(ctx, "demo", "legacy.png", "image/png", bytes.NewReader(content), 4); err != nil {
		t.Fatal(err)
	}
	cli.must(ctx, "page", "data", "put", "--page", page, "--collection", "assets", "--id", "legacy", "--json", `{"file":"legacy.png"}`)
	var dry api.PageAttachmentMigrationReport
	json.Unmarshal([]byte(cli.must(ctx, "page", "file", "migrate", "--project", "demo", "--dry-run")), &dry)
	if dry.Sources != 1 || dry.Moved != 0 {
		t.Fatal(dry)
	}
	var report api.PageAttachmentMigrationReport
	json.Unmarshal([]byte(cli.must(ctx, "page", "file", "migrate", "--project", "demo")), &report)
	if report.Moved != 1 {
		t.Fatal(report)
	}
	p, err := admin.GetProject(ctx, "demo", true)
	if err != nil || len(p.Docs) != 0 {
		t.Fatal("still in project", p, err)
	}
	rc, _, err := admin.OpenProjectFile(ctx, "demo", "legacy.png")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(b, content) {
		t.Fatal("old GET failed")
	}
	if _, err = admin.ResolvePageAttachment(ctx, page, "legacy.png"); err != nil {
		t.Fatal(err)
	}
	if code, _, errout := cli.run(ctx, "page", "file", "migrate", "--project", "demo", "--limit", "101"); code == 0 || !strings.Contains(errout, "limit") {
		t.Fatal("invalid batch size", code, errout)
	}
}

// Storage policy has its own low-disk tests. Normal attachment flows must not
// depend on the free space on the test machine's temporary volume.
func startAttachmentHub(t *testing.T) *hub {
	t.Helper()
	policy := app.DefaultProjectStoragePolicy()
	policy.DiskMinFreeBytes, policy.DiskMinFreePercent = 0, 0
	db := filepath.Join(t.TempDir(), "hub.db")
	native, err := hubserver.Build(hubserver.Options{DBPath: db, AdminToken: adminToken, Policy: app.DefaultMessagePolicy(), OfficialDir: testutil.OfficialDir(t), ProjectStorage: &policy})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(native.Handler)
	h := &hub{t: t, db: db, h: native, url: srv.URL, srv: srv}
	t.Cleanup(h.stop)
	return h
}
