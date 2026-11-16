//go:build darwin && !purego

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>

// The red number on the app's icon in the Dock ("" takes it away).
static void mycoSetBadge(const char *label) {
	NSString *s = [NSString stringWithUTF8String:label];
	dispatch_async(dispatch_get_main_queue(), ^{
		[[NSApp dockTile] setBadgeLabel:([s length] ? s : nil)];
	});
}
*/
import "C"

import (
	"strconv"
	"unsafe"
)

// setBadge shows how many conversations have something unread on the
// app's Dock icon (0 takes it away; past 99, "99+").
var setBadge = func(n int) {
	label := ""
	switch {
	case n > 99:
		label = "99+"
	case n > 0:
		label = strconv.Itoa(n)
	}
	cs := C.CString(label)
	defer C.free(unsafe.Pointer(cs))
	C.mycoSetBadge(cs)
}
