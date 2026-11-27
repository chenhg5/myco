package api

import "time"

// PerfReport answers GET /v1/admin/perf: how long the hub's requests and
// database statements took over the last Minutes, slowest first (by p95).
// Only names and numbers: route patterns and statement names, no ids,
// parameters or message text.
type PerfReport struct {
	Minutes    int        `json:"minutes"`
	Requests   []PerfStat `json:"requests"`
	Statements []PerfStat `json:"statements"`
	// Database is the current workspace's primary database pool, when available.
	Database *DatabasePoolStats `json:"database,omitempty"`
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

// DatabasePoolStats is a live pool snapshot. Wait counters are cumulative
// since StartedAt, independent of PerfReport.Minutes. Pending waits have
// incremented WaitCount but add their duration only when they finish.
// No database paths, workspace names or connection strings are exposed.
type DatabasePoolStats struct {
	At                 time.Time `json:"at"`
	StartedAt          time.Time `json:"started_at"`
	MaxOpenConnections int       `json:"max_open_connections"`
	OpenConnections    int       `json:"open_connections"`
	InUse              int       `json:"in_use"`
	Idle               int       `json:"idle"`
	WaitCount          int64     `json:"wait_count"`
	WaitDurationMS     float64   `json:"wait_duration_ms"`
}
