package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/chenhg5/myco/internal/domain"
)

// KeyRepo implements domain.KeyRepository.
type KeyRepo struct{ db *sql.DB }

// NewKeyRepo returns a SQLite-backed public key store.
func NewKeyRepo(db *sql.DB) *KeyRepo { return &KeyRepo{db: db} }

func (r *KeyRepo) Put(ctx context.Context, k *domain.PublicKey) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO e2e_keys (kind, id, enc_key, sign_key, updated_at) VALUES (?,?,?,?,?)
		ON CONFLICT(kind, id) DO UPDATE SET enc_key=excluded.enc_key, sign_key=excluded.sign_key, updated_at=excluded.updated_at`,
		k.Kind, k.ID, k.Enc, k.Sign, ms(k.UpdatedAt))
	return internal(err)
}

func (r *KeyRepo) Get(ctx context.Context, kind, id string) (*domain.PublicKey, error) {
	k := domain.PublicKey{Kind: kind, ID: id}
	var at int64
	err := r.db.QueryRowContext(ctx, `SELECT enc_key, sign_key, updated_at FROM e2e_keys WHERE kind=? AND id=?`, kind, id).Scan(&k.Enc, &k.Sign, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "no key for %s %s", kind, id)
	}
	if err != nil {
		return nil, internal(err)
	}
	k.UpdatedAt = fromMS(at)
	return &k, nil
}

// SettingRepo implements domain.SettingRepository.
type SettingRepo struct{ db *sql.DB }

// NewSettingRepo returns a SQLite-backed settings store.
func NewSettingRepo(db *sql.DB) *SettingRepo { return &SettingRepo{db: db} }

func (r *SettingRepo) Get(ctx context.Context, key string) (string, error) {
	var v string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM hub_settings WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, internal(err)
}

func (r *SettingRepo) Set(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO hub_settings (key, value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return internal(err)
}
