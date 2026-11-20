//go:build darwin && !purego

package main

// #include <stdlib.h>
import "C"

import "unsafe"

// Sound from a recording (audio_darwin.go): mono, at rate.
//
//export mycoAudioSamples
func mycoAudioSamples(id C.int, data *C.float, frames C.int, rate C.double) {
	audioMu.Lock()
	f := audioSink[int(id)]
	audioMu.Unlock()
	if f == nil || frames <= 0 || data == nil {
		return
	}
	s := make([]float32, int(frames))
	copy(s, unsafe.Slice((*float32)(unsafe.Pointer(data)), int(frames)))
	f(s, int(rate))
}
