package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/myco-ai/myco/internal/domain"
)

// TemplateRepo implements domain.TemplateRepository.
type TemplateRepo struct{ db *sql.DB }

// NewTemplateRepo returns a SQLite-backed template library.
func NewTemplateRepo(db *sql.DB) *TemplateRepo { return &TemplateRepo{db: db} }

var _ domain.TemplateRepository = (*TemplateRepo)(nil)

const templateCols = `id, slug, name, description, kind, latest, publisher, owner, created_at, updated_at`

func (r *TemplateRepo) Create(ctx context.Context, t *domain.Template) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO templates (`+templateCols+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.Slug, t.Name, t.Description, t.Kind, t.Latest, t.Publisher, t.Owner, ms(t.CreatedAt), ms(t.UpdatedAt))
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "template %s exists", t.Slug).WithSuggestion("publish a new version of it")
	}
	return internal(err)
}

func (r *TemplateRepo) Save(ctx context.Context, t *domain.Template) error {
	res, err := r.db.ExecContext(ctx, `UPDATE templates SET name=?, description=?, kind=?, latest=?, updated_at=? WHERE id=?`,
		t.Name, t.Description, t.Kind, t.Latest, ms(t.UpdatedAt), t.ID)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "template %s not found", t.ID)
	}
	return nil
}

func scanTemplate(s scanner) (*domain.Template, error) {
	var (
		t                domain.Template
		created, updated int64
	)
	if err := s.Scan(&t.ID, &t.Slug, &t.Name, &t.Description, &t.Kind, &t.Latest, &t.Publisher, &t.Owner, &created, &updated); err != nil {
		return nil, err
	}
	t.CreatedAt, t.UpdatedAt = fromMS(created), fromMS(updated)
	return &t, nil
}

func notTemplate(id string) error {
	return domain.Errf(domain.CodeNotFound, "template %s not found", id).WithSuggestion("list templates with: myco template list")
}

func (r *TemplateRepo) Get(ctx context.Context, id string) (*domain.Template, error) {
	t, err := scanTemplate(r.db.QueryRowContext(ctx, `SELECT `+templateCols+` FROM templates WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notTemplate(id)
	}
	return t, internal(err)
}

func (r *TemplateRepo) BySlug(ctx context.Context, slug string) (*domain.Template, error) {
	t, err := scanTemplate(r.db.QueryRowContext(ctx, `SELECT `+templateCols+` FROM templates WHERE slug=?`, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notTemplate(slug)
	}
	return t, internal(err)
}

func (r *TemplateRepo) List(ctx context.Context) ([]*domain.Template, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+templateCols+` FROM templates ORDER BY updated_at DESC`)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []*domain.Template{}
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, t)
	}
	return out, internal(rows.Err())
}

func (r *TemplateRepo) Delete(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM template_versions WHERE template=?`, id); err != nil {
		return internal(err)
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM templates WHERE id=?`, id)
	return internal(err)
}

const templateVersionCols = `template, version, blob, sha256, size, files, manifest, guide, by_name, at, icon`

func (r *TemplateRepo) AddVersion(ctx context.Context, v *domain.TemplateVersion) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO template_versions (`+templateVersionCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		v.Template, v.Version, v.Blob, v.SHA256, v.Size, v.Files, v.Manifest, v.Guide, v.By, ms(v.At), v.Icon)
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "version %s of this template exists", v.Version).WithSuggestion("raise the version in myco.json")
	}
	return internal(err)
}

func (r *TemplateRepo) Versions(ctx context.Context, template string) ([]*domain.TemplateVersion, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+templateVersionCols+` FROM template_versions WHERE template=?`, template)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []*domain.TemplateVersion{}
	for rows.Next() {
		var (
			v  domain.TemplateVersion
			at int64
		)
		if err := rows.Scan(&v.Template, &v.Version, &v.Blob, &v.SHA256, &v.Size, &v.Files, &v.Manifest, &v.Guide, &v.By, &at, &v.Icon); err != nil {
			return nil, internal(err)
		}
		v.At = fromMS(at)
		out = append(out, &v)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	// newest first, by version
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && domain.CompareVersions(out[j].Version, out[j-1].Version) > 0; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

func (r *TemplateRepo) DeleteVersion(ctx context.Context, template, version string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM template_versions WHERE template=? AND version=?`, template, version)
	return internal(err)
}
