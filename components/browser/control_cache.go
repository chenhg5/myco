package main

import "time"

const controlRetryWindow = 2 * time.Minute

// Closed sessions retain only a short retry window. The component can remain
// running indefinitely without turning its bounded cache into a lifetime quota.
func (e *engine) pruneControls(now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	expired := map[string]*controlError{}
	for id, c := range e.controls {
		c.mu.Lock()
		if c.closed && !c.closedAt.IsZero() && !now.Before(c.closedAt.Add(controlRetryWindow)) {
			expired[id] = controlFailure(c.code, c.reason)
			c.steps = nil
			delete(e.controls, id)
		}
		c.mu.Unlock()
	}
	for key, request := range e.controlRequests {
		select {
		case <-request.done:
		default:
			continue
		}
		orphan := request.session == "" && !now.Before(request.created.Add(controlRetryWindow))
		terminal := expired[request.session]
		if terminal != nil && !request.retainUntil.IsZero() && now.Before(request.retainUntil) {
			if request.method == "open" {
				request.terminal = terminal
				e.controlCacheBytes -= len(request.raw)
				request.raw = nil
			}
			continue
		}
		retired := !request.retainUntil.IsZero() && !now.Before(request.retainUntil) && e.controls[request.session] == nil
		if terminal != nil || orphan || retired {
			delete(e.controlRequests, key)
			e.controlCacheBytes -= len(request.raw)
		}
	}
}
func (e *engine) reapControls() {
	defer close(e.controlDone)
	timer := time.NewTicker(15 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-e.controlStop:
			return
		case now := <-timer.C:
			e.pruneControls(now)
		}
	}
}
