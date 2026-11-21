//go:build darwin && !purego

package main

import "C"

// A press on the quick panel's window, from the native monitor: where it
// landed, in points from the window's top left.
//
//export mycoQuickClick
func mycoQuickClick(x, y C.double) {
	if q := theQuick; q != nil {
		go q.click(float64(x), float64(y))
	}
}
