package sqlite

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

// The hub runs these on every pull, reply, agent listing and overview. With
// a million stored messages a scan of the table takes 0.1-0.5 s each, on the
// hub's only connection, so each must be answered from an index whose size
// follows the open backlog (or, for the recent list, read in index order).
func TestHotMessageQueriesUseIndexes(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, c := range []struct {
		name, query, index string
		args               []any
	}{
		{"after recipients (every reply, done, fail)", afterRecipientsSQL, "idx_messages_after", []any{"msg_x"}},
		{"due recipients (dispatcher sweep)", dueRecipientsSQL, "idx_messages_open", []any{1}},
		{"open counts (agent list, overview)", openCountsSQL, "idx_messages_open", nil},
		{"acked activity (agent list, overview)", ackedActivitySQL, "idx_messages_open", nil},
		{"due for one agent (every pull)", `SELECT id FROM messages ` + dueForWhere + ` ORDER BY created_at, id LIMIT ?`, "idx_messages_open", []any{"a", 1, 20}},
		{"open for one agent (overview)", `SELECT id FROM messages ` + listOpenWhere, "idx_messages_open", []any{"a", 10}},
		{"recent (overview)", `SELECT id FROM messages ` + listRecentAllWhere, "idx_messages_updated", []any{50}},
	} {
		rows, err := db.Query(`EXPLAIN QUERY PLAN `+c.query, c.args...)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var plan []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, detail)
		}
		rows.Close()
		// The first step is how the messages themselves are read (later
		// steps are the afterReady lookup by id and sorting).
		if len(plan) == 0 || !strings.Contains(plan[0], c.index) {
			t.Errorf("%s: want %s first, plan is:\n  %s", c.name, c.index, strings.Join(plan, "\n  "))
		}
	}
}

// The recent lists and a pair's messages are read in index order and stop
// once the page is full: a GROUP BY over every message, or reading all of a
// pair's messages to sort them, grows with the hub and runs on every
// refresh of the App (34k messages: 15-45 ms; 100k: 50-200 ms).
func TestRecentListsReadIndexesInOrder(t *testing.T) {
	db, err := OpenTimed(":memory:", Timing{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := NewMessageRepo(db)
	ctx := t.Context()
	console := "@console:"
	cases := []struct {
		name string
		run  func()
		// want: every plan of a statement on messages uses one of these,
		// and none sorts or scans the table
		want []string
	}{
		{"recent (no filter)", func() { r.ListRecentThreads(ctx, domain.RecentFilter{Limit: 30}) }, []string{"idx_messages_updated"}},
		{"recent, older page", func() { r.ListRecentThreads(ctx, domain.RecentFilter{Limit: 30, Before: 1}) }, []string{"idx_messages_updated", "idx_messages_thread"}},
		{"conversations", func() {
			r.ListRecentThreads(ctx, domain.RecentFilter{SenderPrefix: console, Skip: []domain.AgentName{"n"}, Limit: 30})
		},
			[]string{"idx_messages_updated", "idx_messages_thread", "idx_messages_from", "idx_messages_inbox", "idx_messages_from_updated", "idx_messages_to_updated"}},
		{"conversations of an agent", func() {
			r.ListRecentThreads(ctx, domain.RecentFilter{SenderPrefix: console, Match: []domain.AgentName{"a"}, Limit: 30})
		}, []string{"idx_messages_thread", "idx_messages_from", "idx_messages_inbox", "idx_messages_from_updated", "idx_messages_to_updated"}},
		{"pairs", func() {
			r.ListRecentPairs(ctx, domain.RecentFilter{AllPairs: true, Skip: []domain.AgentName{"admin"}, Limit: 30})
		},
			[]string{"idx_messages_updated", "idx_messages_from"}},
		{"between", func() { r.ListBetween(ctx, "a", "b", 5, 20) }, []string{"idx_messages_from", "INTEGER PRIMARY KEY"}},
	}
	// Some rows, so that each list has pairs and threads to look at.
	t0 := time.Unix(1_800_000_000, 0)
	for i, p := range [][2]string{{"a", "b"}, {"b", "a"}, {console + "x", "a"}, {"a", "c"}} {
		m, err := domain.NewMessage(fmt.Sprintf("m%d", i), domain.AgentName(p[0]), domain.AgentName(p[1]), domain.TypeTask, "x", "", "", "", t0.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Create(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	type stmt struct {
		q    string
		args []any
	}
	for _, c := range cases {
		var got []stmt
		statementHook = func(q string, args []driver.NamedValue) {
			s := stmt{q: q}
			for _, a := range args {
				s.args = append(s.args, a.Value)
			}
			got = append(got, s)
		}
		c.run()
		statementHook = nil
		if len(got) == 0 {
			t.Fatalf("%s: ran nothing", c.name)
		}
		for _, s := range got {
			rows, err := db.Query(`EXPLAIN QUERY PLAN `+s.q, s.args...)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			var plan []string
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				plan = append(plan, detail)
			}
			rows.Close()
			for _, step := range plan {
				bad := step == "SCAN messages" || strings.HasPrefix(step, "SCAN messages ") && !strings.Contains(step, "idx_messages_updated") ||
					strings.Contains(step, "TEMP B-TREE FOR GROUP BY") || strings.Contains(step, "TEMP B-TREE FOR ORDER BY") && c.name != "conversations of an agent" && c.name != "between"
				used := !strings.Contains(step, "messages") || strings.Contains(step, "subquery")
				for _, w := range c.want {
					used = used || strings.Contains(step, w)
				}
				if bad || !used {
					t.Errorf("%s: %q in plan:\n  %s\n  for %s", c.name, step, strings.Join(plan, "\n  "), s.q)
				}
			}
		}
	}
}
