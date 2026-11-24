package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

func TestPageQuerySQLiteMatchesFallback(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewPageDataRepo(db)
	bodies := []string{
		`{"n":2,"status":"progress","assignee":"alex","created":"2026-11-25T09:00:00+08:00","flag":true,"value":null}`,
		`{"n":1,"status":"todo","assignee":null,"created":"2026-11-25T01:00:00Z","flag":false}`,
		`{"n":2,"status":"done","assignee":"alex","created":"2026-11-24","value":3}`,
		`{"n":10,"status":"progress","created":"2026-11-26T01:00:00Z","flag":1,"value":"3"}`,
		`{"status":"progress","assignee":"other","created":"not a date","value":false}`,
		`{"n":null,"created":"2026","value":["a", 1]}`,
		`{"n":9007199254740993,"created":"2026-11-24T22:00:00-03:00","value":{"z":1, "a":2}}`,
		`{"n":9007199254740992,"created":"2026-02-30"}`,
		`{"created":"2026-11-25T24:00:00Z"}`,
	}
	for i, body := range bodies {
		rec := &domain.PageRecord{Page: "pg_test", Collection: "issues", ID: fmt.Sprint("i", i), Data: body, UpdatedAt: time.Now()}
		if _, err := repo.Put(context.Background(), rec, 0); err != nil {
			t.Fatal(err)
		}
	}
	records, _ := repo.Records(context.Background(), "pg_test", "issues")
	seq, _ := repo.Seq(context.Background(), "pg_test")
	cases := []domain.PageQueryInput{
		{}, {Fields: "n,status", Sort: "-n", Limit: 2}, {Sort: "n"}, {Sort: "-n,status"}, {Sort: "value"}, {Sort: "-value"},
		{Where: []string{"status:progress"}}, {Where: []string{"status:ne:progress"}},
		{Where: []string{"assignee:alex", "status:in:progress,done"}}, {Where: []string{`n:in:[1,10,null]`}},
		{Where: []string{"assignee:exists"}}, {Where: []string{"assignee:exists:false"}},
		{Where: []string{"status:eq:x' OR 1=1--"}}, {Where: []string{"value:null"}}, {Where: []string{`value:"3"`}}, {Where: []string{"value:ne:null"}},
		{Where: []string{"n:gt:1"}}, {Where: []string{"n:gte:2"}}, {Where: []string{"n:lt:10"}}, {Where: []string{"n:lte:2"}},
		{Where: []string{"flag:true"}}, {Where: []string{"flag:1"}},
		{Where: []string{"created:gte:2026-11-25T01:00:00Z"}}, {Where: []string{"created:lt:2026-11-25"}}, {Where: []string{"created:gte:2026-11-26"}},
		{Where: []string{"status:progress"}, Count: true, Limit: 1}, {Where: []string{"n:gt:9999999999999999"}, Count: true},
	}
	for _, in := range cases {
		t.Run(fmt.Sprint(in), func(t *testing.T) {
			q, e := domain.ParsePageQuery(in)
			if e != nil {
				t.Fatal(e)
			}
			got, e := repo.Query(context.Background(), "pg_test", "issues", q)
			if e != nil {
				t.Fatal(e)
			}
			want, e := domain.ApplyPageQuery(records, "pg_test", "issues", seq, q)
			if e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("SQL/local mismatch:\nSQL %+v\nlocal %+v\nIDs %v / %v", got, want, queryIDs(got), queryIDs(want))
			}
		})
	}
	q, _ := domain.ParsePageQuery(domain.PageQueryInput{Sort: "-n", Limit: 2, Fields: "n"})
	seen := map[string]bool{}
	for {
		result, err := repo.Query(context.Background(), "pg_test", "issues", q)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range result.Records {
			if seen[r.ID] {
				t.Fatal("duplicate across pages")
			}
			seen[r.ID] = true
			if r.Version != 1 {
				t.Fatal("lost version")
			}
		}
		if result.Next == "" {
			break
		}
		q.After = result.Next
	}
	if len(seen) != len(records) {
		t.Fatal("pagination skipped records")
	}
	firstQ, _ := domain.ParsePageQuery(domain.PageQueryInput{Sort: "-n", Limit: 2})
	first, _ := repo.Query(context.Background(), "pg_test", "issues", firstQ)
	wrong := firstQ
	wrong.Sort = append([]domain.RecordSort(nil), firstQ.Sort...)
	wrong.After = first.Next
	wrong.Sort[0].Desc = false
	if _, err := repo.Query(context.Background(), "pg_test", "issues", wrong); !domain.IsCode(err, domain.CodeInvalidArgument) {
		t.Fatalf("cross-query cursor accepted: %v", err)
	}
	for _, scope := range [][2]string{{"pg_other", "issues"}, {"pg_test", "comments"}} {
		cross := firstQ
		cross.After = first.Next
		if _, err := repo.Query(context.Background(), scope[0], scope[1], cross); !domain.IsCode(err, domain.CodeInvalidArgument) {
			t.Fatalf("cross-page/collection cursor accepted: %v", err)
		}
	}
	firstQ.After = first.Next
	repo.Put(context.Background(), &domain.PageRecord{Page: "pg_test", Collection: "issues", ID: "new", Data: `{"n":50}`, UpdatedAt: time.Now()}, 0)
	if _, err := repo.Query(context.Background(), "pg_test", "issues", firstQ); !domain.IsCode(err, domain.CodeConflict) {
		t.Fatalf("stale cursor accepted: %v", err)
	}
}
func queryIDs(r domain.RecordQueryResult) []string {
	out := []string{}
	for _, v := range r.Records {
		out = append(out, v.ID+":"+v.Data)
	}
	return out
}

func TestPageQueryTenThousandRecords(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, _ := db.Begin()
	st, err := tx.Prepare(`INSERT INTO page_records(page,collection,id,data,version,updated_at,pos) VALUES('pg_perf','issues',?,?,1,0,?)`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 10000; i++ {
		b, _ := json.Marshal(map[string]any{"n": i, "title": fmt.Sprint("Task ", i), "status": []string{"todo", "progress", "done"}[i%3], "assignee": fmt.Sprint("agent", i%10), "desc": strings.Repeat("demo content ", 80)})
		if _, err := st.Exec(fmt.Sprint("i", i), string(b), i); err != nil {
			t.Fatal(err)
		}
	}
	st.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	repo := NewPageDataRepo(db)
	for _, in := range []domain.PageQueryInput{{Fields: "n,title", Sort: "-n", Limit: 1}, {Where: []string{"assignee:agent1", "status:progress"}, Count: true}, {Fields: "n,status", Sort: "-n", Limit: 100}} {
		q, _ := domain.ParsePageQuery(in)
		start := time.Now()
		r, err := repo.Query(context.Background(), "pg_perf", "issues", q)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("10k records, query %+v: %s, returned %d records, count=%v", in, time.Since(start), len(r.Records), func() any {
			if r.Count != nil {
				return *r.Count
			}
			return nil
		}())
		if in.Limit == 1 && (len(r.Records) != 1 || !strings.Contains(r.Records[0].Data, `10000`)) {
			t.Fatal("wrong maximum number")
		}
	}
	plan, err := db.Query(`EXPLAIN QUERY PLAN SELECT id FROM page_records WHERE page='pg_perf' AND collection='issues' ORDER BY (json_type(data,'$.n') IS NULL),json_extract(data,'$.n') DESC,collection,pos,id LIMIT 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	details := ""
	for plan.Next() {
		var a, b, c int
		var d string
		plan.Scan(&a, &b, &c, &d)
		details += d
	}
	if !strings.Contains(details, "idx_page_records_n") || strings.Contains(details, "TEMP B-TREE") {
		t.Fatalf("maximum-number query lost its index: %s", details)
	}
	t.Log(details)
}
