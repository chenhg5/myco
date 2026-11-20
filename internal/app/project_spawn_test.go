package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/infra/notify"
	"github.com/chenhg5/myco/internal/infra/security"
	"github.com/chenhg5/myco/internal/infra/sqlite"
)

// A terminal in a project lets the project's agents ask that computer for
// new sessions in the terminal's folder, without being on its machine
// node's allow list; nothing else changes.
func TestProjectTerminalLetsItsAgentsSpawn(t *testing.T) {
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

	reg := func(name, harness, machine, workdir string, allow ...string) *domain.Agent {
		t.Helper()
		out, err := agents.Register(ctx, app.RegisterInput{Name: name, AllowFrom: allow, Binding: domain.Binding{Harness: harness, Workdir: workdir}})
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
	reg("wsl-machine", "machine", "m_wsl", "", "glenn-bot")
	reg("wsl-shell", "exec", "m_wsl", "/srv/harness", "coord")
	reg("mac-shell", "exec", "m_mac", "/srv/harness", "coord")
	coord := reg("coord", "claude", "", "")
	now := time.Now()
	p := &domain.Project{ID: "prj_1", Name: "cc", Visibility: "members", CreatedAt: now, UpdatedAt: now}
	if err := pr.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	put := func(agent string) {
		if err := pr.PutMember(ctx, &domain.ProjectMember{Project: p.ID, Agent: domain.AgentName(agent), Role: "member", AddedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	send := func(body string) (*app.SendOutput, error) {
		return msgs.Send(ctx, coord, app.SendInput{To: "wsl-machine", Type: "task", Body: body})
	}
	spawn := `{"cli":"codex","dir":"/srv/harness/work"}`

	// Not in a project with a terminal there: refused, as before.
	put("coord")
	put("mac-shell") // a terminal on another computer does not count
	if _, err := send(spawn); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("no terminal on that computer: %v", err)
	}
	put("wsl-shell")
	out, err := send(spawn)
	if err != nil || out.Message.GrantedBy != "project:cc" {
		t.Fatalf("spawn in the terminal's folder: %v %+v", err, out)
	}
	for _, body := range []string{
		`{"cli":"codex","dir":"/srv/other"}`,          // outside its folder
		`{"cli":"codex","dir":"/srv/harness-2"}`,      // only looks like it
		`{"cli":"codex","dir":"/srv/harness/../etc"}`, // climbs out
		`{"cli":"exec","dir":"/srv/harness"}`,         // a new terminal
		`{"cli":"codex","dir":"/srv/harness","allow_from":["coord"]}`,
		`hello`, // not a spawn request
	} {
		if _, err := send(body); !domain.IsCode(err, domain.CodePermissionDenied) {
			t.Fatalf("%s: want refused, got %v", body, err)
		}
	}
	// Archived projects grant nothing.
	p.Archived = true
	if err := pr.Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := send(spawn); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("archived project: %v", err)
	}
}
