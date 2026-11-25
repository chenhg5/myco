package domain

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

const MaxPageAttachment = 20 << 20

// PageAttachment is owned by a page, never by a project or a message grant.
type PageAttachment struct {
	ID          string    `json:"file_id"`
	Page        string    `json:"page"`
	Collection  string    `json:"collection"`
	Name        string    `json:"name"`
	Blob        string    `json:"-"`
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256"`
	ContentType string    `json:"content_type"`
	By          string    `json:"by"`
	State       string    `json:"state"`
	Key         string    `json:"-"`
	LegacyName  string    `json:"legacy_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
}

type PageAttachmentUsage struct {
	FilesUsed    int   `json:"files_used"`
	FilesMax     int   `json:"files_max"`
	BytesUsed    int64 `json:"bytes_used"`
	BytesMax     int64 `json:"bytes_max"`
	MaxFileBytes int64 `json:"max_file_bytes"`
}

type PageAttachmentReference struct {
	Field, Value string
	Legacy       bool
}

// PageAttachmentReferences recognizes declared file fields and the old text
// field named file. It does not grant access to arbitrary JSON/string IDs.
func PageAttachmentReferences(manifest, collection, data string) []PageAttachmentReference {
	var obj map[string]any
	if json.Unmarshal([]byte(data), &obj) != nil {
		return nil
	}
	var fields map[string]string
	if m, err := ParseManifest([]byte(manifest)); err == nil {
		fields = m.Data[collection].Fields
	}
	out := []PageAttachmentReference{}
	for field, value := range obj {
		if fields[field] == "file" {
			if v, ok := value.(map[string]any); ok {
				if id, ok := v["file_id"].(string); ok && id != "" {
					out = append(out, PageAttachmentReference{Field: field, Value: id})
				}
			}
		} else if field == "file" && (fields[field] == "text" || manifest == "") {
			if name, ok := value.(string); ok && name != "" {
				out = append(out, PageAttachmentReference{Field: field, Value: name, Legacy: true})
			}
		}
	}
	return out
}

func IsPageAttachment(id string) bool { return strings.HasPrefix(id, "paf_") }

type PageAttachmentTarget struct{ Page, Collection string }
type PageAttachmentMigration struct {
	ProjectID, ProjectName string
	Doc                    ProjectDoc
	Signature              string
	Targets                []PageAttachmentTarget
}
type PageAttachmentMigrationIssue struct {
	Page       string `json:"page"`
	Collection string `json:"collection"`
	Record     string `json:"record"`
	Field      string `json:"field"`
	Name       string `json:"name"`
	Reason     string `json:"reason"`
}

// Admission, record references, migration and deletion use SQLite transactions.
type PageAttachmentRepository interface {
	Reserve(context.Context, *PageAttachment, int, int64, bool) (*PageAttachment, bool, error)
	Uploaded(context.Context, string) error
	Get(context.Context, string, string) (*PageAttachment, error)
	List(context.Context, string, string, int) ([]*PageAttachment, error)
	Usage(context.Context, string) (int, int64, error)
	Remove(context.Context, string, string) error
	Legacy(context.Context, string, string, string) ([]*PageAttachment, error)
	Candidates(context.Context, string) ([]PageAttachmentMigration, []PageAttachmentMigrationIssue, error)
	CommitMigration(context.Context, PageAttachmentMigration, []*PageAttachment) error
	Sweep(context.Context, time.Time) error
	Garbage(context.Context, int) ([]string, error)
	ForgetGarbage(context.Context, string) error
}

type PageAttachmentMigrationResult struct {
	Project string `json:"project"`
	Name    string `json:"name"`
	Files   int    `json:"files"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256,omitempty"`
	Error   string `json:"error,omitempty"`
}
type PageAttachmentMigrationReport struct {
	DryRun    bool                            `json:"dry_run"`
	Sources   int                             `json:"sources"`
	Moved     int                             `json:"moved"`
	Remaining int                             `json:"remaining"`
	Results   []PageAttachmentMigrationResult `json:"results"`
	Skipped   []PageAttachmentMigrationIssue  `json:"skipped"`
}
