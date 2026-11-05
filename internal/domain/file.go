package domain

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// File is an uploaded blob that messages can reference as attachments.
// Access is limited to the uploader and agents that received it in a message.
type File struct {
	ID          string
	Name        string
	Size        int64
	SHA256      string
	ContentType string
	Owner       AgentName
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

// Expired reports whether the file's retention has passed.
func (f *File) Expired(now time.Time) bool {
	return !f.ExpiresAt.IsZero() && now.After(f.ExpiresAt)
}

// Attachment is the snapshot of a file stored on a message.
type Attachment struct {
	FileID      string `json:"file_id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type,omitempty"`
}

// AttachmentOf snapshots f.
func AttachmentOf(f *File) Attachment {
	return Attachment{FileID: f.ID, Name: f.Name, Size: f.Size, SHA256: f.SHA256, ContentType: f.ContentType}
}

// MaxAttachments bounds attachments per message.
const MaxAttachments = 20

// SanitizeFileName keeps a safe basename: no directories, no control
// characters, no leading dots, at most 200 bytes.
func SanitizeFileName(name string) (string, error) {
	name = filepath.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '/' {
			return -1
		}
		return r
	}, name)
	name = strings.TrimLeft(name, ".")
	if name == "" || name == "." {
		return "", Errf(CodeInvalidArgument, "invalid file name").WithSuggestion("pass a regular file name such as report.md")
	}
	if len(name) > 200 {
		ext := filepath.Ext(name)
		if len(ext) > 20 {
			ext = ""
		}
		name = name[:200-len(ext)] + ext
	}
	return name, nil
}

// FileRepository persists file metadata and access grants.
type FileRepository interface {
	Create(ctx context.Context, f *File) error
	Get(ctx context.Context, id string) (*File, error)
	// Grant lets agent download file id (idempotent).
	Grant(ctx context.Context, id string, agent AgentName) error
	CanAccess(ctx context.Context, id string, agent AgentName) (bool, error)
	ListExpired(ctx context.Context, now time.Time, limit int) ([]*File, error)
	Delete(ctx context.Context, id string) error
}

// BlobStore holds file contents (local disk, S3, ...).
type BlobStore interface {
	// Put stores r under id, reading at most max bytes, and returns the
	// size and hex SHA-256. Exceeding max returns CodeInvalidArgument.
	Put(ctx context.Context, id string, r io.Reader, max int64) (size int64, sha256 string, err error)
	Open(ctx context.Context, id string) (io.ReadCloser, error)
	Delete(ctx context.Context, id string) error
}
