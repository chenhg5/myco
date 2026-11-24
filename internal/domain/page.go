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
	Owner  string
	Latest int
	// Live: not files but a server on its publisher's computer (a dev
	// server on Port there), forwarded while `myco page serve` runs.
	Live bool
	Port int
	// Template is the template it was made from ("" for none), at
	// TemplateVersion: its data has that template's structure.
	Template        string
	TemplateVersion string
	// Manifest is that template's myco.json, kept with the page so that it
	// stands on its own (its data's structure, its permissions).
	Manifest string
	// Editors is who may change its records and shared state (v0.22.2):
	// "" everyone who sees it, PageEditorsManagers only those who manage it.
	Editors   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Who may change a page's records and shared state.
const (
	PageEditorsEveryone = ""
	PageEditorsManagers = "managers"
)

// ParsePageEditors checks who may change a page ("everyone" or "" for all who
// see it, "managers").
func ParsePageEditors(s string) (string, error) {
	switch s {
	case "", "everyone":
		return PageEditorsEveryone, nil
	case PageEditorsManagers:
		return PageEditorsManagers, nil
	}
	return "", Errf(CodeInvalidArgument, "editors %q: everyone or managers", s)
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

// PageState is a page's shared state (the bridge's myco.update): JSON that
// the people looking at the page and agents read and write, with a version
// so that nobody overwrites what they did not see.
type PageState struct {
	Page      string
	State     string // JSON ("" before anyone wrote)
	Version   int
	UpdatedBy string
	UpdatedAt time.Time
}

// MaxPageState bounds a page's shared state.
const MaxPageState = 256 << 10

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
	// State returns a page's shared state (version 0 when never written).
	State(ctx context.Context, page string) (*PageState, error)
	// PutState writes it when its version is still ifVersion; else
	// CodeConflict.
	PutState(ctx context.Context, s *PageState, ifVersion int) error
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
