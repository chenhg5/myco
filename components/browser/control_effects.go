package main

import (
	"context"
	"errors"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"time"
)

// A successful input is not a settled page. Keep the two-second causal window
// open, and wait for navigations begun inside it to commit even if a server is
// slow. Domain correction completes before the logical action enters a draft.
func (c *controlSession) waitEffects(ctx context.Context, p *rod.Page, after uint64, oldTabs map[proto.TargetTargetID]bool) error {
	if c.dialogOpen() {
		return nil
	}
	_ = c.barrier(ctx, p)
	popupHandled := false
	for {
		c.enforceHosts(ctx)
		if c.hasHostViolation() {
			return errors.New("host_not_allowed")
		}
		c.mu.Lock()
		until, pending, popup, wake := c.navigationUntil, c.navigationPending, c.pendingPopup, c.wake
		c.mu.Unlock()
		if popup && !popupHandled {
			event, err := c.browser.waitObserved(ctx, after, func(evt observation) bool { return evt.Kind == "target" && !oldTabs[evt.Target] })
			if err == nil {
				err = c.waitPopup(ctx, event.Target)
			}
			if err != nil {
				if c.hasHostViolation() {
					return errors.New("host_not_allowed")
				}
				return errors.New("outcome_unknown")
			}
			popupHandled = true
			continue
		}
		if !pending && !time.Now().Before(until) {
			break
		}
		if c.dialogOpen() {
			return nil
		}
		var clock <-chan time.Time
		var timer *time.Timer
		if time.Now().Before(until) {
			timer = time.NewTimer(time.Until(until))
			clock = timer.C
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return errors.New("outcome_unknown")
		case <-wake:
		case <-clock:
		}
		if timer != nil {
			timer.Stop()
		}
	}
	if err := c.waitReady(ctx, p); err != nil {
		return errors.New("outcome_unknown")
	}
	c.enforceHosts(ctx)
	if c.hasHostViolation() {
		return errors.New("host_not_allowed")
	}
	return nil
}
