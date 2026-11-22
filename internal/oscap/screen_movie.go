package oscap

import (
	"context"
	"time"
)

// Native completion means the movie container has been finalized, not merely
// that capture stopped. In particular SIGINT on screencapture is not completion.
type screenMovieBackend struct {
	state       func() int // 0 preparing, 1 recording, 2 finalized, -1 failed
	failure     func() string
	stop, close func()
}
type screenMovieTiming struct{ poll, start, finish time.Duration }

func waitScreenMovie(ctx context.Context, seconds int, emit Emit, backend screenMovieBackend, timing screenMovieTiming) error {
	defer backend.close()
	ticker := time.NewTicker(timing.poll)
	defer ticker.Stop()
	startup := time.NewTimer(timing.start)
	defer startup.Stop()
	var limit, stopTimeout <-chan time.Time
	var durationTimer, finishTimer *time.Timer
	defer func() {
		if durationTimer != nil {
			durationTimer.Stop()
		}
		if finishTimer != nil {
			finishTimer.Stop()
		}
	}()
	started, stopping := false, false
	stop := func() {
		if stopping {
			return
		}
		stopping = true
		backend.stop()
		finishTimer = time.NewTimer(timing.finish)
		stopTimeout = finishTimer.C
	}
	cancelled := ctx.Done()
	for {
		select {
		case <-cancelled:
			cancelled = nil
			stop()
		case <-limit:
			limit = nil
			stop()
		case <-startup.C:
			if !started {
				stop()
				return errf(CodeFailed, "screen recording did not start; check Screen Recording permission")
			}
		case <-stopTimeout:
			return errf(CodeFailed, "screen recording did not finish; the original remains in the recording folder")
		case <-ticker.C:
			state := backend.state()
			if state == -1 {
				message := backend.failure()
				backend.stop()
				return errf(CodeFailed, "screen recording failed: %s", message)
			}
			if state == 2 {
				return nil
			}
			if state == 1 && !started {
				started = true
				startup.Stop()
				emit("started", map[string]any{})
				durationTimer = time.NewTimer(time.Duration(seconds) * time.Second)
				limit = durationTimer.C
			}
		}
	}
}
