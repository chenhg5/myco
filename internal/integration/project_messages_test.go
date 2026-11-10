package integration_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
)

// Project conversations: a new thread is put in a project by its first
// message (its members may; others may not), every later message in it
// carries the project, the recent list filters by it, and deleting the
// project keeps the conversations without it.
func TestProjectMessages(t *testing.T) {
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
	lead, worker, outsider := enroll("lead"), enroll("worker"), enroll("outsider")
	if _, err := admin.CreateProject(c, api.CreateProjectRequest{Name: "myco"}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"lead", "worker"} {
		if _, err := admin.PutProjectMember(c, "myco", n, api.ProjectMemberRequest{}); err != nil {
			t.Fatal(err)
		}
	}

	// Only members put a thread in the project.
	if _, err := outsider.Send(c, api.SendRequest{To: "worker", Body: "hi", Project: "myco"}, false); err == nil {
		t.Fatal("a non-member started a thread in the project")
	}
	if _, err := lead.Send(c, api.SendRequest{To: "worker", Body: "x", Project: "nope"}, false); err == nil {
		t.Fatal("sent into a project that does not exist")
	}
	task, err := lead.Send(c, api.SendRequest{To: "worker", Body: "add the filter", Project: "myco"}, false)
	if err != nil || task.Message.Project != "myco" {
		t.Fatalf("send: %v %+v", err, task)
	}
	// The worker gets the task with its project, and its reply carries it.
	got, err := worker.Pull(c, 1, time.Second)
	if err != nil || len(got) != 1 || got[0].Project != "myco" {
		t.Fatalf("pull: %v %+v", err, got)
	}
	reply, err := worker.Reply(c, got[0].ID, "done", false)
	if err != nil || reply.Project != "myco" {
		t.Fatalf("reply: %v %+v", err, reply)
	}
	// A follow-up in the thread belongs to it, whatever --project says.
	follow, err := lead.Send(c, api.SendRequest{To: "worker", Body: "and docs", ThreadID: task.Message.ThreadID, Project: "other"}, false)
	if err != nil || follow.Message.Project != "myco" {
		t.Fatalf("follow-up: %v %+v", err, follow)
	}
	// A thread outside the project.
	if _, err := lead.Send(c, api.SendRequest{To: "worker", Body: "unrelated"}, false); err != nil {
		t.Fatal(err)
	}
	// From the app, also into the project.
	if _, err := admin.ConsoleSend(c, api.SendRequest{To: "lead", Body: "status?", Project: "myco"}); err != nil {
		t.Fatalf("console send: %v", err)
	}

	all, err := admin.Recent(c, "", "", "", 0, 0)
	if err != nil || len(all.Items) != 3 {
		t.Fatalf("recent: %v %d", err, len(all.Items))
	}
	in, err := admin.RecentIn(c, "myco", "", "", "", 0, 0)
	if err != nil || len(in.Items) != 2 {
		t.Fatalf("recent in myco: %v %+v", err, in)
	}
	for _, it := range in.Items {
		if it.Project != "myco" {
			t.Fatalf("item %+v", it)
		}
	}

	// Archived: no new threads; deleted: conversations stay, unmarked.
	yes := true
	if _, err := admin.UpdateProject(c, "myco", api.UpdateProjectRequest{Archived: &yes}); err != nil {
		t.Fatal(err)
	}
	if _, err := lead.Send(c, api.SendRequest{To: "worker", Body: "more", Project: "myco"}, false); err == nil {
		t.Fatal("started a thread in an archived project")
	}
	if err := admin.DeleteProject(c, "myco"); err != nil {
		t.Fatal(err)
	}
	thread, err := lead.Thread(c, task.Message.ThreadID)
	if err != nil || len(thread) == 0 || thread[0].Project != "" {
		t.Fatalf("after delete: %v %+v", err, thread)
	}
}
