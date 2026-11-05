package runner

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
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
