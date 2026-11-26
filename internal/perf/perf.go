// Package perf keeps how long a hub's requests and database statements took
// over the last hour, so an admin can see which one is slow. It keeps names
// (a route pattern, a statement's name), counts and durations: never
// parameters, paths with ids or message text.
package perf

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"
)

// Keep is how far back a Recorder remembers.
const Keep = time.Hour

const (
	buckets   = 64
	base      = 100 * time.Microsecond // the first bucket's upper bound
	growth    = 1.25                   // each next bucket's bound is this much larger
	keysLimit = 1000                   // distinct names a minute keeps ("other" after)
)

// Recorder collects durations by name, per minute. Safe for concurrent use;
// a nil Recorder records nothing.
type Recorder struct {
	mu      sync.Mutex
	minutes map[int64]map[string]*hist
	now     func() time.Time
}

type hist struct {
	n, rows, maxRows int64
	max              time.Duration
	b                [buckets]uint32
}

// New returns an empty Recorder.
func New() *Recorder { return &Recorder{minutes: map[int64]map[string]*hist{}, now: time.Now} }

func bucket(d time.Duration) int {
	if d <= base {
		return 0
	}
	i := int(math.Ceil(math.Log(float64(d)/float64(base)) / math.Log(growth)))
	return min(i, buckets-1)
}

// bound is the upper bound of bucket i.
func bound(i int) time.Duration {
	return time.Duration(float64(base) * math.Pow(growth, float64(i)))
}

// Add records one request or statement: how long it took and how many rows
// it read or changed (0 when that means nothing).
func (r *Recorder) Add(name string, d time.Duration, rows int64) {
	if r == nil {
		return
	}
	now := r.now()
	m := now.Unix() / 60
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := r.minutes[m]
	if keys == nil {
		keys = map[string]*hist{}
		r.minutes[m] = keys
		for old := range r.minutes {
			if old <= m-int64(Keep/time.Minute) {
				delete(r.minutes, old)
			}
		}
	}
	h := keys[name]
	if h == nil {
		if len(keys) >= keysLimit {
			name = "other"
			h = keys[name]
		}
		if h == nil {
			h = &hist{}
			keys[name] = h
		}
	}
	h.n++
	h.rows += rows
	h.maxRows = max(h.maxRows, rows)
	h.max = max(h.max, d)
	h.b[bucket(d)]++
}

// Stat is one name's numbers over a window.
type Stat struct {
	Name          string
	Count         int64
	P50, P95, Max time.Duration
	// Total is the time all of them took together.
	Total time.Duration
	// Rows: on average and at most (statements).
	AvgRows, MaxRows int64
}

// Report sums the last `window` (at most Keep), slowest first by p95, then
// by max. limit > 0 keeps that many.
func (r *Recorder) Report(window time.Duration, limit int) []Stat {
	if r == nil {
		return nil
	}
	window = min(max(window, time.Minute), Keep)
	now := r.now().Unix() / 60
	from := now - int64(window/time.Minute) + 1
	sum := map[string]*hist{}
	r.mu.Lock()
	for m, keys := range r.minutes {
		if m < from || m > now {
			continue
		}
		for name, h := range keys {
			s := sum[name]
			if s == nil {
				s = &hist{}
				sum[name] = s
			}
			s.n += h.n
			s.rows += h.rows
			s.maxRows = max(s.maxRows, h.maxRows)
			s.max = max(s.max, h.max)
			for i, c := range h.b {
				s.b[i] += c
			}
		}
	}
	r.mu.Unlock()
	out := make([]Stat, 0, len(sum))
	for name, h := range sum {
		var total time.Duration
		for i, c := range h.b {
			total += time.Duration(c) * min(bound(i), h.max)
		}
		out = append(out, Stat{Name: name, Count: h.n, P50: h.quantile(0.5), P95: h.quantile(0.95), Max: h.max,
			Total: total, AvgRows: h.rows / max(h.n, 1), MaxRows: h.maxRows})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].P95 != out[j].P95 {
			return out[i].P95 > out[j].P95
		}
		if out[i].Max != out[j].Max {
			return out[i].Max > out[j].Max
		}
		return out[i].Name < out[j].Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// quantile: the upper bound of the bucket holding it (never above the max).
func (h *hist) quantile(q float64) time.Duration {
	want := int64(math.Ceil(q * float64(h.n)))
	var seen int64
	for i, c := range h.b {
		seen += int64(c)
		if seen >= want && c > 0 {
			return min(bound(i), h.max)
		}
	}
	return h.max
}

type routeKey struct{}

type requestInfo struct {
	route   string
	waiting bool
}

// WithRoute gives the request's context a place for the route pattern that
// matched it, filled by SetRoute deeper in, read back with Route.
func WithRoute(ctx context.Context) context.Context {
	return context.WithValue(ctx, routeKey{}, new(requestInfo))
}

// SetRoute records the pattern that matched (no-op without WithRoute).
func SetRoute(ctx context.Context, pattern string) {
	if p, ok := ctx.Value(routeKey{}).(*requestInfo); ok && pattern != "" {
		p.route = pattern
	}
}

// Route is the pattern SetRoute recorded, or "".
func Route(ctx context.Context) string {
	if p, ok := ctx.Value(routeKey{}).(*requestInfo); ok {
		return p.route
	}
	return ""
}

// MarkWaiting marks a request that waits on purpose after its handler has
// decoded wait parameters (which may be in the body, not the URL).
func MarkWaiting(ctx context.Context) {
	if p, ok := ctx.Value(routeKey{}).(*requestInfo); ok {
		p.waiting = true
	}
}

// Waiting reports the handler's explicit long-poll marker.
func Waiting(ctx context.Context) bool {
	p, ok := ctx.Value(routeKey{}).(*requestInfo)
	return ok && p.waiting
}

// Timings is what a hub keeps: its requests (by route) and its database
// statements (by name).
type Timings struct{ Requests, Statements *Recorder }

// NewTimings returns empty Timings.
func NewTimings() *Timings { return &Timings{Requests: New(), Statements: New()} }

// RequestsOrNil and StatementsOrNil: the recorder, or nil for nil Timings.
func (t *Timings) RequestsOrNil() *Recorder {
	if t == nil {
		return nil
	}
	return t.Requests
}

// StatementsOrNil: see RequestsOrNil.
func (t *Timings) StatementsOrNil() *Recorder {
	if t == nil {
		return nil
	}
	return t.Statements
}
