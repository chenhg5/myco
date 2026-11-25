package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

type PageAttachmentPolicy struct {
	FilesMax int
	BytesMax int64
}

func DefaultPageAttachmentPolicy() PageAttachmentPolicy { return PageAttachmentPolicy{1000, 2 << 30} }
func (p PageAttachmentPolicy) Validate() error {
	if p.FilesMax < 1 || p.FilesMax > 10000 || p.BytesMax < 1 || p.BytesMax > 20<<30 {
		return domain.Errf(domain.CodeInvalidArgument, "page attachment defaults must be 1..10000 files and 1..21474836480 bytes")
	}
	return nil
}

type PageAttachmentService struct {
	pages  *PageService
	repo   domain.PageAttachmentRepository
	blobs  domain.BlobStore
	ids    IDGenerator
	clock  Clock
	policy PageAttachmentPolicy
	disk   func(int64) error
	mu     sync.Mutex
}

func NewPageAttachmentService(pages *PageService, repo domain.PageAttachmentRepository, blobs domain.BlobStore, ids IDGenerator, clock Clock, policy PageAttachmentPolicy, disk func(int64) error) *PageAttachmentService {
	s := &PageAttachmentService{pages: pages, repo: repo, blobs: blobs, ids: ids, clock: clock, policy: policy, disk: disk}
	pages.attachmentMu = &s.mu
	return s
}
func (s *PageAttachmentService) page(ctx context.Context, by Actor, id, collection string, write bool) (*domain.Page, error) {
	v, err := s.pages.Get(ctx, by, id)
	if err != nil {
		return nil, err
	}
	p := v.Page
	if p.Live {
		return nil, domain.Errf(domain.CodeInvalidArgument, "live pages do not have page attachments")
	}
	if write {
		if _, err = domain.ParseCollection(collection); err != nil {
			return nil, err
		}
		if sc := schema(p); sc != nil {
			if _, ok := sc[collection]; !ok {
				return nil, domain.Errf(domain.CodeInvalidArgument, "page has no collection %s", collection)
			}
		}
		if !s.pages.CanEdit(by, p, collection) {
			return nil, readOnly(p, collection)
		}
	}
	return p, nil
}
func (s *PageAttachmentService) Usage(ctx context.Context, by Actor, page string) (domain.PageAttachmentUsage, error) {
	u := domain.PageAttachmentUsage{FilesMax: s.policy.FilesMax, BytesMax: s.policy.BytesMax, MaxFileBytes: domain.MaxPageAttachment}
	if _, err := s.page(ctx, by, page, "", false); err != nil {
		return u, err
	}
	var err error
	u.FilesUsed, u.BytesUsed, err = s.repo.Usage(ctx, page)
	return u, err
}
func (s *PageAttachmentService) List(ctx context.Context, by Actor, page, after string, limit int) ([]*domain.PageAttachment, error) {
	if _, err := s.page(ctx, by, page, "", false); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, page, after, limit)
}
func (s *PageAttachmentService) Get(ctx context.Context, by Actor, page, id string) (*domain.PageAttachment, error) {
	if _, err := s.page(ctx, by, page, "", false); err != nil {
		return nil, err
	}
	a, err := s.repo.Get(ctx, page, id)
	if err == nil && !a.ExpiresAt.IsZero() && !s.clock.Now().Before(a.ExpiresAt) {
		return nil, domain.Errf(domain.CodeNotFound, "unassociated page upload expired")
	}
	return a, err
}
func (s *PageAttachmentService) Resolve(ctx context.Context, by Actor, page, name string) (*domain.PageAttachment, error) {
	if _, err := s.page(ctx, by, page, "", false); err != nil {
		return nil, err
	}
	xs, err := s.repo.Legacy(ctx, page, "", name)
	if err != nil {
		return nil, err
	}
	if len(xs) == 0 {
		return nil, domain.Errf(domain.CodeNotFound, "no page attachment for this legacy name")
	}
	return s.Get(ctx, by, page, xs[0].ID)
}
func (s *PageAttachmentService) Open(ctx context.Context, by Actor, page, id string) (*domain.PageAttachment, io.ReadCloser, error) {
	a, err := s.Get(ctx, by, page, id)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.blobs.Open(ctx, a.Blob)
	return a, rc, err
}

type PageAttachmentInput struct{ Page, Collection, Name, ContentType, Key, LegacyName, By string }

func (s *PageAttachmentService) Put(ctx context.Context, by Actor, in PageAttachmentInput, r io.Reader) (*domain.PageAttachment, error) {
	if s.pages.encrypted != nil && s.pages.encrypted(ctx) {
		return nil, domain.Errf(domain.CodeInvalidState, "this workspace has end-to-end encryption on; page files would be kept in plain text")
	}
	if _, err := s.page(ctx, by, in.Page, in.Collection, true); err != nil {
		return nil, err
	}
	if in.Key == "" || len(in.Key) > 200 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "an upload key of 1..200 characters is required")
	}
	name, err := domain.SanitizeFileName(in.Name)
	if err != nil {
		return nil, err
	}
	in.Name = name
	if in.LegacyName != "" {
		n, e := domain.SanitizeFileName(in.LegacyName)
		if e != nil || n != in.LegacyName {
			return nil, domain.Errf(domain.CodeInvalidArgument, "invalid legacy attachment name")
		}
	}
	if s.disk != nil {
		if err = s.disk(0); err != nil {
			return nil, err
		}
	}
	data, err := io.ReadAll(io.LimitReader(r, domain.MaxPageAttachment+1))
	if err != nil {
		return nil, err
	}
	if len(data) > domain.MaxPageAttachment {
		return nil, domain.Errf(domain.CodeInvalidArgument, "page files are limited to 20 MiB")
	}
	if in.ContentType == "" || len(in.ContentType) > 200 || strings.ContainsAny(in.ContentType, "\r\n") {
		in.ContentType = http.DetectContentType(data)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err = s.page(ctx, by, in.Page, in.Collection, true); err != nil {
		return nil, err
	}
	if s.disk != nil {
		if err = s.disk(int64(len(data))); err != nil {
			return nil, err
		}
	}
	a := s.attachment(in, data)
	a, fresh, err := s.repo.Reserve(ctx, a, s.policy.FilesMax, s.policy.BytesMax, false)
	if err != nil || !fresh {
		return a, err
	}
	if err = s.store(ctx, a, data); err == nil {
		err = s.repo.Uploaded(ctx, a.ID)
	}
	if err != nil {
		_ = s.repo.Remove(context.WithoutCancel(ctx), a.Page, a.ID)
		return nil, err
	}
	return s.repo.Get(ctx, a.Page, a.ID)
}
func (s *PageAttachmentService) attachment(in PageAttachmentInput, data []byte) *domain.PageAttachment {
	sum := sha256.Sum256(data)
	now := s.clock.Now()
	return &domain.PageAttachment{ID: s.ids.New("paf"), Page: in.Page, Collection: in.Collection, Name: in.Name, Blob: s.ids.New("pab"), Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), ContentType: in.ContentType, By: in.By, Key: in.Key, LegacyName: in.LegacyName, State: "uploading", CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
}
func (s *PageAttachmentService) store(ctx context.Context, a *domain.PageAttachment, data []byte) error {
	n, sum, err := s.blobs.Put(ctx, a.Blob, bytes.NewReader(data), domain.MaxPageAttachment)
	if err != nil {
		return err
	}
	if n != a.Size || sum != a.SHA256 {
		return domain.Errf(domain.CodeInvalidState, "page attachment content verification failed")
	}
	return nil
}
func (s *PageAttachmentService) Remove(ctx context.Context, by Actor, page, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := s.repo.Get(ctx, page, id)
	if err != nil {
		return err
	}
	p, err := s.page(ctx, by, page, a.Collection, true)
	if err != nil {
		return err
	}
	if a.State == "pending" && a.By != by.Name() && !s.pages.Manages(by, p) {
		return denied("remove another writer's unassociated upload")
	}
	return s.repo.Remove(ctx, page, id)
}

// LegacyOpen is a read-only compatibility view, not a project document. The
// page's current permissions apply even when the caller can see the project.
func (s *PageAttachmentService) LegacyOpen(ctx context.Context, by Actor, project, name string) (*domain.PageAttachment, io.ReadCloser, error) {
	xs, err := s.repo.Legacy(ctx, "", project, name)
	if err != nil {
		return nil, nil, err
	}
	var chosen *domain.PageAttachment
	for _, a := range xs {
		if _, e := s.page(ctx, by, a.Page, "", false); e != nil {
			if domain.IsCode(e, domain.CodeNotFound) || domain.IsCode(e, domain.CodePermissionDenied) {
				continue
			}
			return nil, nil, e
		}
		if chosen != nil && chosen.SHA256 != a.SHA256 {
			return nil, nil, domain.Errf(domain.CodeConflict, "legacy file name is ambiguous; use the page attachment API")
		}
		chosen = a
	}
	if chosen == nil {
		return nil, nil, domain.Errf(domain.CodeNotFound, "project file not found")
	}
	return s.Open(ctx, by, chosen.Page, chosen.ID)
}

type PageAttachmentMigrationResult = domain.PageAttachmentMigrationResult
type PageAttachmentMigrationReport = domain.PageAttachmentMigrationReport

func (s *PageAttachmentService) Migrate(ctx context.Context, by Actor, project string, dry bool, limit int) (*PageAttachmentMigrationReport, error) {
	if !by.Manages() || by.Agent != nil || by.Via != nil {
		return nil, denied("migrate page attachments (workspace owner/admin only)")
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Existing project writes use this same admission lock. Page records are
	// checked again by signature inside the commit transaction.
	s.pages.projects.storageMu.Lock()
	defer s.pages.projects.storageMu.Unlock()
	sources, skipped, err := s.repo.Candidates(ctx, project)
	if err != nil {
		return nil, err
	}
	report := &PageAttachmentMigrationReport{DryRun: dry, Sources: len(sources), Skipped: skipped, Results: []PageAttachmentMigrationResult{}}
	if len(sources) > limit {
		report.Remaining = len(sources) - limit
		sources = sources[:limit]
	}
	for _, source := range sources {
		result := PageAttachmentMigrationResult{Project: source.ProjectName, Name: source.Doc.Name, Files: len(source.Targets), Size: source.Doc.Size}
		if !source.Doc.IsBlob() {
			result.Size = int64(len(source.Doc.Body))
		}
		if dry {
			report.Results = append(report.Results, result)
			continue
		}
		data, e := s.migrationData(ctx, source.Doc)
		if e == nil {
			sum := sha256.Sum256(data)
			result.SHA256 = hex.EncodeToString(sum[:])
			e = s.migrateSource(ctx, source, data)
		}
		if e != nil {
			result.Error = e.Error()
		} else {
			report.Moved++
		}
		report.Results = append(report.Results, result)
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
	}
	return report, nil
}
func (s *PageAttachmentService) migrationData(ctx context.Context, d domain.ProjectDoc) ([]byte, error) {
	if !d.IsBlob() {
		return []byte(d.Body), nil
	}
	rc, err := s.blobs.Open(ctx, d.Blob)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, domain.MaxPageAttachment+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) != d.Size || len(b) > domain.MaxPageAttachment {
		return nil, domain.Errf(domain.CodeInvalidState, "source file size does not match its metadata")
	}
	return b, nil
}
func (s *PageAttachmentService) migrateSource(ctx context.Context, source domain.PageAttachmentMigration, data []byte) error {
	files := []*domain.PageAttachment{}
	committed := false
	defer func() {
		if !committed {
			for _, a := range files {
				_ = s.repo.Remove(context.WithoutCancel(ctx), a.Page, a.ID)
			}
		}
	}()
	for _, target := range source.Targets {
		if s.disk != nil {
			if err := s.disk(int64(len(data))); err != nil {
				return err
			}
		}
		key := sha256.Sum256([]byte("migration:" + source.ProjectID + ":" + source.Signature + ":" + target.Page))
		ct := source.Doc.ContentType
		if ct == "" {
			ct = http.DetectContentType(data)
		}
		a := s.attachment(PageAttachmentInput{Page: target.Page, Collection: target.Collection, Name: source.Doc.Name, ContentType: ct, Key: hex.EncodeToString(key[:]), By: source.Doc.UpdatedBy, LegacyName: source.Doc.Name}, data)
		a, fresh, err := s.repo.Reserve(ctx, a, s.policy.FilesMax, s.policy.BytesMax, true)
		if err != nil {
			return err
		}
		files = append(files, a)
		if fresh {
			if err = s.store(ctx, a, data); err != nil {
				return err
			}
		}
		// Verify the independent stored copy before releasing project ownership.
		rc, err := s.blobs.Open(ctx, a.Blob)
		if err != nil {
			return err
		}
		h := sha256.New()
		n, readErr := io.Copy(h, io.LimitReader(rc, domain.MaxPageAttachment+1))
		closeErr := rc.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if n != a.Size || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
			return domain.Errf(domain.CodeInvalidState, "migrated content verification failed")
		}

	}
	if err := s.repo.CommitMigration(ctx, source, files); err != nil {
		return err
	}
	committed = true
	return nil
}
func (s *PageAttachmentService) Cleanup(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.repo.Sweep(ctx, s.clock.Now()); err != nil {
		return err
	}
	ids, err := s.repo.Garbage(ctx, 1000)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.blobs.Delete(ctx, id); err != nil {
			return err
		}
		if err = s.repo.ForgetGarbage(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
func (s *PageAttachmentService) Run(ctx context.Context, log *slog.Logger) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		r, err := s.Migrate(ctx, Actor{Admin: true}, "", false, 25)
		if log != nil {
			if err != nil && ctx.Err() == nil {
				log.Warn("page attachment migration failed", "error", err)
			} else if r != nil {
				failed := 0
				for _, result := range r.Results {
					if result.Error != "" {
						failed++
					}
				}
				if failed > 0 || len(r.Skipped) > 0 {
					log.Warn("page attachment migration needs attention", "failed", failed, "unmatched", len(r.Skipped), "suggestion", "inspect with myco page file migrate --dry-run")
				}
				if r.Moved > 0 {
					log.Info("page attachments migrated", "documents", r.Moved, "remaining", r.Remaining)
				}
			}
		}
		if err = s.Cleanup(ctx); err != nil && log != nil && ctx.Err() == nil {
			log.Warn("page attachment cleanup failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
