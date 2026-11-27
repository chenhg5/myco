package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

func TestCoordinationProjectionAndFirstAck(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	repo := NewMessageRepo(db)
	now := time.Unix(1800000000, 0)
	put := func(id, from, to, thread string, typ domain.MessageType, at time.Time, reply string) *domain.Message {
		t.Helper()
		m, e := domain.NewMessage(id, domain.AgentName(from), domain.AgentName(to), typ, "private body must not be read", thread, reply, "", at)
		if e != nil {
			t.Fatal(e)
		}
		if e = repo.Create(ctx, m); e != nil {
			t.Fatal(e)
		}
		return m
	}
	m := put("t1", "coordinator", "worker", "", domain.TypeTask, now, "")
	mutate := func(at time.Time, fn func(*domain.Message) error) {
		t.Helper()
		_, e := repo.Mutate(ctx, m.ID, func(m *domain.Message) ([]*domain.Message, error) { return nil, fn(m) })
		if e != nil {
			t.Fatal(at, e)
		}
	}
	mutate(now, func(m *domain.Message) error { m.Claim(now.Add(time.Second), time.Minute, 5); return nil })
	mutate(now, func(m *domain.Message) error { return m.Ack("worker", now.Add(10*time.Second)) })
	mutate(now, func(m *domain.Message) error { return m.Ack("worker", now.Add(20*time.Second)) })
	mutate(now, func(m *domain.Message) error { return m.Stop("coordinator", now.Add(30*time.Second)) })
	put("t2", "coordinator", "worker", "t1", domain.TypeTask, now.Add(40*time.Second), "")
	put("t3", "coordinator", "other", "t1", domain.TypeTask, now.Add(50*time.Second), "")
	put("unrelated", "stranger", "worker", "", domain.TypeTask, now, "")
	stats := NewCoordinationRepo(db)
	filter := domain.CoordinationFilter{Since: now.Add(-time.Second), Until: now.Add(time.Minute), Names: []domain.AgentName{"coordinator"}}
	rows, err := stats.TaskTimings(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("scope leaked: %+v", rows)
	}
	r := rows[0]
	if !r.Stopped || !r.Redone || !r.Reassigned || !r.Started.Equal(now.Add(10*time.Second)) {
		t.Fatalf("wrong projection %+v", r)
	}
	filter.Names = []domain.AgentName{}
	rows, err = stats.TaskTimings(ctx, filter)
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	// Legacy rows have no invented queue time; never backfill from updated_at.
	var started int64
	if err = db.QueryRow(`SELECT first_acked_at FROM messages WHERE id='unrelated'`).Scan(&started); err != nil || started != 0 {
		t.Fatal(started, err)
	}
	plan, err := db.Query(`EXPLAIN QUERY PLAN SELECT id FROM messages INDEXED BY idx_messages_task_stats WHERE type='task' AND created_at>=?`, now.UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	found := false
	for plan.Next() {
		var id, parent, unused int
		var detail string
		if err := plan.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		found = found || strings.Contains(detail, "idx_messages_task_stats")
	}
	if !found {
		t.Fatal("task index not used")
	}
}

func TestCoordinationMigrationKeepsLegacyQueueUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hub.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = legacy.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err = legacy.Exec(`INSERT INTO messages(id,from_agent,to_agent,thread_id,type,body,status,created_at,updated_at) VALUES('old','sender','worker','old','task','example','done',1800000000000,1800000010000)`); err != nil {
		t.Fatal(err)
	}

	if _, err = legacy.Exec(`INSERT INTO messages(id,from_agent,to_agent,thread_id,type,body,status,created_at,updated_at) VALUES('old-open','sender','worker','old-open','task','example','queued',1800000000000,1800000000000)`); err != nil {
		t.Fatal(err)
	}
	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var first int64
	if err = db.QueryRow(`SELECT first_acked_at FROM messages WHERE id='old'`).Scan(&first); err != nil || first != -1 {
		t.Fatal(first, err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := NewCoordinationRepo(db).TaskTimings(context.Background(), domain.CoordinationFilter{Since: time.Unix(1799999999, 0), Until: time.Unix(1800000020, 0)})
	if err != nil || len(rows) != 2 || !rows[0].Started.IsZero() || !rows[0].Done {
		t.Fatal(rows, err)
	}
}

func TestCoordinationLegacyAckAfterMigrationStaysUnknown(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	// A pre-existing row uses the migration's -1 sentinel; normal Create
	// explicitly opts new messages into first-ACK recording with zero.
	_, err = db.Exec(`INSERT INTO messages(id,from_agent,to_agent,thread_id,type,body,status,created_at,updated_at) VALUES('old-open','sender','worker','old-open','task','example','queued',?,?)`, ms(now), ms(now))
	if err != nil {
		t.Fatal(err)
	}
	repo := NewMessageRepo(db)
	_, err = repo.Mutate(ctx, "old-open", func(m *domain.Message) ([]*domain.Message, error) {
		m.Claim(now.Add(time.Second), time.Minute, 5)
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.Mutate(ctx, "old-open", func(m *domain.Message) ([]*domain.Message, error) {
		return nil, m.Ack("worker", now.Add(10*time.Second))
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := NewCoordinationRepo(db).TaskTimings(ctx, domain.CoordinationFilter{Since: now.Add(-time.Second), Until: now.Add(time.Minute)})
	if err != nil || len(rows) != 1 || !rows[0].Started.IsZero() {
		t.Fatalf("legacy resumed task acquired an invented first queue sample: %+v %v", rows, err)
	}
}
