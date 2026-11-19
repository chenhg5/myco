package app

import (
	"context"
	"sync"
	"time"
)

// A task's signs of life (its recipient reads it, reports progress, ask,
// uploads its trace) are kept with the task, so the overview can tell a
// task being worked on from one left open (see OverviewService.staleAfter)
// and a restarted hub still knows. They come often (a runner reads its
// task every few seconds): written at most every touchEvery per task.
const touchEvery = 30 * time.Second

type touches struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// TouchActive records that task id's recipient is working on it now.
func (s *MessageService) TouchActive(ctx context.Context, id string) {
	now := s.clock.Now()
	s.touched.mu.Lock()
	if s.touched.last == nil {
		s.touched.last = map[string]time.Time{}
	}
	if last, ok := s.touched.last[id]; ok && now.Sub(last) < touchEvery {
		s.touched.mu.Unlock()
		return
	}
	s.touched.last[id] = now
	if len(s.touched.last) > 2000 {
		for k, t := range s.touched.last {
			if now.Sub(t) > time.Hour {
				delete(s.touched.last, k)
			}
		}
	}
	s.touched.mu.Unlock()
	_ = s.messages.TouchActive(ctx, id, now)
}
