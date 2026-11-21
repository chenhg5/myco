package main

import (
	"math"
	"sync"
)

type inputEvent struct {
	Kind   string  `json:"kind"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Button int     `json:"button"`
	Clicks int     `json:"clicks,omitempty"`
	Down   bool    `json:"down"`
	Code   string  `json:"code"`
	Delta  float64 `json:"delta"`
}

var keyMu sync.Mutex
var pressed = map[string]bool{}

func (e inputEvent) valid() bool {
	if math.IsNaN(e.X) || math.IsNaN(e.Y) || math.IsInf(e.X, 0) || math.IsInf(e.Y, 0) {
		return false
	}
	switch e.Kind {
	case "move", "button":
		return e.X >= 0 && e.X <= 1 && e.Y >= 0 && e.Y <= 1 && e.Button >= 0 && e.Button <= 2 && e.Clicks >= 0 && e.Clicks <= 3
	case "key":
		_, ok := keyCodes[e.Code]
		return ok
	case "scroll":
		return !math.IsNaN(e.Delta) && !math.IsInf(e.Delta, 0) && math.Abs(e.Delta) <= 1200
	case "release":
		return true
	}
	return false
}
func inject(e inputEvent) error {
	keyMu.Lock()
	defer keyMu.Unlock()
	if e.Kind == "release" {
		releaseLocked()
		return nil
	}
	if e.Kind == "key" {
		if e.Down {
			pressed[e.Code] = true
		} else {
			delete(pressed, e.Code)
		}
	}
	if e.Kind == "button" {
		code := []string{"mouse0", "mouse1", "mouse2"}[e.Button]
		if e.Down {
			pressed[code] = true
		} else {
			delete(pressed, code)
		}
	}
	return nativeInput(e)
}
func releaseKeys() { keyMu.Lock(); defer keyMu.Unlock(); releaseLocked() }
func releaseLocked() {
	for code := range pressed {
		delete(pressed, code)
		if len(code) == 6 && code[:5] == "mouse" {
			_ = nativeInput(inputEvent{Kind: "release_button", Button: int(code[5] - '0')})
		} else {
			_ = nativeInput(inputEvent{Kind: "key", Code: code})
		}
	}
	clear(pressed)
}
