//go:build darwin && !purego

package main

import (
	"context"
	"testing"
)

// Listing what can be recorded asks for no permission: the microphone and,
// on macOS 14.2+, the Mac's sound and the apps that have played some.
func TestMacAudioSources(t *testing.T) {
	list, err := macAudio{}.Sources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 || list[0].ID != "mic" {
		t.Fatalf("%+v", list)
	}
	for _, s := range list {
		t.Logf("%s %q playing=%v", s.ID, s.Name, s.Playing)
	}
}
