//go:build !windows && !(darwin && !purego)

package main

import "github.com/chenhg5/myco/internal/console"

// No speech to text here (the page uses the browser's own when it has one).
var (
	listen  func(lang string, h *console.Hearing) (func(), error)
	dictate func(lang string) error
)
