package main

import (
	"context"
	"math"
	"sync/atomic"
	"time"

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

// Wait against a monotonic deadline, so delayed UI ticks never add extra seconds.
// A cancelled countdown never publishes zero (permission to start capture).
func screenCountdown(ctx context.Context, seconds int, interval time.Duration, update func(int)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline := time.Now().Add(time.Duration(seconds) * interval)
	update(seconds)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := ctx.Err(); err != nil {
				return err
			}
			left := int(math.Ceil(float64(time.Until(deadline)) / float64(interval)))
			if left <= 0 {
				update(0)
				return nil
			}
			update(left)
		}
	}
}
