package sqlite

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

const quotaAccountSchema = `
CREATE TABLE IF NOT EXISTS quota_account_secret (id INTEGER PRIMARY KEY CHECK(id=1), secret BLOB NOT NULL CHECK(length(secret)=32));
CREATE TABLE IF NOT EXISTS quota_account_snapshots (
 owner TEXT NOT NULL, provider TEXT NOT NULL, plan TEXT NOT NULL, account_key TEXT NOT NULL,
 at_ns INTEGER NOT NULL, snapshot TEXT NOT NULL, reported_by TEXT NOT NULL, reported_at_ns INTEGER NOT NULL,
 PRIMARY KEY(owner,provider,plan,account_key));
CREATE TABLE IF NOT EXISTS quota_account_aliases (
 owner TEXT NOT NULL, provider TEXT NOT NULL, plan TEXT NOT NULL, email_key TEXT NOT NULL,
 stable_key TEXT NOT NULL, ambiguous INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(owner,provider,plan,email_key));
`

// The random Hub-local key stays in this private database and its backups.
// Neither normalized identifiers nor derived keys enter quota DTOs/logs.
func migrateQuotaAccounts(db *sql.DB) error {
	if _, err := db.Exec(quotaAccountSchema); err != nil {
		return err
	}
	if err := addColumns(db, "account_quotas", [][2]string{{"reported_by", "TEXT NOT NULL DEFAULT ''"}, {"reported_at_ns", "INTEGER NOT NULL DEFAULT 0"}}); err != nil {
		return err
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM quota_account_secret`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	// Read before beginning the write transaction (Open initially has one connection).
	type old struct{ id, name, owner, machine, machineOwner, runtime, harness string }
	rows, err := db.Query(`SELECT a.id,a.name,a.owner,a.machine_id,COALESCE(m.owner,''),a.runtime,a.harness FROM agents a LEFT JOIN machines m ON m.id=a.machine_id`)
	if err != nil {
		return err
	}
	var agents []old
	for rows.Next() {
		var a old
		if err = rows.Scan(&a.id, &a.name, &a.owner, &a.machine, &a.machineOwner, &a.runtime, &a.harness); err != nil {
			rows.Close()
			return err
		}
		agents = append(agents, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return err
	}
	res, err := tx.Exec(`INSERT OR IGNORE INTO quota_account_secret(id,secret) VALUES(1,?)`, secret)
	if err != nil {
		return err
	}
	inserted, _ := res.RowsAffected()
	if inserted == 0 {
		return tx.Commit()
	}
	for _, a := range agents {
		var rt domain.Runtime
		if json.Unmarshal([]byte(a.runtime), &rt) != nil {
			continue
		}
		owner := a.owner
		if a.machineOwner != "" {
			owner = a.machineOwner
		}
		if owner == "" || owner != a.owner {
			continue
		}
		if q := rt.Quota; q != nil && q.ProfileRef != "" {
			var raw string
			if tx.QueryRow(`SELECT snapshot FROM account_quotas WHERE owner=? AND machine=? AND provider=? AND profile=?`, owner, a.machine, q.Provider, q.ProfileRef).Scan(&raw) == nil {
				var snapshot domain.Quota
				if json.Unmarshal([]byte(raw), &snapshot) == nil {
					rt.Quota = &snapshot
				}
			}
		}
		agent := &domain.Agent{ID: a.id, Name: domain.AgentName(a.name), Owner: a.owner, Runtime: rt, Binding: domain.Binding{Harness: a.harness}}
		if rt.Quota != nil {
			rt.Quota.ReportedAt = rt.Quota.CheckedAt
			if rt.Quota.ReportedAt.IsZero() {
				rt.Quota.ReportedAt = rt.Quota.At
			}
			agent.Runtime = rt
		}
		if err = publishAccountQuota(context.Background(), tx, secret, owner, agent, time.Now().UTC()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func quotaIdentityKey(secret []byte, owner, provider, plan, kind, value string) string {
	mac := hmac.New(sha256.New, secret)
	for _, v := range []string{"quota-account-v1", owner, provider, plan, kind, value} {
		mac.Write([]byte{0})
		mac.Write([]byte(v))
	}
	return hex.EncodeToString(mac.Sum(nil))
}
func quotaSecret(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) ([]byte, error) {
	var secret []byte
	if err := q.QueryRowContext(ctx, `SELECT secret FROM quota_account_secret WHERE id=1`).Scan(&secret); err != nil {
		return nil, err
	}
	if len(secret) != 32 {
		return nil, errors.New("quota account key unavailable")
	}
	return secret, nil
}

type quotaQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func accountLookupKey(ctx context.Context, q quotaQuerier, secret []byte, owner string, a *domain.Agent) (provider, plan, key string, err error) {
	provider, plan = domain.SubscriptionQuotaContext(a)
	if provider == "" || owner == "" {
		return "", "", "", nil
	}
	if a.Runtime.AccountIDHash != "" {
		return provider, plan, quotaIdentityKey(secret, owner, provider, plan, "id", a.Runtime.AccountIDHash), nil
	}
	email := domain.NormalizeQuotaAccount(a.Runtime.Account)
	if email == "" {
		return "", "", "", nil
	}
	key = quotaIdentityKey(secret, owner, provider, plan, "email", email)
	var stable string
	var ambiguous int
	err = q.QueryRowContext(ctx, `SELECT stable_key,ambiguous FROM quota_account_aliases WHERE owner=? AND provider=? AND plan=? AND email_key=?`, owner, provider, plan, key).Scan(&stable, &ambiguous)
	if err == sql.ErrNoRows {
		return provider, plan, key, nil
	}
	if err != nil {
		return "", "", "", err
	}
	if ambiguous != 0 {
		return "", "", "", nil
	}
	return provider, plan, stable, nil
}

func publishAccountQuota(ctx context.Context, tx *sql.Tx, secret []byte, owner string, a *domain.Agent, now time.Time) error {
	provider, plan := domain.SubscriptionQuotaContext(a)
	if provider == "" || owner == "" {
		return nil
	}
	if email := domain.NormalizeQuotaAccount(a.Runtime.Account); email != "" && a.Runtime.AccountIDHash != "" {
		emailKey := quotaIdentityKey(secret, owner, provider, plan, "email", email)
		stable := quotaIdentityKey(secret, owner, provider, plan, "id", a.Runtime.AccountIDHash)
		_, err := tx.ExecContext(ctx, `INSERT INTO quota_account_aliases(owner,provider,plan,email_key,stable_key) VALUES(?,?,?,?,?) ON CONFLICT(owner,provider,plan,email_key) DO UPDATE SET ambiguous=CASE WHEN stable_key!=excluded.stable_key THEN 1 ELSE ambiguous END`, owner, provider, plan, emailKey, stable)
		if err != nil {
			return err
		}
	}
	_, _, key, err := accountLookupKey(ctx, tx, secret, owner, a)
	if err != nil || key == "" {
		return err
	}
	q := a.Runtime.Quota
	if q == nil || q.At.IsZero() || q.At.After(now.Add(5*time.Minute)) || len(q.Windows) == 0 || q.Status != "" && q.Status != "ok" {
		return nil
	}
	snapshot := *q
	snapshot.Provider, snapshot.Plan = provider, plan
	snapshot.Shared = false
	snapshot.SharedAt = time.Time{}
	reportedAt := now
	if !q.ReportedAt.IsZero() {
		reportedAt = q.ReportedAt
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO quota_account_snapshots(owner,provider,plan,account_key,at_ns,snapshot,reported_by,reported_at_ns) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(owner,provider,plan,account_key) DO UPDATE SET at_ns=excluded.at_ns,snapshot=excluded.snapshot,reported_by=excluded.reported_by,reported_at_ns=excluded.reported_at_ns WHERE excluded.at_ns>quota_account_snapshots.at_ns`, owner, provider, plan, key, q.At.UnixNano(), string(data), a.ID, reportedAt.UnixNano())
	return err
}

func (r *AgentRepo) GetAccountQuota(ctx context.Context, owner string, a *domain.Agent) (*domain.Quota, error) {
	if owner == "" || owner != a.Owner {
		return nil, domain.Errf(domain.CodeNotFound, "quota not found")
	}
	if provider, _ := domain.SubscriptionQuotaContext(a); provider == "" || a.Runtime.AccountIDHash == "" && domain.NormalizeQuotaAccount(a.Runtime.Account) == "" {
		return nil, domain.Errf(domain.CodeNotFound, "quota not found")
	}
	secret, err := quotaSecret(ctx, r.db)
	if err != nil {
		return nil, internal(err)
	}
	provider, plan, key, err := accountLookupKey(ctx, r.db, secret, owner, a)
	if err != nil {
		return nil, internal(err)
	}
	if key == "" {
		return nil, domain.Errf(domain.CodeNotFound, "quota not found")
	}
	var data, by string
	var reported int64
	err = r.db.QueryRowContext(ctx, `SELECT snapshot,reported_by,reported_at_ns FROM quota_account_snapshots WHERE owner=? AND provider=? AND plan=? AND account_key=?`, owner, provider, plan, key).Scan(&data, &by, &reported)
	if err == sql.ErrNoRows {
		return nil, domain.Errf(domain.CodeNotFound, "quota not found")
	}
	if err != nil {
		return nil, internal(err)
	}
	var q domain.Quota
	if json.Unmarshal([]byte(data), &q) != nil {
		return nil, domain.Errf(domain.CodeInternal, "invalid quota snapshot")
	}
	q.ReportedBy, q.ReportedAt = by, time.Unix(0, reported).UTC()
	return &q, nil
}
