package cli

import (
	"context"
	"sync"
)

type forkWorker struct {
	cancel context.CancelFunc
	done   chan struct{}
}
type forkWorkers struct {
	mu      sync.Mutex
	running map[string]*forkWorker
	blocked map[string]bool
}

func newForkWorkers() *forkWorkers {
	return &forkWorkers{running: map[string]*forkWorker{}, blocked: map[string]bool{}}
}
func forkWorkerKey(path, token string) string { return path + "\x00" + token }
func (s *forkWorkers) add(path, token string, cancel context.CancelFunc, done chan struct{}) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := forkWorkerKey(path, token)
	if s.blocked[key] {
		return false
	}
	s.running[key] = &forkWorker{cancel, done}
	return true
}
func (s *forkWorkers) remove(path, token string) {
	s.mu.Lock()
	delete(s.running, forkWorkerKey(path, token))
	s.mu.Unlock()
}
func (s *forkWorkers) stop(ctx context.Context, path, token string) error {
	s.mu.Lock()
	key := forkWorkerKey(path, token)
	s.blocked[key] = true
	w := s.running[key]
	s.mu.Unlock()
	if w == nil {
		return nil
	}
	w.cancel()
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *forkWorkers) stopPath(ctx context.Context, path string) error {
	s.mu.Lock()
	wait := []*forkWorker{}
	for key, w := range s.running {
		if len(key) > len(path) && key[:len(path)+1] == path+"\x00" {
			s.blocked[key] = true
			wait = append(wait, w)
		}
	}
	s.mu.Unlock()
	for _, w := range wait {
		w.cancel()
	}
	for _, w := range wait {
		select {
		case <-w.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
