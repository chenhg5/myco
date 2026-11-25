package main

import (
	"context"
	"errors"
	"github.com/go-rod/rod/lib/proto"
)

type observation struct {
	Sequence    uint64
	Kind        string
	Target      proto.TargetTargetID
	Session     proto.TargetSessionID
	Opener      proto.TargetTargetID
	OpenerFrame proto.PageFrameID
	ID          string
	Filename    string
	Error       string
}

func (s *session) observe(event observation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sequence++
	event.Sequence = s.sequence
	s.observations = append(s.observations, event)
	if len(s.observations) > 1024 {
		s.observations = append([]observation(nil), s.observations[len(s.observations)-1024:]...)
	}
	if s.observed != nil {
		close(s.observed)
	}
	s.observed = make(chan struct{})
}
func (s *session) cursor() uint64 { s.mu.Lock(); defer s.mu.Unlock(); return s.sequence }
func (s *session) waitObserved(ctx context.Context, after uint64, match func(observation) bool) (observation, error) {
	for {
		s.mu.Lock()
		if len(s.observations) > 0 && after > 0 && s.observations[0].Sequence > after+1 {
			s.mu.Unlock()
			return observation{}, errors.New("observation_lost")
		}
		for _, evt := range s.observations {
			if evt.Sequence > after && match(evt) {
				s.mu.Unlock()
				return evt, nil
			}
		}
		if s.observed == nil {
			s.observed = make(chan struct{})
		}
		changed := s.observed
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return observation{}, ctx.Err()
		case <-s.ctx.Done():
			return observation{}, s.ctx.Err()
		case <-changed:
		}
	}
}
