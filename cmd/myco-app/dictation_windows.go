//go:build windows

package main

import (
	"fmt"
	"syscall"
	"time"
)

var keybdEvent = syscall.NewLazyDLL("user32.dll").NewProc("keybd_event")

const (
	vkLWin         = 0x5B
	vkH            = 0x48
	keyeventfKeyUp = 0x0002
)

// dictate opens Windows voice typing (Win+H), which listens and types into
// the field that has the keyboard (the page focused the message box first).
var dictate = func() error {
	if err := keybdEvent.Find(); err != nil {
		return fmt.Errorf("voice typing is not available: %w", err)
	}
	press := func(vk, flags uintptr) { _, _, _ = keybdEvent.Call(vk, 0, flags, 0) }
	press(vkLWin, 0)
	press(vkH, 0)
	time.Sleep(20 * time.Millisecond)
	press(vkH, keyeventfKeyUp)
	press(vkLWin, keyeventfKeyUp)
	return nil
}
