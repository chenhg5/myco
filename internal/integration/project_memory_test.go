package integration_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
)

// Any member node adds to the project's memory; a node outside the project
// does not. The memory comes with the project, and the brief holds 16000
// characters.
func TestProjectMemory(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	glenn := h.admin()
	enroll := func(name string) *client.Client {
		enr, err := glenn.CreateEnrollment(c, api.CreateEnrollmentRequest{NamePrefix: name, MaxUses: 1}, false)
		if err != nil {
			t.Fatal(err)
		}
		res, err := h.client(enr.Token).Enroll(c, api.EnrollRequest{Name: name, Binding: api.Binding{Harness: "claude", SessionID: "s-" + name}}, false)
		if err != nil {
			t.Fatal(err)
		}
		return h.client(res.Token)
	}
	worker, stranger := enroll("worker"), enroll("stranger")
	if _, err := glenn.CreateProject(c, api.CreateProjectRequest{Name: "myco", Goal: "ship"}); err != nil {
		t.Fatal(err)
	}
	if _, err := glenn.PutProjectMember(c, "myco", "worker", api.ProjectMemberRequest{}); err != nil {
		t.Fatal(err)
	}

	p, err := worker.GetProject(c, "myco", false)
	if err != nil || p.Memory == nil || p.Memory.Body != "" {
		t.Fatalf("an empty memory comes with the project: %v %+v", err, p.Memory)
	}
	d, err := worker.AddProjectMemory(c, "myco", "glenn wants progress reports in Chinese,\n at most three bullets")
	if err != nil || !strings.Contains(d.Body, "- glenn wants progress reports in Chinese, at most three bullets (worker, ") {
		t.Fatalf("a member adds to the memory: %v %q", err, d.Body)
	}
	if _, err := glenn.AddProjectMemory(c, "myco", "Release only when glenn asks"); err != nil {
		t.Fatalf("the owner adds to the memory: %v", err)
	}
	if _, err := stranger.AddProjectMemory(c, "myco", "noise"); err == nil {
		t.Fatal("a node outside the project added to its memory")
	}
	if _, err := worker.AddProjectMemory(c, "myco", "  "); err == nil {
		t.Fatal("an empty entry was added")
	}
	p, err = worker.GetProject(c, "myco", false)
	if err != nil || strings.Count(p.Memory.Body, "\n- ") != 2 || strings.Contains(p.Memory.Body, "noise") {
		t.Fatalf("memory: %v %q", err, p.Memory.Body)
	}
	// It is a document: pulled and edited like the others.
	if doc, err := glenn.GetProjectDoc(c, "myco", api.ProjectMemory); err != nil || doc.Body != p.Memory.Body {
		t.Fatalf("memory document: %v %+v", err, doc)
	}

	brief := strings.Repeat("长", 16000)
	if _, err := glenn.UpdateProject(c, "myco", api.UpdateProjectRequest{Brief: &brief}); err != nil {
		t.Fatalf("a 16000-character brief: %v", err)
	}
	brief += "x"
	if _, err := glenn.UpdateProject(c, "myco", api.UpdateProjectRequest{Brief: &brief}); err == nil {
		t.Fatal("a brief over 16000 characters was kept")
	}
}
