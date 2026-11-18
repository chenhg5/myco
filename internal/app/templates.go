package app

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/chenhg5/myco/internal/domain"
)

// TemplateService: the workspace's library of templates (templates.md).
// Everyone in the workspace (people and agents) sees the library and makes
// pages from its templates. Whoever publishes a template (a person, or an
// agent for its owner) publishes its new versions and deletes it, as do the
// workspace's managers. A page made from a template keeps its own copy of
// the files and the manifest: deleting or updating the template leaves it
// as it is.
type TemplateService struct {
	repo  domain.TemplateRepository
	blobs domain.BlobStore
	pages *PageService
	data  *PageDataService
	ids   IDGenerator
	clock Clock
}

// NewTemplateService wires the library.
func NewTemplateService(repo domain.TemplateRepository, blobs domain.BlobStore, pages *PageService, data *PageDataService, ids IDGenerator, clock Clock) *TemplateService {
	return &TemplateService{repo: repo, blobs: blobs, pages: pages, data: data, ids: ids, clock: clock}
}

// TemplateView is a template, its versions (newest first) and its latest
// manifest and agent's guide.
type TemplateView struct {
	Template *domain.Template
	Versions []*domain.TemplateVersion
	Manifest *domain.Manifest
	Guide    string
}

func (s *TemplateService) manages(by Actor, t *domain.Template) bool {
	switch {
	case by.Manages():
		return true
	case by.Agent != nil:
		return string(by.Agent.Name) == t.Publisher || (t.Owner != "" && by.Agent.Owner == t.Owner)
	case by.Member != nil:
		return t.Owner != "" && by.Member.ID == t.Owner
	}
	return false
}

// Manages reports whether by may publish versions of t and delete it.
func (s *TemplateService) Manages(by Actor, t *domain.Template) bool { return s.manages(by, t) }

func (s *TemplateService) find(ctx context.Context, idOrSlug string) (*domain.Template, error) {
	if strings.HasPrefix(idOrSlug, "tp_") {
		return s.repo.Get(ctx, idOrSlug)
	}
	return s.repo.BySlug(ctx, idOrSlug)
}

func (s *TemplateService) view(ctx context.Context, t *domain.Template) (*TemplateView, error) {
	vs, err := s.repo.Versions(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	tv := &TemplateView{Template: t, Versions: vs}
	if len(vs) > 0 {
		tv.Manifest, _ = domain.ParseManifest([]byte(vs[0].Manifest))
		tv.Guide = vs[0].Guide
	}
	return tv, nil
}

// List lists the library, newest first.
func (s *TemplateService) List(ctx context.Context, by Actor) ([]*TemplateView, error) {
	ts, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*TemplateView, 0, len(ts))
	for _, t := range ts {
		tv, err := s.view(ctx, t)
		if err != nil {
			return nil, err
		}
		out = append(out, tv)
	}
	return out, nil
}

// Get returns one (by id or by its manifest's id).
func (s *TemplateService) Get(ctx context.Context, by Actor, idOrSlug string) (*TemplateView, error) {
	t, err := s.find(ctx, idOrSlug)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, t)
}

func readZipFile(z *zip.Reader, name string, max int64) ([]byte, bool, error) {
	for _, f := range z.File {
		if f.Name != name {
			continue
		}
		if f.UncompressedSize64 > uint64(max) {
			return nil, true, domain.Errf(domain.CodeInvalidArgument, "%s is larger than %d KB", name, max>>10)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, true, err
		}
		defer rc.Close()
		b, err := io.ReadAll(io.LimitReader(rc, max+1))
		return b, true, err
	}
	return nil, false, nil
}

// Publish adds a template (a zip of its folder, myco.json at its root) to
// the library, or a new version of one with the same manifest id.
func (s *TemplateService) Publish(ctx context.Context, by Actor, r io.Reader, byName string) (*TemplateView, error) {
	if s.blobs == nil {
		return nil, domain.Errf(domain.CodeNotFound, "templates are not enabled on this hub")
	}
	if s.pages.encrypted != nil && s.pages.encrypted(ctx) {
		return nil, domain.Errf(domain.CodeInvalidState, "this workspace has end-to-end encryption on; templates and their data would be kept in plain text")
	}
	blob := s.ids.New("tpv")
	size, sum, err := s.blobs.Put(ctx, blob, r, domain.MaxPageSize)
	if err != nil {
		if domain.IsCode(err, domain.CodeInvalidArgument) {
			return nil, domain.Errf(domain.CodeInvalidArgument, "the template is larger than %d MB", domain.MaxPageSize>>20)
		}
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = s.blobs.Delete(context.WithoutCancel(ctx), blob)
		}
	}()
	z, closer, err := s.pages.openZip(ctx, blob)
	if err != nil {
		return nil, domain.Errf(domain.CodeInvalidArgument, "the upload is not a zip of the template's folder").
			WithSuggestion("publish with: myco template publish --dir <folder>")
	}
	raw, found, err := readZipFile(z, "myco.json", 64<<10)
	if err == nil && !found {
		err = domain.Errf(domain.CodeInvalidArgument, "the template has no myco.json").
			WithSuggestion("start one with: myco template init --dir <folder> --name <name>")
	}
	if err != nil {
		closer()
		return nil, err
	}
	m, err := domain.ParseManifest(raw)
	if err != nil {
		closer()
		return nil, err
	}
	guide, _, err := readZipFile(z, m.Agent, domain.MaxTemplateGuide)
	if err != nil {
		closer()
		return nil, err
	}
	if sample, ok, err := readZipFile(z, m.Sample, 4<<20); err != nil {
		closer()
		return nil, err
	} else if ok {
		var parsed map[string][]json.RawMessage
		if json.Unmarshal(sample, &parsed) != nil {
			closer()
			return nil, domain.Errf(domain.CodeInvalidArgument, "%s should be {\"collection\": [records…]}", m.Sample)
		}
	}
	closer()
	files, _, err := s.pages.check(ctx, blob, m.Entry)
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()
	t, err := s.repo.BySlug(ctx, m.ID)
	fresh := domain.IsCode(err, domain.CodeNotFound)
	switch {
	case fresh:
		all, err := s.repo.List(ctx)
		if err != nil {
			return nil, err
		}
		if len(all) >= domain.MaxTemplates {
			return nil, domain.Errf(domain.CodeInvalidArgument, "this workspace already has %d templates", domain.MaxTemplates).
				WithSuggestion("delete templates no longer used (myco template rm)")
		}
		owner := by.MemberID()
		if by.Agent != nil {
			owner = by.Agent.Owner
		}
		t = &domain.Template{ID: s.ids.New("tp"), Slug: m.ID, Publisher: byName, Owner: owner, CreatedAt: now}
	case err != nil:
		return nil, err
	default:
		if !s.manages(by, t) {
			return nil, domain.Errf(domain.CodePermissionDenied, "template %s belongs to someone else", m.ID).
				WithSuggestion("use an id of your own in myco.json, e.g. yourname/%s", m.ID[strings.LastIndex(m.ID, "/")+1:])
		}
		if domain.CompareVersions(m.Version, t.Latest) <= 0 {
			return nil, domain.Errf(domain.CodeConflict, "%s is already at version %s", m.ID, t.Latest).
				WithSuggestion("raise the version in myco.json, e.g. to a higher patch or minor version")
		}
	}
	t.Name, t.Description, t.Kind, t.Latest, t.UpdatedAt = m.Name, m.Description, m.Kind, m.Version, now
	if fresh {
		err = s.repo.Create(ctx, t)
	} else {
		err = s.repo.Save(ctx, t)
	}
	if err != nil {
		return nil, err
	}
	clean, _ := json.Marshal(m)
	v := &domain.TemplateVersion{Template: t.ID, Version: m.Version, Blob: blob, SHA256: sum, Size: size, Files: files,
		Manifest: string(clean), Guide: string(guide), By: byName, At: now}
	if err := s.repo.AddVersion(ctx, v); err != nil {
		return nil, err
	}
	keep = true
	vs, err := s.repo.Versions(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	for len(vs) > domain.TemplateVersionsKept {
		old := vs[len(vs)-1]
		if err := s.repo.DeleteVersion(ctx, t.ID, old.Version); err == nil {
			_ = s.blobs.Delete(ctx, old.Blob)
		}
		vs = vs[:len(vs)-1]
	}
	return s.view(ctx, t)
}

// Delete takes a template out of the library; pages made from it stay.
func (s *TemplateService) Delete(ctx context.Context, by Actor, idOrSlug string) (*domain.Template, error) {
	tv, err := s.Get(ctx, by, idOrSlug)
	if err != nil {
		return nil, err
	}
	if !s.manages(by, tv.Template) {
		return nil, domain.Errf(domain.CodePermissionDenied, "you may not delete template %s", tv.Template.Slug).
			WithSuggestion("whoever published it, their agents and the workspace's admins may")
	}
	if err := s.repo.Delete(ctx, tv.Template.ID); err != nil {
		return nil, err
	}
	for _, v := range tv.Versions {
		_ = s.blobs.Delete(ctx, v.Blob)
	}
	return tv.Template, nil
}

// UseInput is a page to make from a template: its name, its project, the
// version ("" for the latest), and whether to start with the template's
// sample data.
type UseInput struct {
	Name    string
	Project string
	Version string
	Sample  bool
	By      string
}

// Use makes a page from a template.
func (s *TemplateService) Use(ctx context.Context, by Actor, idOrSlug string, in UseInput) (*PageView, *TemplateView, error) {
	tv, err := s.Get(ctx, by, idOrSlug)
	if err != nil {
		return nil, nil, err
	}
	var v *domain.TemplateVersion
	for _, x := range tv.Versions {
		if in.Version == "" || x.Version == in.Version {
			v = x
			break
		}
	}
	if v == nil {
		return nil, nil, domain.Errf(domain.CodeNotFound, "template %s has no version %s", tv.Template.Slug, in.Version).
			WithSuggestion("its versions: myco template get --template %s", tv.Template.Slug)
	}
	m, err := domain.ParseManifest([]byte(v.Manifest))
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = m.Name
	}
	rc, err := s.blobs.Open(ctx, v.Blob)
	if err != nil {
		return nil, nil, err
	}
	pv, err := s.pages.Publish(ctx, by, PublishInput{Name: in.Name, Project: in.Project, Entry: m.Entry, By: in.By}, rc)
	rc.Close()
	if err != nil {
		return nil, nil, err
	}
	p := pv.Page
	p.Template, p.TemplateVersion, p.Manifest = tv.Template.ID, v.Version, v.Manifest
	if err := s.pages.pages.Save(ctx, p); err != nil {
		return nil, nil, err
	}
	if in.Sample {
		z, closer, err := s.pages.openZip(ctx, v.Blob)
		if err == nil {
			raw, ok, _ := readZipFile(z, m.Sample, 4<<20)
			closer()
			var sample map[string][]json.RawMessage
			if ok && json.Unmarshal(raw, &sample) == nil {
				if err := s.data.Seed(ctx, by, p.ID, sample, in.By); err != nil {
					return pv, tv, err
				}
			}
		}
	}
	return pv, tv, nil
}
