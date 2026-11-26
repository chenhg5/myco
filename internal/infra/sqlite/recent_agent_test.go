package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

func TestAgentRecentPreservesOrderLimitsAndSelfMessages(t *testing.T) {
	ctx := context.Background()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewMessageRepo(db)
	base := time.Unix(1_800_000_000, 0)
	bodies := map[string]string{}
	for i := 0; i < 120; i++ {
		from, to := domain.AgentName("target"), domain.AgentName("peer")
		switch i % 4 {
		case 1:
			from, to = to, from
		case 2:
			to = from
		case 3:
			from = "other"
		}
		id := fmt.Sprintf("msg_%03d", i)
		body := strings.Repeat("example payload ", 300) + id
		m, e := domain.NewMessage(id, from, to, domain.TypeTask, body, "", "", "", base.Add(time.Duration(i)*time.Second))
		if e != nil {
			t.Fatal(e)
		}
		m.UpdatedAt = base.Add(time.Duration((i*13)%7) * time.Hour)
		if e = repo.Create(ctx, m); e != nil {
			t.Fatal(e)
		}
		bodies[id] = body
	}
	check := func(agent domain.AgentName, limit int) {
		t.Helper()
		rows, e := db.QueryContext(ctx, `SELECT id FROM messages WHERE from_agent=? OR to_agent=? ORDER BY updated_at DESC,id LIMIT ?`, agent, agent, limit)
		if e != nil {
			t.Fatal(e)
		}
		want := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				t.Fatal(e)
			}
			want = append(want, id)
		}
		if e = rows.Err(); e != nil {
			t.Fatal(e)
		}
		rows.Close()
		got, e := repo.ListRecent(ctx, agent, limit)
		if e != nil {
			t.Fatal(e)
		}
		ids := []string{}
		for _, m := range got {
			ids = append(ids, m.ID)
			if m.Body != bodies[m.ID] {
				t.Fatal("message body changed")
			}
		}
		if !reflect.DeepEqual(ids, want) {
			t.Fatalf("agent=%s limit=%d: got %v want %v", agent, limit, ids, want)
		}
	}
	for _, a := range []domain.AgentName{"target", "peer", "missing"} {
		for _, n := range []int{0, 1, 2, 17, 200, -1} {
			check(a, n)
		}
	}
	_, err = repo.Mutate(ctx, "msg_000", func(m *domain.Message) ([]*domain.Message, error) {
		m.UpdatedAt = base.Add(100 * time.Hour)
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	check("target", 17)
}

func TestAgentRecentIndexesUpgradeAndBoundEachSide(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	existing, e := domain.NewMessage("existing", "target", "peer", domain.TypeTask, "kept data", "", "", "", time.Unix(1_800_000_000, 0))
	if e != nil {
		t.Fatal(e)
	}
	if e = NewMessageRepo(db).Create(t.Context(), existing); e != nil {
		t.Fatal(e)
	}
	// Simulate a pre-upgrade database; reopening must add both indexes.
	for _, name := range []string{"idx_messages_from_updated", "idx_messages_to_updated", "idx_messages_recent", "idx_messages_thread_parties"} {
		if _, err = db.Exec("DROP INDEX " + name); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, name := range []string{"idx_messages_recent", "idx_messages_thread_parties"} {
		var count int
		if e := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&count); e != nil || count != 1 {
			t.Fatalf("upgrade missing %s: count=%d err=%v", name, count, e)
		}
	}
	kept, e := NewMessageRepo(db).ListRecent(t.Context(), "target", 20)
	if e != nil || len(kept) != 1 || kept[0].ID != "existing" || kept[0].Body != "kept data" {
		t.Fatalf("upgrade lost existing data: rows=%d err=%v", len(kept), e)
	}
	rows, err := db.Query("EXPLAIN QUERY PLAN SELECT id FROM messages "+recentAgentWhere, "target", 20, "target", 20, 20)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type node struct {
		parent int
		detail string
	}
	nodes := map[int]node{}
	plan := []string{}
	for rows.Next() {
		var a, b, c int
		var detail string
		if err = rows.Scan(&a, &b, &c, &detail); err != nil {
			t.Fatal(err)
		}
		nodes[a] = node{b, detail}
		plan = append(plan, detail)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan, "\n")
	for _, name := range []string{"idx_messages_from_updated", "idx_messages_to_updated"} {
		if !strings.Contains(joined, "USING COVERING INDEX "+name) {
			t.Fatalf("missing bounded covering lookup %s:\n%s", name, joined)
		}
	}
	for _, step := range plan {
		if step == "SCAN messages" || strings.HasPrefix(step, "SCAN messages ") {
			t.Fatalf("unbounded message scan: %s", joined)
		}
	}
	// Co-routines supply each direction's limited, index-ordered rowids.
	// SQLite may sort those outputs to merge/deduplicate the union, and
	// sort the final bounded result. It must not sort inside a co-routine.
	routines := 0
	for _, n := range nodes {
		if strings.HasPrefix(n.detail, "CO-ROUTINE") {
			routines++
		}
	}
	if routines != 2 {
		t.Fatalf("expected two bounded direction queries:\n%s", joined)
	}
	for _, n := range nodes {
		if !strings.Contains(n.detail, "TEMP B-TREE FOR ORDER BY") {
			continue
		}
		for parent := n.parent; parent != 0; {
			ancestor, ok := nodes[parent]
			if !ok {
				break
			}
			if strings.HasPrefix(ancestor.detail, "CO-ROUTINE") {
				t.Fatalf("sorted history before limiting:\n%s", joined)
			}
			parent = ancestor.parent
		}
	}
}
