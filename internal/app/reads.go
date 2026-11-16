package app

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// How far a person has read each of their conversations (with an agent,
// by its name): the App's unread dots and the badge on its icon. Kept per
// viewer in the hub's settings, like hidden messages, so reading on one
// device clears the dot on the others.

// MaxReads is how many conversations a viewer's marks keep (the ones read
// longest ago go).
const MaxReads = 2000

// ReadService keeps each viewer's read marks.
type ReadService struct {
	settings domain.SettingRepository
	mu       sync.Mutex
}

func NewReadService(settings domain.SettingRepository) *ReadService {
	return &ReadService{settings: settings}
}

func readsKey(viewer string) string { return "reads:" + viewer }

// Reads answers the viewer's marks: conversation → read up to (RFC 3339).
func (s *ReadService) Reads(ctx context.Context, viewer string) (map[string]string, error) {
	raw, err := s.settings.Get(ctx, readsKey(viewer))
	if err != nil || raw == "" {
		return map[string]string{}, err
	}
	var marks map[string]string
	if json.Unmarshal([]byte(raw), &marks) != nil || marks == nil {
		return map[string]string{}, nil
	}
	return marks, nil
}

// Mark moves the viewer's marks forward (a mark older than the one kept
// changes nothing) and answers them all.
func (s *ReadService) Mark(ctx context.Context, viewer string, marks map[string]string) (map[string]string, error) {
	if viewer == "" {
		return nil, domain.Errf(domain.CodeInvalidArgument, "no viewer")
	}
	if len(marks) > 500 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "at most 500 conversations at a time")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out, err := s.Reads(ctx, viewer)
	if err != nil {
		return nil, err
	}
	for k, v := range marks {
		if k == "" || len(k) > 128 {
			return nil, domain.Errf(domain.CodeInvalidArgument, "a conversation is named by its agent (1 to 128 characters)")
		}
		at, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return nil, domain.Errf(domain.CodeInvalidArgument, "read marks are RFC 3339 times")
		}
		if old, err := time.Parse(time.RFC3339Nano, out[k]); err == nil && !at.After(old) {
			continue
		}
		out[k] = at.UTC().Format(time.RFC3339Nano)
	}
	if len(out) > MaxReads {
		keys := make([]string, 0, len(out))
		for k := range out {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return out[keys[i]] < out[keys[j]] })
		for _, k := range keys[:len(out)-MaxReads] {
			delete(out, k)
		}
	}
	raw, _ := json.Marshal(out)
	if err := s.settings.Set(ctx, readsKey(viewer), string(raw)); err != nil {
		return nil, err
	}
	return out, nil
}
