package app

import (
	"context"

	"github.com/myco-ai/myco/internal/domain"
)

const MaxProjectFiles = 10000
const MaxProjectBytes int64 = 20 << 30

// ProjectStoragePolicy bounds non-context files and total logical bytes.
// Text documents kept inline have their own fixed 100-document limit.
type ProjectStoragePolicy struct {
	FilesMax           int
	BytesMax           int64
	DiskMinFreeBytes   int64
	DiskMinFreePercent int
}

func DefaultProjectStoragePolicy() ProjectStoragePolicy {
	return ProjectStoragePolicy{FilesMax: 1000, BytesMax: 2 << 30, DiskMinFreeBytes: 2 << 30, DiskMinFreePercent: 5}
}
func (p ProjectStoragePolicy) Validate() error {
	if p.FilesMax < 1 || p.FilesMax > MaxProjectFiles || p.BytesMax < 1 || p.BytesMax > MaxProjectBytes || p.DiskMinFreeBytes < 0 || p.DiskMinFreePercent < 0 || p.DiskMinFreePercent > 99 {
		return domain.Errf(domain.CodeInvalidArgument, "invalid project storage policy: files must be 1..10000, bytes 1..21474836480, disk reserve nonnegative and percent 0..99")
	}
	return nil
}

// SetStoragePolicy is wired before serving requests. The injected disk check
// checks all storage volumes before reading an upload, and again before writing.
func (s *ProjectService) SetStoragePolicy(p ProjectStoragePolicy, check func(int64) error) error {
	if err := p.Validate(); err != nil {
		return err
	}
	s.storagePolicy = p
	s.checkDisk = check
	return nil
}

type ProjectStorage struct {
	FilesUsed, FilesMax, DocsUsed int
	BytesUsed, BytesMax           int64
}

func (s *ProjectService) Storage(v *ProjectView) ProjectStorage {
	u := ProjectStorage{FilesMax: s.storagePolicy.FilesMax, BytesMax: s.storagePolicy.BytesMax}
	if v.Project.FilesMax > 0 {
		u.FilesMax = v.Project.FilesMax
	}
	if v.Project.BytesMax > 0 {
		u.BytesMax = v.Project.BytesMax
	}
	for _, d := range v.Docs {
		if d.IsBlob() {
			u.FilesUsed++
		} else {
			u.DocsUsed++
		}
		if d.IsBlob() {
			u.BytesUsed += d.Size
		} else {
			u.BytesUsed += int64(len(d.Body))
		}
	}
	return u
}
func (s *ProjectService) checkStorage(v *ProjectView, next *domain.ProjectDoc) error {
	u := s.Storage(v)
	adjust := func(d *domain.ProjectDoc, sign int) {
		if d.IsBlob() {
			u.FilesUsed += sign
			u.BytesUsed += int64(sign) * d.Size
		} else {
			u.DocsUsed += sign
			u.BytesUsed += int64(sign) * int64(len(d.Body))
		}
	}
	if old := v.doc(next.Name); old != nil {
		adjust(old, -1)
	}
	adjust(next, 1)
	if u.DocsUsed > domain.MaxProjectDocs {
		return domain.Errf(domain.CodeInvalidArgument, "project has %d/%d context documents", u.DocsUsed, domain.MaxProjectDocs).WithSuggestion("merge or remove context documents")
	}
	if u.FilesUsed > u.FilesMax {
		return domain.Errf(domain.CodeInvalidArgument, "project file quota exceeded: %d/%d files", u.FilesUsed, u.FilesMax).WithSuggestion("remove files or ask a workspace owner/admin to raise the project storage limit")
	}
	if u.BytesUsed > u.BytesMax {
		return domain.Errf(domain.CodeInvalidArgument, "project storage quota exceeded: %d/%d bytes", u.BytesUsed, u.BytesMax).WithSuggestion("remove files or ask a workspace owner/admin to raise the project storage limit")
	}
	if s.checkDisk != nil {
		return s.checkDisk(next.Size)
	}
	return nil
}

// SetStorageLimits is restricted to workspace managers, not project owners or agents.
// Zero restores inheritance. Existing over-quota files stay readable and removable.
func (s *ProjectService) SetStorageLimits(ctx context.Context, by Actor, name string, files int, bytes int64) (*ProjectView, error) {
	if !by.Manages() || by.Agent != nil || by.Via != nil {
		return nil, denied("change project storage limits (workspace owner or admin only)")
	}
	if files < 0 || files > MaxProjectFiles || bytes < 0 || bytes > MaxProjectBytes {
		return nil, domain.Errf(domain.CodeInvalidArgument, "project limits must be 0..10000 files and 0..21474836480 bytes (0 inherits Hub defaults)")
	}
	s.storageMu.Lock()
	defer s.storageMu.Unlock()
	v, err := s.Get(ctx, by, name)
	if err != nil {
		return nil, err
	}
	v.Project.FilesMax = files
	v.Project.BytesMax = bytes
	v.Project.UpdatedAt = s.clock.Now()
	if err = s.projects.SaveStorage(ctx, v.Project); err != nil {
		return nil, err
	}
	return v, nil
}
