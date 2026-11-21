package main

import "sync"

// A denied or not-yet-effective permission must not repeatedly open a system
// dialog. Recheck access each time (including after revocation), but prompt at
// most once in this process. Restarting the App starts a new request session.
type screenPrompt struct {
	mu        sync.Mutex
	requested bool
}

func (p *screenPrompt) allowed(check, request func() bool) bool {
	if check() {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if check() {
		return true
	}
	if p.requested {
		return false
	}
	p.requested = true
	return request()
}
