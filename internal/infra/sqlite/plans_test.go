package sqlite

import (
	"strings"
	"testing"
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
