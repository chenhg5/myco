//go:build darwin && !purego

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

extern void mycoQuickClick(double x, double y);

// A press on the quick panel, seen before the web view: a non-activating
// panel that is not key gets its first click eaten by the web view (which
// only makes it key), so "open the chat" did nothing (as on the island,
// island_darwin.go). Watched here, the press is passed to the page, at its
// point from the top left; the page acts once per press.
static id mycoQuickMonitor = nil;
static void mycoQuickWatch(void *win) {
	NSWindow *window = (__bridge NSWindow *)win;
	if (mycoQuickMonitor || !window) return;
	mycoQuickMonitor = [[NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskLeftMouseDown handler:^NSEvent *(NSEvent *e) {
		if (e.window == window && window.contentView) {
			NSView *cv = window.contentView;
			NSPoint p = [cv convertPoint:e.locationInWindow fromView:nil];
			double y = cv.isFlipped ? p.y : cv.bounds.size.height - p.y;
			mycoQuickClick(p.x, y);
		}
		return e;
	}] retain];
}
*/
import "C"

import "fmt"

// theQuick receives the presses the native monitor sees (one panel).
var theQuick *quick

// watchClicks starts passing presses on the panel to its page (once; on
// the main thread, with the window made).
func (q *quick) watchClicks() {
	if p := q.win.NativeWindow(); p != nil {
		theQuick = q
		C.mycoQuickWatch(p)
	}
}

// click passes a press on the panel to its page, at that point.
func (q *quick) click(x, y float64) {
	if q.win.IsVisible() {
		q.win.ExecJS(fmt.Sprintf(`window.dispatchEvent(new CustomEvent("quick-click",{detail:{x:%f,y:%f}}))`, x, y))
	}
}
