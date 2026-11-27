package loadtest

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/client"
)

// A bounded logarithmic histogram. Percentiles are bucket upper bounds (10%
// resolution, 1 microsecond floor), never unbounded per-request arrays.
type histogram struct {
	bins       [256]uint64
	N          uint64
	Total, Max float64
}

func (h *histogram) add(d time.Duration) {
	v := float64(d) / float64(time.Millisecond)
	i := int(math.Ceil(math.Log1p(v*1000) / math.Log(1.1)))
	i = max(0, min(255, i))
	h.bins[i]++
	h.N++
	h.Total += v
	h.Max = max(h.Max, v)
}
func (h histogram) percentile(p float64) float64 {
	if h.N == 0 {
		return 0
	}
	need := uint64(math.Ceil(float64(h.N) * p))
	var n uint64
	for i, v := range h.bins {
		n += v
		if n >= need {
			return min(h.Max, (math.Pow(1.1, float64(i))-1)/1000)
		}
	}
	return h.Max
}

type Latency struct {
	Count uint64  `json:"count"`
	P50   float64 `json:"p50_ms_upper"`
	P95   float64 `json:"p95_ms_upper"`
	P99   float64 `json:"p99_ms_upper"`
	Max   float64 `json:"max_ms"`
	Total float64 `json:"total_ms"`
}

func (h histogram) view() Latency {
	return Latency{h.N, h.percentile(.5), h.percentile(.95), h.percentile(.99), h.Max, h.Total}
}

type operation struct {
	lat, lag histogram
	Errors   map[string]uint64
	Canceled uint64
}
type Operation struct {
	Phase       string            `json:"phase"`
	Name        string            `json:"name"`
	Latency     Latency           `json:"latency"`
	ScheduleLag Latency           `json:"schedule_lag"`
	Errors      map[string]uint64 `json:"errors"`
	Canceled    uint64            `json:"shutdown_canceled"`
}
type statistics struct {
	mu  sync.Mutex
	ops map[string]*operation
}

func (s *statistics) record(phase, name string, lag, elapsed time.Duration, e error, shutdown bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := phase + "\n" + name
	if s.ops == nil {
		s.ops = map[string]*operation{}
	}
	v := s.ops[key]
	if v == nil {
		v = &operation{Errors: map[string]uint64{}}
		s.ops[key] = v
	}
	v.lat.add(elapsed)
	if lag >= 0 {
		v.lag.add(lag)
	}
	if e != nil {
		if shutdown && errors.Is(e, context.Canceled) {
			v.Canceled++
		} else {
			code := "transport"
			var ve validationError
			if errors.As(e, &ve) {
				code = "validation"
			}
			var ce *client.Error
			if errors.As(e, &ce) {
				code = ce.Code
			}
			if errors.Is(e, context.DeadlineExceeded) {
				code = "deadline"
			}
			v.Errors[code]++
		}
	}
}
func (s *statistics) snapshot() []Operation {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Operation{}
	for key, v := range s.ops {
		at := 0
		for key[at] != '\n' {
			at++
		}
		errs := map[string]uint64{}
		for k, n := range v.Errors {
			errs[k] = n
		}
		out = append(out, Operation{Phase: key[:at], Name: key[at+1:], Latency: v.lat.view(), ScheduleLag: v.lag.view(), Errors: errs, Canceled: v.Canceled})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Phase == out[j].Phase {
			return out[i].Name < out[j].Name
		}
		return out[i].Phase < out[j].Phase
	})
	return out
}

// take resets interval counters under the same lock used by recording.
func (s *statistics) take() []Operation {
	s.mu.Lock()
	old := s.ops
	s.ops = nil
	s.mu.Unlock()
	x := statistics{ops: old}
	return x.snapshot()
}

// Acceptance is distinct from HTTP success. Small samples are not promoted to
// reliable tail estimates; report their counts and retain their exact maxima.
func assess(ops []Operation) []string {
	var failures []string
	for _, op := range ops {
		if op.Phase != "steady" {
			continue
		}
		if op.Name == "scheduler_dispatch" {
			if op.ScheduleLag.Count >= 20 && op.ScheduleLag.P95 > 100 {
				failures = append(failures, "generator_schedule_lag_p95_above_100ms")
			}
			continue
		}
		if strings.Contains(op.Name, "intentional wait") || op.Name == "scheduler_drop" {
			continue
		}
		p95, p99 := 300.0, 1000.0
		if strings.HasPrefix(op.Name, "GET ") {
			p95, p99 = 500, 1500
		}
		if op.Name == "task scheduled-to-received" || op.Name == "task send-to-received" {
			p95, p99 = 1000, 0
		}
		if op.Name == "task scheduled-to-validated" {
			continue
		}
		if op.Latency.Count >= 20 && op.Latency.P95 > p95 {
			failures = append(failures, op.Name+": p95 above target")
		}
		if p99 > 0 && op.Latency.Count >= 100 && op.Latency.P99 > p99 {
			failures = append(failures, op.Name+": p99 above target")
		}
	}
	return failures
}
