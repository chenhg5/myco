package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/quota"
)

// startQuota keeps collection independent of task arrivals and shares a local
// snapshot across every agent using the same login on this computer.
func (r *runtimeReporter) startQuota(ctx context.Context, dir string, input func(providerChoice) quota.Input, current func() providerChoice) {
	if r == nil {
		return
	}
	store, err := quota.NewStore(filepath.Join(dir, "quota"))
	if err != nil {
		if r.log != nil {
			r.log.Warn("account quota collection unavailable", "reason", "local_cache")
		}
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.quotaCancel = cancel
	r.quotaStore = store
	r.quotaInput = input
	r.mu.Unlock()
	go func() {
		tick := time.NewTicker(20 * time.Second)
		defer tick.Stop()
		for {
			ch := current()
			r.mu.Lock()
			r.choice = ch
			r.mu.Unlock()
			src := store.Discover(input(ch))
			r.bindQuota(src)
			q := store.Get(ctx, src)
			// A provider or login can change while the native/HTTP request is in
			// flight. Never publish that observation as the replacement account.
			fresh := store.Discover(input(current()))
			if ctx.Err() != nil {
				return
			}
			if src.SameLogin(fresh) {
				r.setQuota(src.Ref, &q)
			} else {
				r.bindQuota(fresh)
			}
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
}
func (r *runtimeReporter) bindQuota(src quota.Source) {
	r.mu.Lock()
	changed := r.quotaSource.Ref != src.Ref
	r.quotaSource = src
	if changed {
		r.quota = &api.Quota{Source: src.Provider, Provider: src.Provider, ProfileRef: src.Ref, Plan: src.Plan, SourceKind: src.Kind, Status: src.Status, CheckedAt: time.Now().UTC(), Windows: []api.QuotaWindow{}}
		// A pending acquisition is not a successful zero-used observation.
		if r.quota.Status == "ok" {
			r.quota.Status = "transient_error"
			r.quota.ErrorCode = "pending"
		}
	}
	r.mu.Unlock()
	if changed {
		r.flushLater()
	}
}
func (r *runtimeReporter) setQuota(ref string, q *api.Quota) {
	r.mu.Lock()
	if r.stopped || r.quotaSource.Ref != ref {
		r.mu.Unlock()
		return
	}
	// A concurrent live event is newer than a cache/native poll.
	if r.quota != nil && r.quota.At.After(q.At) && !r.quota.At.After(time.Now().Add(time.Minute)) {
		copy := *q
		copy.At, copy.Windows = r.quota.At, r.quota.Windows
		q = &copy
	}
	r.quota = q
	r.mu.Unlock()
	r.flushLater()
}
func quotaEnvironment(extra []string) map[string]string {
	out := map[string]string{}
	for _, pair := range append(os.Environ(), extra...) {
		k, v, ok := strings.Cut(pair, "=")
		if ok {
			out[k] = v
		}
	}
	return out
}

func (r *runtimeReporter) quotaModel() string { r.mu.Lock(); defer r.mu.Unlock(); return r.model }
