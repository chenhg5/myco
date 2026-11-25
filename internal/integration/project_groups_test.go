package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
)

// Project member groups: a project always says which groups it has (the
// key is how the App tells the hub supports them); its editors (a person's
// console token, a coordinator) create, rename and delete groups and move
// members in one PATCH; a plain member may not.
func TestProjectGroups(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	ct, err := admin.CreateConsoleToken(c, "alice-mac")
	if err != nil {
		t.Fatal(err)
	}
	alice := h.client(ct.Token)
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
	lead, web := enroll("lead"), enroll("web")
	enroll("api")
	if _, err := alice.CreateProject(c, api.CreateProjectRequest{Name: "myco"}); err != nil {
		t.Fatal(err)
	}
	coord := "coordinator"
	for _, a := range []string{"lead", "web", "api"} {
		req := api.ProjectMemberRequest{}
		if a == "lead" {
			req.Role = &coord
		}
		if _, err := admin.PutProjectMember(c, "myco", a, req); err != nil {
			t.Fatal(err)
		}
	}

	// The key is there, empty, before any group exists.
	req, _ := http.NewRequest(http.MethodGet, h.url+"/v1/projects/myco", nil)
	req.Header.Set("Authorization", "Bearer "+ct.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(raw), `"groups":[]`) || strings.Contains(string(raw), `"group":`) {
		t.Fatalf("GET project: %s", raw)
	}

	groupOf := func(p *api.Project, agent string) string {
		for _, m := range p.Members {
			if m.Name == agent {
				return m.Group
			}
		}
		t.Fatalf("%s not in %s", agent, p.Name)
		return ""
	}
	// A person (console token) creates groups and sorts members, in one PATCH.
	groups := []string{"前端", "后端"}
	p, err := alice.SetProjectGroups(c, "myco", &groups, map[string]string{"web": "前端", "api": "后端"})
	if err != nil || !slices.Equal(p.Groups, groups) || groupOf(p, "web") != "前端" || groupOf(p, "api") != "后端" || groupOf(p, "lead") != "" {
		t.Fatalf("create groups: %v %+v", err, p)
	}
	// A coordinator renames 后端 to 服务端 and deletes 前端.
	groups = []string{"服务端"}
	p, err = lead.SetProjectGroups(c, "myco", &groups, map[string]string{"api": "服务端"})
	if err != nil || !slices.Equal(p.Groups, groups) || groupOf(p, "api") != "服务端" || groupOf(p, "web") != "" {
		t.Fatalf("rename and delete: %v %+v", err, p)
	}
	// A plain member may not.
	if _, err := web.SetProjectGroups(c, "myco", nil, map[string]string{"web": "服务端"}); !isCode(err, "permission_denied") {
		t.Fatalf("member edits groups: %v", err)
	}
	// Bad requests change nothing.
	for name, r := range map[string]api.UpdateProjectRequest{
		"not a member":  {MemberGroups: map[string]string{"stranger": "服务端"}},
		"unknown group": {MemberGroups: map[string]string{"web": "设计"}},
		"duplicate":     {Groups: &[]string{"a", "a"}},
		"empty name":    {Groups: &[]string{" "}},
	} {
		if _, err := alice.UpdateProject(c, "myco", r); !isCode(err, "invalid_argument") {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// PUT member sets or clears one member's group.
	g := "服务端"
	if p, err = alice.PutProjectMember(c, "myco", "web", api.ProjectMemberRequest{Group: &g}); err != nil || groupOf(p, "web") != "服务端" {
		t.Fatalf("put member group: %v", err)
	}
	g = "设计"
	if _, err = alice.PutProjectMember(c, "myco", "web", api.ProjectMemberRequest{Group: &g}); !isCode(err, "invalid_argument") {
		t.Fatalf("unknown group on put: %v", err)
	}
	g = ""
	if p, err = alice.PutProjectMember(c, "myco", "web", api.ProjectMemberRequest{Group: &g}); err != nil || groupOf(p, "web") != "" {
		t.Fatalf("clear group: %v", err)
	}
	// Other edits keep the groups; the list shows them too.
	title := "myco 开发"
	if p, err = alice.UpdateProject(c, "myco", api.UpdateProjectRequest{Title: &title}); err != nil || !slices.Equal(p.Groups, []string{"服务端"}) || groupOf(p, "api") != "服务端" {
		t.Fatalf("title edit: %v %+v", err, p)
	}
	list, err := alice.ListProjects(c, false)
	if err != nil || len(list) != 1 || !slices.Equal(list[0].Groups, []string{"服务端"}) {
		t.Fatalf("list: %v %+v", err, list)
	}

	// Passed on (the App's console backend re-encodes projects): a project
	// from an older hub, without groups, stays without the key.
	b, _ := json.Marshal(api.Project{Name: "old"})
	if strings.Contains(string(b), `"groups"`) {
		t.Fatalf("old hub's project gained groups: %s", b)
	}
	b, _ = json.Marshal(api.Project{Name: "new", Groups: []string{}})
	if !strings.Contains(string(b), `"groups":[]`) {
		t.Fatalf("new hub's project lost groups: %s", b)
	}
}

func isCode(err error, code string) bool {
	e, ok := client.AsError(err)
	return ok && e.Code == code
}
