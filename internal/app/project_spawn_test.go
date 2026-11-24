package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/notify"
	"github.com/myco-ai/myco/internal/infra/security"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

// A project lets its coordinators ask the computers it allows for new
// agent sessions, without being on their machine nodes' allow lists. Its
// terminals have nothing to do with it.
func TestProjectMachinesLetCoordinatorsSpawn(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ar, pr := sqlite.NewAgentRepo(db), sqlite.NewProjectRepo(db)
	agents := app.NewAgentService(ar, security.Tokens{}, security.IDs{}, app.SystemClock{}, time.Minute, nil)
	msgs := app.NewMessageService(ar, sqlite.NewMessageRepo(db), notify.NewMemory(), security.IDs{}, app.SystemClock{}, app.DefaultMessagePolicy())
	msgs.SetProjects(pr)

	reg := func(name, harness, machine string, allow ...string) *domain.Agent {
		t.Helper()
		out, err := agents.Register(ctx, app.RegisterInput{Name: name, AllowFrom: allow, Binding: domain.Binding{Harness: harness}})
		if err != nil {
			t.Fatal(err)
		}
		a, err := agents.Authenticate(ctx, out.Token)
		if err != nil {
			t.Fatal(err)
		}
		if machine != "" {
			a.MachineID = machine
			if err := ar.SetMachine(ctx, a.Name, machine); err != nil {
				t.Fatal(err)
			}
		}
		return a
	}
	reg("wsl-machine", "machine", "m_wsl", "glenn-bot")
	reg("wsl-shell", "exec", "m_wsl", "coord")
	coord := reg("coord", "claude", "")
	helper := reg("helper", "claude", "")
	now := time.Now()
	p := &domain.Project{ID: "prj_1", Name: "cc", Visibility: "members", CreatedAt: now, UpdatedAt: now}
	if err := pr.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	put := func(agent string, role domain.ProjectRole) {
		if err := pr.PutMember(ctx, &domain.ProjectMember{Project: p.ID, Agent: domain.AgentName(agent), Role: role, AddedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	put("coord", domain.ProjectRoleCoordinator)
	put("helper", "member")
	put("wsl-shell", "member") // a terminal there grants nothing
	send := func(from *domain.Agent, body string) (*app.SendOutput, error) {
		return msgs.Send(ctx, from, app.SendInput{To: "wsl-machine", Type: "task", Body: body})
	}
	spawn := `{"cli":"codex","dir":"~/code/app"}`
	if _, err := send(coord, spawn); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("computer not allowed for the project: %v", err)
	}
	p.Machines = []string{"m_wsl"}
	if err := pr.Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	out, err := send(coord, spawn)
	if err != nil || out.Message.GrantedBy != "project:cc" {
		t.Fatalf("coordinator on an allowed computer: %v %+v", err, out)
	}
	if _, err := send(helper, spawn); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("a member that does not coordinate: %v", err)
	}
	for _, body := range []string{
		`{"cli":"exec","dir":"~/code"}`, // a new terminal
		`{"cli":"codex","dir":"~/code","allow_from":["coord"]}`,
		`hello`, // not a spawn request
	} {
		if _, err := send(coord, body); !domain.IsCode(err, domain.CodePermissionDenied) {
			t.Fatalf("%s: want refused, got %v", body, err)
		}
	}
	p.Archived = true
	if err := pr.Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := send(coord, spawn); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("archived project: %v", err)
	}
}
