package hubserver

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/blob"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

func TestWorkspaceStartsPageAttachmentMigration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := t.TempDir()
	dir := filepath.Join(root, "workspaces", "demo")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(filepath.Join(dir, "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	project := sqlite.NewProjectRepo(db)
	if err = project.Create(ctx, &domain.Project{ID: "prj_demo", Name: "demo", Owner: "owner", Visibility: "team", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	store := blob.FS{Dir: filepath.Join(dir, "files")}
	if _, _, err = store.Put(ctx, "source_demo", bytes.NewReader([]byte{1, 2, 3}), 20<<20); err != nil {
		t.Fatal(err)
	}
	if err = project.PutDoc(ctx, &domain.ProjectDoc{Project: "prj_demo", Name: "legacy.png", Blob: "source_demo", Size: 3, ContentType: "image/png", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err = sqlite.NewPageRepo(db).Create(ctx, &domain.Page{ID: "pg_demo", Name: "Assets", Project: "demo", Owner: "owner", Manifest: `{"id":"demo/assets","name":"Assets","version":"1.0.0","data":{"assets":{"fields":{"file":"text"}}}}`, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err = sqlite.NewPageDataRepo(db).Put(ctx, &domain.PageRecord{Page: "pg_demo", Collection: "assets", ID: "image", Data: `{"file":"legacy.png"}`, UpdatedAt: now}, 0); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	policy := app.DefaultProjectStoragePolicy()
	policy.DiskMinFreeBytes = 0
	policy.DiskMinFreePercent = 0
	ws := newWorkspaces(ctx, Options{DBPath: filepath.Join(root, "hub.db"), AdminToken: "test-workspace-admin", Policy: app.DefaultMessagePolicy(), ProjectStorage: &policy}, nil)
	w, err := ws.open("demo")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.close()
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		out, e := w.hub.PageAttachments.List(ctx, app.Actor{Admin: true}, "pg_demo", "", 10)
		if e != nil {
			t.Fatal(e)
		}
		if len(out) == 1 {
			if out[0].Size != 3 || out[0].State != "ready" {
				t.Fatal(out[0])
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("workspace never migrated its referenced project file")
		case <-tick.C:
		}
	}
}
