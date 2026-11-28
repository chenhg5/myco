package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

// GetQuota reads only the authenticated owner's machine/profile namespace.
func (r *AgentRepo) GetQuota(ctx context.Context, owner, machine, provider, profile string) (*domain.Quota, error) {
	var data, by string
	var reported int64
	err := r.db.QueryRowContext(ctx, `SELECT snapshot,reported_by,reported_at_ns FROM account_quotas WHERE owner=? AND machine=? AND provider=? AND profile=?`, owner, machine, provider, profile).Scan(&data, &by, &reported)
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
	q.ReportedBy = by
	if reported > 0 {
		q.ReportedAt = time.Unix(0, reported).UTC()
	}
	return &q, nil
}

func (r *AgentRepo) setAccountRuntime(ctx context.Context, name domain.AgentName, rt domain.Runtime) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	var owner, machine, actualOwner, id, harness string
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(NULLIF(m.owner,''),a.owner),a.machine_id,a.owner,a.id,a.harness FROM agents a LEFT JOIN machines m ON m.id=a.machine_id WHERE a.name=?`, name).Scan(&owner, &machine, &actualOwner, &id, &harness); err != nil {
		return internal(err)
	}
	now := rt.QuotaReportedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	secret, err := quotaSecret(ctx, tx)
	if err != nil {
		return internal(err)
	}
	if owner == actualOwner {
		a := &domain.Agent{ID: id, Name: name, Owner: actualOwner, Runtime: rt, Binding: domain.Binding{Harness: harness}}
		if err = publishAccountQuota(ctx, tx, secret, owner, a, now); err != nil {
			return internal(err)
		}
	}
	if rt.Quota != nil && rt.Quota.ProfileRef != "" {
		raw := *rt.Quota
		q := raw
		q.Shared = false
		q.SharedAt = time.Time{}
		q.MachineID = machine
		var prevData, by string
		var reported int64
		err = tx.QueryRowContext(ctx, `SELECT snapshot,reported_by,reported_at_ns FROM account_quotas WHERE owner=? AND machine=? AND provider=? AND profile=?`, owner, machine, q.Provider, q.ProfileRef).Scan(&prevData, &by, &reported)
		if err != nil && err != sql.ErrNoRows {
			return internal(err)
		}
		fresh := true
		if err == nil {
			var prev domain.Quota
			if err = json.Unmarshal([]byte(prevData), &prev); err != nil {
				return internal(err)
			}
			if prev.Plan == q.Plan || prev.Plan == "" || q.Plan == "" {
				if q.ErrorCode == "pending" || q.ErrorCode == "collector_busy" || (prev.ErrorCode != "pending" && prev.ErrorCode != "collector_busy" && prev.CheckedAt.After(q.CheckedAt)) {
					q = prev
					fresh = false
				} else if q.Status != "ok" || !q.At.After(prev.At) {
					q.At, q.Windows = prev.At, prev.Windows
					fresh = false
				}
			}
		}
		if fresh {
			by = id
			reported = now.UnixNano()
		}
		data, err := json.Marshal(q)
		if err != nil {
			return internal(err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO account_quotas(owner,machine,provider,profile,snapshot,reported_by,reported_at_ns) VALUES(?,?,?,?,?,?,?) ON CONFLICT(owner,machine,provider,profile) DO UPDATE SET snapshot=excluded.snapshot,reported_by=excluded.reported_by,reported_at_ns=excluded.reported_at_ns`, owner, machine, q.Provider, q.ProfileRef, string(data), by, reported); err != nil {
			return internal(err)
		}
		// Keep this agent's connection status, independently of the shared numbers.
		rt.Quota = &domain.Quota{Provider: raw.Provider, Source: raw.Source, ProfileRef: raw.ProfileRef, MachineID: machine, Plan: raw.Plan, SourceKind: raw.SourceKind, Status: raw.Status, ErrorCode: raw.ErrorCode, CheckedAt: raw.CheckedAt}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE agents SET runtime=? WHERE name=?`, runtimeJSON(rt), name); err != nil {
		return internal(err)
	}
	return internal(tx.Commit())
}
