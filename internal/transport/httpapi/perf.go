package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/perf"
)

// SetPerf gives GET /v1/admin/perf what it reports (nil: not available).
func (s *Server) SetPerf(p *perf.Timings) { s.perf = p }

// SetDatabaseStats installs a nonblocking snapshot of this workspace's
// primary pool. It must not acquire a database connection.
func (s *Server) SetDatabaseStats(f func() api.DatabasePoolStats) { s.databaseStats = f }

// routePattern: the pattern that matched, with its method.
func routePattern(r *http.Request) string {
	if r.Pattern == "" || strings.Contains(r.Pattern, " ") {
		return r.Pattern
	}
	return r.Method + " " + r.Pattern
}

// GET /v1/admin/perf?minutes=15&limit=20
func (s *Server) adminPerf(w http.ResponseWriter, r *http.Request) {
	if s.perf == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "this hub does not keep timings"))
		return
	}
	minutes, limit := 15, 20
	q := r.URL.Query()
	if v := q.Get("minutes"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > int(perf.Keep/time.Minute) {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "minutes must be 1-60"))
			return
		}
		minutes = n
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "limit must be 1-500"))
			return
		}
		limit = n
	}
	window := time.Duration(minutes) * time.Minute
	out := api.PerfReport{Minutes: minutes, Requests: perfStats(s.perf.Requests.Report(window, limit)),
		Statements: perfStats(s.perf.Statements.Report(window, limit))}
	if s.databaseStats != nil {
		stats := s.databaseStats()
		out.Database = &stats
	}
	writeJSON(w, http.StatusOK, out)
}

func perfStats(in []perf.Stat) []api.PerfStat {
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	out := []api.PerfStat{}
	for _, st := range in {
		out = append(out, api.PerfStat{Name: st.Name, Count: st.Count, P50MS: ms(st.P50), P95MS: ms(st.P95), MaxMS: ms(st.Max),
			TotalMS: ms(st.Total), AvgRows: st.AvgRows, MaxRows: st.MaxRows})
	}
	return out
}
