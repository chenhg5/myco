package loadtest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func (w *workload) observe(ctx context.Context, dir string, pid int) {
	file, e := os.OpenFile(filepath.Join(dir, "samples.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		w.stop("observation_open_failed")
		return
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	var mu sync.Mutex
	emit := func(v any) bool {
		mu.Lock()
		e := enc.Encode(v)
		mu.Unlock()
		if e != nil {
			w.stop("observation_write_failed")
			return false
		}
		return true
	}
	// The last pool sample includes durations of canceled, previously pending waits.
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if p, e := w.f.admin.c.Perf(c, 1, 50); e == nil {
			emit(map[string]any{"type": "perf", "at": time.Now().UTC(), "phase": "final", "report": p})
		}
		emit(map[string]any{"type": "client_interval", "at": time.Now().UTC(), "phase": "final", "operations": w.interval.take()})
	}()
	resources, stopResources := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); w.observeResources(resources, dir, pid, emit) }()
	defer func() { stopResources(); <-done }()
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	var badSince time.Time
	for {
		at := time.Now()
		phase := w.phase(at)
		pc, cancel := context.WithTimeout(ctx, w.config.RequestTimeout)
		p, e := w.f.admin.c.Perf(pc, 1, 50)
		cancel()
		if e == nil {
			if p.Database == nil {
				w.stop("pool_metrics_missing")
			}
			if !emit(map[string]any{"type": "perf", "at": time.Now().UTC(), "phase": phase, "report": p}) {
				return
			}
		} else if ctx.Err() == nil {
			w.stop("perf_collection_failed")
		}
		ops := w.interval.take()
		if !emit(map[string]any{"type": "client_interval", "at": time.Now().UTC(), "phase": phase, "operations": ops}) {
			return
		}
		w.mu.Lock()
		tasks := w.summary
		w.mu.Unlock()
		if e = writeJSON(filepath.Join(dir, "progress.json"), map[string]any{"at": time.Now().UTC(), "phase": phase, "scheduled": w.scheduled.Load(), "admitted": w.admitted.Load(), "dropped": w.dropped.Load(), "tasks": tasks, "operations": w.stats.snapshot()}); e != nil {
			w.stop("progress_write_failed")
		}
		var count, fail uint64
		bad := false
		for _, op := range ops {
			if op.Phase != "steady" || strings.Contains(op.Name, "intentional wait") || strings.HasPrefix(op.Name, "task ") || op.Name == "scheduler_dispatch" {
				continue
			}
			count += op.Latency.Count
			for _, v := range op.Errors {
				fail += v
			}
			if op.Latency.Count >= 20 && op.Latency.P99 > 5000 {
				bad = true
			}
		}
		if count > 0 && float64(fail)/float64(count) > .01 {
			bad = true
		}
		if w.dropped.Load() > 0 {
			bad = true
		}
		if phase == "steady" && bad {
			if badSince.IsZero() {
				badSince = time.Now()
			}
			if time.Since(badSince) >= 30*time.Second {
				w.stop("sustained_error_latency_or_client_saturation")
			}
		} else {
			badSince = time.Time{}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// This sampler is independent of HTTP perf collection. A stalled metrics call
// must not smooth away the disk/CPU sample at the time of a write spike.
func (w *workload) observeResources(ctx context.Context, dir string, pid int, emit func(any) bool) {
	var previous *resourceSample
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		at := time.Now()
		res := readResources(pid, dir, previous)
		previous = &res
		wal := fileBytes(filepath.Join(dir, "hub.db-wal"))
		if wal > w.peakWAL.Load() {
			w.peakWAL.Store(wal)
		}
		if !emit(map[string]any{"type": "resources", "at": at.UTC(), "phase": w.phase(at), "sample": res, "active_polls": w.activePolls.Load(), "wal_bytes": wal, "db_bytes": fileBytes(filepath.Join(dir, "hub.db"))}) {
			return
		}
		if res.DiskFree != nil && *res.DiskFree < 5<<30 {
			w.stop("disk_free_below_5GiB")
		}
		if res.DiskFree != nil && res.DiskTotal != nil && *res.DiskTotal > 0 && float64(*res.DiskFree)/float64(*res.DiskTotal) < .2 {
			w.stop("disk_free_below_20_percent")
		}
		if res.Hub.RSSBytes > 2<<30 {
			w.stop("hub_rss_above_2GiB")
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
