package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/myco-ai/myco/internal/domain"
)

// GetQuota reads only the authenticated owner's machine/profile namespace.
func (r *AgentRepo) GetQuota(ctx context.Context, owner, machine, provider, profile string) (*domain.Quota, error) {
	var data string
	err := r.db.QueryRowContext(ctx, `SELECT snapshot FROM account_quotas WHERE owner=? AND machine=? AND provider=? AND profile=?`, owner, machine, provider, profile).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, domain.Errf(domain.CodeNotFound, "quota not found")
	}
	if err != nil {
		return nil, internal(err)
	}
	var q domain.Quota
	if err = json.Unmarshal([]byte(data), &q); err != nil {
		return nil, internal(err)
	}
	return &q, nil
}

func (r *AgentRepo) setAccountRuntime(ctx context.Context, name domain.AgentName, rt domain.Runtime) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	var owner, machine string
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(NULLIF(m.owner,''),a.owner),a.machine_id FROM agents a LEFT JOIN machines m ON m.id=a.machine_id WHERE a.name=?`, name).Scan(&owner, &machine); err != nil {
		return internal(err)
	}
	q := *rt.Quota
	q.MachineID = machine
	var prevData string
	err = tx.QueryRowContext(ctx, `SELECT snapshot FROM account_quotas WHERE owner=? AND machine=? AND provider=? AND profile=?`, owner, machine, q.Provider, q.ProfileRef).Scan(&prevData)
	if err != nil && err != sql.ErrNoRows {
		return internal(err)
	}
	if err == nil {
		var prev domain.Quota
		if err = json.Unmarshal([]byte(prevData), &prev); err != nil {
			return internal(err)
		}
		if q.ErrorCode == "pending" || q.ErrorCode == "collector_busy" || (prev.ErrorCode != "pending" && prev.ErrorCode != "collector_busy" && prev.CheckedAt.After(q.CheckedAt)) {
			q = prev
		} else if q.Status != "ok" || q.At.Before(prev.At) {
			q.At, q.Windows = prev.At, prev.Windows
		}
	}
	data, err := json.Marshal(q)
	if err != nil {
		return internal(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO account_quotas(owner,machine,provider,profile,snapshot) VALUES(?,?,?,?,?) ON CONFLICT(owner,machine,provider,profile) DO UPDATE SET snapshot=excluded.snapshot`, owner, machine, q.Provider, q.ProfileRef, string(data)); err != nil {
		return internal(err)
	}
	// Only a reference is persisted per agent. Shared values live in one row.
	rt.Quota = &domain.Quota{Provider: q.Provider, ProfileRef: q.ProfileRef, MachineID: machine}
	if _, err = tx.ExecContext(ctx, `UPDATE agents SET runtime=? WHERE name=?`, runtimeJSON(rt), name); err != nil {
		return internal(err)
	}
	return internal(tx.Commit())
}
