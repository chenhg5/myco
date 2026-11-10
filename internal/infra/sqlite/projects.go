package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/chenhg5/myco/internal/domain"
)

// ProjectRepo implements domain.ProjectRepository.
type ProjectRepo struct{ db *sql.DB }

// NewProjectRepo returns a SQLite-backed project repository.
func NewProjectRepo(db *sql.DB) *ProjectRepo { return &ProjectRepo{db: db} }

var _ domain.ProjectRepository = (*ProjectRepo)(nil)

const projectCols = `id, name, title, goal, brief, links, visibility, owner, archived, created_at, updated_at`

func linksJSON(l []domain.ProjectLink) string {
	if len(l) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(l)
	return string(b)
}

func (r *ProjectRepo) Create(ctx context.Context, p *domain.Project) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO projects (`+projectCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.Title, p.Goal, p.Brief, linksJSON(p.Links), string(p.Visibility), p.Owner, p.Archived, ms(p.CreatedAt), ms(p.UpdatedAt))
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "a project named %s already exists", p.Name)
	}
	return internal(err)
}

func (r *ProjectRepo) Save(ctx context.Context, p *domain.Project) error {
	res, err := r.db.ExecContext(ctx, `UPDATE projects SET title=?, goal=?, brief=?, links=?, visibility=?, owner=?, archived=?, updated_at=? WHERE id=?`,
		p.Title, p.Goal, p.Brief, linksJSON(p.Links), string(p.Visibility), p.Owner, p.Archived, ms(p.UpdatedAt), p.ID)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "project %s not found", p.Name)
	}
	return nil
}

func scanProject(s scanner) (*domain.Project, error) {
	var (
		p                domain.Project
		links, vis       string
		created, updated int64
	)
	if err := s.Scan(&p.ID, &p.Name, &p.Title, &p.Goal, &p.Brief, &links, &vis, &p.Owner, &p.Archived, &created, &updated); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(links), &p.Links)
	p.Visibility = domain.ProjectVisibility(vis)
	p.CreatedAt, p.UpdatedAt = fromMS(created), fromMS(updated)
	return &p, nil
}

func (r *ProjectRepo) Get(ctx context.Context, name string) (*domain.Project, error) {
	p, err := scanProject(r.db.QueryRowContext(ctx, `SELECT `+projectCols+` FROM projects WHERE name=? OR id=?`, name, name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "project %s not found", name).
			WithSuggestion("list projects with: myco project list")
	}
	return p, internal(err)
}

func (r *ProjectRepo) List(ctx context.Context) ([]*domain.Project, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+projectCols+` FROM projects ORDER BY archived, updated_at DESC`)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, p)
	}
	return out, internal(rows.Err())
}

func (r *ProjectRepo) Delete(ctx context.Context, id string) error {
	// Its conversations stay, without the project.
	for _, q := range []string{`UPDATE messages SET project='' WHERE project=(SELECT name FROM projects WHERE id=?)`, `DELETE FROM project_members WHERE project=?`, `DELETE FROM project_docs WHERE project=?`, `DELETE FROM projects WHERE id=?`} {
		if _, err := r.db.ExecContext(ctx, q, id); err != nil {
			return internal(err)
		}
	}
	return nil
}

func (r *ProjectRepo) PutMember(ctx context.Context, m *domain.ProjectMember) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO project_members (project, agent, role, note, added_by, added_at) VALUES (?,?,?,?,?,?)
		ON CONFLICT(project, agent) DO UPDATE SET role=excluded.role, note=excluded.note`,
		m.Project, m.Agent, string(m.Role), m.Note, m.AddedBy, ms(m.AddedAt))
	return internal(err)
}

func (r *ProjectRepo) DeleteMember(ctx context.Context, project string, agent domain.AgentName) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM project_members WHERE project=? AND agent=?`, project, agent)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "%s is not in this project", agent)
	}
	return nil
}

func (r *ProjectRepo) members(ctx context.Context, where string, arg any) ([]*domain.ProjectMember, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT project, agent, role, note, added_by, added_at FROM project_members WHERE `+where+` ORDER BY added_at`, arg)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.ProjectMember
	for rows.Next() {
		var (
			m     domain.ProjectMember
			role  string
			added int64
		)
		if err := rows.Scan(&m.Project, &m.Agent, &role, &m.Note, &m.AddedBy, &added); err != nil {
			return nil, internal(err)
		}
		m.Role, m.AddedAt = domain.ProjectRole(role), fromMS(added)
		out = append(out, &m)
	}
	return out, internal(rows.Err())
}

func (r *ProjectRepo) Members(ctx context.Context, project string) ([]*domain.ProjectMember, error) {
	return r.members(ctx, "project=?", project)
}

func (r *ProjectRepo) MembershipsOf(ctx context.Context, agent domain.AgentName) ([]*domain.ProjectMember, error) {
	return r.members(ctx, "agent=?", agent)
}

func (r *ProjectRepo) DeleteAgent(ctx context.Context, agent domain.AgentName) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM project_members WHERE agent=?`, agent)
	return internal(err)
}

func (r *ProjectRepo) PutDoc(ctx context.Context, d *domain.ProjectDoc) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO project_docs (project, name, body, updated_by, updated_at) VALUES (?,?,?,?,?)
		ON CONFLICT(project, name) DO UPDATE SET body=excluded.body, updated_by=excluded.updated_by, updated_at=excluded.updated_at`,
		d.Project, d.Name, d.Body, d.UpdatedBy, ms(d.UpdatedAt))
	return internal(err)
}

func (r *ProjectRepo) GetDoc(ctx context.Context, project, name string) (*domain.ProjectDoc, error) {
	var (
		d  domain.ProjectDoc
		at int64
	)
	err := r.db.QueryRowContext(ctx, `SELECT project, name, body, updated_by, updated_at FROM project_docs WHERE project=? AND name=?`, project, name).
		Scan(&d.Project, &d.Name, &d.Body, &d.UpdatedBy, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "document %s not found", name)
	}
	if err != nil {
		return nil, internal(err)
	}
	d.UpdatedAt = fromMS(at)
	return &d, nil
}

func (r *ProjectRepo) DeleteDoc(ctx context.Context, project, name string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM project_docs WHERE project=? AND name=?`, project, name)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "document %s not found", name)
	}
	return nil
}

func (r *ProjectRepo) Docs(ctx context.Context, project string) ([]*domain.ProjectDoc, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT project, name, body, updated_by, updated_at FROM project_docs WHERE project=? ORDER BY name`, project)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.ProjectDoc
	for rows.Next() {
		var (
			d  domain.ProjectDoc
			at int64
		)
		if err := rows.Scan(&d.Project, &d.Name, &d.Body, &d.UpdatedBy, &at); err != nil {
			return nil, internal(err)
		}
		d.UpdatedAt = fromMS(at)
		out = append(out, &d)
	}
	return out, internal(rows.Err())
}
