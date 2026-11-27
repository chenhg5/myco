package sqlite_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

func TestReassignAuditAndAttachmentGrantAreAtomic(t *testing.T) {
	c := context.Background()
	db := openMem(t)
	r := sqlite.NewMessageRepo(db)
	m := newMsg(t, "task", "sender", "old", domain.TypeTask, t0)
	m.Attachments = []domain.Attachment{{FileID: "file", Name: "demo.txt"}}
	_, err := db.Exec(`INSERT INTO files(id,name,size,sha256,owner,created_at) VALUES('file','demo.txt',1,'hash','sender',1)`)
	mustNoErr(t, err)
	mustNoErr(t, r.Create(c, m))
	_, err = db.Exec(`CREATE TRIGGER reject_reassign_audit BEFORE INSERT ON audit_log WHEN NEW.action='message.reassign' BEGIN SELECT RAISE(ABORT,'synthetic audit failure'); END`)
	mustNoErr(t, err)
	apply := func(m *domain.Message) (*domain.AuditEntry, error) {
		m.To = "new"
		m.ReassignNote = "handover"
		m.UpdatedAt = t0.Add(time.Second)
		return &domain.AuditEntry{ID: "audit", At: m.UpdatedAt, ActorName: "sender", ActorKind: "agent", Action: "message.reassign", Target: m.ID}, nil
	}
	if _, err = r.ReassignTask(c, m.ID, apply); err == nil {
		t.Fatal("audit failure must abort reassignment")
	}
	stored := must(r.Get(c, m.ID))
	if stored.To != "old" || stored.ReassignNote != "" {
		t.Fatalf("partial reassignment: %+v", stored)
	}
	var grants int
	mustNoErr(t, db.QueryRow(`SELECT COUNT(*) FROM file_access WHERE file_id='file' AND agent='new'`).Scan(&grants))
	if grants != 0 {
		t.Fatal("failed reassignment granted attachment")
	}
	_, err = db.Exec(`DROP TRIGGER reject_reassign_audit`)
	mustNoErr(t, err)
	moved := must(r.ReassignTask(c, m.ID, apply))
	if moved.ID != m.ID || moved.To != "new" || moved.Body != m.Body || len(moved.Attachments) != 1 {
		t.Fatalf("lost payload: %+v", moved)
	}
	mustNoErr(t, db.QueryRow(`SELECT COUNT(*) FROM file_access WHERE file_id='file' AND agent='new'`).Scan(&grants))
	if grants != 1 {
		t.Fatal("new recipient missing attachment grant")
	}
}

func TestReassignRacesClaimWithoutDoubleDelivery(t *testing.T) {
	c := context.Background()
	r := sqlite.NewMessageRepo(openMem(t))
	m := newMsg(t, "task", "sender", "old", domain.TypeTask, t0)
	mustNoErr(t, r.Create(c, m))
	start := make(chan struct{})
	var wg sync.WaitGroup
	var claimed []*domain.Message
	var claimErr, moveErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		claimed, claimErr = r.ClaimDue(c, "old", t0.Add(time.Second), 1, nil, func(m *domain.Message) bool { return m.Claim(t0.Add(time.Second), time.Minute, 3) })
	}()
	go func() {
		defer wg.Done()
		<-start
		_, moveErr = r.ReassignTask(c, m.ID, func(m *domain.Message) (*domain.AuditEntry, error) {
			if m.Status != domain.StatusQueued || m.EverDelivered {
				return nil, domain.Errf(domain.CodeInvalidState, "already claimed")
			}
			m.To = "new"
			return &domain.AuditEntry{ID: "audit", At: t0, ActorKind: "agent", Action: "message.reassign", Target: m.ID}, nil
		})
	}()
	close(start)
	wg.Wait()
	mustNoErr(t, claimErr)
	stored := must(r.Get(c, m.ID))
	if len(claimed) > 0 {
		if moveErr == nil || stored.To != "old" || !stored.EverDelivered {
			t.Fatalf("claimed and reassigned: %+v %v", stored, moveErr)
		}
	} else if moveErr != nil || stored.To != "new" {
		t.Fatalf("neither delivery nor reassignment: %+v %v", stored, moveErr)
	}
	if stored.EverDelivered {
		_, err := r.Mutate(c, m.ID, func(m *domain.Message) ([]*domain.Message, error) {
			m.Status = domain.StatusQueued
			m.Attempts = 0
			return nil, nil
		})
		mustNoErr(t, err)
		if !must(r.Get(c, m.ID)).EverDelivered {
			t.Fatal("recovery erased first delivery evidence")
		}
	}
}

func TestCoordinationMigrationAndMetadata(t *testing.T) {
	c := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db := must(sqlite.Open(path))
	r := sqlite.NewMessageRepo(db)
	fresh := newMsg(t, "fresh", "sender", "old", domain.TypeTask, t0)
	fresh.Body = strings.Repeat("large-body", 100000)
	mustNoErr(t, r.Create(c, fresh))
	child := newMsg(t, "child", "old", "new", domain.TypeTask, t0.Add(time.Second))
	child.ParentID = fresh.ID
	mustNoErr(t, r.Create(c, child))
	meta := must(r.TaskMetadata(c, fresh.ID))
	if meta.Body != "" {
		t.Fatal("metadata selected body")
	}
	kids := must(r.TaskChildren(c, meta, 10))
	if len(kids) != 1 || kids[0].Body != "" || kids[0].ParentID != fresh.ID {
		t.Fatalf("metadata children: %+v", kids)
	}
	// Simulate the old schema and an old recovered task with no attempts left.
	_, err := db.Exec(`UPDATE messages SET hold_wake='legacy-wake',updated_at=updated_at+1 WHERE id='child'`)
	mustNoErr(t, err)
	_, err = db.Exec(`ALTER TABLE messages DROP COLUMN ever_delivered`)
	mustNoErr(t, err)
	// A failed backfill must not leave a new column that makes the next open
	// skip the safety backfill and accept an old previously delivered task.
	_, err = db.Exec(`CREATE TRIGGER deny_coordination_migration BEFORE UPDATE ON messages WHEN NEW.id='child' BEGIN SELECT RAISE(ABORT,'synthetic migration failure'); END`)
	mustNoErr(t, err)
	if failed, err := sqlite.Open(path); err == nil {
		failed.Close()
		t.Fatal("migration fault was ignored")
	} else if !strings.Contains(err.Error(), "synthetic migration failure") {
		t.Fatalf("unexpected migration failure: %v", err)
	}
	var partial int
	mustNoErr(t, db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name='ever_delivered'`).Scan(&partial))
	if partial != 0 {
		t.Fatal("failed migration left a partially initialized delivery guard")
	}
	_, err = db.Exec(`DROP TRIGGER deny_coordination_migration`)
	mustNoErr(t, err)
	mustNoErr(t, db.Close())
	db = must(sqlite.Open(path))
	r = sqlite.NewMessageRepo(db)
	if !must(r.Get(c, "fresh")).EverDelivered || !must(r.Get(c, "child")).EverDelivered {
		t.Fatal("legacy tasks with unprovable delivery history must be protected")
	}
	post := newMsg(t, "post-migration", "sender", "old", domain.TypeTask, t0)
	mustNoErr(t, r.Create(c, post))
	_, err = db.Exec(`UPDATE messages SET updated_at=updated_at+1 WHERE id='post-migration'`)
	mustNoErr(t, err)
	mustNoErr(t, db.Close())
	db = must(sqlite.Open(path))
	defer db.Close()
	if must(sqlite.NewMessageRepo(db).Get(c, "post-migration")).EverDelivered {
		t.Fatal("reopening reran legacy backfill")
	}
}

func TestTaskChildrenFindDelegationsFromReplyContext(t *testing.T) {
	c := context.Background()
	r := sqlite.NewMessageRepo(openMem(t))
	root := newMsg(t, "root", "boss", "lead", domain.TypeTask, t0)
	reply := newMsg(t, "reply", "lead", "boss", domain.TypeReply, t0.Add(time.Second))
	reply.ReplyTo = root.ID
	child := newMsg(t, "child", "boss", "helper", domain.TypeTask, t0.Add(2*time.Second))
	child.ParentID = reply.ID
	for _, m := range []*domain.Message{root, reply, child} {
		mustNoErr(t, r.Create(c, m))
	}
	kids := must(r.TaskChildren(c, root, 10))
	if len(kids) != 1 || kids[0].ID != child.ID || kids[0].ParentID != reply.ID {
		t.Fatalf("reply delegation lost: %+v", kids)
	}
}
