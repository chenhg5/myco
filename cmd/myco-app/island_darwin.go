//go:build darwin && !purego

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// The screen with a notch (a MacBook's built-in display), else the first.
static NSScreen *mycoNotchScreen(int *notched) {
	*notched = 0;
	if (@available(macOS 12.0, *)) {
		for (NSScreen *s in [NSScreen screens]) {
			if (s.safeAreaInsets.top > 0) { *notched = 1; return s; }
		}
	}
	return [NSScreen screens].firstObject;
}

// The notch's width and height in points; without a notch, width 0 and the
// menu bar's height.
static int mycoNotch(double *w, double *h) {
	int notched = 0;
	NSScreen *s = mycoNotchScreen(&notched);
	if (!s) { *w = 0; *h = 24; return 0; }
	if (notched) {
		if (@available(macOS 12.0, *)) {
			*w = s.frame.size.width - s.auxiliaryTopLeftArea.size.width - s.auxiliaryTopRightArea.size.width;
			*h = s.safeAreaInsets.top;
		}
		return 1;
	}
	*w = 0;
	*h = NSMaxY(s.frame) - NSMaxY(s.visibleFrame);
	if (*h <= 0) *h = 24;
	return 0;
}

// Puts the island window at the top centre of that screen, w×h points,
// above the menu bar, with nothing of its own drawn (the page draws it).
// Idle (passThrough), clicks go through it to whatever is underneath.
static void mycoIslandPlace(void *win, double w, double h, int passThrough) {
	NSWindow *window = (__bridge NSWindow *)win;
	dispatch_async(dispatch_get_main_queue(), ^{
		int notched = 0;
		NSScreen *s = mycoNotchScreen(&notched);
		if (!s || !window) return;
		NSRect f = s.frame;
		[window setLevel:NSStatusWindowLevel + 1];
		[window setHasShadow:NO];
		[window setOpaque:NO];
		[window setBackgroundColor:[NSColor clearColor]];
		[window setIgnoresMouseEvents:passThrough ? YES : NO];
		[window setFrame:NSMakeRect(NSMidX(f) - w / 2, NSMaxY(f) - h, w, h) display:YES];
	});
}
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// island is the strip of the notch that grows while agents work (as a
// phone's island does): a small window over the notch whose page draws
// the black pill, the walking mushroom and who is at work.
type island struct {
	mu   sync.Mutex
	win  *application.WebviewWindow
	open func(hub, agent string)
}

// notchSize reports the notch (width 0 without one) in points.
func notchSize() (w, h float64) {
	var cw, ch C.double
	C.mycoNotch(&cw, &ch)
	return float64(cw), float64(ch)
}

// newIsland creates the window; its page grows it when someone is at work,
// through the handler below.
func newIsland(app *application.App, open func(hub, agent string)) *island {
	w, h := notchSize()
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "island",
		Title:     "myco island",
		Width:     1,
		Height:    1,
		Frameless: true,
		// Never hidden: a hidden window's page may be put to sleep and miss
		// the moment someone starts working. Idle, it is one point big and
		// lets clicks through.
		Hidden:            false,
		IgnoreMouseEvents: true,
		BackgroundType:    application.BackgroundTypeTransparent,
		URL:               fmt.Sprintf("/?island=1&nw=%d&nh=%d", int(w), int(h)),
		Mac: application.MacWindow{
			Backdrop:      application.MacBackdropTransparent,
			DisableShadow: true,
			WindowClass:   application.MacWindowClassPanel,
			PanelPreferences: application.MacPanelPreferences{
				NonActivating: true,
			},
			WindowLevel: application.MacWindowLevelStatus,
			CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
				application.MacWindowCollectionBehaviorStationary |
				application.MacWindowCollectionBehaviorIgnoresCycle |
				application.MacWindowCollectionBehaviorFullScreenAuxiliary,
		},
	})
	return &island{win: win, open: open}
}

// handler serves the island page's two requests, everything else as before:
//
//	POST /app/island/frame {w, h, visible}: its size, and whether to show it
//	POST /app/island/open {hub, agent}: open the app at that member
func (is *island) handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/app/island/") {
			next.ServeHTTP(w, r)
			return
		}
		switch strings.TrimPrefix(r.URL.Path, "/app/island/") {
		case "frame":
			var in struct {
				W, H    float64
				Visible bool
			}
			if json.NewDecoder(r.Body).Decode(&in) != nil {
				http.Error(w, "bad frame", http.StatusBadRequest)
				return
			}
			is.frame(in.W, in.H, in.Visible)
		case "open":
			var in struct{ Hub, Agent string }
			if json.NewDecoder(r.Body).Decode(&in) != nil {
				http.Error(w, "bad target", http.StatusBadRequest)
				return
			}
			if is.open != nil {
				is.open(in.Hub, in.Agent)
			}
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
}

func (is *island) frame(w, h float64, visible bool) {
	is.mu.Lock()
	defer is.mu.Unlock()
	if is.win == nil {
		return
	}
	p := is.win.NativeWindow()
	if p == nil {
		return
	}
	if !visible {
		// One point, clicks passing through: there but out of the way.
		C.mycoIslandPlace(p, 1, 1, 1)
		return
	}
	if w < 40 || h < 10 || w > 800 || h > 600 {
		return
	}
	C.mycoIslandPlace(p, C.double(w), C.double(h), 0)
}
