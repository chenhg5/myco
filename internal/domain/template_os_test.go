package domain

import (
	"strings"
	"testing"
)

// A manifest declares the computer's capabilities it uses (os) and the
// hosts it reaches (net); unknown groups and malformed hosts are refused.
func TestManifestOSAndNet(t *testing.T) {
	base := `{"id":"glenn/sea","name":"航海","version":"1.0.0"%s}`
	ok := func(extra string) *Manifest {
		t.Helper()
		m, err := ParseManifest([]byte(strings.Replace(base, "%s", extra, 1)))
		if err != nil {
			t.Fatalf("%s: %v", extra, err)
		}
		return m
	}
	bad := func(extra, want string) {
		t.Helper()
		_, err := ParseManifest([]byte(strings.Replace(base, "%s", extra, 1)))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: want an error with %q, got %v", extra, want, err)
		}
	}
	m := ok(`,"os":["files","microphone","network"],"net":["api.example.com","*.example.org","192.168.1.20:8080","localhost:5173"]`)
	if len(m.OS) != 3 || len(m.Net) != 4 || m.Net[1] != "*.example.org" {
		t.Fatalf("parsed: %+v %+v", m.OS, m.Net)
	}
	if m := ok(``); m.OS != nil || m.Net != nil {
		t.Fatal("none declared: none")
	}
	bad(`,"os":["camera"]`, `os "camera"`)
	bad(`,"os":["network"],"net":["https://api.example.com"]`, `net "https://api.example.com"`)
	bad(`,"os":["network"],"net":["API.example.com"]`, `net "API.example.com"`)
	bad(`,"os":["network"],"net":["*"]`, `net "*"`)
	bad(`,"os":["network"],"net":["a.*.com"]`, `net "a.*.com"`)
	bad(`,"net":["api.example.com"]`, `os has no "network"`)
}
