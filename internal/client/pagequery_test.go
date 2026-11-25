package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/myco-ai/myco/internal/api"
)

func TestPageQueryOldHubFallback(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("query_version") != "1" {
			t.Error("missing query negotiation")
		}
		json.NewEncoder(w).Encode(api.PageRecords{Seq: 5, Records: []api.PageRecord{
			{Collection: "issues", ID: "a", Version: 3, Data: json.RawMessage(`{"n":1,"status":"progress","assignee":"alex"}`)},
			{Collection: "issues", ID: "b", Version: 2, Data: json.RawMessage(`{"n":3,"status":"progress","assignee":"alex"}`)},
			{Collection: "issues", ID: "c", Version: 1, Data: json.RawMessage(`{"n":2,"status":"todo"}`)},
		}})
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "test")
	q := api.PageQuery{Fields: "n", Where: []string{"status:progress", "assignee:alex"}, Sort: "-n", Limit: 1}
	first, err := c.QueryPageRecords(context.Background(), "pg_test", "issues", q)
	if err != nil || !first.LocalFallback || len(first.Records) != 1 || first.Records[0].ID != "b" || first.Records[0].Version != 2 || string(first.Records[0].Data) != `{"n":3}` || first.Next == "" {
		t.Fatalf("first: %+v %v", first, err)
	}
	q.After = first.Next
	next, err := c.QueryPageRecords(context.Background(), "pg_test", "issues", q)
	if err != nil || len(next.Records) != 1 || next.Records[0].ID != "a" || next.Next != "" {
		t.Fatalf("next: %+v %v", next, err)
	}
	q.After = ""
	q.Count = true
	count, err := c.QueryPageRecords(context.Background(), "pg_test", "issues", q)
	if err != nil || count.Count == nil || *count.Count != 2 || len(count.Records) != 0 || !count.LocalFallback {
		t.Fatalf("count: %+v %v", count, err)
	}
	if calls != 3 {
		t.Fatalf("extra fallback fetches: %d", calls)
	}
}

func TestPageQueryDoesNotDowngradeErrors(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 409} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(status)
				w.Write([]byte(`{"error":"permission_denied","message":"denied"}`))
			}))
			defer srv.Close()
			c, _ := New(srv.URL, "test")
			_, err := c.QueryPageRecords(context.Background(), "pg_test", "issues", api.PageQuery{Count: true})
			var ce *Error
			if !errors.As(err, &ce) || ce.Status != status || calls != 1 {
				t.Fatalf("unexpected fallback: %v calls %d", err, calls)
			}
		})
	}
}
