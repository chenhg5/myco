//go:build darwin && !purego

package main

// #include <stdlib.h>
import "C"

// What the recognizer heard so far in this utterance.
//
//export mycoVoiceHeard
func mycoVoiceHeard(text *C.char, final C.int) {
	if ev := theVoice.events(); ev != nil && text != nil {
		ev.Heard(C.GoString(text), final != 0)
	}
}

// Everything queued to play has been played.
//
//export mycoVoicePlayed
func mycoVoicePlayed() {
	if ev := theVoice.events(); ev != nil {
		ev.Played()
	}
}

// The microphone stopped: err is a reason ("speech-denied", "mic-denied",
// "unavailable", "microphone") or nil.
//
//export mycoVoiceEnded
func mycoVoiceEnded(err *C.char) {
	ev := theVoice.events()
	if ev == nil {
		return
	}
	msg := ""
	if err != nil {
		msg = C.GoString(err)
	}
	ev.Ended(msg)
}
