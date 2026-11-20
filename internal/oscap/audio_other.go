//go:build !linux

package oscap

// Here, what records sound is set by the App (SetAudio), if anything.
func osAudio() AudioBackend { return nil }
