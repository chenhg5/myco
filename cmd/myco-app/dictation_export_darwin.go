//go:build darwin && !purego

package main

// #include <stdlib.h>
import "C"

// What the recognizer heard so far (the whole text), from Objective-C.
//
//export mycoHeard
func mycoHeard(sid C.int, text *C.char, final C.int) {
	if h := heard(int(sid)); h != nil && text != nil {
		h.Heard(C.GoString(text), final != 0)
	}
}

// The end of a listening; err is nil when it simply ended, else a reason
// ("speech-denied", "mic-denied", "unavailable", "microphone") or the
// recognizer's own message.
//
//export mycoHeardDone
func mycoHeardDone(sid C.int, err *C.char) {
	h := heardDone(int(sid))
	if h == nil {
		return
	}
	msg := ""
	if err != nil {
		msg = C.GoString(err)
	}
	h.End(msg)
}
