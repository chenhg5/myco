//go:build darwin && purego

package main

// Passing presses on to the panel's page needs cgo; without it, clicks are
// the web view's own.
func (q *quick) watchClicks() {}
