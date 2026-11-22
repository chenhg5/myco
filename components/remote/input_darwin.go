//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework ApplicationServices
#include <ApplicationServices/ApplicationServices.h>
static int allowed(void){return AXIsProcessTrusted();}
static void request_input(void){const void *keys[]={kAXTrustedCheckOptionPrompt};const void *values[]={kCFBooleanTrue};CFDictionaryRef opts=CFDictionaryCreate(NULL,keys,values,1,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);AXIsProcessTrustedWithOptions(opts);CFRelease(opts);}
static void pointer(double x,double y,int kind,int button,int down,int clicks){
 CGRect b=CGDisplayBounds(CGMainDisplayID());CGPoint p=CGPointMake(b.origin.x+x*(b.size.width-1),b.origin.y+y*(b.size.height-1));
 if(kind==2){CGEventRef current=CGEventCreate(NULL);if(current){p=CGEventGetLocation(current);CFRelease(current);}}
 CGEventType t=kCGEventMouseMoved;
 CGMouseButton btn=button==2?kCGMouseButtonRight:button==1?kCGMouseButtonCenter:kCGMouseButtonLeft;
 if(kind){if(button==0)t=down?kCGEventLeftMouseDown:kCGEventLeftMouseUp;else if(button==2)t=down?kCGEventRightMouseDown:kCGEventRightMouseUp;else t=down?kCGEventOtherMouseDown:kCGEventOtherMouseUp;}
 else {if(CGEventSourceButtonState(kCGEventSourceStateCombinedSessionState,kCGMouseButtonLeft))t=kCGEventLeftMouseDragged;else if(CGEventSourceButtonState(kCGEventSourceStateCombinedSessionState,kCGMouseButtonRight))t=kCGEventRightMouseDragged;}
 CGEventRef e=CGEventCreateMouseEvent(NULL,t,p,btn);if(e){if(kind)CGEventSetIntegerValueField(e,kCGMouseEventClickState,clicks);CGEventPost(kCGHIDEventTap,e);CFRelease(e);}
}
static void text_input(const UniChar *chars, unsigned long length){
 for(int down=1;down>=0;down--){CGEventRef e=CGEventCreateKeyboardEvent(NULL,0,down);if(e){CGEventSetFlags(e,0);CGEventKeyboardSetUnicodeString(e,length,chars);CGEventPost(kCGHIDEventTap,e);CFRelease(e);}}
}
static void key(int code,int down,unsigned long long flags){CGEventRef e=CGEventCreateKeyboardEvent(NULL,(CGKeyCode)code,down);if(e){CGEventSetFlags(e,(CGEventFlags)flags);CGEventPost(kCGHIDEventTap,e);CFRelease(e);}}
static void geometry(double *g){CGRect b=CGDisplayBounds(CGMainDisplayID());g[0]=b.origin.x;g[1]=b.origin.y;g[2]=b.size.width;g[3]=b.size.height;}
static void scroll(int delta){CGEventRef e=CGEventCreateScrollWheelEvent(NULL,kCGScrollEventUnitPixel,1,delta);if(e){CGEventPost(kCGHIDEventTap,e);CFRelease(e);}}
*/
import "C"
import (
	"os/exec"
	"strconv"
	"unicode/utf16"
	"unsafe"
)

var keyCodes = map[string]uint16{"KeyA": 0, "KeyS": 1, "KeyD": 2, "KeyF": 3, "KeyH": 4, "KeyG": 5, "KeyZ": 6, "KeyX": 7, "KeyC": 8, "KeyV": 9, "KeyB": 11, "KeyQ": 12, "KeyW": 13, "KeyE": 14, "KeyR": 15, "KeyY": 16, "KeyT": 17, "Digit1": 18, "Digit2": 19, "Digit3": 20, "Digit4": 21, "Digit6": 22, "Digit5": 23, "Equal": 24, "Digit9": 25, "Digit7": 26, "Minus": 27, "Digit8": 28, "Digit0": 29, "BracketRight": 30, "KeyO": 31, "KeyU": 32, "BracketLeft": 33, "KeyI": 34, "KeyP": 35, "Enter": 36, "KeyL": 37, "KeyJ": 38, "Quote": 39, "KeyK": 40, "Semicolon": 41, "Backslash": 42, "Comma": 43, "Slash": 44, "KeyN": 45, "KeyM": 46, "Period": 47, "Tab": 48, "Space": 49, "Backquote": 50, "Backspace": 51, "Escape": 53, "MetaRight": 54, "MetaLeft": 55, "ShiftLeft": 56, "AltLeft": 58, "ControlLeft": 59, "ShiftRight": 60, "AltRight": 61, "ControlRight": 62, "Home": 115, "PageUp": 116, "Delete": 117, "End": 119, "PageDown": 121, "ArrowLeft": 123, "ArrowRight": 124, "ArrowDown": 125, "ArrowUp": 126}

func inputSupported() bool { return C.allowed() != 0 }

// displayGeometry is the main display's origin and size (points): where
// normalized pointer coordinates land.
func displayGeometry() [4]int {
	var g [4]C.double
	C.geometry(&g[0])
	return [4]int{int(g[0]), int(g[1]), int(g[2]), int(g[3])}
}
func nativeInput(e inputEvent) error {
	down := 0
	if e.Down {
		down = 1
	}
	switch e.Kind {
	case "move":
		C.pointer(C.double(e.X), C.double(e.Y), 0, 0, 0, 0)
	case "button":
		C.pointer(C.double(e.X), C.double(e.Y), 1, C.int(e.Button), C.int(down), C.int(max(1, e.Clicks)))
	case "release_button":
		C.pointer(0, 0, 2, C.int(e.Button), 0, 1)
	case "key":
		flags := uint64(0)
		if pressed["MetaLeft"] || pressed["MetaRight"] {
			flags |= 1 << 20
		}
		if pressed["ShiftLeft"] || pressed["ShiftRight"] {
			flags |= 1 << 17
		}
		if pressed["ControlLeft"] || pressed["ControlRight"] {
			flags |= 1 << 18
		}
		if pressed["AltLeft"] || pressed["AltRight"] {
			flags |= 1 << 19
		}
		C.key(C.int(keyCodes[e.Code]), C.int(down), C.ulonglong(flags))
	case "scroll":
		C.scroll(C.int(-e.Delta))
	}
	return nil
}
func hideProcess(*exec.Cmd) {}

func requestInputPermission() { C.request_input() }

func init() {
	for i, code := range []uint16{122, 120, 99, 118, 96, 97, 98, 100, 101, 109, 103, 111} {
		keyCodes["F"+strconv.Itoa(i+1)] = code
	}
}

func nativeText(text string) error {
	// One Unicode scalar per event preserves surrogate pairs and avoids long
	// keyboard events being truncated by applications.
	for _, r := range text {
		units := utf16.Encode([]rune{r})
		C.text_input((*C.UniChar)(unsafe.Pointer(&units[0])), C.ulong(len(units)))
	}
	return nil
}
