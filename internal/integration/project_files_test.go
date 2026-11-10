package integration_test

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
)

// Project files: members upload any kind of file; small text is read with
// the project, the rest is downloaded; only the uploader or an editor
// replaces or removes one.
func TestProjectFiles(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	enroll := func(name string) *client.Client {
		enr, err := admin.CreateEnrollment(c, api.CreateEnrollmentRequest{NamePrefix: name, MaxUses: 1}, false)
		if err != nil {
			t.Fatal(err)
		}
		res, err := h.client(enr.Token).Enroll(c, api.EnrollRequest{Name: name, Binding: api.Binding{Harness: "claude", SessionID: "s-" + name}}, false)
		if err != nil {
			t.Fatal(err)
		}
		return h.client(res.Token)
	}
	a, b, outsider := enroll("alpha"), enroll("beta"), enroll("outsider")
	if _, err := admin.CreateProject(c, api.CreateProjectRequest{Name: "myco"}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"alpha", "beta"} {
		if _, err := admin.PutProjectMember(c, "myco", n, api.ProjectMemberRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	put := func(cl *client.Client, name, ct string, data []byte) (*api.ProjectDoc, error) {
		return cl.PutProjectFile(c, "myco", name, ct, bytes.NewReader(data), int64(len(data)))
	}
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0, 1, 2, 3}, 100)...)

	// Any member uploads; outsiders do not.
	if _, err := put(outsider, "x.md", "", []byte("hi")); err == nil {
		t.Fatal("a non-member uploaded a file")
	}
	md, err := put(a, "设计.md", "", []byte("# 设计\n\nuse SQLite"))
	if err != nil || md.Binary || md.Name != "设计.md" || md.ContentType == "" {
		t.Fatalf("text: %v %+v", err, md)
	}
	img, err := put(a, "mock up.png", "image/png", png)
	if err != nil || !img.Binary || img.Size != len(png) {
		t.Fatalf("binary: %v %+v", err, img)
	}
	if _, err := put(a, "../evil", "", []byte("x")); err == nil {
		t.Fatal("a name with a folder was accepted")
	}

	// Text comes with the project (for agents and pull); the binary is fetched.
	p, err := b.GetProject(c, "myco", true)
	if err != nil || len(p.Docs) != 2 {
		t.Fatalf("project: %v %+v", err, p)
	}
	for _, d := range p.Docs {
		if d.Name == "设计.md" && !strings.Contains(d.Body, "SQLite") {
			t.Fatalf("text body missing: %+v", d)
		}
		if d.Name == "mock up.png" && (d.Body != "" || !d.Binary) {
			t.Fatalf("binary listed with a body: %+v", d)
		}
	}
	rc, ct, err := b.OpenProjectFile(c, "myco", "mock up.png")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, png) || ct != "image/png" {
		t.Fatalf("download: %q %d bytes", ct, len(got))
	}

	// beta may not replace or remove alpha's file; the hub's admin may.
	if _, err := put(b, "设计.md", "", []byte("mine now")); err == nil {
		t.Fatal("another member replaced the file")
	}
	if err := b.DeleteProjectDoc(c, "myco", "mock up.png"); err == nil {
		t.Fatal("another member removed the file")
	}
	if _, err := put(admin, "设计.md", "", []byte("# v2")); err != nil {
		t.Fatalf("editor replaces: %v", err)
	}
	if err := a.DeleteProjectDoc(c, "myco", "mock up.png"); err != nil {
		t.Fatalf("uploader removes: %v", err)
	}
	if _, _, err := b.OpenProjectFile(c, "myco", "mock up.png"); err == nil {
		t.Fatal("removed file still served")
	}
}
