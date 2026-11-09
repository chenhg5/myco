package main

import "syscall"

// systemPrefersChinese reports whether Windows' display language is Chinese.
func systemPrefersChinese() bool {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")
	if proc.Find() != nil {
		return false
	}
	lang, _, _ := proc.Call()
	return lang&0x3ff == 0x04 // LANG_CHINESE (primary language id)
}
