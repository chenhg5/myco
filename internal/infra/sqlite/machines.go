package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/chenhg5/myco/internal/domain"
)

// MachineRepo implements domain.MachineRepository.
type MachineRepo struct{ db *sql.DB }

// NewMachineRepo returns a SQLite-backed machine repository.
func NewMachineRepo(db *sql.DB) *MachineRepo { return &MachineRepo{db: db} }

var _ domain.MachineRepository = (*MachineRepo)(nil)

const machineCols = `id, fingerprint_hash, name, hostname, os, arch, myco_version, created_at, last_seen_at`

func (r *MachineRepo) Create(ctx context.Context, m *domain.Machine) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO machines (`+machineCols+`) VALUES (?,?,?,?,?,?,?,?,?)`,
		m.ID, m.FingerprintHash, m.Name, m.Hostname, m.OS, m.Arch, m.MycoVersion, ms(m.CreatedAt), ms(m.LastSeenAt))
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "machine already exists")
	}
	return internal(err)
}

func (r *MachineRepo) Save(ctx context.Context, m *domain.Machine) error {
	res, err := r.db.ExecContext(ctx, `UPDATE machines SET name=?, hostname=?, os=?, arch=?, myco_version=?, last_seen_at=? WHERE id=?`,
		m.Name, m.Hostname, m.OS, m.Arch, m.MycoVersion, ms(m.LastSeenAt), m.ID)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "machine %s not found", m.ID)
	}
	return nil
}

func (r *MachineRepo) Get(ctx context.Context, id string) (*domain.Machine, error) {
	return r.one(ctx, `WHERE id=?`, id)
}

func (r *MachineRepo) GetByFingerprint(ctx context.Context, hash string) (*domain.Machine, error) {
	return r.one(ctx, `WHERE fingerprint_hash=?`, hash)
}

func (r *MachineRepo) List(ctx context.Context) ([]*domain.Machine, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+machineCols+` FROM machines ORDER BY created_at`)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.Machine
	for rows.Next() {
		m, err := scanMachine(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, m)
	}
	return out, internal(rows.Err())
}

func (r *MachineRepo) one(ctx context.Context, where string, args ...any) (*domain.Machine, error) {
	m, err := scanMachine(r.db.QueryRowContext(ctx, `SELECT `+machineCols+` FROM machines `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "machine not found").WithSuggestion("list them with: myco machine list")
	}
	return m, internal(err)
}

func scanMachine(s scanner) (*domain.Machine, error) {
	var (
		m             domain.Machine
		created, seen int64
	)
	if err := s.Scan(&m.ID, &m.FingerprintHash, &m.Name, &m.Hostname, &m.OS, &m.Arch, &m.MycoVersion, &created, &seen); err != nil {
		return nil, err
	}
	m.CreatedAt, m.LastSeenAt = fromMS(created), fromMS(seen)
	return &m, nil
}
