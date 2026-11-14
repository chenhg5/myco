package app

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/chenhg5/myco/internal/domain"
)

// Messages a person took out of their own view of a conversation (the
// App's 删除). Nothing is deleted: the agents keep their history, and the
// person can show them again. Kept per viewer in the hub's settings, so
// every device of theirs agrees.

// MaxHidden is how many hidden messages a viewer keeps (the oldest go).
const MaxHidden = 5000

// HiddenService keeps each viewer's hidden messages.
type HiddenService struct {
	settings domain.SettingRepository
	mu       sync.Mutex
}

func NewHiddenService(settings domain.SettingRepository) *HiddenService {
	return &HiddenService{settings: settings}
}

func hiddenKey(viewer string) string { return "hidden:" + viewer }

// Hidden lists the viewer's hidden messages, oldest first.
func (s *HiddenService) Hidden(ctx context.Context, viewer string) ([]string, error) {
	raw, err := s.settings.Get(ctx, hiddenKey(viewer))
	if err != nil || raw == "" {
		return []string{}, err
	}
	var ids []string
	if json.Unmarshal([]byte(raw), &ids) != nil || ids == nil {
		return []string{}, nil
	}
	return ids, nil
}

// Update hides and shows messages for the viewer and returns the list.
func (s *HiddenService) Update(ctx context.Context, viewer string, hide, show []string) ([]string, error) {
	if viewer == "" {
		return nil, domain.Errf(domain.CodeInvalidArgument, "no viewer")
	}
	if len(hide)+len(show) > 500 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "at most 500 messages at a time")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ids, err := s.Hidden(ctx, viewer)
	if err != nil {
		return nil, err
	}
	drop := map[string]bool{}
	for _, id := range show {
		drop[id] = true
	}
	have := map[string]bool{}
	out := make([]string, 0, len(ids)+len(hide))
	for _, id := range ids {
		if !drop[id] && !have[id] {
			have[id] = true
			out = append(out, id)
		}
	}
	for _, id := range hide {
		if id != "" && len(id) <= 128 && !drop[id] && !have[id] {
			have[id] = true
			out = append(out, id)
		}
	}
	if len(out) > MaxHidden {
		out = out[len(out)-MaxHidden:]
	}
	raw, _ := json.Marshal(out)
	if err := s.settings.Set(ctx, hiddenKey(viewer), string(raw)); err != nil {
		return nil, err
	}
	return out, nil
}
