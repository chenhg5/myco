package api

// PerfReport answers GET /v1/admin/perf: how long the hub's requests and
// database statements took over the last Minutes, slowest first (by p95).
// Only names and numbers: route patterns and statement names, no ids,
// parameters or message text.
type PerfReport struct {
	Minutes    int        `json:"minutes"`
	Requests   []PerfStat `json:"requests"`
	Statements []PerfStat `json:"statements"`
}

// PerfStat is one route's or one statement's numbers. Durations are in
// milliseconds; percentiles are estimated (within 25% above the true value,
// never above the maximum).
type PerfStat struct {
	// Name: a route ("GET /v1/threads/{id}") or a statement
	// ("MessageRepo.ListThread: SELECT messages #1a2b").
	Name    string  `json:"name"`
	Count   int64   `json:"count"`
	P50MS   float64 `json:"p50_ms"`
	P95MS   float64 `json:"p95_ms"`
	MaxMS   float64 `json:"max_ms"`
	TotalMS float64 `json:"total_ms"`
	// Rows a statement read or changed: on average and at most.
	AvgRows int64 `json:"avg_rows,omitempty"`
	MaxRows int64 `json:"max_rows,omitempty"`
}
