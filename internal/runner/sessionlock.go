package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SessionLocks keeps one CLI session from being resumed twice at once. A
// session can be the agent of several workspaces (one profile on each hub,
// the same session id): each workspace's runner would otherwise resume it
// when its own message comes, and two turns on one transcript at the same
// time corrupt it. A turn takes the session's lock first; while another
// holds it, the message goes back to the queue for a little later.
//
// The lock is held in this process (runner --all serves every profile from
// one) and, when Dir is set, as a file there, for runners in other
// processes. A file lock is a file created exclusively; its holder touches
// it while the turn runs, and one left behind by a process that died is
// taken over once it has not been touched for StaleAfter.
type SessionLocks struct {
	Dir        string
	StaleAfter time.Duration // default 2m
	Beat       time.Duration // default 30s

	mu    sync.Mutex
	held  map[string]bool
	ended map[string]time.Time
}

// Busy reports whether a turn of this machine's (any runner here) holds
// key's lock now: the session is at work on one of our own tasks, not
// open in someone's terminal.
func (l *SessionLocks) Busy(key string) bool {
	if l == nil || key == "" {
		return false
	}
	l.mu.Lock()
	held := l.held[key]
	l.mu.Unlock()
	if held || l.Dir == "" {
		return held
	}
	st, err := os.Stat(l.path(key))
	return err == nil && time.Since(st.ModTime()) < l.staleAfter()
}

// LastEnd is when a turn of this machine's last let go of key (zero if
// none is known): writes to the session's transcript until then were ours.
func (l *SessionLocks) LastEnd(key string) time.Time {
	if l == nil || key == "" {
		return time.Time{}
	}
	l.mu.Lock()
	t := l.ended[key]
	l.mu.Unlock()
	if l.Dir != "" {
		if st, err := os.Stat(l.path(key) + ".ended"); err == nil && st.ModTime().After(t) {
			t = st.ModTime()
		}
	}
	return t
}

func (l *SessionLocks) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(l.Dir, hex.EncodeToString(sum[:8])+".lock")
}

// noteEnd records that a turn let go of key now.
func (l *SessionLocks) noteEnd(key string) {
	now := time.Now()
	l.mu.Lock()
	if l.ended == nil {
		l.ended = map[string]time.Time{}
	}
	l.ended[key] = now
	l.mu.Unlock()
	if l.Dir != "" {
		p := l.path(key) + ".ended"
		if err := os.WriteFile(p, nil, 0o600); err == nil {
			_ = os.Chtimes(p, now, now)
		}
	}
}

// NewSessionLocks returns locks kept in dir ("" = in this process only).
func NewSessionLocks(dir string) *SessionLocks { return &SessionLocks{Dir: dir} }

// TryLock takes the lock of key (a harness and a session id) if it is free.
// It returns the function that releases it.
func (l *SessionLocks) TryLock(key string) (func(), bool) {
	if l == nil || key == "" {
		return func() {}, true
	}
	l.mu.Lock()
	if l.held == nil {
		l.held = map[string]bool{}
	}
	if l.held[key] {
		l.mu.Unlock()
		return nil, false
	}
	l.held[key] = true
	l.mu.Unlock()
	free := func() {
		l.noteEnd(key)
		l.mu.Lock()
		delete(l.held, key)
		l.mu.Unlock()
	}
	if l.Dir == "" {
		return free, true
	}
	path, err := l.take(key)
	if err != nil {
		free()
		return nil, false
	}
	stop := make(chan struct{})
	go l.beat(path, stop)
	var once sync.Once
	return func() {
		once.Do(func() {
			close(stop)
			os.Remove(path)
			free()
		})
	}, true
}

var errHeld = errors.New("session lock held")

func (l *SessionLocks) take(key string) (string, error) {
	if err := os.MkdirAll(l.Dir, 0o700); err != nil {
		// No place for a file lock: this process's lock still holds.
		return "", nil
	}
	path := l.path(key)
	for try := 0; try < 2; try++ {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			fmt.Fprintf(f, "%d %s\n", os.Getpid(), key)
			f.Close()
			return path, nil
		}
		if !os.IsExist(err) {
			return "", nil // cannot lock across processes here; do not block on it
		}
		st, serr := os.Stat(path)
		if serr != nil || time.Since(st.ModTime()) < l.staleAfter() {
			return "", errHeld
		}
		os.Remove(path) // left by a runner that stopped mid-turn
	}
	return "", errHeld
}

func (l *SessionLocks) beat(path string, stop chan struct{}) {
	if path == "" {
		return
	}
	every := l.Beat
	if every <= 0 {
		every = 30 * time.Second
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			now := time.Now()
			os.Chtimes(path, now, now)
		}
	}
}

func (l *SessionLocks) staleAfter() time.Duration {
	if l.StaleAfter > 0 {
		return l.StaleAfter
	}
	return 2 * time.Minute
}
