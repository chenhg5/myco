package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPerfPoolStatsAndOlderHub(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		pool       bool
	}{
		{"older hub", `{"minutes":1,"requests":[],"statements":[]}`, false},
		{"pool stats", `{"minutes":1,"requests":[],"statements":[],"database":{"at":"2026-11-14T12:00:10Z","started_at":"2026-11-14T12:00:00Z","max_open_connections":1,"open_connections":1,"in_use":1,"idle":0,"wait_count":7,"wait_duration_ms":123.5}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/admin/perf" || r.URL.Query().Get("minutes") != "1" {
					t.Error("wrong perf request")
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c, err := New(srv.URL, "demo-token")
			if err != nil {
				t.Fatal(err)
			}
			out, err := c.Perf(t.Context(), 1, 20)
			if err != nil {
				t.Fatal(err)
			}
			if (out.Database != nil) != tc.pool {
				t.Fatalf("pool presence: %+v", out.Database)
			}
			if tc.pool && (out.Database.WaitCount != 7 || out.Database.WaitDurationMS != 123.5 || out.Database.StartedAt.IsZero()) {
				t.Fatalf("pool data lost: %+v", out.Database)
			}
		})
	}
}
