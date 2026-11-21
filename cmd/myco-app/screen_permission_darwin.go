//go:build darwin && !purego

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework AppKit
#import <CoreGraphics/CoreGraphics.h>
#import <AppKit/AppKit.h>

// TCC must identify the App, not only a transient screencapture child. macOS
// only prompts the first time; a previous denial must be changed in Settings.
static int mycoRequestScreenRecording(void) {
 __block BOOL allowed = NO;
 void (^request)(void) = ^{
  allowed = CGPreflightScreenCaptureAccess();
  if (!allowed) {
   CGRequestScreenCaptureAccess();
   allowed = CGPreflightScreenCaptureAccess();
  }
 };
 if ([NSThread isMainThread]) request();
 else dispatch_sync(dispatch_get_main_queue(), request);
 return allowed ? 1 : 0;
}
*/
import "C"

import "github.com/chenhg5/myco/internal/oscap"

func init() {
	oscap.SetScreenRecordingPermission(func() bool { return C.mycoRequestScreenRecording() != 0 })
}
