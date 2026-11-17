package app

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"mime"
	"os"
	"path"
	"strings"

	"github.com/chenhg5/myco/internal/domain"
)

// PageService: web pages published to the hub (see docs/design/browser.md).
//
// A page is seen by its owner (the publishing agent's owner, or the person
// who published it), by agents of that owner, by the hub's owner and admins,
// and, when it was published to a project, by whoever sees the project.
// Its owner, its owner's agents and the hub's managers publish new versions
// and delete it.
type PageService struct {
	pages    domain.PageRepository
	blobs    domain.BlobStore
	projects *ProjectService
	ids      IDGenerator
	clock    Clock
	// encrypted reports whether the hub keeps end-to-end encrypted content
	// (pages would be in plain text: refused).
	encrypted func(context.Context) bool
}

// NewPageService wires the page use cases.
func NewPageService(pages domain.PageRepository, blobs domain.BlobStore, projects *ProjectService, ids IDGenerator, clock Clock) *PageService {
	return &PageService{pages: pages, blobs: blobs, projects: projects, ids: ids, clock: clock}
}

// SetEncrypted refuses pages on a hub with end-to-end encryption on.
func (s *PageService) SetEncrypted(f func(context.Context) bool) { s.encrypted = f }

// PageView is a page and its versions (newest first).
type PageView struct {
	Page     *domain.Page
	Versions []*domain.PageVersion
}

// PublishInput says what to publish: a new page (Name, Project) or a new
// version of one (Page).
type PublishInput struct {
	Page    string
	Name    string
	Project string
	Entry   string
	By      string // who publishes, for display (agent or person name)
}

func (s *PageService) sees(ctx context.Context, by Actor, p *domain.Page) bool {
	switch {
	case by.Manages():
		return true
	case by.Agent != nil && (string(by.Agent.Name) == p.Publisher || (p.Owner != "" && by.Agent.Owner == p.Owner)):
		return true
	case by.Member != nil && p.Owner != "" && by.Member.ID == p.Owner:
		return true
	}
	return p.Project != "" && s.projects != nil && s.projects.Sees(ctx, by, p.Project)
}

func (s *PageService) manages(by Actor, p *domain.Page) bool {
	switch {
	case by.Manages():
		return true
	case by.Agent != nil:
		return string(by.Agent.Name) == p.Publisher || (p.Owner != "" && by.Agent.Owner == p.Owner)
	case by.Member != nil:
		return p.Owner != "" && by.Member.ID == p.Owner
	}
	return false
}

func pageHidden(id string) error {
	return domain.Errf(domain.CodeNotFound, "page %s not found", id).WithSuggestion("list pages with: myco page list")
}

// Get returns a page for whoever may see it.
func (s *PageService) Get(ctx context.Context, by Actor, id string) (*PageView, error) {
	p, err := s.pages.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !s.sees(ctx, by, p) {
		return nil, pageHidden(id)
	}
	vs, err := s.pages.Versions(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return &PageView{Page: p, Versions: vs}, nil
}

// List returns the pages by may see (of one project, if given), newest first.
func (s *PageService) List(ctx context.Context, by Actor, project string) ([]*PageView, error) {
	all, err := s.pages.List(ctx)
	if err != nil {
		return nil, err
	}
	out := []*PageView{}
	for _, p := range all {
		if project != "" && p.Project != project {
			continue
		}
		if !s.sees(ctx, by, p) {
			continue
		}
		vs, err := s.pages.Versions(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, &PageView{Page: p, Versions: vs})
	}
	return out, nil
}

// Publish stores a zip of a page's files as a new page or a new version.
func (s *PageService) Publish(ctx context.Context, by Actor, in PublishInput, r io.Reader) (*PageView, error) {
	if s.blobs == nil {
		return nil, domain.Errf(domain.CodeNotFound, "pages are not enabled on this hub")
	}
	if s.encrypted != nil && s.encrypted(ctx) {
		return nil, domain.Errf(domain.CodeInvalidState, "this workspace has end-to-end encryption on; a page would be kept in plain text").
			WithSuggestion("send the files as attachments instead (they are encrypted)")
	}
	var (
		p   *domain.Page
		err error
	)
	now := s.clock.Now()
	if in.Page != "" {
		if p, err = s.pages.Get(ctx, in.Page); err != nil {
			return nil, err
		}
		if !s.sees(ctx, by, p) {
			return nil, pageHidden(in.Page)
		}
		if !s.manages(by, p) {
			return nil, domain.Errf(domain.CodePermissionDenied, "you may not publish a new version of page %s", p.ID).
				WithSuggestion("its owner, their agents and the workspace's admins may; publish a page of your own instead")
		}
		if in.Name != "" {
			if p.Name, err = domain.ParsePageName(in.Name); err != nil {
				return nil, err
			}
		}
	} else {
		name, err := domain.ParsePageName(in.Name)
		if err != nil {
			return nil, err
		}
		project := ""
		if in.Project != "" {
			if s.projects == nil {
				return nil, domain.Errf(domain.CodeNotFound, "this hub has no projects")
			}
			if project, err = s.projects.MayAddFiles(ctx, by, in.Project); err != nil {
				return nil, err
			}
		}
		all, err := s.pages.List(ctx)
		if err != nil {
			return nil, err
		}
		if len(all) >= domain.MaxPages {
			return nil, domain.Errf(domain.CodeInvalidArgument, "this workspace already has %d pages", domain.MaxPages).
				WithSuggestion("delete pages no longer needed (myco page rm), or publish a new version of one")
		}
		owner := by.MemberID()
		if by.Agent != nil {
			owner = by.Agent.Owner
		}
		p = &domain.Page{ID: s.ids.New("pg"), Name: name, Project: project, Publisher: in.By, Owner: owner, CreatedAt: now}
	}

	blob := s.ids.New("pgv")
	size, sum, err := s.blobs.Put(ctx, blob, r, domain.MaxPageSize)
	if err != nil {
		if domain.IsCode(err, domain.CodeInvalidArgument) {
			return nil, domain.Errf(domain.CodeInvalidArgument, "the page is larger than %d MB", domain.MaxPageSize>>20).
				WithSuggestion("leave out what it does not need (node_modules, sources, large videos)")
		}
		return nil, err
	}
	drop := func() { _ = s.blobs.Delete(context.WithoutCancel(ctx), blob) }
	files, entry, err := s.check(ctx, blob, in.Entry)
	if err != nil {
		drop()
		return nil, err
	}
	p.Latest++
	p.UpdatedAt = now
	v := &domain.PageVersion{Page: p.ID, N: p.Latest, Blob: blob, SHA256: sum, Size: size, Entry: entry, Files: files, By: in.By, At: now}
	if p.Latest == 1 {
		err = s.pages.Create(ctx, p)
	} else {
		err = s.pages.Save(ctx, p)
	}
	if err == nil {
		err = s.pages.AddVersion(ctx, v)
	}
	if err != nil {
		drop()
		return nil, err
	}
	vs, err := s.pages.Versions(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	// Older versions are let go.
	for len(vs) > domain.PageVersionsKept {
		old := vs[len(vs)-1]
		if err := s.pages.DeleteVersion(ctx, p.ID, old.N); err == nil {
			_ = s.blobs.Delete(ctx, old.Blob)
		}
		vs = vs[:len(vs)-1]
	}
	return &PageView{Page: p, Versions: vs}, nil
}

// check opens a stored zip and checks its files: how many, how large, their
// paths, and the entry file (given, or index.html, or the only html file).
func (s *PageService) check(ctx context.Context, blob, entry string) (int, string, error) {
	z, closer, err := s.openZip(ctx, blob)
	if err != nil {
		return 0, "", domain.Errf(domain.CodeInvalidArgument, "the upload is not a zip of the page's files").
			WithSuggestion("publish with: myco page publish <folder>")
	}
	defer closer()
	var htmls []string
	have := map[string]bool{}
	n := 0
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name, err := domain.CleanPagePath(f.Name)
		if err != nil {
			return 0, "", err
		}
		if f.UncompressedSize64 > domain.MaxPageFile {
			return 0, "", domain.Errf(domain.CodeInvalidArgument, "%s is larger than %d MB", name, domain.MaxPageFile>>20).
				WithSuggestion("keep large files (videos, data) out of the page, or make them smaller")
		}
		n++
		if n > domain.MaxPageFiles {
			return 0, "", domain.Errf(domain.CodeInvalidArgument, "a page has at most %d files", domain.MaxPageFiles).
				WithSuggestion("publish the built page (e.g. dist/), not its sources")
		}
		have[name] = true
		if ext := strings.ToLower(path.Ext(name)); ext == ".html" || ext == ".htm" {
			htmls = append(htmls, name)
		}
	}
	if n == 0 {
		return 0, "", domain.Errf(domain.CodeInvalidArgument, "the page has no files")
	}
	if entry != "" {
		e, err := domain.CleanPagePath(entry)
		if err != nil {
			return 0, "", err
		}
		if !have[e] {
			return 0, "", domain.Errf(domain.CodeInvalidArgument, "the page has no %s", e).
				WithSuggestion("its html files: %s", strings.Join(htmls, ", "))
		}
		return n, e, nil
	}
	if have["index.html"] {
		return n, "index.html", nil
	}
	if len(htmls) == 1 {
		return n, htmls[0], nil
	}
	if len(htmls) == 0 {
		return 0, "", domain.Errf(domain.CodeInvalidArgument, "the page has no html file").
			WithSuggestion("a page opens on an html file, index.html by default")
	}
	return 0, "", domain.Errf(domain.CodeInvalidArgument, "the page has no index.html").
		WithSuggestion("say which file it opens on with --entry; its html files: %s", strings.Join(htmls, ", "))
}

// openZip reads a stored zip (straight from the file when the store keeps
// files, else into memory).
func (s *PageService) openZip(ctx context.Context, blob string) (*zip.Reader, func(), error) {
	rc, err := s.blobs.Open(ctx, blob)
	if err != nil {
		return nil, func() {}, err
	}
	if f, ok := rc.(*os.File); ok {
		if st, err := f.Stat(); err == nil {
			z, err := zip.NewReader(f, st.Size())
			if err != nil {
				f.Close()
				return nil, func() {}, err
			}
			return z, func() { f.Close() }, nil
		}
	}
	data, err := io.ReadAll(io.LimitReader(rc, domain.MaxPageSize+1))
	rc.Close()
	if err != nil {
		return nil, func() {}, err
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	return z, func() {}, err
}

// PageFile is one file of a version, open for reading.
type PageFile struct {
	Version     *domain.PageVersion
	Path        string
	ContentType string
	Size        int64
	io.ReadCloser
}

// Open returns a file of a page's version (n: 0 for the latest; path: ""
// for its entry file), for whoever may see the page.
func (s *PageService) Open(ctx context.Context, by Actor, id string, n int, file string) (*PageFile, error) {
	pv, err := s.Get(ctx, by, id)
	if err != nil {
		return nil, err
	}
	var v *domain.PageVersion
	for _, x := range pv.Versions {
		if n == 0 || x.N == n {
			v = x // newest first: the latest is the first
			break
		}
	}
	if v == nil {
		return nil, domain.Errf(domain.CodeNotFound, "page %s has no version %d", id, n)
	}
	if file == "" {
		file = v.Entry
	}
	name, err := domain.CleanPagePath(file)
	if err != nil {
		return nil, domain.Errf(domain.CodeNotFound, "no %s in page %s", file, id)
	}
	z, closer, err := s.openZip(ctx, v.Blob)
	if err != nil {
		return nil, err
	}
	for _, f := range z.File {
		if f.FileInfo().IsDir() || path.Clean(f.Name) != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			closer()
			return nil, err
		}
		ct := mime.TypeByExtension(strings.ToLower(path.Ext(name)))
		if ct == "" {
			ct = "application/octet-stream"
		}
		return &PageFile{Version: v, Path: name, ContentType: ct, Size: int64(f.UncompressedSize64),
			ReadCloser: closeBoth{rc, closer}}, nil
	}
	closer()
	return nil, domain.Errf(domain.CodeNotFound, "no %s in page %s", name, id)
}

type closeBoth struct {
	io.ReadCloser
	after func()
}

func (c closeBoth) Close() error {
	err := c.ReadCloser.Close()
	c.after()
	return err
}

// Delete removes a page and all its versions.
func (s *PageService) Delete(ctx context.Context, by Actor, id string) (*domain.Page, error) {
	pv, err := s.Get(ctx, by, id)
	if err != nil {
		return nil, err
	}
	if !s.manages(by, pv.Page) {
		return nil, domain.Errf(domain.CodePermissionDenied, "you may not delete page %s", id).
			WithSuggestion("its owner, their agents and the workspace's admins may")
	}
	if err := s.pages.Delete(ctx, id); err != nil {
		return nil, err
	}
	for _, v := range pv.Versions {
		_ = s.blobs.Delete(ctx, v.Blob)
	}
	return pv.Page, nil
}

// Manages reports whether by may publish versions of p and delete it.
func (s *PageService) Manages(by Actor, p *domain.Page) bool { return s.manages(by, p) }
