package oscap

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
)

// ScreenRecordRegion uses the system screen coordinate space (not movie pixels).
type ScreenRecordRegion = screenRecordRegion

// ScreenRegionPicker keeps recording controls alive until cleanup. A nil stop
// requests selection only. Implemented by the GUI, never by a shell command.
type ScreenRegionPicker func(context.Context, func()) (*ScreenRecordRegion, func(), error)

var regionPicker struct {
	sync.RWMutex
	pick ScreenRegionPicker
}

func SetScreenRegionPicker(p ScreenRegionPicker) {
	regionPicker.Lock()
	regionPicker.pick = p
	regionPicker.Unlock()
}
func chooseScreenRegion(ctx context.Context, stop func()) (*ScreenRecordRegion, func(), error) {
	if ctx.Err() != nil {
		return nil, nil, errf(CodeCancelled, "region selection cancelled")
	}
	regionPicker.RLock()
	p := regionPicker.pick
	regionPicker.RUnlock()
	if p == nil {
		return nil, nil, errf(CodeUnsupported, "region selection needs the myco desktop App")
	}
	return p(ctx, stop)
}
func init() {
	register(&Cap{Name: "screen.region", About: "Select a recording rectangle in the desktop App", Missing: func(*Env) string {
		regionPicker.RLock()
		defer regionPicker.RUnlock()
		if regionPicker.pick == nil {
			return "region selection needs the myco desktop App"
		}
		return ""
	}, Fn: func(ctx context.Context, _ *Env, _ json.RawMessage, _ Emit) (any, error) {
		r, cleanup, err := chooseScreenRegion(ctx, nil)
		if cleanup != nil {
			defer cleanup()
		}
		return r, err
	}})
}

// One reader handles stop during both selection and recording; switching phases
// must not consume and lose a stop command between two separate readers.
func screenRecordingContext(ctx context.Context) (context.Context, context.CancelFunc) {
	next, cancel := context.WithCancel(ctx)
	go func() {
		for {
			select {
			case <-next.Done():
				return
			case b, ok := <-inputOf(ctx):
				if !ok || strings.TrimSpace(string(b)) == "stop" {
					cancel()
					return
				}
			}
		}
	}()
	return next, cancel
}
