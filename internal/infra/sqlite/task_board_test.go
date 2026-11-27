package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

func taskTestDB(t *testing.T) (*sql.DB, context.Context, time.Time) {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, context.Background(), time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
}
func taskTestMessage(t *testing.T, db *sql.DB, now time.Time, id, from, to, project string, status domain.Status) {
	t.Helper()
	m, err := domain.NewMessage(id, domain.AgentName(from), domain.AgentName(to), domain.TypeTask, strings.Repeat("private message ", 1000), "", "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	m.Project = project
	m.Status = status
	if err = NewMessageRepo(db).Create(context.Background(), m); err != nil {
		t.Fatal(err)
	}
}
func TestTaskBoardMetadataIndexes(t *testing.T) {
	db, ctx, now := taskTestDB(t)
	repo := NewTaskMonitorRepo(db)
	for _, status := range []domain.Status{domain.StatusQueued, domain.StatusDelivered, domain.StatusAcked, domain.StatusHeld, domain.StatusDone, domain.StatusFailed} {
		taskTestMessage(t, db, now, string(status), "sender", "worker", "acme", status)
	}
	taskTestMessage(t, db, now, "other", "other", "worker", "acme", domain.StatusQueued)
	for _, f := range []domain.TaskFilter{{Sender: "sender"}, {Project: "acme"}, {Sender: "sender", Project: "acme"}, {ConsoleOwner: "owner"}, {}} {
		q, args := taskQuery(f)
		rows, err := db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+q, args...)
		if err != nil {
			t.Fatal(err)
		}
		var details []string
		for rows.Next() {
			var a, b, c int
			var d string
			if err := rows.Scan(&a, &b, &c, &d); err != nil {
				t.Fatal(err)
			}
			details = append(details, d)
		}
		rows.Close()
		if !strings.Contains(strings.Join(details, "\n"), "idx_tasks_open_") {
			t.Fatalf("history scan: %v", details)
		}
	}
	rows, err := repo.ListTasks(ctx, domain.TaskFilter{Sender: "sender"})
	if err != nil || len(rows) != 4 {
		t.Fatalf("open tasks: %+v %v", rows, err)
	}
	for _, r := range rows {
		if r.StaleAfter != 15*time.Minute || r.Status.Terminal() {
			t.Fatal(r)
		}
	}
	// Cover a real pre-existing hub upgrading in place and reopening idempotently.
	file := filepath.Join(t.TempDir(), "upgrade.db")
	old, err := sql.Open("sqlite", file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = old.Exec(`CREATE TABLE messages(id TEXT PRIMARY KEY,from_agent TEXT NOT NULL,to_agent TEXT NOT NULL,thread_id TEXT NOT NULL,reply_to TEXT NOT NULL DEFAULT '',type TEXT NOT NULL,body TEXT NOT NULL,status TEXT NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,lease_until INTEGER NOT NULL DEFAULT 0,error TEXT NOT NULL DEFAULT '',idempotency_key TEXT,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL);INSERT INTO messages(id,from_agent,to_agent,thread_id,type,body,status,created_at,updated_at)VALUES('old','sender','worker','old','task','legacy body','acked',1,1)`); err != nil {
		t.Fatal(err)
	}
	old.Close()
	for range 2 {
		up, err := Open(file)
		if err != nil {
			t.Fatal(err)
		}
		got, err := NewTaskMonitorRepo(up).ListTasks(ctx, domain.TaskFilter{Sender: "sender"})
		if err != nil || len(got) != 1 || got[0].ID != "old" {
			t.Fatalf("upgrade lost old task: %+v %v", got, err)
		}
		up.Close()
	}
}
func taskTestMachine(t *testing.T, db *sql.DB, now time.Time) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO machines(id,name,owner,fingerprint_hash,created_at,last_seen_at) VALUES('machine','demo','owner','fingerprint',?,?)`, ms(now), ms(now))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b", "c"} {
		if err := NewAgentRepo(db).Create(context.Background(), &domain.Agent{ID: name, Name: domain.AgentName(name), TokenHash: name, Owner: "owner", MachineID: "machine", CreatedAt: now, LastSeenAt: now}); err != nil {
			t.Fatal(err)
		}
	}
}
func TestTaskFailuresCapturedAtomicallyAndMachineStable(t *testing.T) {
	db, ctx, now := taskTestDB(t)
	taskTestMachine(t, db, now)
	repo := NewTaskMonitorRepo(db)
	for _, name := range []string{"a", "b", "c"} {
		taskTestMessage(t, db, now, name, "sender", name, "acme", domain.StatusAcked)
		if _, err := NewMessageRepo(db).Mutate(ctx, name, func(m *domain.Message) ([]*domain.Message, error) {
			m.Status = domain.StatusFailed
			m.UpdatedAt = now
			return nil, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Later metadata updates and machine moves cannot move a recorded failure.
	if _, err := db.Exec(`UPDATE agents SET machine_id='' WHERE name='a';UPDATE messages SET updated_at=updated_at+999999 WHERE id='a'`); err != nil {
		t.Fatal(err)
	}
	got, err := repo.TaskFailures(ctx, now.Add(-time.Minute))
	if err != nil || len(got) != 3 {
		t.Fatalf("failures %+v %v", got, err)
	}
	for _, f := range got {
		if f.Machine != "machine" || f.Incident != 1 || !f.At.Equal(now) {
			t.Fatal(f)
		}
	}
	// One more failure after 10 quiet minutes is a new incident.
	taskTestMessage(t, db, now.Add(11*time.Minute), "later", "sender", "b", "acme", domain.StatusFailed)
	got, err = repo.TaskFailures(ctx, now.Add(10*time.Minute))
	if err != nil || len(got) != 1 || got[0].Incident != 2 {
		t.Fatalf("new incident %+v %v", got, err)
	}
	// Unverified machine ownership must never help form a shared-machine incident.
	if _, err := db.Exec(`UPDATE agents SET owner='different-owner' WHERE name='c'`); err != nil {
		t.Fatal(err)
	}
	taskTestMessage(t, db, now.Add(12*time.Minute), "unverified", "sender", "c", "acme", domain.StatusFailed)
	got, err = repo.TaskFailures(ctx, now.Add(10*time.Minute))
	if err != nil || len(got) != 1 {
		t.Fatalf("spoofed ownership contributed: %+v %v", got, err)
	}
}
func TestTaskAlertAtomicDeliveryCooldownAndCurrentState(t *testing.T) {
	db, ctx, now := taskTestDB(t)
	r := NewTaskMonitorRepo(db)
	taskTestMessage(t, db, now.Add(-time.Hour), "one", "sender", "worker", "", domain.StatusQueued)
	taskTestMessage(t, db, now.Add(-time.Hour), "two", "sender", "worker", "", domain.StatusQueued)
	alerts := []domain.TaskAlert{{Key: "stale:one", Kind: "stale", TaskID: "one", Sender: "sender", Recipient: "worker", At: now}}
	if err := r.StageTaskAlerts(ctx, alerts); err != nil {
		t.Fatal(err)
	}
	build := func(id string) func([]domain.TaskAlert) *domain.Message {
		return func(a []domain.TaskAlert) *domain.Message {
			n, _ := domain.NewMessage(id, "@console", "sender", domain.TypeNote, "Task alerts", "", "", "", now)
			return n
		}
	}
	if _, err := r.DeliverTaskAlerts(ctx, "sender", now, 5*time.Minute, time.Minute, func([]domain.TaskAlert) *domain.Message { return nil }); err == nil {
		t.Fatal("bad builder committed")
	}
	n, err := r.DeliverTaskAlerts(ctx, "sender", now, 5*time.Minute, time.Minute, build("note-1"))
	if err != nil || n == nil {
		t.Fatalf("first delivery: %v %v", n, err)
	}
	alerts = append(alerts, domain.TaskAlert{Key: "stale:two", Kind: "stale", TaskID: "two", Sender: "sender", Recipient: "worker", At: now})
	if err := r.StageTaskAlerts(ctx, alerts); err != nil {
		t.Fatal(err)
	}
	// New repository instance simulates a restart, preserving dedup and cooldown.
	r = NewTaskMonitorRepo(db)
	n, err = r.DeliverTaskAlerts(ctx, "sender", now.Add(5*time.Minute-time.Millisecond), 5*time.Minute, time.Minute, build("too-early"))
	if err != nil || n != nil {
		t.Fatalf("cooldown failed %v %v", n, err)
	}
	// A snapshot that raced with completion must not send an outdated stale note.
	if _, err = db.Exec(`UPDATE messages SET status='done' WHERE id='two'`); err != nil {
		t.Fatal(err)
	}
	n, err = r.DeliverTaskAlerts(ctx, "sender", now.Add(5*time.Minute), 5*time.Minute, time.Minute, build("obsolete"))
	if err != nil || n != nil {
		t.Fatalf("closed task alarm: %v %v", n, err)
	}
	if err = r.StageTaskAlerts(ctx, nil); err != nil {
		t.Fatal(err)
	}
	senders, err := r.PendingTaskAlertSenders(ctx)
	if err != nil || len(senders) != 0 {
		t.Fatal(senders, err)
	}
}

func TestTaskAlertOverflowAndConcurrentDelivery(t *testing.T) {
	db, ctx, now := taskTestDB(t)
	r := NewTaskMonitorRepo(db)
	alerts := []domain.TaskAlert{}
	for i := 0; i < 60; i++ {
		id := fmt.Sprintf("task-%02d", i)
		taskTestMessage(t, db, now.Add(-time.Hour), id, "sender", "worker", "", domain.StatusQueued)
		alerts = append(alerts, domain.TaskAlert{Key: "stale:" + id, Kind: "stale", TaskID: id, Sender: "sender", Recipient: "worker", At: now})
	}
	if err := r.StageTaskAlerts(ctx, alerts); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var committed atomic.Int32
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Go(func() {
			n, err := r.DeliverTaskAlerts(ctx, "sender", now, 5*time.Minute, time.Minute, func(batch []domain.TaskAlert) *domain.Message {
				if len(batch) != 50 {
					t.Errorf("first batch size %d", len(batch))
				}
				n, _ := domain.NewMessage("first", "@console", "sender", domain.TypeNote, "batch", "", "", "", now)
				return n
			})
			if err != nil {
				errs <- err
			}
			if n != nil {
				committed.Add(1)
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if committed.Load() != 1 {
		t.Fatal("multiple alert wakeups", committed.Load())
	}
	if err := r.StageTaskAlerts(ctx, alerts); err != nil {
		t.Fatal(err)
	}
	next := now.Add(5 * time.Minute)
	n, err := r.DeliverTaskAlerts(ctx, "sender", next, 5*time.Minute, time.Minute, func(batch []domain.TaskAlert) *domain.Message {
		if len(batch) != 10 {
			t.Errorf("overflow lost or repeated: %d", len(batch))
		}
		n, _ := domain.NewMessage("second", "@console", "sender", domain.TypeNote, "overflow", "", "", "", next)
		return n
	})
	if err != nil || n == nil {
		t.Fatal(n, err)
	}
	senders, err := r.PendingTaskAlertSenders(ctx)
	if err != nil || len(senders) != 0 {
		t.Fatal("pending after draining overflow", senders, err)
	}
}
