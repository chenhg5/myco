package main

import "github.com/myco-ai/myco/internal/console"

// voiceAudio is this platform's microphone and speaker for voice calls
// (voice_darwin.go sets it; nil elsewhere).
var voiceAudio console.VoiceAudio
