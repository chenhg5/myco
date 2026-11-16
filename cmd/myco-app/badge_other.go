//go:build !darwin || purego

package main

// setBadge: only the Mac app puts a number on its icon (for now).
var setBadge func(n int)
