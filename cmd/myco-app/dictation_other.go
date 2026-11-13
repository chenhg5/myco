//go:build !windows && !(darwin && !purego)

package main

// dictate: no system dictation to start here (the page uses the browser's
// own speech recognition when it has one).
var dictate func() error
