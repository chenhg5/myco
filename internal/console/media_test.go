package console

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMediaPath(t *testing.T) {
	home := t.TempDir()
	rec := filepath.Join(home, ".myco", "recordings", "m1")
	_ = os.MkdirAll(rec, 0o700)
	_ = os.WriteFile(filepath.Join(rec, "me.wav"), []byte("RIFF"), 0o600)
	for _, ext := range []string{".mov", ".mp4", ".webm"} {
		_ = os.WriteFile(filepath.Join(rec, "screen"+ext), []byte("movie"), 0600)
		if _, ok := mediaPath(home, filepath.Join(rec, "screen"+ext)); !ok {
			t.Errorf("recorded video rejected: %s", ext)
		}
	}
	_ = os.WriteFile(filepath.Join(rec, "notes.txt"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(home, "secret.wav"), []byte("x"), 0o600)
	for p, want := range map[string]bool{
		filepath.Join(rec, "me.wav"):                       true,
		"~/.myco/recordings/m1/me.wav":                     true,
		filepath.Join(rec, "notes.txt"):                    false, // not a recording
		filepath.Join(home, "secret.wav"):                  false, // not in recordings
		filepath.Join(rec, "..", "..", "..", "secret.wav"): false,
		"relative/me.wav":                                  false,
		"":                                                 false,
	} {
		if _, ok := mediaPath(home, p); ok != want {
			t.Errorf("%q: %v, want %v", p, ok, want)
		}
	}
	if runtime.GOOS != "windows" {
		// a link in recordings to elsewhere is not followed
		_ = os.Symlink(filepath.Join(home, "secret.wav"), filepath.Join(rec, "link.wav"))
		if _, ok := mediaPath(home, filepath.Join(rec, "link.wav")); ok {
			t.Error("a link out of recordings")
		}
	}
}
