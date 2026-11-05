package harness

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TranscriptFinder locates a session's transcript file by glob patterns and
// caches the result; its mtime tells when the session was last used.
type TranscriptFinder struct {
	// Patterns are filepath.Glob patterns with %s replaced by the session id.
	Patterns func(sessionID string) []string

	mu    sync.Mutex
	cache map[string]string
}

// LastActivity implements BusyProber.
func (f *TranscriptFinder) LastActivity(sessionID string) (time.Time, bool) {
	if sessionID == "" || f.Patterns == nil {
		return time.Time{}, false
	}
	f.mu.Lock()
	path := f.cache[sessionID]
	f.mu.Unlock()
	if path != "" {
		if st, err := os.Stat(path); err == nil {
			return st.ModTime(), true
		}
	}
	for _, pat := range f.Patterns(sessionID) {
		matches, _ := filepath.Glob(pat)
		var newest string
		var newestT time.Time
		for _, m := range matches {
			if st, err := os.Stat(m); err == nil && st.ModTime().After(newestT) {
				newest, newestT = m, st.ModTime()
			}
		}
		if newest != "" {
			f.mu.Lock()
			if f.cache == nil {
				f.cache = map[string]string{}
			}
			f.cache[sessionID] = newest
			f.mu.Unlock()
			return newestT, true
		}
	}
	return time.Time{}, false
}

// HomeDir returns $env (if set) or ~/<fallback>.
func HomeDir(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, fallback)
}
