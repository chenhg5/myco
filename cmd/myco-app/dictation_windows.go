//go:build windows

package main

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/chenhg5/myco/internal/console"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	keybdEvent           = user32.NewProc("keybd_event")
	loadKeyboardLayout   = user32.NewProc("LoadKeyboardLayoutW")
	getForegroundWindow  = user32.NewProc("GetForegroundWindow")
	postMessage          = user32.NewProc("PostMessageW")
	wmInputLangChangeReq = uintptr(0x0050)
	klfActivate          = uintptr(0x00000001)
	vkLWin, vkH          = uintptr(0x5B), uintptr(0x48)
	keyeventfKeyUp       = uintptr(0x0002)
)

// Keyboard layouts for the app's languages: voice typing listens in the
// language of the window's keyboard.
var layoutOf = map[string]string{"zh-CN": "00000804", "en-US": "00000409"}

// listen: Windows has no app-side recognizer here; voice typing does it.
var listen func(lang string, h *console.Hearing) (func(), error)

// dictate switches this window's input language to lang (the app's
// language, not the system's), then opens Windows voice typing (Win+H),
// which listens in it and types into the field that has the keyboard (the
// page focused the message box first).
var dictate = func(lang string) error {
	if err := keybdEvent.Find(); err != nil {
		return fmt.Errorf("voice typing is not available: %w", err)
	}
	if klid, ok := layoutOf[lang]; ok {
		if p, err := syscall.UTF16PtrFromString(klid); err == nil {
			if hkl, _, _ := loadKeyboardLayout.Call(uintptr(unsafe.Pointer(p)), klfActivate); hkl != 0 {
				if hwnd, _, _ := getForegroundWindow.Call(); hwnd != 0 {
					_, _, _ = postMessage.Call(hwnd, wmInputLangChangeReq, 0, hkl)
					time.Sleep(120 * time.Millisecond)
				}
			}
		}
	}
	press := func(vk, flags uintptr) { _, _, _ = keybdEvent.Call(vk, 0, flags, 0) }
	press(vkLWin, 0)
	press(vkH, 0)
	time.Sleep(20 * time.Millisecond)
	press(vkH, keyeventfKeyUp)
	press(vkLWin, keyeventfKeyUp)
	return nil
}
