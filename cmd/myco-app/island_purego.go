//go:build darwin && purego

package main

import (
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// island needs cgo (the notch's size and the window's place); without it
// there is none.
type island struct{}

func newIsland(*application.App, func(hub, agent string)) *island { return &island{} }

func (*island) handler(next http.Handler) http.Handler { return next }
