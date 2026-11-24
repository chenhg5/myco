package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"testing"

	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

// A hub database from before member groups: Open adds the columns, the old
// project has no groups and its members none, and groups then round-trip.
func TestProjectGroupsMigrateAndRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	mustNoErr(t, err)
	_, err = raw.Exec(`CREATE TABLE projects (
	  id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, title TEXT NOT NULL DEFAULT '', goal TEXT NOT NULL DEFAULT '',
	  brief TEXT NOT NULL DEFAULT '', links TEXT NOT NULL DEFAULT '[]', visibility TEXT NOT NULL DEFAULT 'members',
	  owner TEXT NOT NULL DEFAULT '', archived INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
	  machines TEXT NOT NULL DEFAULT '[]');
	CREATE TABLE project_members (
	  project TEXT NOT NULL, agent TEXT NOT NULL, role TEXT NOT NULL DEFAULT 'member', note TEXT NOT NULL DEFAULT '',
	  added_by TEXT NOT NULL DEFAULT '', added_at INTEGER NOT NULL, PRIMARY KEY (project, agent));
	INSERT INTO projects (id, name, title, created_at, updated_at) VALUES ('prj_1', 'myco', 'myco', 1000, 1000);
	INSERT INTO project_members (project, agent, role, note, added_at) VALUES ('prj_1', 'web', 'member', 'UI', 1000), ('prj_1', 'api', 'member', '', 1001);`)
	mustNoErr(t, err)
	mustNoErr(t, raw.Close())

	db, err := sqlite.Open(path)
	mustNoErr(t, err)
	defer db.Close()
	r := sqlite.NewProjectRepo(db)
	p, err := r.Get(ctx, "myco")
	mustNoErr(t, err)
	ms, err := r.Members(ctx, p.ID)
	mustNoErr(t, err)
	if len(p.Groups) != 0 || len(ms) != 2 || ms[0].Group != "" || ms[0].Note != "UI" {
		t.Fatalf("migrated: %+v %+v", p, ms[0])
	}

	groups := []string{"前端", "后端"}
	changed, err := p.ApplyGroups(ms, &groups, map[domain.AgentName]string{"web": "前端", "api": "后端"}, t0)
	mustNoErr(t, err)
	mustNoErr(t, r.SaveGroups(ctx, p, changed))
	// Save (other edits) leaves the groups alone.
	p.Title, p.Groups = "renamed", nil
	mustNoErr(t, r.Save(ctx, p))
	p, err = r.Get(ctx, "myco")
	mustNoErr(t, err)
	ms, err = r.Members(ctx, p.ID)
	mustNoErr(t, err)
	if !slices.Equal(p.Groups, groups) || p.Title != "renamed" || ms[0].Group != "前端" || ms[1].Group != "后端" {
		t.Fatalf("round trip: %q %q %q %q", p.Groups, p.Title, ms[0].Group, ms[1].Group)
	}
	// PutMember keeps the group it is given.
	ms[1].Note = "API"
	mustNoErr(t, r.PutMember(ctx, ms[1]))
	ms, _ = r.Members(ctx, p.ID)
	if ms[1].Group != "后端" || ms[1].Note != "API" {
		t.Fatalf("put member: %+v", ms[1])
	}
	// A member that is gone fails the whole change.
	p.Groups = []string{"x"}
	err = r.SaveGroups(ctx, p, []*domain.ProjectMember{{Agent: "ghost", Group: "x"}})
	wantCode(t, err, domain.CodeNotFound)
	if p, _ = r.Get(ctx, "myco"); !slices.Equal(p.Groups, groups) {
		t.Fatalf("rolled back: %q", p.Groups)
	}
}
