package quota

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

// Store coalesces account sampling across agents and runner processes. It
// contains quota snapshots and a random identity salt, never provider keys.
type Store struct {
	dir  string
	salt []byte
}
type cached struct {
	Quota    api.Quota            `json:"quota"`
	Next     time.Time            `json:"next"`
	WindowAt map[string]time.Time `json:"window_at,omitempty"`
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, errors.New("quota cache unavailable")
	}
	unlock, err := lockCache(filepath.Join(dir, "identity.lock"))
	for i := 0; err != nil && i < 20; i++ {
		time.Sleep(10 * time.Millisecond)
		unlock, err = lockCache(filepath.Join(dir, "identity.lock"))
	}
	if err != nil {
		return nil, errors.New("quota cache busy")
	}
	defer unlock()
	path := filepath.Join(dir, "identity")
	salt, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		salt = make([]byte, 32)
		if _, err = rand.Read(salt); err != nil {
			return nil, err
		}
		err = os.WriteFile(path, salt, 0600)
	}
	if err != nil || len(salt) != 32 {
		return nil, errors.New("quota identity unavailable")
	}
	return &Store{dir: dir, salt: salt}, nil
}
func (s *Store) Discover(in Input) Source { return Discover(in, s.salt) }
func (s *Store) path(src Source) string   { return filepath.Join(s.dir, src.Ref+".json") }
func (s *Store) Get(ctx context.Context, src Source) api.Quota {
	path := s.path(src)
	var prev cached
	if !readJSON(path, &prev) {
		prev = cached{}
	}
	if prev.Quota.At.After(time.Now().Add(time.Minute)) || prev.Quota.CheckedAt.After(time.Now().Add(time.Minute)) {
		prev = cached{}
	}
	if prev.Quota.ProfileRef == src.Ref && time.Now().Before(prev.Next) {
		return prev.Quota
	}
	unlock, err := lockCache(path + ".lock")
	if err != nil {
		if prev.Quota.ProfileRef == src.Ref {
			return prev.Quota
		}
		return api.Quota{Source: src.Provider, Provider: src.Provider, ProfileRef: src.Ref, Plan: src.Plan, SourceKind: src.Kind, Status: "transient_error", ErrorCode: "collector_busy", CheckedAt: time.Now().UTC(), Windows: []api.QuotaWindow{}}
	}
	defer unlock()
	if !readJSON(path, &prev) {
		prev = cached{}
	}
	if prev.Quota.At.After(time.Now().Add(time.Minute)) || prev.Quota.CheckedAt.After(time.Now().Add(time.Minute)) {
		prev = cached{}
	}
	if prev.Quota.ProfileRef == src.Ref && time.Now().Before(prev.Next) {
		return prev.Quota
	}
	probe, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	q, retry := collect(probe, src)
	if src.valid != nil && !src.valid() {
		q.Status, q.ErrorCode = "transient_error", "binding_changed"
		q.At = time.Time{}
		q.Windows = []api.QuotaWindow{}
		if prev.Quota.ProfileRef == src.Ref {
			q.At, q.Windows = prev.Quota.At, prev.Quota.Windows
		}
		return q // never cache a result whose credentials changed during the read
	}
	if ctx.Err() != nil {
		return q
	}
	fresh := q.Status == "ok" && !q.At.Before(prev.Quota.At)
	if prev.Quota.ProfileRef == src.Ref && (q.Status != "ok" || q.At.Before(prev.Quota.At)) {
		q.At, q.Windows = prev.Quota.At, prev.Quota.Windows
	}
	next := q.CheckedAt.Add(5 * time.Minute)
	if q.Status != "ok" {
		next = q.CheckedAt.Add(15 * time.Minute)
	}
	if src.Kind == "cache" {
		next = q.CheckedAt.Add(time.Minute)
	}
	if retry.After(next) {
		next = retry
	}
	times := prev.WindowAt
	if fresh {
		// A full cache read can be newer than the oldest window but older than
		// a live event for another window. Preserve that newer measurement.
		old := map[string]api.QuotaWindow{}
		oldTimes := map[string]time.Time{}
		for _, w := range prev.Quota.Windows {
			key := windowKey(w)
			old[key] = w
			oldTimes[key] = prev.Quota.At
			if at := prev.WindowAt[key]; !at.IsZero() {
				oldTimes[key] = at
			}
		}
		times = map[string]time.Time{}
		observed := q.At
		for i, w := range q.Windows {
			key := windowKey(w)
			times[key] = observed
			if oldTimes[key].After(observed) {
				q.Windows[i] = old[key]
				times[key] = oldTimes[key]
			}
		}
		q.At = time.Time{}
		for _, at := range times {
			if q.At.IsZero() || at.Before(q.At) {
				q.At = at
			}
		}
	}
	_ = s.write(path, cached{Quota: q, Next: next, WindowAt: times})
	return q
}

// Event merges incremental native windows without making an untouched
// window look newly sampled. At is the oldest included observation.
func (s *Store) Event(src Source, q api.Quota) *api.Quota {
	if q.ProfileRef != src.Ref || q.At.IsZero() || len(q.Windows) == 0 {
		return nil
	}
	path := s.path(src)
	unlock, err := lockCache(path + ".lock")
	if err != nil {
		return nil
	}
	defer unlock()
	var prev cached
	if !readJSON(path, &prev) {
		prev = cached{}
	}
	if prev.Quota.At.After(time.Now().Add(time.Minute)) || prev.Quota.CheckedAt.After(time.Now().Add(time.Minute)) {
		prev = cached{}
	}
	if prev.Quota.ProfileRef == src.Ref && q.At.Before(prev.Quota.At) {
		return &prev.Quota
	}
	windows := map[string]api.QuotaWindow{}
	times := map[string]time.Time{}
	if prev.Quota.ProfileRef == src.Ref {
		for _, w := range prev.Quota.Windows {
			key := windowKey(w)
			windows[key] = w
			times[key] = prev.Quota.At
			if at := prev.WindowAt[key]; !at.IsZero() {
				times[key] = at
			}
		}
	}
	changed := false
	for _, w := range q.Windows {
		key := windowKey(w)
		if times[key].After(q.At) {
			continue
		}
		windows[key], times[key] = w, q.At
		changed = true
	}
	if !changed {
		return &prev.Quota
	}
	if len(windows) > 16 {
		return nil
	}
	keys := make([]string, 0, len(windows))
	for key := range windows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := q
	out.Windows = []api.QuotaWindow{}
	out.At = time.Time{}
	for _, key := range keys {
		out.Windows = append(out.Windows, windows[key])
		if out.At.IsZero() || times[key].Before(out.At) {
			out.At = times[key]
		}
	}
	next := time.Now().Add(5 * time.Minute)
	if src.Kind == "cache" {
		next = time.Now().Add(time.Minute)
	}
	if err = s.write(path, cached{Quota: out, Next: next, WindowAt: times}); err != nil {
		return nil
	}
	return &out
}
func windowKey(w api.QuotaWindow) string { return w.Name + "\x00" + w.ModelScope }
func (s *Store) write(path string, c cached) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.dir, "quota-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
