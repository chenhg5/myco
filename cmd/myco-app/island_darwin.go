//go:build darwin && !purego

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <objc/runtime.h>
#import <math.h>

// Nothing behind the page is drawn: the web view's own (white) background
// and the views holding it are made clear, so only the pill shows.
static void mycoClearBackgrounds(NSView *v) {
	if ([v isKindOfClass:[WKWebView class]]) {
		@try { [v setValue:@NO forKey:@"drawsBackground"]; } @catch (NSException *e) {}
		if (@available(macOS 12.0, *)) {
			[(WKWebView *)v setUnderPageBackgroundColor:[NSColor clearColor]];
		}
	}
	v.wantsLayer = YES;
	v.layer.backgroundColor = [NSColor clearColor].CGColor;
	for (NSView *sub in v.subviews) mycoClearBackgrounds(sub);
}

// macOS keeps titled windows below the menu bar (constrainFrameRect); the
// island belongs on top of it, over the notch.
static NSRect mycoNoConstrain(id self, SEL _cmd, NSRect r, NSScreen *s) { return r; }
// A borderless window cannot become key by default, and the web view in it
// then drops clicks (pointing still works). The island may become key; as a
// non-activating panel that leaves the app you are in active.
static BOOL mycoCanBecomeKey(id self, SEL _cmd) { return YES; }

// The island's window: borderless (no frame, corners or edge of its own),
// non-activating, and free to sit over the menu bar. Only this window's
// class is changed: the app's main window is a different class.
static void mycoIslandFree(NSWindow *window) {
	static BOOL done = NO;
	SEL sel = @selector(constrainFrameRect:toScreen:);
	if (!done) {
		Method m = class_getInstanceMethod([window class], sel);
		class_replaceMethod([window class], sel, (IMP)mycoNoConstrain, m ? method_getTypeEncoding(m) : "{CGRect={CGPoint=dd}{CGSize=dd}}@:{CGRect={CGPoint=dd}{CGSize=dd}}@");
		class_replaceMethod([window class], @selector(canBecomeKeyWindow), (IMP)mycoCanBecomeKey, "c@:");
		done = YES;
	}
	NSWindowStyleMask mask = NSWindowStyleMaskBorderless;
	if ([window isKindOfClass:[NSPanel class]]) mask |= NSWindowStyleMaskNonactivatingPanel;
	if (window.styleMask != mask) window.styleMask = mask;
	if (window.contentView) mycoClearBackgrounds(window.contentView);
}

// Whether the pointer is on the pill: pw×ph points, at the top centre of
// the notch screen.
static int mycoMouseOver(double pw, double ph) {
	@autoreleasepool {
		int notched = 0;
		NSScreen *s = mycoNotchScreen(&notched);
		if (!s) return 0;
		NSRect f = s.frame;
		NSPoint m = [NSEvent mouseLocation];
		return fabs(m.x - NSMidX(f)) <= pw / 2 + 1 && m.y >= NSMaxY(f) - ph - 1 && m.y <= NSMaxY(f) + 1;
	}
}

// Brings the app to the front (the island itself never activates it).
static void mycoActivate(void) {
	dispatch_async(dispatch_get_main_queue(), ^{ [NSApp activateIgnoringOtherApps:YES]; });
}

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
			// As tall as the menu bar beside the notch (when it shows), so the
			// pill's bottom lines up with it; the notch's own inset is a
			// little less on some models.
			double bar = NSMaxY(s.frame) - NSMaxY(s.visibleFrame);
			*h = MAX(s.safeAreaInsets.top, bar);
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
		mycoIslandFree(window);
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
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// island is the strip of the notch that grows while agents work (as a
// phone's island does): a small window over the notch whose page draws
// the black pill, the walking mushroom and who is at work.
type island struct {
	mu   sync.Mutex
	win  *application.WebviewWindow
	open func(hub, agent string)
	// While shown the window keeps one size (as big as the open island):
	// resizing it as the pill springs made the page blink. Clicks pass
	// through it except on the pill (pw×ph), which the page reports; the
	// pointer is watched here, and the page told when it is on the pill.
	shown      bool
	winW, winH float64
	pw, ph     float64
	hovered    bool
}

// activateApp brings the app in front, for a member picked in the island.
func activateApp() { C.mycoActivate() }

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
			// The page draws the rounded bottom; the window stays square.
			CornerType:  application.MacWindowCornerTypeSquare,
			WindowClass: application.MacWindowClassPanel,
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
	is := &island{win: win, open: open}
	go is.watchPointer()
	return is
}

// watchPointer lets clicks through except on the pill, and tells the page
// when the pointer comes onto it or leaves (it opens and closes on that).
func (is *island) watchPointer() {
	t := time.NewTicker(60 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		is.mu.Lock()
		if !is.shown {
			is.mu.Unlock()
			continue
		}
		in := C.mycoMouseOver(C.double(is.pw), C.double(is.ph)) != 0
		changed := in != is.hovered
		is.hovered = in
		is.mu.Unlock()
		if changed {
			is.win.SetIgnoreMouseEvents(!in)
			is.win.ExecJS(fmt.Sprintf(`window.dispatchEvent(new CustomEvent("island-hover",{detail:%t}))`, in))
		}
	}
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
				PW, PH  float64
				Visible bool
			}
			if json.NewDecoder(r.Body).Decode(&in) != nil {
				http.Error(w, "bad frame", http.StatusBadRequest)
				return
			}
			is.frame(in.W, in.H, in.PW, in.PH, in.Visible)
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

// frame sizes the window (w×h) and records the pill (pw×ph) clicks may
// land on; not visible, the window is one point and out of the way.
func (is *island) frame(w, h, pw, ph float64, visible bool) {
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
		is.shown, is.hovered = false, false
		if is.winW != 1 || is.winH != 1 {
			C.mycoIslandPlace(p, 1, 1, 1)
			is.winW, is.winH = 1, 1
		}
		return
	}
	if w < 40 || h < 10 || w > 800 || h > 600 {
		return
	}
	is.pw, is.ph = pw, ph
	is.shown = true
	// Only a new size moves the window: the pill springs inside it.
	if w != is.winW || h != is.winH {
		pass := 1
		if is.hovered {
			pass = 0
		}
		C.mycoIslandPlace(p, C.double(w), C.double(h), C.int(pass))
		is.winW, is.winH = w, h
	}
}
