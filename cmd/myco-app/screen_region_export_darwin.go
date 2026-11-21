//go:build darwin && !purego

package main

import "C"

//export mycoScreenRegionSelected
func mycoScreenRegionSelected(ident C.int, x, y, w, h, top C.double, cancelled C.int) {
	nativeRegions.Lock()
	s := nativeRegions.sessions[int(ident)]
	nativeRegions.Unlock()
	if s == nil {
		return
	}
	r := regionAnswer{cancelled: cancelled != 0}
	if !r.cancelled {
		r.region = screenRegionFromCocoa(float64(x), float64(y), float64(w), float64(h), float64(top))
	}
	select {
	case s.answer <- r:
	default:
	}
}

//export mycoScreenRegionStopped
func mycoScreenRegionStopped(ident C.int) {
	nativeRegions.Lock()
	s := nativeRegions.sessions[int(ident)]
	nativeRegions.Unlock()
	if s != nil && s.stop != nil {
		s.stop()
	}
}
