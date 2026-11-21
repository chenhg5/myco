package main

import (
	"os/exec"
	"strconv"
	"syscall"
)

var user32 = syscall.NewLazyDLL("user32.dll")
var mouseEvent = user32.NewProc("mouse_event")
var keyEvent = user32.NewProc("keybd_event")
var keyCodes = map[string]uint16{"Enter": 13, "Escape": 27, "Backspace": 8, "Tab": 9, "Space": 32, "ArrowLeft": 37, "ArrowUp": 38, "ArrowRight": 39, "ArrowDown": 40, "Delete": 46, "Home": 36, "End": 35, "PageUp": 33, "PageDown": 34, "ShiftLeft": 160, "ShiftRight": 161, "ControlLeft": 162, "ControlRight": 163, "AltLeft": 164, "AltRight": 165, "MetaLeft": 91, "MetaRight": 92, "Minus": 189, "Equal": 187, "BracketLeft": 219, "BracketRight": 221, "Backslash": 220, "Semicolon": 186, "Quote": 222, "Backquote": 192, "Comma": 188, "Period": 190, "Slash": 191}

func init() {
	for i := 1; i <= 12; i++ {
		keyCodes["F"+strconv.Itoa(i)] = uint16(111 + i)
	}
	for c := 'A'; c <= 'Z'; c++ {
		keyCodes["Key"+string(c)] = uint16(c)
	}
	for c := '0'; c <= '9'; c++ {
		keyCodes["Digit"+string(c)] = uint16(c)
	}
}
func inputSupported() bool { return true }
func nativeInput(e inputEvent) error {
	switch e.Kind {
	case "move", "button":
		// gdigrab captures the virtual desktop, including monitors left of primary.
		mouseEvent.Call(0x8000|0x4000|1, uintptr(e.X*65535), uintptr(e.Y*65535), 0, 0)
		if e.Kind == "button" {
			flags := []uintptr{2, 32, 8}
			f := flags[e.Button]
			if !e.Down {
				f *= 2
			}
			mouseEvent.Call(f, 0, 0, 0, 0)
		}
	case "release_button":
		f := []uintptr{4, 64, 16}[e.Button]
		mouseEvent.Call(f, 0, 0, 0, 0)
	case "key":
		f := uintptr(0)
		if keyCodes[e.Code] >= 33 && keyCodes[e.Code] <= 46 || e.Code == "ControlRight" || e.Code == "AltRight" || e.Code == "MetaLeft" || e.Code == "MetaRight" {
			f |= 1
		}
		if !e.Down {
			f |= 2
		}
		keyEvent.Call(uintptr(keyCodes[e.Code]), 0, f, 0)
	case "scroll":
		v := int32(-e.Delta)
		mouseEvent.Call(0x0800, 0, 0, uintptr(uint32(v)), 0)
	}
	return nil
}
func hideProcess(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }

func requestInputPermission() {}
