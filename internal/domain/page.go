package domain

import (
	"context"
	"path"
	"strings"
	"time"
	"unicode/utf8"
)

// Page is a web page an agent (or a person) published to the hub: a folder
// of static files (HTML, scripts, styles, pictures) people open in the App.
// Each publish is a version; a version is kept as one zip in the blob store
// and its files are served from it.
type Page struct {
	ID   string
	Name string
	// Project is the name of the project it was published to ("" for none):
	// whoever sees the project sees the page.
	Project string
	// Publisher is who published it first: an agent's name, or a person's
	// (console) name.
	Publisher string
	// Owner is the member id of the person it belongs to (the publishing
	// agent's owner, or the person): they see and manage it.
	Owner     string
	Latest    int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// PageVersion is one publish of a page.
type PageVersion struct {
	Page   string
	N      int
	Blob   string
	SHA256 string
	Size   int64
	// Entry is the file a page opens on (index.html by default).
	Entry string
	Files int
	By    string
	At    time.Time
}

// PageRepository stores pages and their versions.
type PageRepository interface {
	Create(ctx context.Context, p *Page) error
	Save(ctx context.Context, p *Page) error
	Get(ctx context.Context, id string) (*Page, error)
	List(ctx context.Context) ([]*Page, error)
	Delete(ctx context.Context, id string) error
	AddVersion(ctx context.Context, v *PageVersion) error
	// Versions lists a page's versions, newest first.
	Versions(ctx context.Context, page string) ([]*PageVersion, error)
	DeleteVersion(ctx context.Context, page string, n int) error
}

const (
	// MaxPageSize bounds one version (the zip as uploaded).
	MaxPageSize = 50 << 20
	// MaxPageFile bounds one file in it, unpacked.
	MaxPageFile = 20 << 20
	// MaxPageFiles bounds how many files one version has.
	MaxPageFiles = 2000
	// MaxPages bounds how many pages a workspace keeps.
	MaxPages = 200
	// PageVersionsKept: older versions are let go.
	PageVersionsKept = 10
	maxPageName      = 80
)

// ParsePageName checks a page's name (shown to people).
func ParsePageName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > maxPageName || strings.ContainsAny(s, "\r\n\t") {
		return "", Errf(CodeInvalidArgument, "invalid page name %q", s).
			WithSuggestion("use a short name people will recognise, such as 周报 · 第 46 周 (up to 80 characters, one line)")
	}
	return s, nil
}

// CleanPagePath checks the path of a file in a page: relative, no "..",
// no hidden parts, forward slashes. It returns the cleaned path.
func CleanPagePath(p string) (string, error) {
	bad := p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") || strings.ContainsRune(p, 0) || !utf8.ValidString(p)
	if !bad {
		c := path.Clean(p)
		if c == "." || c != strings.TrimSuffix(p, "/") {
			bad = true
		}
		for _, part := range strings.Split(c, "/") {
			if part == ".." || part == "" || strings.HasPrefix(part, ".") {
				bad = true
			}
		}
		if !bad {
			return c, nil
		}
	}
	return "", Errf(CodeInvalidArgument, "invalid path in page: %q", p).
		WithSuggestion("files of a page are relative paths without .. or hidden folders")
}
