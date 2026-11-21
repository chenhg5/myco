package main

import (
	"math"
	"sync/atomic"

	"github.com/chenhg5/myco/internal/oscap"
)

var screenPickerInstall func()
var screenPickerChinese atomic.Bool

// Cocoa has a bottom-left origin; screencapture uses the primary screen's
// top-left. Keep negative coordinates for monitors above or left of primary.
func screenRegionFromCocoa(x, y, w, h, primaryTop float64) *oscap.ScreenRecordRegion {
	if math.IsNaN(x+y+w+h+primaryTop) || math.IsInf(x+y+w+h+primaryTop, 0) || w < 16 || h < 16 {
		return nil
	}
	left, top := math.Floor(x), math.Floor(primaryTop-y-h)
	right, bottom := math.Ceil(x+w), math.Ceil(primaryTop-y)
	return &oscap.ScreenRecordRegion{X: int(left), Y: int(top), W: int(right - left), H: int(bottom - top)}
}
