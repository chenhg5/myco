package console

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The phone ships the same list of voices as the desktop App (it has no
// backend to ask): changed here, it is copied there.
func TestThePhoneShipsTheSameVoices(t *testing.T) {
	path := filepath.Join("..", "..", "ios", "MycoKit", "Sources", "MycoCore", "Resources", "doubao-voices.json")
	phone, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skip("no ios/ in this checkout")
	}
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(phone, doubaoVoicesJSON) {
		t.Fatalf("%s differs from voice_doubao_voices.json: copy the file over it", path)
	}
}
