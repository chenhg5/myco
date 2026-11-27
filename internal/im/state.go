package im

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// conversation is what a bridge remembers of one chat-app conversation.
type conversation struct {
	Ref      Ref    `json:"ref"`
	ThreadID string `json:"thread_id,omitempty"` // the myco thread
	// Task is the open task, if any; Agent is who has it (or had the last).
	Task   string    `json:"task,omitempty"`
	Agent  string    `json:"agent,omitempty"`
	Status string    `json:"status_message,omitempty"` // the status message's id
	User   string    `json:"user,omitempty"`           // who gave the open task
	At     time.Time `json:"at"`
	// For the front desk: the open task's question and latest progress, the
	// last result, and the recent talk.
	Asking     string    `json:"asking,omitempty"`
	Progress   string    `json:"progress,omitempty"`
	LastAnswer string    `json:"last_answer,omitempty"`
	Desk       []deskMsg `json:"desk,omitempty"`
}

// state is a bridge's memory, kept next to its config.
type state struct {
	mu    sync.Mutex
	path  string
	Convs map[string]*conversation `json:"conversations"`
}

const keepConversations = 90 * 24 * time.Hour

func statePath(dir, name string) string { return filepath.Join(dir, name+".state.json") }

func loadState(dir, name string) (*state, error) {
	s := &state{path: statePath(dir, name), Convs: map[string]*conversation{}}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	if s.Convs == nil {
		s.Convs = map[string]*conversation{}
	}
	return s, nil
}

// save writes the state; the caller holds mu.
func (s *state) save() error {
	if s.path == "" {
		return nil // tests
	}
	for k, c := range s.Convs {
		if c.Task == "" && time.Since(c.At) > keepConversations {
			delete(s.Convs, k)
		}
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(s.path, b)
}
