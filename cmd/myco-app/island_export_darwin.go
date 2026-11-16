//go:build darwin && !purego

package main

import "C"

// A press on the island's window, from the native monitor: where it
// landed, in points from the window's top left.
//
//export mycoIslandClick
func mycoIslandClick(x, y C.double) {
	if is := theIsland; is != nil {
		go is.click(float64(x), float64(y))
	}
}
