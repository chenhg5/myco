package runner

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/chenhg5/myco/internal/api"
)

// FileSessions persists thread→session mappings as a JSON file.
type FileSessions struct {
	path string
	mu   sync.Mutex
	m    map[string]string
}

// NewFileSessions loads (or starts) a session map at path.
func NewFileSessions(path string) (*FileSessions, error) {
	s := &FileSessions{path: path, m: map[string]string{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.m); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *FileSessions) Get(thread string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[thread]
}

func (s *FileSessions) Put(thread, session string) error {
	if session == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m[thread] == session {
		return nil
	}
	s.m[thread] = session
	b, err := json.MarshalIndent(s.m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// MemorySessions is an in-memory SessionStore (tests, ephemeral runners).
type MemorySessions struct {
	mu sync.Mutex
	m  map[string]string
}

func NewMemorySessions() *MemorySessions { return &MemorySessions{m: map[string]string{}} }

func (s *MemorySessions) Get(thread string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[thread]
}

func (s *MemorySessions) Put(thread, session string) error {
	if session != "" {
		s.mu.Lock()
		s.m[thread] = session
		s.mu.Unlock()
	}
	return nil
}

// Binder persists the agent's session binding on the hub.
type Binder interface {
	Bind(ctx context.Context, b api.Binding) (*api.Agent, error)
}

// HubSessions is the SessionStore for ScopeAgent: the session ID lives in the
// agent's binding on the hub, so the agent can always be found again (from
// any machine) and a new session is never silently started once bound.
type HubSessions struct {
	hub     Binder
	mu      sync.Mutex
	session string
}

// NewHubSessions starts from the binding the hub already knows.
func NewHubSessions(hub Binder, current string) *HubSessions {
	return &HubSessions{hub: hub, session: current}
}

func (s *HubSessions) Get(string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session
}

func (s *HubSessions) Put(_ string, session string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session == "" || session == s.session {
		return nil
	}
	if _, err := s.hub.Bind(context.Background(), api.Binding{SessionID: session}); err != nil {
		return err
	}
	s.session = session
	return nil
}
