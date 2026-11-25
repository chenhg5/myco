package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/blob"
	"github.com/myco-ai/myco/internal/infra/security"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

const attachmentManifest = `{"id":"demo/assets","name":"Assets","version":"1.0.0","data":{"assets":{"fields":{"file":"text","attachment":"file","title":"text"}},"settings":{"write":"managers","fields":{"document":"file"}}}}`

type attachmentFixture struct {
	db       *sql.DB
	project  *ProjectService
	pages    *PageService
	pageRepo *sqlite.PageRepo
	data     *PageDataService
	repo     *sqlite.PageAttachmentRepo
	service  *PageAttachmentService
	store    blob.FS
}

func pageAttachmentFixture(t *testing.T, policy PageAttachmentPolicy) *attachmentFixture {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ids := security.IDs{}
	clock := SystemClock{}
	store := blob.FS{Dir: t.TempDir()}
	projects := NewProjectService(sqlite.NewProjectRepo(db), sqlite.NewAgentRepo(db), ids, clock)
	projects.SetBlobs(store)
	if _, err = projects.Create(context.Background(), Actor{Admin: true}, "owner", "demo", "Demo", "", "team"); err != nil {
		t.Fatal(err)
	}
	pr := sqlite.NewPageRepo(db)
	pages := NewPageService(pr, store, projects, ids, clock)
	f := &attachmentFixture{db: db, project: projects, pages: pages, pageRepo: pr, repo: sqlite.NewPageAttachmentRepo(db), store: store}
	f.service = NewPageAttachmentService(pages, f.repo, store, ids, clock, policy, nil)
	f.data = NewPageDataService(pages, sqlite.NewPageDataRepo(db), ids, clock)
	f.addPage(t, "pg_demo", "demo", "owner")
	return f
}
func (f *attachmentFixture) addPage(t *testing.T, id, project, owner string) {
	t.Helper()
	p := &domain.Page{ID: id, Name: id, Project: project, Owner: owner, Publisher: "owner", Manifest: attachmentManifest, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := f.pageRepo.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}
func (f *attachmentFixture) put(t *testing.T, key string, b []byte) *domain.PageAttachment {
	t.Helper()
	a, err := f.service.Put(context.Background(), Actor{Admin: true}, PageAttachmentInput{Page: "pg_demo", Collection: "assets", Name: key + ".png", ContentType: "image/png", Key: key, By: "admin"}, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func (f *attachmentFixture) record(t *testing.T, page, coll, id, data string) {
	t.Helper()
	if _, err := f.data.Put(context.Background(), Actor{Admin: true}, page, PutInput{Collection: coll, ID: id, Data: data, IfVersion: 0, By: "admin"}); err != nil {
		t.Fatal(err)
	}
}
func TestPageAttachmentQuotaReferencesAndPageDeletion(t *testing.T) {
	f := pageAttachmentFixture(t, PageAttachmentPolicy{1, 4})
	ctx := context.Background()
	admin := Actor{Admin: true}
	a := f.put(t, "one", []byte{0, 1, 2, 3})
	again, err := f.service.Put(ctx, admin, PageAttachmentInput{Page: a.Page, Collection: a.Collection, Name: a.Name, ContentType: a.ContentType, Key: "one", By: "admin"}, bytes.NewReader([]byte{0, 1, 2, 3}))
	if err != nil || again.ID != a.ID {
		t.Fatal("not idempotent", err)
	}
	if _, err = f.service.Put(ctx, admin, PageAttachmentInput{Page: a.Page, Collection: a.Collection, Name: a.Name, ContentType: a.ContentType, Key: "one", By: "admin"}, bytes.NewReader([]byte{0})); !domain.IsCode(err, domain.CodeConflict) {
		t.Fatal("key content conflict", err)
	}
	if _, err = f.service.Put(ctx, admin, PageAttachmentInput{Page: a.Page, Collection: a.Collection, Name: "two", Key: "two"}, bytes.NewReader([]byte{0})); err == nil {
		t.Fatal("exceeded quota")
	}
	u, err := f.service.Usage(ctx, admin, a.Page)
	if err != nil || u.FilesUsed != 1 || u.BytesUsed != 4 {
		t.Fatalf("usage %+v %v", u, err)
	}
	p, err := f.project.Get(ctx, admin, "demo")
	if err != nil || len(p.Docs) != 0 {
		t.Fatal("page file entered project context", err)
	}
	f.record(t, a.Page, "assets", "one", fmt.Sprintf(`{"attachment":{"file_id":%q}}`, a.ID))
	if err = f.service.Remove(ctx, admin, a.Page, a.ID); !domain.IsCode(err, domain.CodeConflict) {
		t.Fatal("removed referenced file", err)
	}
	f.addPage(t, "pg_other", "demo", "owner")
	if _, err = f.data.Put(ctx, admin, "pg_other", PutInput{Collection: "assets", ID: "cross", Data: fmt.Sprintf(`{"attachment":{"file_id":%q}}`, a.ID), IfVersion: 0}); err == nil {
		t.Fatal("cross-page file reference accepted")
	}
	if _, err = f.service.Get(ctx, admin, "pg_other", a.ID); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatal("cross-page attachment was readable", err)
	}
	if _, err = f.pages.Delete(ctx, admin, a.Page); err != nil {
		t.Fatal(err)
	}
	if err = f.service.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Open(ctx, a.Blob); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatal("page deletion did not collect its content", err)
	}
}
func TestPageAttachmentCollectionPermissionsAndExpiry(t *testing.T) {
	f := pageAttachmentFixture(t, DefaultPageAttachmentPolicy())
	ctx := context.Background()
	viewer := Actor{Member: &domain.Member{ID: "viewer", Name: "viewer", Role: domain.RoleMember}}
	if _, err := f.service.Put(ctx, viewer, PageAttachmentInput{Page: "pg_demo", Collection: "settings", Name: "private", Key: "private"}, strings.NewReader("x")); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatal("collection readonly bypass", err)
	}
	f.addPage(t, "pg_private", "", "other")
	secret, err := f.service.Put(ctx, Actor{Admin: true}, PageAttachmentInput{Page: "pg_private", Collection: "assets", Name: "secret", Key: "secret"}, strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Get(ctx, viewer, "pg_private", secret.ID); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatal("page permission bypass", err)
	}
	a := f.put(t, "pending", []byte("x"))
	ready := f.put(t, "ready", []byte("x"))
	f.record(t, "pg_demo", "assets", "ready", fmt.Sprintf(`{"attachment":{"file_id":%q}}`, ready.ID))
	if err = f.repo.Sweep(ctx, time.Now().Add(25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.repo.Get(ctx, "pg_demo", a.ID); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatal("expired pending upload retained", err)
	}
	if _, err = f.repo.Get(ctx, "pg_demo", ready.ID); err != nil {
		t.Fatal("referenced file expired", err)
	}
}
func TestPageAttachmentConcurrentQuotaAndDiskRefusal(t *testing.T) {
	f := pageAttachmentFixture(t, PageAttachmentPolicy{1, 10})
	var wg sync.WaitGroup
	var mu sync.Mutex
	n := 0
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := f.service.Put(context.Background(), Actor{Admin: true}, PageAttachmentInput{Page: "pg_demo", Collection: "assets", Name: "data", Key: fmt.Sprint(i)}, strings.NewReader("x"))
			if err == nil {
				mu.Lock()
				n++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if n != 1 {
		t.Fatal("quota oversubscribed", n)
	}
	f.service.disk = func(int64) error { return domain.Errf(domain.CodeInvalidState, "disk is low") }
	if _, err := f.service.Put(context.Background(), Actor{Admin: true}, PageAttachmentInput{Page: "pg_demo", Collection: "assets", Name: "data", Key: "low"}, unreadAttachment{}); err == nil {
		t.Fatal("disk not checked before body")
	}
}

type unreadAttachment struct{}

func (unreadAttachment) Read([]byte) (int, error) { panic("low disk must not read upload body") }
func TestPageAttachmentGenericMigrationAndIndependentOwners(t *testing.T) {
	f := pageAttachmentFixture(t, PageAttachmentPolicy{1, 1})
	ctx := context.Background()
	admin := Actor{Admin: true}
	f.addPage(t, "pg_second", "demo", "owner")
	data := []byte{0, 1, 2, 3}
	d, err := f.project.PutFile(ctx, admin, "demo", "diagram.png", "image/png", "owner", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.project.PutDoc(ctx, admin, "demo", "keep.md", "valuable context", "owner"); err != nil {
		t.Fatal(err)
	}
	f.record(t, "pg_demo", "assets", "image", `{"file":"diagram.png"}`)
	f.record(t, "pg_second", "settings", "image", fmt.Sprintf(`{"document":{"file_id":%q}}`, d.Blob))
	f.record(t, "pg_demo", "assets", "missing", `{"file":"missing.png"}`)
	dry, err := f.service.Migrate(ctx, admin, "demo", true, 100)
	if err != nil || dry.Sources != 1 || dry.Moved != 0 {
		t.Fatal("dry run", dry, err)
	}
	report, err := f.service.Migrate(ctx, admin, "demo", false, 100)
	if err != nil || report.Moved != 1 || report.Results[0].Error != "" {
		t.Fatalf("migration %+v %v", report, err)
	}
	p, err := f.project.Get(ctx, admin, "demo")
	if err != nil || len(p.Docs) != 1 || p.Docs[0].Name != "keep" {
		t.Fatal("project context was not separated", err)
	}
	one, err := f.service.Resolve(ctx, admin, "pg_demo", "diagram.png")
	if err != nil {
		t.Fatal(err)
	}
	two, err := f.service.Resolve(ctx, admin, "pg_second", d.Blob)
	if err != nil {
		t.Fatal(err)
	}
	if one.ID == two.ID || one.Blob == two.Blob || one.Blob == d.Blob {
		t.Fatal("shared blob ownership")
	}
	sum := sha256.Sum256(data)
	if one.SHA256 != hex.EncodeToString(sum[:]) || two.SHA256 != one.SHA256 {
		t.Fatal("migration hash mismatch")
	}
	if err = f.service.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Open(ctx, d.Blob); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatal("old project blob retained", err)
	}
	_, rc, err := f.service.LegacyOpen(ctx, admin, "demo", "diagram.png")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, data) {
		t.Fatal("legacy content changed")
	}
	again, err := f.service.Migrate(ctx, admin, "demo", false, 100)
	if err != nil || again.Sources != 0 || len(again.Skipped) != 1 {
		t.Fatalf("not idempotent: %+v %v", again, err)
	}
	if _, err = f.pages.Delete(ctx, admin, "pg_demo"); err != nil {
		t.Fatal(err)
	}
	if err = f.service.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	kept, err := f.store.Open(ctx, two.Blob)
	if err != nil {
		t.Fatal("deleting one page damaged another", err)
	}
	if err = kept.Close(); err != nil {
		t.Fatal(err)
	}
}

type corruptAttachmentStore struct{ domain.BlobStore }

func (s corruptAttachmentStore) Put(ctx context.Context, id string, r io.Reader, max int64) (int64, string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return 0, "", err
	}
	sum := sha256.Sum256(b)
	_, _, err = s.BlobStore.Put(ctx, id, strings.NewReader("corrupted"), max)
	return int64(len(b)), hex.EncodeToString(sum[:]), err
}
func TestPageAttachmentMigrationPreservesSourceOnCorruption(t *testing.T) {
	f := pageAttachmentFixture(t, DefaultPageAttachmentPolicy())
	ctx := context.Background()
	admin := Actor{Admin: true}
	if _, err := f.project.PutFile(ctx, admin, "demo", "source.png", "image/png", "owner", bytes.NewReader([]byte{0, 1})); err != nil {
		t.Fatal(err)
	}
	f.record(t, "pg_demo", "assets", "source", `{"file":"source.png"}`)
	f.service.blobs = corruptAttachmentStore{f.store}
	r, err := f.service.Migrate(ctx, admin, "demo", false, 100)
	if err != nil || r.Moved != 0 || r.Results[0].Error == "" {
		t.Fatal("bad copy was committed", r, err)
	}
	p, err := f.project.Get(ctx, admin, "demo")
	if err != nil || len(p.Docs) != 1 {
		t.Fatal("source lost", err)
	}
	u, err := f.service.Usage(ctx, admin, "pg_demo")
	if err != nil || u.FilesUsed != 0 {
		t.Fatal("failed staging leaked quota", u, err)
	}
}

type changingMigrationRepo struct {
	domain.PageAttachmentRepository
	before func()
}

func (r *changingMigrationRepo) CommitMigration(ctx context.Context, source domain.PageAttachmentMigration, a []*domain.PageAttachment) error {
	if r.before != nil {
		f := r.before
		r.before = nil
		f()
	}
	return r.PageAttachmentRepository.CommitMigration(ctx, source, a)
}
func TestPageAttachmentMigrationChecksRecordContentAndProjectScope(t *testing.T) {
	f := pageAttachmentFixture(t, DefaultPageAttachmentPolicy())
	ctx := context.Background()
	admin := Actor{Admin: true}
	if _, err := f.project.PutFile(ctx, admin, "demo", "source.png", "image/png", "owner", bytes.NewReader([]byte{0, 1})); err != nil {
		t.Fatal(err)
	}
	if _, err := f.project.Create(ctx, admin, "owner", "other", "Other", "", "team"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.project.PutFile(ctx, admin, "other", "foreign.png", "image/png", "owner", bytes.NewReader([]byte{0})); err != nil {
		t.Fatal(err)
	}
	f.record(t, "pg_demo", "assets", "source", `{"file":"source.png","title":"before"}`)
	f.record(t, "pg_demo", "assets", "foreign", `{"file":"foreign.png"}`)
	f.service.repo = &changingMigrationRepo{PageAttachmentRepository: f.repo, before: func() {
		if err := f.data.Remove(ctx, admin, "pg_demo", "assets", "source", 1, "admin"); err != nil {
			t.Fatal(err)
		}
		f.record(t, "pg_demo", "assets", "source", `{"file":"source.png","title":"after"}`)
	}}
	r, err := f.service.Migrate(ctx, admin, "demo", false, 100)
	if err != nil || r.Moved != 0 || r.Results[0].Error == "" || len(r.Skipped) != 1 {
		t.Fatal("record race or foreign project not detected", r, err)
	}
	p, err := f.project.Get(ctx, admin, "demo")
	if err != nil || len(p.Docs) != 1 {
		t.Fatal("racing source lost", err)
	}
	r, err = f.service.Migrate(ctx, admin, "demo", false, 100)
	if err != nil || r.Moved != 1 {
		t.Fatal("retry did not migrate latest record", r, err)
	}
	p, err = f.project.Get(ctx, admin, "other")
	if err != nil || len(p.Docs) != 1 {
		t.Fatal("foreign project document moved", err)
	}
}
func TestPageAttachmentMigrationResumesReservedUpload(t *testing.T) {
	f := pageAttachmentFixture(t, DefaultPageAttachmentPolicy())
	ctx := context.Background()
	admin := Actor{Admin: true}
	data := []byte{0, 1, 2}
	d, err := f.project.PutFile(ctx, admin, "demo", "source.png", "image/png", "owner", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	f.record(t, "pg_demo", "assets", "source", `{"file":"source.png"}`)
	candidates, _, err := f.repo.Candidates(ctx, "demo")
	if err != nil || len(candidates) != 1 {
		t.Fatal(candidates, err)
	}
	source := candidates[0]
	key := sha256.Sum256([]byte("migration:" + source.ProjectID + ":" + source.Signature + ":pg_demo"))
	a := f.service.attachment(PageAttachmentInput{Page: "pg_demo", Collection: "assets", Name: d.Name, ContentType: d.ContentType, Key: hex.EncodeToString(key[:]), By: d.UpdatedBy, LegacyName: d.Name}, data)
	if _, _, err = f.repo.Reserve(ctx, a, 1000, 2<<30, true); err != nil {
		t.Fatal(err)
	}
	if err = f.service.store(ctx, a, data); err != nil {
		t.Fatal(err)
	}
	// This is the durable state left by process death before the DB commit.
	result, err := f.service.Migrate(ctx, admin, "demo", false, 100)
	if err != nil || result.Moved != 1 {
		t.Fatal(result, err)
	}
	got, err := f.service.Resolve(ctx, admin, "pg_demo", d.Name)
	if err != nil || got.ID != a.ID {
		t.Fatal("staged ID was not resumed", got, err)
	}
	n, _, err := f.repo.Usage(ctx, "pg_demo")
	if err != nil || n != 1 {
		t.Fatal("restart duplicated quota", n, err)
	}
}

func TestPageAttachmentEncryptedWorkspaceRefusesPlaintextUpload(t *testing.T) {
	f := pageAttachmentFixture(t, DefaultPageAttachmentPolicy())
	f.pages.SetEncrypted(func(context.Context) bool { return true })
	if _, err := f.service.Put(context.Background(), Actor{Admin: true}, PageAttachmentInput{Page: "pg_demo", Collection: "assets", Name: "sample.png", Key: "key"}, bytes.NewReader([]byte{1})); !domain.IsCode(err, domain.CodeInvalidState) {
		t.Fatal(err)
	}
}

func TestPageAttachmentPolicyBounds(t *testing.T) {
	for _, p := range []PageAttachmentPolicy{{0, 1}, {10001, 1}, {1, 0}, {1, 20<<30 + 1}} {
		if p.Validate() == nil {
			t.Fatalf("accepted %+v", p)
		}
	}
	for _, p := range []PageAttachmentPolicy{DefaultPageAttachmentPolicy(), {10000, 20 << 30}, {1, 1}} {
		if err := p.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPageAttachmentExpiredUploadCannotBeLinkedAndRetryReplacesIt(t *testing.T) {
	f := pageAttachmentFixture(t, PageAttachmentPolicy{1, 4})
	ctx := context.Background()
	admin := Actor{Admin: true}
	old := f.put(t, "retry", []byte{1, 2, 3, 4})
	if _, err := f.db.Exec(`UPDATE page_attachments SET expires_at=? WHERE id=?`, time.Now().Add(-time.Minute).UnixMilli(), old.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.data.Put(ctx, admin, old.Page, PutInput{Collection: "assets", ID: "image", Data: fmt.Sprintf(`{"attachment":{"file_id":%q}}`, old.ID), By: "admin"}); !domain.IsCode(err, domain.CodeInvalidArgument) {
		t.Fatalf("expired upload linked: %v", err)
	}
	fresh, err := f.service.Put(ctx, admin, PageAttachmentInput{Page: old.Page, Collection: old.Collection, Name: old.Name, ContentType: old.ContentType, Key: "retry", By: "admin"}, bytes.NewReader([]byte{1, 2, 3, 4}))
	if err != nil || fresh.ID == old.ID {
		t.Fatalf("expired retry: %+v %v", fresh, err)
	}
	f.record(t, fresh.Page, "assets", "image", fmt.Sprintf(`{"attachment":{"file_id":%q}}`, fresh.ID))
	if err = f.service.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if rc, e := f.store.Open(ctx, old.Blob); e == nil {
		rc.Close()
		t.Fatal("expired blob survived cleanup")
	}
	rc, err := f.store.Open(ctx, fresh.Blob)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil || !bytes.Equal(b, []byte{1, 2, 3, 4}) {
		t.Fatal("retry blob changed", err)
	}
	usage, err := f.service.Usage(ctx, admin, fresh.Page)
	if err != nil || usage.FilesUsed != 1 || usage.BytesUsed != 4 {
		t.Fatal(usage, err)
	}
}

func TestPageAttachmentMigrationLeavesAmbiguousLegacyNames(t *testing.T) {
	f := pageAttachmentFixture(t, DefaultPageAttachmentPolicy())
	ctx := context.Background()
	admin := Actor{Admin: true}
	original, err := f.project.PutFile(ctx, admin, "demo", "original.png", "image/png", "owner", bytes.NewReader([]byte{1, 2, 3}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.project.PutFile(ctx, admin, "demo", original.Blob, "image/png", "owner", bytes.NewReader([]byte{4, 5, 6})); err != nil {
		t.Fatal(err)
	}
	f.record(t, "pg_demo", "assets", "first", `{"file":"original.png"}`)
	f.record(t, "pg_demo", "assets", "second", fmt.Sprintf(`{"file":%q}`, original.Blob))
	f.record(t, "pg_demo", "settings", "typed", fmt.Sprintf(`{"document":{"file_id":%q}}`, original.Blob))
	report, err := f.service.Migrate(ctx, admin, "demo", false, 100)
	if err != nil || report.Moved != 0 || len(report.Skipped) != 3 {
		t.Fatalf("ambiguous migration: %+v %v", report, err)
	}
	p, err := f.project.Get(ctx, admin, "demo")
	if err != nil || len(p.Docs) != 2 {
		t.Fatal("ambiguous sources were removed", err)
	}
}

func TestPageAttachmentBackgroundBatchContinuesAfterUnreadableSource(t *testing.T) {
	f := pageAttachmentFixture(t, DefaultPageAttachmentPolicy())
	ctx := context.Background()
	admin := Actor{Admin: true}
	bad, err := f.project.PutFile(ctx, admin, "demo", "a-missing.png", "image/png", "owner", bytes.NewReader([]byte{1, 2, 3}))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.Delete(ctx, bad.Blob); err != nil {
		t.Fatal(err)
	}
	if _, err = f.project.PutFile(ctx, admin, "demo", "b-good.png", "image/png", "owner", bytes.NewReader([]byte{4, 5, 6})); err != nil {
		t.Fatal(err)
	}
	f.record(t, "pg_demo", "assets", "bad", `{"file":"a-missing.png"}`)
	f.record(t, "pg_demo", "assets", "good", `{"file":"b-good.png"}`)
	first, err := f.service.migrate(ctx, admin, "demo", false, 1, "")
	if err != nil || len(first.Results) != 1 || first.Results[0].Error == "" {
		t.Fatalf("missing original report: %+v %v", first, err)
	}
	last := first.Results[0]
	next, err := f.service.migrate(ctx, admin, "demo", false, 1, last.Project+"\x00"+last.Name)
	if err != nil || next.Moved != 1 || next.Results[0].Name != "b-good.png" {
		t.Fatalf("failed original blocked next batch: %+v %v", next, err)
	}
	p, err := f.project.Get(ctx, admin, "demo")
	if err != nil || len(p.Docs) != 1 || p.Docs[0].Name != "a-missing.png" {
		t.Fatal("unreadable original was removed", err)
	}
}
