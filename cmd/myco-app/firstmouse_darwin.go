//go:build darwin && !purego

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <objc/runtime.h>

static BOOL mycoAcceptsFirstMouse(id self, SEL _cmd, NSEvent *event) { return YES; }

// A click on the window while another app is in front normally only brings
// the window forward, and WKWebView drops it, so buttons (say, 完成 after
// pasting a link into another app) need a second click. Let the first click
// through, as most Mac apps do for their own content.
static void mycoEnableFirstMouse(void) {
	Class c = [WKWebView class];
	SEL s = @selector(acceptsFirstMouse:);
	Method m = class_getInstanceMethod(c, s);
	const char *types = m ? method_getTypeEncoding(m) : "B@:@";
	class_replaceMethod(c, s, (IMP)mycoAcceptsFirstMouse, types);
}
*/
import "C"

func init() { C.mycoEnableFirstMouse() }
