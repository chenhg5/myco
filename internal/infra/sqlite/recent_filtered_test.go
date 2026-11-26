package sqlite

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

func TestFilteredRecentThreadsPreserveThreadWideFiltersAndPaging(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := NewMessageRepo(db)
	base := time.Unix(1_800_000_000, 0)
	type item struct{ thread, from, to, project, body string }
	data := []item{
		{"t1", "@console:demo", "alpha", "red", "100% complete"},
		{"t1", "beta", "charlie", "blue", "later answer"},
		{"t2", "alpha", "node", "red", "node task"},
		{"t3", "@console:demo", "beta", "red", "100x complete"},
		{"t4", "outsider", "charlie", "blue", "private"},
		{"t5", "skip", "@console:demo", "red", "skipped"},
		{"t6", "@console:demo", "alpha", "blue", "console task"},
		{"t6", "alpha", "node", "blue", "node answer"},
	}
	for i, d := range data {
		m, e := domain.NewMessage(fmt.Sprintf("m%d", i), domain.AgentName(d.from), domain.AgentName(d.to), domain.TypeTask, d.body, "", "", "", base.Add(time.Duration(i)*time.Second))
		if e != nil {
			t.Fatal(e)
		}
		m.ThreadID = d.thread
		m.Project = d.project
		if e = r.Create(t.Context(), m); e != nil {
			t.Fatal(e)
		}
	}
	cases := []struct {
		name string
		f    domain.RecentFilter
		want []string
	}{
		{"visible threads", domain.RecentFilter{SenderPrefix: "@console:", Nodes: []domain.AgentName{"node"}, Skip: []domain.AgentName{"skip"}}, []string{"t3", "t1"}},
		{"scope and label can match different messages", domain.RecentFilter{Involve: []domain.AgentName{"alpha"}, Match: []domain.AgentName{"charlie"}}, []string{"t1"}},
		{"project and label can match different messages", domain.RecentFilter{Project: "red", Match: []domain.AgentName{"charlie"}}, []string{"t1"}},
		{"scope plus console", domain.RecentFilter{SenderPrefix: "@console:", Involve: []domain.AgentName{"beta"}}, []string{"t3", "t1"}},
		{"literal body search", domain.RecentFilter{Query: "100%"}, []string{"t1"}},
		{"empty permission scope", domain.RecentFilter{Involve: []domain.AgentName{}}, nil},
		{"older page excludes a whole newer thread", domain.RecentFilter{Involve: []domain.AgentName{"alpha"}, Before: ms(base.Add(7 * time.Second))}, []string{"t2", "t1"}},
		{"page boundary is exclusive", domain.RecentFilter{Before: ms(base.Add(time.Second))}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, e := r.ListRecentThreads(t.Context(), c.f)
			if e != nil {
				t.Fatal(e)
			}
			var ids []string
			for _, v := range got {
				ids = append(ids, v.ID)
			}
			if !reflect.DeepEqual(ids, c.want) {
				t.Fatalf("got %v want %v", ids, c.want)
			}
		})
	}

	// Pair counts apply filters to messages, unlike thread-wide membership.
	for _, c := range []struct {
		name    string
		f       domain.RecentFilter
		count   int
		updated int
	}{
		{"all node messages", domain.RecentFilter{Nodes: []domain.AgentName{"node"}}, 2, 7},
		{"project count", domain.RecentFilter{Nodes: []domain.AgentName{"node"}, Project: "red"}, 1, 2},
		{"literal search count", domain.RecentFilter{Nodes: []domain.AgentName{"node"}, Query: "node task"}, 1, 2},
		{"newer pair excluded", domain.RecentFilter{Nodes: []domain.AgentName{"node"}, Before: ms(base.Add(7 * time.Second))}, 0, 0},
		{"filtered older pair retained", domain.RecentFilter{Nodes: []domain.AgentName{"node"}, Project: "red", Before: ms(base.Add(7 * time.Second))}, 1, 2},
		{"permission excludes pair", domain.RecentFilter{Nodes: []domain.AgentName{"node"}, Involve: []domain.AgentName{"outsider"}}, 0, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, e := r.ListRecentPairs(t.Context(), c.f)
			if e != nil {
				t.Fatal(e)
			}
			if c.count == 0 {
				if len(got) != 0 {
					t.Fatalf("unexpected pair: %v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Other != "alpha" || got[0].Node != "node" || got[0].Messages != c.count || !got[0].UpdatedAt.Equal(base.Add(time.Duration(c.updated)*time.Second)) {
				t.Fatalf("wrong pair count/time: %v", got)
			}
		})
	}
	// Updating an old message changes the thread's latest time. Paging must
	// not leak its older rows back onto an older page.
	_, err = r.Mutate(t.Context(), "m0", func(m *domain.Message) ([]*domain.Message, error) { m.UpdatedAt = base.Add(time.Hour); return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.ListRecentThreads(t.Context(), domain.RecentFilter{Project: "red", Before: ms(base.Add(7 * time.Second))})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range got {
		if v.ID == "t1" {
			t.Fatal("updated thread appeared on older page")
		}
	}
}

// Filtered refreshes read only index metadata, including the
// permission/project/exclusion checks. A large body must not be fetched
// just to decide whether its thread or pair belongs on the page.
func TestFilteredRecentPlansStayCovering(t *testing.T) {
	db, err := OpenTimed(":memory:", Timing{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := NewMessageRepo(db)
	f := domain.RecentFilter{SenderPrefix: "@console:", Nodes: []domain.AgentName{"node"}, Skip: []domain.AgentName{"skip"}, Involve: []domain.AgentName{"alpha"}, Match: []domain.AgentName{"beta"}, Project: "demo", Before: 1, Limit: 30}
	type stmt struct {
		q    string
		args []any
	}
	var statements []stmt
	statementHook = func(q string, args []driver.NamedValue) {
		s := stmt{q: q}
		for _, a := range args {
			s.args = append(s.args, a.Value)
		}
		statements = append(statements, s)
	}
	_, e1 := r.ListRecentThreads(t.Context(), f)
	_, e2 := r.ListRecentPairs(t.Context(), f)
	statementHook = nil
	if e1 != nil || e2 != nil {
		t.Fatalf("threads=%v pairs=%v", e1, e2)
	}
	if len(statements) != 2 {
		t.Fatalf("got %d queries", len(statements))
	}
	for _, s := range statements {
		rows, e := db.Query("EXPLAIN QUERY PLAN "+s.q, s.args...)
		if e != nil {
			t.Fatal(e)
		}
		var plan []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if e = rows.Scan(&id, &parent, &unused, &detail); e != nil {
				t.Fatal(e)
			}
			plan = append(plan, detail)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			t.Fatal(e)
		}
		for _, step := range plan {
			if (strings.Contains(step, "SCAN messages") || strings.Contains(step, "SEARCH messages") || strings.Contains(step, "SEARCH t ") || strings.Contains(step, "SEARCH p ") || strings.Contains(step, "SEARCH newer ")) && !strings.Contains(step, "COVERING INDEX") {
				t.Errorf("table lookup in metadata refresh: %s\n%s", step, strings.Join(plan, "\n"))
			}
		}
		// Only the before-page check is correlated. Membership filters
		// must be materialized once, including for long invisible threads.
		if strings.Count(strings.Join(plan, "\n"), "CORRELATED") > 1 {
			t.Fatalf("membership check repeated for each message: %v", plan)
		}
		if !strings.Contains(strings.Join(plan, "\n"), "COVERING INDEX idx_messages_recent") {
			t.Fatalf("missing covering recent index: %v", plan)
		}
	}
}
