package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/myco-ai/myco/internal/domain"
)

// Members' model providers (i115). The key is only ever an end-to-end
// envelope here: the hub cannot open it.
const modelProviderSchema = `CREATE TABLE IF NOT EXISTS model_providers (
 id TEXT PRIMARY KEY, owner TEXT NOT NULL, created_at INTEGER NOT NULL, data TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_model_providers_owner ON model_providers(owner, created_at);
`

type ModelProviderRepo struct{ db *sql.DB }

func NewModelProviderRepo(db *sql.DB) *ModelProviderRepo { return &ModelProviderRepo{db} }

func (r *ModelProviderRepo) List(ctx context.Context, owner string) ([]*domain.ModelProvider, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT data FROM model_providers WHERE owner=? ORDER BY created_at, id`, owner)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []*domain.ModelProvider{}
	for rows.Next() {
		var data string
		if err = rows.Scan(&data); err != nil {
			return nil, internal(err)
		}
		var p domain.ModelProvider
		if err = json.Unmarshal([]byte(data), &p); err != nil {
			return nil, internal(err)
		}
		out = append(out, &p)
	}
	return out, internal(rows.Err())
}

func (r *ModelProviderRepo) Get(ctx context.Context, id string) (*domain.ModelProvider, error) {
	var data string
	err := r.db.QueryRowContext(ctx, `SELECT data FROM model_providers WHERE id=?`, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "model provider %s not found", id)
	}
	if err != nil {
		return nil, internal(err)
	}
	var p domain.ModelProvider
	if err = json.Unmarshal([]byte(data), &p); err != nil {
		return nil, internal(err)
	}
	return &p, nil
}

func (r *ModelProviderRepo) Create(ctx context.Context, p *domain.ModelProvider) error {
	b, _ := json.Marshal(p)
	_, err := r.db.ExecContext(ctx, `INSERT INTO model_providers(id,owner,created_at,data) VALUES(?,?,?,?)`, p.ID, p.Owner, ms(p.CreatedAt), string(b))
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "model provider %s exists", p.ID)
	}
	return internal(err)
}

func (r *ModelProviderRepo) Update(ctx context.Context, p *domain.ModelProvider) error {
	b, _ := json.Marshal(p)
	res, err := r.db.ExecContext(ctx, `UPDATE model_providers SET data=? WHERE id=? AND owner=?`, string(b), p.ID, p.Owner)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "model provider %s not found", p.ID)
	}
	return nil
}

func (r *ModelProviderRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM model_providers WHERE id=?`, id)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "model provider %s not found", id)
	}
	return nil
}

func (r *ModelProviderRepo) AnyHubHeld(ctx context.Context) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM model_providers WHERE json_extract(data,'$.key_hub') IS NOT NULL AND json_extract(data,'$.key_hub')!=''`).Scan(&n)
	if err != nil {
		return false, internal(err)
	}
	return n > 0, nil
}
