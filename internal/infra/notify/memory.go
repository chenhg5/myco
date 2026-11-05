// Package notify provides Notifier implementations.
package notify

import "sync"

// Memory is an in-process Notifier. Each Publish wakes every current
// subscriber of the topic exactly once.
type Memory struct {
	mu     sync.Mutex
	topics map[string]map[*sub]struct{}
}

type sub struct {
	ch   chan struct{}
	once sync.Once
}

// NewMemory returns an empty in-process notifier.
func NewMemory() *Memory { return &Memory{topics: map[string]map[*sub]struct{}{}} }

// Subscribe registers interest in the next Publish on topic.
func (m *Memory) Subscribe(topic string) (<-chan struct{}, func()) {
	s := &sub{ch: make(chan struct{})}
	m.mu.Lock()
	if m.topics[topic] == nil {
		m.topics[topic] = map[*sub]struct{}{}
	}
	m.topics[topic][s] = struct{}{}
	m.mu.Unlock()
	return s.ch, func() {
		m.mu.Lock()
		if subs := m.topics[topic]; subs != nil {
			delete(subs, s)
			if len(subs) == 0 {
				delete(m.topics, topic)
			}
		}
		m.mu.Unlock()
	}
}

// Publish wakes all current subscribers of topic.
func (m *Memory) Publish(topic string) {
	m.mu.Lock()
	subs := m.topics[topic]
	delete(m.topics, topic)
	m.mu.Unlock()
	for s := range subs {
		s.once.Do(func() { close(s.ch) })
	}
}
