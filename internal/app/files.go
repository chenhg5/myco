package app

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// FilePolicy bounds uploads.
type FilePolicy struct {
	MaxSize int64         // bytes per file
	TTL     time.Duration // retention (0 = keep forever)
}

// DefaultFilePolicy: 50 MB per file, kept 7 days.
func DefaultFilePolicy() FilePolicy { return FilePolicy{MaxSize: 50 << 20, TTL: 7 * 24 * time.Hour} }

// FileService uploads and serves attachment files.
type FileService struct {
	files  domain.FileRepository
	blobs  domain.BlobStore
	ids    IDGenerator
	clock  Clock
	policy FilePolicy
	log    *slog.Logger
}

// NewFileService wires the file use cases.
func NewFileService(files domain.FileRepository, blobs domain.BlobStore, ids IDGenerator, clock Clock, p FilePolicy, log *slog.Logger) *FileService {
	return &FileService{files: files, blobs: blobs, ids: ids, clock: clock, policy: p, log: log}
}

// Policy returns the upload limits.
func (s *FileService) Policy() FilePolicy { return s.policy }

// Upload stores r as a new file owned by agent.
func (s *FileService) Upload(ctx context.Context, agent *domain.Agent, name, contentType string, r io.Reader) (*domain.File, error) {
	name, err := domain.SanitizeFileName(name)
	if err != nil {
		return nil, err
	}
	if len(contentType) > 200 || strings.ContainsAny(contentType, "\r\n") {
		contentType = ""
	}
	id := s.ids.New("f")
	size, sum, err := s.blobs.Put(ctx, id, r, s.policy.MaxSize)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	f := &domain.File{ID: id, Name: name, Size: size, SHA256: sum, ContentType: contentType, Owner: agent.Name, CreatedAt: now}
	if s.policy.TTL > 0 {
		f.ExpiresAt = now.Add(s.policy.TTL)
	}
	if err := s.files.Create(ctx, f); err != nil {
		_ = s.blobs.Delete(ctx, id)
		return nil, err
	}
	return f, nil
}

func (s *FileService) authorize(ctx context.Context, agent *domain.Agent, id string) (*domain.File, error) {
	f, err := s.files.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	ok, err := s.files.CanAccess(ctx, id, agent.Name)
	if err != nil {
		return nil, err
	}
	if !ok {
		// Do not reveal that the file exists.
		return nil, domain.Errf(domain.CodeNotFound, "file %s not found", id)
	}
	if f.Expired(s.clock.Now()) {
		return nil, domain.Errf(domain.CodeNotFound, "file %s expired at %s", id, f.ExpiresAt.Format(time.RFC3339)).
			WithSuggestion("ask the sender to send it again")
	}
	return f, nil
}

// Meta returns file metadata the agent may access.
func (s *FileService) Meta(ctx context.Context, agent *domain.Agent, id string) (*domain.File, error) {
	return s.authorize(ctx, agent, id)
}

// Open returns the file and its content for an authorized agent.
func (s *FileService) Open(ctx context.Context, agent *domain.Agent, id string) (*domain.File, io.ReadCloser, error) {
	f, err := s.authorize(ctx, agent, id)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.blobs.Open(ctx, id)
	return f, rc, err
}

// OpenScoped returns a file for someone reading from the console: any file
// when the whole hub is in view, else one shared with one of their names
// (their console sender, the nodes they own), as the messages they may read.
func (s *FileService) OpenScoped(ctx context.Context, sc Scope, id string) (*domain.File, io.ReadCloser, error) {
	f, err := s.files.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	ok := sc.All
	for _, n := range sc.Names {
		if ok {
			break
		}
		if ok, err = s.files.CanAccess(ctx, id, n); err != nil {
			return nil, nil, err
		}
	}
	if !ok {
		// Do not reveal that the file exists.
		return nil, nil, domain.Errf(domain.CodeNotFound, "file %s not found", id)
	}
	if f.Expired(s.clock.Now()) {
		return nil, nil, domain.Errf(domain.CodeNotFound, "file %s expired at %s", id, f.ExpiresAt.Format(time.RFC3339)).
			WithSuggestion("ask the sender to send it again")
	}
	rc, err := s.blobs.Open(ctx, id)
	return f, rc, err
}

// Sweep deletes expired files; it returns how many were removed.
func (s *FileService) Sweep(ctx context.Context) (int, error) {
	expired, err := s.files.ListExpired(ctx, s.clock.Now(), 500)
	if err != nil {
		return 0, err
	}
	for _, f := range expired {
		if err := s.blobs.Delete(ctx, f.ID); err != nil {
			return 0, err
		}
		if err := s.files.Delete(ctx, f.ID); err != nil {
			return 0, err
		}
	}
	return len(expired), nil
}

// Run sweeps expired files every interval until ctx ends.
func (s *FileService) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := s.Sweep(ctx); err != nil {
				s.log.Warn("file sweep failed", "err", err)
			} else if n > 0 {
				s.log.Info("expired files removed", "count", n)
			}
		}
	}
}

// attachments resolves file ids the sender may share into snapshots.
func resolveAttachments(ctx context.Context, files domain.FileRepository, clock Clock, sender domain.AgentName, ids []string) ([]domain.Attachment, error) {
	var out []domain.Attachment
	var seen []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || slices.Contains(seen, id) {
			continue
		}
		seen = append(seen, id)
		if files == nil {
			return nil, domain.Errf(domain.CodeInvalidArgument, "this hub does not store files")
		}
		f, err := files.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		ok, err := files.CanAccess(ctx, id, sender)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, domain.Errf(domain.CodeNotFound, "file %s not found", id)
		}
		if f.Expired(clock.Now()) {
			return nil, domain.Errf(domain.CodeNotFound, "file %s expired", id).WithSuggestion("upload it again")
		}
		out = append(out, domain.AttachmentOf(f))
	}
	if len(out) > domain.MaxAttachments {
		return nil, domain.Errf(domain.CodeInvalidArgument, "too many attachments (%d, max %d)", len(out), domain.MaxAttachments)
	}
	return out, nil
}

func grantAttachments(ctx context.Context, files domain.FileRepository, atts []domain.Attachment, to domain.AgentName) error {
	for _, a := range atts {
		if err := files.Grant(ctx, a.FileID, to); err != nil {
			return err
		}
	}
	return nil
}
