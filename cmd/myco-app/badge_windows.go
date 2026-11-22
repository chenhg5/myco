package main

import "github.com/wailsapp/wails/v3/pkg/application"

// appBadge sets the number on the App's icon (0: none).
type appBadge func(n int)

// newAppBadge: none on Windows yet (Wails draws the taskbar overlay with
// an image library this build does not carry); the toasts still show.
func newAppBadge(*application.App) appBadge { return nil }
