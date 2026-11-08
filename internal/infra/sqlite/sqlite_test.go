package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/infra/sqlite"
)

// t0 is a fixed, millisecond-aligned time: the repositories store times as
// Unix milliseconds, so round-trips are only exact at that precision.
var t0 = time.Date(2026, 1, 2, 3, 4, 5, 6_000_000, time.UTC)

func openMem(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func wantCode(t *testing.T, err error, code domain.ErrorCode) {
	t.Helper()
	if !domain.IsCode(err, code) {
		t.Fatalf("want %s error, got %v", code, err)
	}
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newAgent(t *testing.T, name, tokenHash string, spec domain.AgentSpec) *domain.Agent {
	t.Helper()
	a, err := domain.NewAgent("agt_"+name, domain.AgentName(name), spec, tokenHash, t0)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	return a
}

func newMsg(t *testing.T, id string, from, to string, typ domain.MessageType, created time.Time) *domain.Message {
	t.Helper()
	m, err := domain.NewMessage(id, domain.AgentName(from), domain.AgentName(to), typ, "body of "+id, "", "", "", created)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	return m
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func ids(ms []*domain.Message) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}

// ---- schema / migrations ----

func TestOpenMemoryCreatesSchema(t *testing.T) {
	db := openMem(t)
	for _, table := range []string{"agents", "messages", "enrollments", "files", "file_access", "aliases"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil || n != 1 {
			t.Errorf("table %s: n=%d err=%v", table, n, err)
		}
	}
}

func TestOpenFileReopenKeepsDataAndMigrationsAreIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hub.db")
	db, err := sqlite.Open(path)
	mustNoErr(t, err)
	a := newAgent(t, "alice", "h-alice", domain.AgentSpec{Labels: []string{"team=x"}})
	mustNoErr(t, sqlite.NewAgentRepo(db).Create(ctx, a))
	mustNoErr(t, sqlite.NewMessageRepo(db).Create(ctx, newMsg(t, "m1", "alice", "alice", domain.TypeNote, t0)))
	mustNoErr(t, sqlite.NewAliasRepo(db).Set(ctx, &domain.Alias{Name: "boss", Target: "alice", CreatedAt: t0, UpdatedAt: t0}))
	countCols := func(db *sql.DB) int {
		var n int
		mustNoErr(t, db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('agents')`).Scan(&n))
		return n
	}
	cols := countCols(db)
	mustNoErr(t, db.Close())

	// Re-open twice: migrations run again and must neither fail nor change
	// the schema, and the data must still be there.
	for i := range 2 {
		db, err = sqlite.Open(path)
		if err != nil {
			t.Fatalf("reopen %d: %v", i, err)
		}
		if got := countCols(db); got != cols {
			t.Errorf("reopen %d: agents has %d columns, want %d", i, got, cols)
		}
		got, err := sqlite.NewAgentRepo(db).Get(ctx, "alice")
		if err != nil || !slices.Equal(got.Labels, []string{"team=x"}) {
			t.Fatalf("reopen %d: agent %+v %v", i, got, err)
		}
		if _, err := sqlite.NewMessageRepo(db).Get(ctx, "m1"); err != nil {
			t.Fatalf("reopen %d: message: %v", i, err)
		}
		if al, err := sqlite.NewAliasRepo(db).Get(ctx, "boss"); err != nil || al.Target != "alice" {
			t.Fatalf("reopen %d: alias %+v %v", i, al, err)
		}
		mustNoErr(t, db.Close())
	}
}

func TestOpenMigratesLegacySchema(t *testing.T) {
	// A database created before the phase-2 columns existed: Open must add
	// them (with defaults) and keep the old rows readable.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	mustNoErr(t, err)
	_, err = raw.Exec(`CREATE TABLE agents (
	  id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, description TEXT NOT NULL DEFAULT '',
	  capabilities TEXT NOT NULL DEFAULT '[]', delivery TEXT NOT NULL DEFAULT 'pull',
	  allow_from TEXT NOT NULL DEFAULT '[]', token_hash TEXT NOT NULL UNIQUE,
	  created_at INTEGER NOT NULL, last_seen_at INTEGER NOT NULL DEFAULT 0);
	INSERT INTO agents (id, name, capabilities, token_hash, created_at) VALUES ('agt_1', 'legacy', '["code"]', 'h1', 1000);`)
	mustNoErr(t, err)
	mustNoErr(t, raw.Close())

	db, err := sqlite.Open(path)
	mustNoErr(t, err)
	defer db.Close()
	a, err := sqlite.NewAgentRepo(db).Get(ctx, "legacy")
	mustNoErr(t, err)
	if a.Delivery.Mode != "pull" || !slices.Equal(a.Capabilities, []string{"code"}) || len(a.Labels) != 0 || a.Binding.Harness != "" {
		t.Fatalf("legacy agent: %+v", a)
	}
	if !a.CreatedAt.Equal(time.UnixMilli(1000)) || !a.LastSeenAt.IsZero() {
		t.Fatalf("legacy times: created %v last seen %v", a.CreatedAt, a.LastSeenAt)
	}
}

// ---- agents ----

func TestAgentRepoRoundTrip(t *testing.T) {
	ctx := context.Background()
	r := sqlite.NewAgentRepo(openMem(t))
	a := newAgent(t, "alice", "hash-a", domain.AgentSpec{
		DisplayName:  "Alice",
		Description:  "does things",
		Capabilities: []string{"code", "shell"},
		Labels:       []string{"gpu", "team=loops"},
		Binding:      domain.Binding{Harness: "claude", SessionID: "sess-1", Environment: "laptop", Workdir: "/w"},
		Delivery:     domain.Delivery{Mode: "webhook", URL: "https://x/y", Secret: "s3", Params: map[string]string{"k": "v"}},
		AllowFrom:    []domain.AgentName{"bob"},
	})
	a.EnrolledBy = "enr_1"
	mustNoErr(t, r.Create(ctx, a))

	got, err := r.Get(ctx, "alice")
	mustNoErr(t, err)
	if got.ID != a.ID || got.DisplayName != "Alice" || got.Description != "does things" || got.TokenHash != "hash-a" || got.EnrolledBy != "enr_1" {
		t.Errorf("scalars: %+v", got)
	}
	if !slices.Equal(got.Capabilities, a.Capabilities) || !slices.Equal(got.Labels, a.Labels) || !slices.Equal(got.AllowFrom, a.AllowFrom) {
		t.Errorf("lists: caps %v labels %v allow %v", got.Capabilities, got.Labels, got.AllowFrom)
	}
	if got.Binding != a.Binding {
		t.Errorf("binding: %+v", got.Binding)
	}
	if got.Delivery.Mode != "webhook" || got.Delivery.URL != "https://x/y" || got.Delivery.Secret != "s3" || got.Delivery.Params["k"] != "v" {
		t.Errorf("delivery: %+v", got.Delivery)
	}
	if !got.CreatedAt.Equal(t0) || !got.LastSeenAt.IsZero() || !got.StatusAt.IsZero() {
		t.Errorf("times: %v %v %v", got.CreatedAt, got.LastSeenAt, got.StatusAt)
	}

	// Save: labels, status, token rotation and binding are all persisted.
	mustNoErr(t, got.SetLabels([]string{"role=dev"}, []string{"gpu"}))
	mustNoErr(t, got.SetStatus("working", t0.Add(time.Minute)))
	got.RotateToken("hash-a2")
	mustNoErr(t, got.Bind(domain.Binding{SessionID: "sess-2"}))
	mustNoErr(t, r.Save(ctx, got))
	again, err := r.Get(ctx, "alice")
	mustNoErr(t, err)
	if !slices.Equal(again.Labels, []string{"role=dev", "team=loops"}) {
		t.Errorf("labels after save: %v", again.Labels)
	}
	if again.StatusText != "working" || !again.StatusAt.Equal(t0.Add(time.Minute)) || again.TokenHash != "hash-a2" || again.Binding.SessionID != "sess-2" {
		t.Errorf("after save: %+v", again)
	}
	if _, err := r.GetByTokenHash(ctx, "hash-a"); !domain.IsCode(err, domain.CodeNotFound) {
		t.Errorf("old token hash still resolves: %v", err)
	}
	if byTok, err := r.GetByTokenHash(ctx, "hash-a2"); err != nil || byTok.Name != "alice" {
		t.Errorf("GetByTokenHash: %+v %v", byTok, err)
	}
}

func TestAgentRepoNilListsRoundTripAsEmpty(t *testing.T) {
	ctx := context.Background()
	r := sqlite.NewAgentRepo(openMem(t))
	mustNoErr(t, r.Create(ctx, newAgent(t, "bare", "h", domain.AgentSpec{})))
	got, err := r.Get(ctx, "bare")
	mustNoErr(t, err)
	if len(got.Capabilities) != 0 || len(got.Labels) != 0 || len(got.AllowFrom) != 0 || got.Delivery.Mode != domain.DeliveryPull {
		t.Fatalf("bare agent: %+v", got)
	}
}

func TestAgentRepoConflicts(t *testing.T) {
	ctx := context.Background()
	r := sqlite.NewAgentRepo(openMem(t))
	mustNoErr(t, r.Create(ctx, newAgent(t, "alice", "h1", domain.AgentSpec{})))
	tests := []struct {
		name string
		a    *domain.Agent
	}{
		{"same name", func() *domain.Agent { a := newAgent(t, "alice", "h2", domain.AgentSpec{}); a.ID = "other"; return a }()},
		{"same token hash", newAgent(t, "bob", "h1", domain.AgentSpec{})},
		{"same id", func() *domain.Agent {
			a := newAgent(t, "carol", "h3", domain.AgentSpec{})
			a.ID = "agt_alice"
			return a
		}()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wantCode(t, r.Create(ctx, tc.a), domain.CodeConflict)
		})
	}
	list, err := r.List(ctx)
	mustNoErr(t, err)
	if len(list) != 1 {
		t.Fatalf("conflicting creates left rows: %d", len(list))
	}
}

func TestAgentRepoNotFound(t *testing.T) {
	ctx := context.Background()
	r := sqlite.NewAgentRepo(openMem(t))
	_, err := r.Get(ctx, "ghost")
	wantCode(t, err, domain.CodeNotFound)
	var de *domain.Error
	if !errors.As(err, &de) || de.Suggestion == "" {
		t.Errorf("Get not-found should carry a suggestion: %#v", err)
	}
	_, err = r.GetByTokenHash(ctx, "nope")
	wantCode(t, err, domain.CodeNotFound)
	_, err = r.FindBySession(ctx, "claude", "s")
	wantCode(t, err, domain.CodeNotFound)
	wantCode(t, r.Save(ctx, newAgent(t, "ghost", "h", domain.AgentSpec{})), domain.CodeNotFound)
	wantCode(t, r.Delete(ctx, "ghost"), domain.CodeNotFound)
	// Touch on a missing agent is a silent no-op.
	mustNoErr(t, r.Touch(ctx, "ghost", t0))
}

func TestAgentRepoListTouchFindDelete(t *testing.T) {
	ctx := context.Background()
	r := sqlite.NewAgentRepo(openMem(t))
	for _, n := range []string{"carol", "alice", "bob"} {
		mustNoErr(t, r.Create(ctx, newAgent(t, n, "h-"+n, domain.AgentSpec{Binding: domain.Binding{Harness: "claude", SessionID: "s-" + n}})))
	}
	list, err := r.List(ctx)
	mustNoErr(t, err)
	var names []domain.AgentName
	for _, a := range list {
		names = append(names, a.Name)
	}
	if !slices.Equal(names, []domain.AgentName{"alice", "bob", "carol"}) {
		t.Fatalf("List order: %v", names)
	}

	mustNoErr(t, r.Touch(ctx, "bob", t0.Add(time.Hour)))
	bob, err := r.Get(ctx, "bob")
	mustNoErr(t, err)
	if !bob.LastSeenAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("Touch: %v", bob.LastSeenAt)
	}

	got, err := r.FindBySession(ctx, "claude", "s-carol")
	if err != nil || got.Name != "carol" {
		t.Errorf("FindBySession: %+v %v", got, err)
	}
	_, err = r.FindBySession(ctx, "codex", "s-carol") // harness must match too
	wantCode(t, err, domain.CodeNotFound)

	mustNoErr(t, r.Delete(ctx, "bob"))
	_, err = r.Get(ctx, "bob")
	wantCode(t, err, domain.CodeNotFound)
	list, _ = r.List(ctx)
	if len(list) != 2 {
		t.Fatalf("after delete: %d agents", len(list))
	}
	// The name is free again.
	mustNoErr(t, r.Create(ctx, newAgent(t, "bob", "h-bob-2", domain.AgentSpec{})))
}

func TestAgentRepoEmptyList(t *testing.T) {
	list, err := sqlite.NewAgentRepo(openMem(t)).List(context.Background())
	if err != nil || len(list) != 0 {
		t.Fatalf("%v %v", list, err)
	}
}

// ---- aliases ----

func TestAliasRepo(t *testing.T) {
	ctx := context.Background()
	db := openMem(t)
	r := sqlite.NewAliasRepo(db)

	_, err := r.Get(ctx, "reviewer")
	wantCode(t, err, domain.CodeNotFound)
	wantCode(t, r.Delete(ctx, "reviewer"), domain.CodeNotFound)

	mustNoErr(t, r.Set(ctx, &domain.Alias{Name: "reviewer", Target: "alice", CreatedAt: t0, UpdatedAt: t0}))
	mustNoErr(t, r.Set(ctx, &domain.Alias{Name: "lead", Target: "alice", CreatedAt: t0, UpdatedAt: t0}))
	mustNoErr(t, r.Set(ctx, &domain.Alias{Name: "ops", Target: "bob", CreatedAt: t0, UpdatedAt: t0}))

	got, err := r.Get(ctx, "reviewer")
	mustNoErr(t, err)
	if got.Target != "alice" || !got.CreatedAt.Equal(t0) || !got.UpdatedAt.Equal(t0) {
		t.Fatalf("Get: %+v", got)
	}

	// Re-pointing keeps created_at (even if the caller passes another one)
	// and updates target and updated_at.
	later := t0.Add(time.Hour)
	mustNoErr(t, r.Set(ctx, &domain.Alias{Name: "reviewer", Target: "bob", CreatedAt: later, UpdatedAt: later}))
	got, err = r.Get(ctx, "reviewer")
	mustNoErr(t, err)
	if got.Target != "bob" || !got.CreatedAt.Equal(t0) || !got.UpdatedAt.Equal(later) {
		t.Fatalf("re-point: %+v", got)
	}

	list, err := r.List(ctx)
	mustNoErr(t, err)
	var names []domain.AgentName
	for _, a := range list {
		names = append(names, a.Name)
	}
	if !slices.Equal(names, []domain.AgentName{"lead", "ops", "reviewer"}) {
		t.Fatalf("List: %v", names)
	}

	mustNoErr(t, r.Delete(ctx, "lead"))
	_, err = r.Get(ctx, "lead")
	wantCode(t, err, domain.CodeNotFound)
}

func TestAliasRepoDeleteForTarget(t *testing.T) {
	// The schema has no foreign keys: deleting an agent row does not touch
	// its aliases. The cascade is AgentService.Remove calling
	// DeleteForTarget (pinned end-to-end in the httpapi tests).
	ctx := context.Background()
	db := openMem(t)
	agents, aliases := sqlite.NewAgentRepo(db), sqlite.NewAliasRepo(db)
	mustNoErr(t, agents.Create(ctx, newAgent(t, "alice", "h", domain.AgentSpec{})))
	for _, n := range []domain.AgentName{"a1", "a2"} {
		mustNoErr(t, aliases.Set(ctx, &domain.Alias{Name: n, Target: "alice", CreatedAt: t0, UpdatedAt: t0}))
	}
	mustNoErr(t, aliases.Set(ctx, &domain.Alias{Name: "b1", Target: "bob", CreatedAt: t0, UpdatedAt: t0}))

	mustNoErr(t, agents.Delete(ctx, "alice"))
	if list, _ := aliases.List(ctx); len(list) != 3 {
		t.Fatalf("repo-level agent delete should not touch aliases, have %d", len(list))
	}
	mustNoErr(t, aliases.DeleteForTarget(ctx, "alice"))
	list, err := aliases.List(ctx)
	mustNoErr(t, err)
	if len(list) != 1 || list[0].Name != "b1" {
		t.Fatalf("after DeleteForTarget: %+v", list)
	}
	// No aliases for target: no error.
	mustNoErr(t, aliases.DeleteForTarget(ctx, "nobody"))
}

func TestAliasRepoDeleteDangling(t *testing.T) {
	ctx := context.Background()
	db := openMem(t)
	agents, aliases := sqlite.NewAgentRepo(db), sqlite.NewAliasRepo(db)
	mustNoErr(t, agents.Create(ctx, newAgent(t, "alice", "h", domain.AgentSpec{})))
	mustNoErr(t, aliases.Set(ctx, &domain.Alias{Name: "lead", Target: "alice", CreatedAt: t0, UpdatedAt: t0}))
	for _, n := range []domain.AgentName{"ghost1", "ghost2"} { // targets that no longer exist
		mustNoErr(t, aliases.Set(ctx, &domain.Alias{Name: n, Target: "gone", CreatedAt: t0, UpdatedAt: t0}))
	}
	n, err := aliases.DeleteDangling(ctx)
	mustNoErr(t, err)
	if n != 2 {
		t.Fatalf("removed %d, want 2", n)
	}
	list, err := aliases.List(ctx)
	mustNoErr(t, err)
	if len(list) != 1 || list[0].Name != "lead" {
		t.Fatalf("after DeleteDangling: %+v", list)
	}
	if n, err := aliases.DeleteDangling(ctx); err != nil || n != 0 {
		t.Fatalf("second sweep: %d %v", n, err)
	}
}

// ---- messages ----

func TestMessageRepoCreateGet(t *testing.T) {
	ctx := context.Background()
	r := sqlite.NewMessageRepo(openMem(t))
	m, err := domain.NewMessage("m1", "alice", "bob", domain.TypeTask, "hello", "", "", "key-1", t0)
	mustNoErr(t, err)
	m.BroadcastID = "bc_1"
	m.Attachments = []domain.Attachment{{FileID: "f1", Name: "a.txt", Size: 3, SHA256: "abc", ContentType: "text/plain"}}
	mustNoErr(t, r.Create(ctx, m))

	got, err := r.Get(ctx, "m1")
	mustNoErr(t, err)
	if got.From != "alice" || got.To != "bob" || got.ThreadID != "m1" || got.Type != domain.TypeTask || got.Body != "hello" ||
		got.Status != domain.StatusQueued || got.IdempotencyKey != "key-1" || got.BroadcastID != "bc_1" ||
		!got.CreatedAt.Equal(t0) || !got.UpdatedAt.Equal(t0) || !got.LeaseUntil.IsZero() || got.Attempts != 0 {
		t.Fatalf("round trip: %+v", got)
	}
	if len(got.Attachments) != 1 || got.Attachments[0] != m.Attachments[0] {
		t.Fatalf("attachments: %+v", got.Attachments)
	}

	_, err = r.Get(ctx, "nope")
	wantCode(t, err, domain.CodeNotFound)
	wantCode(t, r.Create(ctx, m), domain.CodeConflict) // same id
}

func TestMessageRepoIdempotencyKey(t *testing.T) {
	ctx := context.Background()
	r := sqlite.NewMessageRepo(openMem(t))
	mk := func(id, from, key string) *domain.Message {
		m, err := domain.NewMessage(id, domain.AgentName(from), "bob", domain.TypeNote, "x", "", "", key, t0)
		mustNoErr(t, err)
		return m
	}
	mustNoErr(t, r.Create(ctx, mk("m1", "alice", "k")))
	wantCode(t, r.Create(ctx, mk("m2", "alice", "k")), domain.CodeConflict)
	// The key is scoped per sender.
	mustNoErr(t, r.Create(ctx, mk("m3", "carol", "k")))
	// Messages without a key never collide (stored as NULL).
	mustNoErr(t, r.Create(ctx, mk("m4", "alice", "")))
	mustNoErr(t, r.Create(ctx, mk("m5", "alice", "")))

	got, err := r.FindByIdempotencyKey(ctx, "alice", "k")
	if err != nil || got.ID != "m1" {
		t.Fatalf("FindByIdempotencyKey: %+v %v", got, err)
	}
	if got, err := r.FindByIdempotencyKey(ctx, "carol", "k"); err != nil || got.ID != "m3" {
		t.Fatalf("FindByIdempotencyKey carol: %+v %v", got, err)
	}
	_, err = r.FindByIdempotencyKey(ctx, "bob", "k")
	wantCode(t, err, domain.CodeNotFound)
	// An empty key does not match the key-less messages.
	_, err = r.FindByIdempotencyKey(ctx, "alice", "")
	wantCode(t, err, domain.CodeNotFound)
	if m4, _ := r.Get(ctx, "m4"); m4.IdempotencyKey != "" {
		t.Fatalf("key-less message read back with key %q", m4.IdempotencyKey)
	}
}

func TestMessageRepoClaimDue(t *testing.T) {
	ctx := context.Background()
	lease := 30 * time.Second
	claimer := func(now time.Time, maxAttempts int) func(*domain.Message) bool {
		return func(m *domain.Message) bool { return m.Claim(now, lease, maxAttempts) }
	}
	now := t0.Add(time.Hour)

	t.Run("only due messages of the recipient, oldest first, respects limit", func(t *testing.T) {
		r := sqlite.NewMessageRepo(openMem(t))
		mustNoErr(t, r.Create(ctx, newMsg(t, "q2", "alice", "bob", domain.TypeTask, t0.Add(2*time.Second))))
		mustNoErr(t, r.Create(ctx, newMsg(t, "q1", "alice", "bob", domain.TypeTask, t0.Add(1*time.Second))))
		mustNoErr(t, r.Create(ctx, newMsg(t, "q3", "alice", "bob", domain.TypeTask, t0.Add(3*time.Second))))
		mustNoErr(t, r.Create(ctx, newMsg(t, "other", "alice", "carol", domain.TypeTask, t0)))
		done := newMsg(t, "done", "alice", "bob", domain.TypeTask, t0)
		done.Status = domain.StatusDone
		mustNoErr(t, r.Create(ctx, done))
		acked := newMsg(t, "acked", "alice", "bob", domain.TypeTask, t0)
		acked.Status = domain.StatusAcked
		mustNoErr(t, r.Create(ctx, acked))

		got, err := r.ClaimDue(ctx, "bob", now, 2, nil, claimer(now, 5))
		mustNoErr(t, err)
		if !slices.Equal(ids(got), []string{"q1", "q2"}) {
			t.Fatalf("first claim: %v", ids(got))
		}
		for _, m := range got {
			if m.Status != domain.StatusDelivered || m.Attempts != 1 || !m.LeaseUntil.Equal(now.Add(lease)) {
				t.Errorf("claimed %s: %+v", m.ID, m)
			}
			stored, _ := r.Get(ctx, m.ID)
			if stored.Status != domain.StatusDelivered || stored.Attempts != 1 || !stored.LeaseUntil.Equal(now.Add(lease)) {
				t.Errorf("stored %s not persisted: %+v", m.ID, stored)
			}
		}
		got, err = r.ClaimDue(ctx, "bob", now, 10, nil, claimer(now, 5))
		mustNoErr(t, err)
		if !slices.Equal(ids(got), []string{"q3"}) {
			t.Fatalf("second claim: %v", ids(got))
		}
		got, err = r.ClaimDue(ctx, "bob", now, 10, nil, claimer(now, 5))
		mustNoErr(t, err)
		if len(got) != 0 {
			t.Fatalf("third claim should be empty: %v", ids(got))
		}
	})

	t.Run("types filter", func(t *testing.T) {
		r := sqlite.NewMessageRepo(openMem(t))
		mustNoErr(t, r.Create(ctx, newMsg(t, "task", "alice", "bob", domain.TypeTask, t0)))
		mustNoErr(t, r.Create(ctx, newMsg(t, "steer", "alice", "bob", domain.TypeSteer, t0.Add(time.Second))))
		mustNoErr(t, r.Create(ctx, newMsg(t, "note", "alice", "bob", domain.TypeNote, t0.Add(2*time.Second))))

		got, err := r.ClaimDue(ctx, "bob", now, 10, []domain.MessageType{domain.TypeSteer}, claimer(now, 5))
		mustNoErr(t, err)
		if !slices.Equal(ids(got), []string{"steer"}) {
			t.Fatalf("steer only: %v", ids(got))
		}
		got, err = r.ClaimDue(ctx, "bob", now, 10, []domain.MessageType{domain.TypeNote, domain.TypeTask}, claimer(now, 5))
		mustNoErr(t, err)
		if !slices.Equal(ids(got), []string{"task", "note"}) {
			t.Fatalf("note+task: %v", ids(got))
		}
	})

	t.Run("lease", func(t *testing.T) {
		r := sqlite.NewMessageRepo(openMem(t))
		mustNoErr(t, r.Create(ctx, newMsg(t, "m", "alice", "bob", domain.TypeTask, t0)))
		got, err := r.ClaimDue(ctx, "bob", now, 10, nil, claimer(now, 5))
		mustNoErr(t, err)
		if len(got) != 1 {
			t.Fatalf("initial claim: %v", ids(got))
		}
		// Lease not expired: not claimable, not listed as due.
		before := now.Add(lease - time.Millisecond)
		if got, _ := r.ClaimDue(ctx, "bob", before, 10, nil, claimer(before, 5)); len(got) != 0 {
			t.Fatalf("claimed under active lease: %v", ids(got))
		}
		if due, _ := r.ListDue(ctx, "bob", before, 10); len(due) != 0 {
			t.Fatalf("ListDue under active lease: %v", ids(due))
		}
		if rs, _ := r.DueRecipients(ctx, before); len(rs) != 0 {
			t.Fatalf("DueRecipients under active lease: %v", rs)
		}
		// Exactly at lease expiry the message is due again.
		at := now.Add(lease)
		if due, _ := r.ListDue(ctx, "bob", at, 10); len(due) != 1 {
			t.Fatalf("ListDue at lease expiry: %v", ids(due))
		}
		if rs, _ := r.DueRecipients(ctx, at); !slices.Equal(rs, []domain.AgentName{"bob"}) {
			t.Fatalf("DueRecipients at expiry: %v", rs)
		}
		got, err = r.ClaimDue(ctx, "bob", at, 10, nil, claimer(at, 5))
		mustNoErr(t, err)
		if len(got) != 1 || got[0].Attempts != 2 || !got[0].LeaseUntil.Equal(at.Add(lease)) {
			t.Fatalf("re-claim after expiry: %+v", got)
		}
		// An acked message is never redelivered, whatever its lease.
		_, err = r.Mutate(ctx, "m", func(m *domain.Message) ([]*domain.Message, error) { return nil, m.Ack("bob", at) })
		mustNoErr(t, err)
		far := at.Add(time.Hour)
		if got, _ := r.ClaimDue(ctx, "bob", far, 10, nil, claimer(far, 5)); len(got) != 0 {
			t.Fatalf("acked message redelivered: %v", ids(got))
		}
	})

	t.Run("max attempts fails the message", func(t *testing.T) {
		r := sqlite.NewMessageRepo(openMem(t))
		m := newMsg(t, "m", "alice", "bob", domain.TypeTask, t0)
		m.Status, m.Attempts, m.LeaseUntil = domain.StatusDelivered, 2, t0.Add(time.Second)
		mustNoErr(t, r.Create(ctx, m))
		mustNoErr(t, r.Create(ctx, newMsg(t, "fresh", "alice", "bob", domain.TypeTask, t0.Add(time.Second))))

		got, err := r.ClaimDue(ctx, "bob", now, 1, nil, claimer(now, 2))
		mustNoErr(t, err)
		// The exhausted message is failed out and does not count against
		// the limit, so the next one is still claimed in the same call.
		if !slices.Equal(ids(got), []string{"fresh"}) {
			t.Fatalf("claimed: %v", ids(got))
		}
		stored, _ := r.Get(ctx, "m")
		if stored.Status != domain.StatusFailed || stored.Error == "" || stored.Attempts != 2 {
			t.Fatalf("exhausted message: %+v", stored)
		}
		far := now.Add(time.Hour)
		for _, d := range must(r.ListDue(ctx, "bob", far, 10)) {
			if d.ID == "m" {
				t.Fatalf("failed message still due")
			}
		}
	})
}

func TestMessageRepoMutate(t *testing.T) {
	ctx := context.Background()
	r := sqlite.NewMessageRepo(openMem(t))
	m := newMsg(t, "task", "alice", "bob", domain.TypeTask, t0)
	m.Status = domain.StatusDelivered
	mustNoErr(t, r.Create(ctx, m))

	// A failing fn changes nothing.
	_, err := r.Mutate(ctx, "task", func(m *domain.Message) ([]*domain.Message, error) {
		m.Status = domain.StatusDone
		return nil, domain.Errf(domain.CodeInvalidState, "nope")
	})
	wantCode(t, err, domain.CodeInvalidState)
	if s, _ := r.Get(ctx, "task"); s.Status != domain.StatusDelivered {
		t.Fatalf("failed mutate persisted: %s", s.Status)
	}

	// A created message that cannot be inserted rolls the change back.
	_, err = r.Mutate(ctx, "task", func(m *domain.Message) ([]*domain.Message, error) {
		rep, err := m.Reply("bob", "task", "dup id", t0) // id collides with the task
		return []*domain.Message{rep}, err
	})
	wantCode(t, err, domain.CodeConflict)
	if s, _ := r.Get(ctx, "task"); s.Status != domain.StatusDelivered {
		t.Fatalf("rolled-back mutate persisted: %s", s.Status)
	}

	// Reply: the task closes and the reply is stored atomically.
	later := t0.Add(time.Minute)
	got, err := r.Mutate(ctx, "task", func(m *domain.Message) ([]*domain.Message, error) {
		rep, err := m.Reply("bob", "reply", "pong", later)
		return []*domain.Message{rep}, err
	})
	mustNoErr(t, err)
	if got.Status != domain.StatusDone {
		t.Fatalf("returned: %+v", got)
	}
	stored, _ := r.Get(ctx, "task")
	if stored.Status != domain.StatusDone || !stored.UpdatedAt.Equal(later) {
		t.Fatalf("stored: %+v", stored)
	}
	rep, err := r.FindReply(ctx, "task")
	if err != nil || rep.ID != "reply" || rep.From != "bob" || rep.To != "alice" || rep.ThreadID != "task" || rep.Type != domain.TypeReply {
		t.Fatalf("FindReply: %+v %v", rep, err)
	}

	_, err = r.Mutate(ctx, "missing", func(*domain.Message) ([]*domain.Message, error) { return nil, nil })
	wantCode(t, err, domain.CodeNotFound)
}

func TestMessageRepoFindReplyIgnoresProgressAndSteer(t *testing.T) {
	ctx := context.Background()
	r := sqlite.NewMessageRepo(openMem(t))
	task := newMsg(t, "task", "alice", "bob", domain.TypeTask, t0)
	task.Status = domain.StatusAcked
	mustNoErr(t, r.Create(ctx, task))
	p, err := task.Progress("bob", "prog", "50%", t0.Add(time.Second))
	mustNoErr(t, err)
	mustNoErr(t, r.Create(ctx, p))
	s, err := task.Steer("alice", "steer", "faster", t0.Add(2*time.Second))
	mustNoErr(t, err)
	mustNoErr(t, r.Create(ctx, s))
	_, err = r.FindReply(ctx, "task")
	wantCode(t, err, domain.CodeNotFound)

	ev, err := task.Fail("bob", "event", "broke", t0.Add(3*time.Second))
	mustNoErr(t, err)
	mustNoErr(t, r.Create(ctx, ev))
	got, err := r.FindReply(ctx, "task")
	if err != nil || got.ID != "event" || got.Type != domain.TypeEvent {
		t.Fatalf("FindReply event: %+v %v", got, err)
	}
}

func TestMessageRepoThreadsAndListings(t *testing.T) {
	ctx := context.Background()
	r := sqlite.NewMessageRepo(openMem(t))
	root := newMsg(t, "root", "alice", "bob", domain.TypeTask, t0)
	mustNoErr(t, r.Create(ctx, root))
	follow, err := domain.NewMessage("follow", "bob", "alice", domain.TypeNote, "more", "root", "", "", t0.Add(2*time.Second))
	mustNoErr(t, err)
	mid, err := domain.NewMessage("mid", "alice", "bob", domain.TypeNote, "mid", "root", "", "", t0.Add(time.Second))
	mustNoErr(t, err)
	mustNoErr(t, r.Create(ctx, follow))
	mustNoErr(t, r.Create(ctx, mid))
	mustNoErr(t, r.Create(ctx, newMsg(t, "elsewhere", "carol", "dave", domain.TypeNote, t0)))

	th, err := r.ListThread(ctx, "root")
	mustNoErr(t, err)
	if !slices.Equal(ids(th), []string{"root", "mid", "follow"}) {
		t.Fatalf("ListThread: %v", ids(th))
	}
	if th, _ := r.ListThread(ctx, "nope"); len(th) != 0 {
		t.Fatalf("unknown thread: %v", ids(th))
	}

	recent, err := r.ListRecent(ctx, "", 2)
	mustNoErr(t, err)
	if !slices.Equal(ids(recent), []string{"follow", "mid"}) {
		t.Fatalf("ListRecent all: %v", ids(recent))
	}
	recent, err = r.ListRecent(ctx, "carol", 10)
	mustNoErr(t, err)
	if !slices.Equal(ids(recent), []string{"elsewhere"}) {
		t.Fatalf("ListRecent carol: %v", ids(recent))
	}

	// Open counts / open list.
	_, err = r.ClaimDue(ctx, "bob", t0.Add(time.Hour), 1, nil, func(m *domain.Message) bool { return m.Claim(t0.Add(time.Hour), time.Minute, 5) })
	mustNoErr(t, err)
	counts, err := r.OpenCounts(ctx)
	mustNoErr(t, err)
	if counts["bob"][domain.StatusDelivered] != 1 || counts["bob"][domain.StatusQueued] != 1 || counts["alice"][domain.StatusQueued] != 1 || counts["dave"][domain.StatusQueued] != 1 {
		t.Fatalf("OpenCounts: %v", counts)
	}
	open, err := r.ListOpen(ctx, "bob", 10)
	mustNoErr(t, err)
	// In progress first, then what is still queued.
	if len(open) != 2 || open[0].ID != "root" || open[1].Status != domain.StatusQueued {
		t.Fatalf("ListOpen: %v", ids(open))
	}
}

func TestMessageRepoCreateManyAndBroadcast(t *testing.T) {
	ctx := context.Background()
	r := sqlite.NewMessageRepo(openMem(t))
	var batch []*domain.Message
	for i, to := range []string{"bob", "carol"} {
		m := newMsg(t, fmt.Sprintf("b%d", i), "alice", to, domain.TypeTask, t0.Add(time.Duration(i)*time.Second))
		m.BroadcastID = "bc"
		batch = append(batch, m)
	}
	mustNoErr(t, r.CreateMany(ctx, batch))
	got, err := r.ListBroadcast(ctx, "bc")
	mustNoErr(t, err)
	if !slices.Equal(ids(got), []string{"b0", "b1"}) {
		t.Fatalf("ListBroadcast: %v", ids(got))
	}

	// All or nothing: a duplicate in the batch stores none of it.
	bad := []*domain.Message{newMsg(t, "x1", "alice", "bob", domain.TypeNote, t0), newMsg(t, "b0", "alice", "bob", domain.TypeNote, t0)}
	wantCode(t, r.CreateMany(ctx, bad), domain.CodeConflict)
	_, err = r.Get(ctx, "x1")
	wantCode(t, err, domain.CodeNotFound)
}

// ---- enrollments ----

func newEnrollment(t *testing.T, id, hash string, s domain.EnrollmentSpec) *domain.Enrollment {
	t.Helper()
	e, err := domain.NewEnrollment(id, hash, s, t0)
	if err != nil {
		t.Fatalf("NewEnrollment: %v", err)
	}
	return e
}

func TestEnrollmentRepoRoundTrip(t *testing.T) {
	ctx := context.Background()
	r := sqlite.NewEnrollmentRepo(openMem(t))
	e := newEnrollment(t, "enr_1", "h1", domain.EnrollmentSpec{NamePrefix: "cloud-", Harness: "claude",
		AllowFrom: []domain.AgentName{"alice"}, Capabilities: []string{"code"}, Labels: []string{"env=ci"},
		Note: "ci", MaxUses: 3, TTL: time.Hour})
	mustNoErr(t, r.Create(ctx, e))

	for _, get := range []func() (*domain.Enrollment, error){
		func() (*domain.Enrollment, error) { return r.Get(ctx, "enr_1") },
		func() (*domain.Enrollment, error) { return r.GetByTokenHash(ctx, "h1") },
	} {
		got, err := get()
		mustNoErr(t, err)
		if got.ID != "enr_1" || got.NamePrefix != "cloud-" || got.Harness != "claude" || got.Note != "ci" || got.MaxUses != 3 || got.Uses != 0 ||
			!slices.Equal(got.AllowFrom, e.AllowFrom) || !slices.Equal(got.Capabilities, e.Capabilities) || !slices.Equal(got.Labels, e.Labels) ||
			!got.CreatedAt.Equal(t0) || !got.ExpiresAt.Equal(t0.Add(time.Hour)) || !got.RevokedAt.IsZero() || got.Kind != "" {
			t.Fatalf("round trip: %+v", got)
		}
	}
	_, err := r.Get(ctx, "nope")
	wantCode(t, err, domain.CodeNotFound)
	_, err = r.GetByTokenHash(ctx, "nope")
	wantCode(t, err, domain.CodeNotFound)
	wantCode(t, r.Create(ctx, newEnrollment(t, "enr_2", "h1", domain.EnrollmentSpec{NamePrefix: "x-"})), domain.CodeConflict)

	// Link + machine kinds round-trip their extra columns; List is ordered by creation.
	link := newEnrollment(t, "enr_link", "hl", domain.EnrollmentSpec{Kind: domain.EnrollmentLink})
	link.CreatedAt = t0.Add(time.Second)
	mustNoErr(t, r.Create(ctx, link))
	mach := newEnrollment(t, "enr_m", "hm", domain.EnrollmentSpec{Kind: domain.EnrollmentMachine, ParentID: "enr_link", Machine: "box", NamePrefix: "box-"})
	mach.CreatedAt = t0.Add(2 * time.Second)
	mustNoErr(t, r.Create(ctx, mach))
	list, err := r.List(ctx)
	mustNoErr(t, err)
	if len(list) != 3 || list[0].ID != "enr_1" || list[1].Kind != domain.EnrollmentLink || list[2].ParentID != "enr_link" || list[2].Machine != "box" {
		t.Fatalf("List: %+v", list)
	}
}

func TestEnrollmentRepoConsumeUse(t *testing.T) {
	ctx := context.Background()
	r := sqlite.NewEnrollmentRepo(openMem(t))
	mustNoErr(t, r.Create(ctx, newEnrollment(t, "lim", "h1", domain.EnrollmentSpec{NamePrefix: "a-", MaxUses: 2})))
	mustNoErr(t, r.Create(ctx, newEnrollment(t, "unl", "h2", domain.EnrollmentSpec{NamePrefix: "b-"})))

	mustNoErr(t, r.ConsumeUse(ctx, "lim"))
	mustNoErr(t, r.ConsumeUse(ctx, "lim"))
	wantCode(t, r.ConsumeUse(ctx, "lim"), domain.CodePermissionDenied)
	if e, _ := r.Get(ctx, "lim"); e.Uses != 2 {
		t.Fatalf("uses after exhaustion: %d", e.Uses)
	}
	for range 5 {
		mustNoErr(t, r.ConsumeUse(ctx, "unl"))
	}
	if e, _ := r.Get(ctx, "unl"); e.Uses != 5 {
		t.Fatalf("unlimited uses: %d", e.Uses)
	}
	// Unknown id reads as "no uses left" (no row updated).
	wantCode(t, r.ConsumeUse(ctx, "ghost"), domain.CodePermissionDenied)
}

func TestEnrollmentRepoRevokeAndExpiry(t *testing.T) {
	ctx := context.Background()
	r := sqlite.NewEnrollmentRepo(openMem(t))
	e := newEnrollment(t, "e", "h", domain.EnrollmentSpec{NamePrefix: "a-", TTL: time.Minute})
	mustNoErr(t, r.Create(ctx, e))

	got, _ := r.Get(ctx, "e")
	if err := got.Usable(t0.Add(30*time.Second), true); err != nil {
		t.Fatalf("usable before expiry: %v", err)
	}
	if err := got.Usable(t0.Add(2*time.Minute), true); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("expired enrollment usable: %v", err)
	}

	got.Revoke(t0.Add(10 * time.Second))
	got.SetToken("h-new")
	mustNoErr(t, r.Save(ctx, got))
	again, _ := r.Get(ctx, "e")
	if !again.RevokedAt.Equal(t0.Add(10*time.Second)) || again.TokenHash != "h-new" {
		t.Fatalf("after save: %+v", again)
	}
	if err := again.Usable(t0.Add(20*time.Second), false); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("revoked enrollment usable: %v", err)
	}
	if _, err := r.GetByTokenHash(ctx, "h"); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("old token hash still resolves: %v", err)
	}
	wantCode(t, r.Save(ctx, newEnrollment(t, "ghost", "x", domain.EnrollmentSpec{NamePrefix: "a-"})), domain.CodeNotFound)
}

// ---- files ----

func TestFileRepo(t *testing.T) {
	ctx := context.Background()
	r := sqlite.NewFileRepo(openMem(t))
	mk := func(id string, expires time.Time) *domain.File {
		return &domain.File{ID: id, Name: id + ".txt", Size: 42, SHA256: "sum-" + id, ContentType: "text/plain", Owner: "alice", CreatedAt: t0, ExpiresAt: expires}
	}
	mustNoErr(t, r.Create(ctx, mk("f1", t0.Add(time.Hour))))
	mustNoErr(t, r.Create(ctx, mk("f2", t0.Add(2*time.Hour))))
	mustNoErr(t, r.Create(ctx, mk("forever", time.Time{})))

	got, err := r.Get(ctx, "f1")
	mustNoErr(t, err)
	if *got != *mk("f1", t0.Add(time.Hour)) {
		t.Fatalf("round trip: %+v", got)
	}
	_, err = r.Get(ctx, "nope")
	wantCode(t, err, domain.CodeNotFound)

	// The owner can access its upload; others only after a grant (idempotent).
	if ok, err := r.CanAccess(ctx, "f1", "alice"); err != nil || !ok {
		t.Fatalf("owner access: %v %v", ok, err)
	}
	if ok, _ := r.CanAccess(ctx, "f1", "bob"); ok {
		t.Fatal("bob has access before grant")
	}
	mustNoErr(t, r.Grant(ctx, "f1", "bob"))
	mustNoErr(t, r.Grant(ctx, "f1", "bob"))
	if ok, _ := r.CanAccess(ctx, "f1", "bob"); !ok {
		t.Fatal("bob has no access after grant")
	}
	if ok, _ := r.CanAccess(ctx, "f2", "bob"); ok {
		t.Fatal("grant leaked to another file")
	}

	// Expiry sweep: strictly after expires_at, never for zero expiry, bounded by limit.
	tests := []struct {
		now   time.Time
		limit int
		want  int
	}{
		{t0.Add(30 * time.Minute), 10, 0},
		{t0.Add(time.Hour), 10, 0}, // not strictly after
		{t0.Add(time.Hour + time.Millisecond), 10, 1},
		{t0.Add(3 * time.Hour), 10, 2},
		{t0.Add(3 * time.Hour), 1, 1},
	}
	for _, tc := range tests {
		exp, err := r.ListExpired(ctx, tc.now, tc.limit)
		mustNoErr(t, err)
		if len(exp) != tc.want {
			t.Errorf("ListExpired(%v, %d) = %d files, want %d", tc.now.Sub(t0), tc.limit, len(exp), tc.want)
		}
		for _, f := range exp {
			if f.ID == "forever" || !f.Expired(tc.now) {
				t.Errorf("ListExpired returned unexpired %s", f.ID)
			}
		}
	}

	mustNoErr(t, r.Delete(ctx, "f1"))
	_, err = r.Get(ctx, "f1")
	wantCode(t, err, domain.CodeNotFound)
	if ok, _ := r.CanAccess(ctx, "f1", "bob"); ok {
		t.Fatal("grant survived file deletion")
	}
	// Deleting a missing file is not an error (the sweeper may race).
	mustNoErr(t, r.Delete(ctx, "f1"))
	// Duplicate id fails (as an internal error; the interface specifies no code).
	if err := r.Create(ctx, mk("f2", time.Time{})); err == nil {
		t.Fatal("duplicate file id accepted")
	}
}
