package templates

import "time"

// SetRunWaits shortens Run's waits for a test; it returns the undo.
func SetRunWaits(start, retry, max time.Duration) func() {
	s, r, m := startWait, firstRetry, firstRetryMax
	startWait, firstRetry, firstRetryMax = start, retry, max
	return func() { startWait, firstRetry, firstRetryMax = s, r, m }
}
