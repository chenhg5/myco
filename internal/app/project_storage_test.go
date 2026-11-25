package app_test

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/blob"
	"github.com/myco-ai/myco/internal/infra/security"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

func storageFixture(t *testing.T) (*app.ProjectService, *sqlite.ProjectRepo) {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	pr := sqlite.NewProjectRepo(db)
	s := app.NewProjectService(pr, sqlite.NewAgentRepo(db), security.IDs{}, app.SystemClock{})
	s.SetBlobs(blob.FS{Dir: t.TempDir()})
	if _, err = s.Create(context.Background(), app.Actor{Admin: true}, "", "quota", "Quota", "", "team"); err != nil {
		t.Fatal(err)
	}
	return s, pr
}
func TestProjectStorageSeparateCountsAndTransitions(t *testing.T) {
	s, _ := storageFixture(t)
	ctx := context.Background()
	by := app.Actor{Admin: true}
	p := app.DefaultProjectStoragePolicy()
	p.FilesMax = 1
	if err := s.SetStoragePolicy(p, nil); err != nil {
		t.Fatal(err)
	}
	put := func(name, ct string, b []byte) error {
		_, e := s.PutFile(ctx, by, "quota", name, ct, "owner", bytes.NewReader(b))
		return e
	}
	if e := put("picture.png", "image/png", []byte{0}); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 100; i++ {
		if _, e := s.PutDoc(ctx, by, "quota", fmt.Sprintf("doc%d", i), "text", "owner"); e != nil {
			t.Fatal(i, e)
		}
	}
	if e := put("another.png", "image/png", []byte{0}); e == nil {
		t.Fatal("exceeded file count")
	}
	if _, e := s.AddMemory(ctx, by, "quota", "memory", "owner"); e == nil {
		t.Fatal("exceeded context document count")
	}
	if e := put("picture.png", "text/plain", []byte("text")); e == nil {
		t.Fatal("binary-to-context transition exceeded documents")
	}
	if e := put("picture.png", "image/png", []byte{0, 1}); e != nil {
		t.Fatal("replacement at quota", e)
	}
	if e := s.DeleteDoc(ctx, by, "quota", "doc0", "owner"); e != nil {
		t.Fatal(e)
	}
	if e := put("picture.png", "text/plain", []byte("text")); e != nil {
		t.Fatal(e)
	}
	if e := put("new.png", "image/png", []byte{0}); e != nil {
		t.Fatal(e)
	}
	v, e := s.Get(ctx, by, "quota")
	if e != nil {
		t.Fatal(e)
	}
	u := s.Storage(v)
	if u.FilesUsed != 1 || u.DocsUsed != 100 || u.BytesUsed != 401 {
		t.Fatalf("usage %+v", u)
	}
}
func TestProjectStorageBytesAndOverrides(t *testing.T) {
	s, pr := storageFixture(t)
	ctx := context.Background()
	by := app.Actor{Admin: true}
	p := app.DefaultProjectStoragePolicy()
	p.FilesMax = 2
	p.BytesMax = 8
	if e := s.SetStoragePolicy(p, nil); e != nil {
		t.Fatal(e)
	}
	if _, e := s.PutDoc(ctx, by, "quota", "text", "abc", "owner"); e != nil {
		t.Fatal(e)
	}
	put := func(data []byte) error {
		_, e := s.PutFile(ctx, by, "quota", "binary.bin", "application/octet-stream", "owner", bytes.NewReader(data))
		return e
	}
	if e := put([]byte{0, 1, 2, 3, 4}); e != nil {
		t.Fatal(e)
	}
	if e := put([]byte{0, 1, 2, 3, 4, 5}); e == nil {
		t.Fatal("exceeded bytes")
	}
	if e := put([]byte{0}); e != nil {
		t.Fatal("replacement must subtract old size", e)
	}
	for _, actor := range []app.Actor{{Member: &domain.Member{ID: "ordinary", Role: domain.RoleMember}}, {Admin: true, Agent: &domain.Agent{Name: "agent"}}, {Admin: true, Via: &domain.Agent{Name: "via"}}} {
		if _, e := s.SetStorageLimits(ctx, actor, "quota", 5, 100); !domain.IsCode(e, domain.CodePermissionDenied) {
			t.Fatalf("unauthorized override: %v", e)
		}
	}
	for _, limits := range []struct {
		f int
		b int64
	}{{10001, 1}, {1, app.MaxProjectBytes + 1}, {-1, 0}, {0, -1}} {
		if _, e := s.SetStorageLimits(ctx, by, "quota", limits.f, limits.b); e == nil {
			t.Fatal("accepted invalid override", limits)
		}
	}
	v, e := s.SetStorageLimits(ctx, by, "quota", 5, 100)
	if e != nil {
		t.Fatal(e)
	}
	old := *v.Project
	if u := s.Storage(v); u.FilesMax != 5 || u.BytesMax != 100 {
		t.Fatal(u)
	}
	// Ordinary saves never replace quota fields from a stale project snapshot.
	if _, e = s.SetStorageLimits(ctx, by, "quota", 6, 200); e != nil {
		t.Fatal(e)
	}
	old.Title = "renamed"
	if e = pr.Save(ctx, &old); e != nil {
		t.Fatal(e)
	}
	v, e = s.Get(ctx, by, "quota")
	if e != nil || v.Project.FilesMax != 6 {
		t.Fatalf("lost override: %+v %v", v, e)
	}
	v, e = s.SetStorageLimits(ctx, by, "quota", 0, 0)
	if e != nil {
		t.Fatal(e)
	}
	if u := s.Storage(v); u.FilesMax != 2 || u.BytesMax != 8 {
		t.Fatal(u)
	}
}
func TestProjectStorageConcurrentAdmissionAndDiskRejection(t *testing.T) {
	s, _ := storageFixture(t)
	ctx := context.Background()
	by := app.Actor{Admin: true}
	p := app.DefaultProjectStoragePolicy()
	p.FilesMax = 1
	if e := s.SetStoragePolicy(p, nil); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.PutFile(ctx, by, "quota", fmt.Sprintf("f%d.png", i), "image/png", "owner", bytes.NewReader([]byte{0}))
			if e == nil {
				mu.Lock()
				accepted++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if accepted != 1 {
		t.Fatalf("admitted %d for one slot", accepted)
	}
	if e := s.SetStoragePolicy(p, func(n int64) error { return domain.Errf(domain.CodeInvalidState, "disk low") }); e != nil {
		t.Fatal(e)
	}
	if _, e := s.PutFile(ctx, by, "quota", "blocked.png", "image/png", "owner", unreadableUpload{}); !domain.IsCode(e, domain.CodeInvalidState) {
		t.Fatal(e)
	}
	if _, e := s.PutDoc(ctx, by, "quota", "doc", "text", "owner"); !domain.IsCode(e, domain.CodeInvalidState) {
		t.Fatal(e)
	}
}

type unreadableUpload struct{}

func (unreadableUpload) Read([]byte) (int, error) { panic("read upload before checking disk reserve") }
