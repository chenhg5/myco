//go:build darwin && !purego

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// Edit ▸ Start Dictation: macOS listens and types into the field that has
// the keyboard (the page focused the message box first). The system asks
// to turn Dictation on the first time, and handles the microphone itself.
static void mycoStartDictation(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp sendAction:@selector(startDictation:) to:nil from:nil];
	});
}
*/
import "C"

// dictate starts macOS Dictation in the window's focused field.
var dictate = func() error {
	C.mycoStartDictation()
	return nil
}
