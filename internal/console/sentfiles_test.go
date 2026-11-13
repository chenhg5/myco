package console

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

// What the app uploaded is read back from its own sealed copy.
func TestSentFiles(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	c, err := OpenCache(filepath.Join(t.TempDir(), "cache.db"), key)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	png := []byte("\x89PNG not really")
	if err := c.PutSent("hub_a", "f_1", "截图.png", "image/png", png); err != nil {
		t.Fatal(err)
	}
	name, ct, data, ok := c.Sent("hub_a", "f_1")
	if !ok || name != "截图.png" || ct != "image/png" || string(data) != string(png) {
		t.Fatalf("Sent = %q %q %q %v", name, ct, data, ok)
	}
	// Another hub's file of the same id, or another id, is not this one.
	if _, _, _, ok := c.Sent("hub_b", "f_1"); ok {
		t.Fatal("a copy answered for another hub")
	}
	if _, _, _, ok := c.Sent("hub_a", "f_2"); ok {
		t.Fatal("a copy answered for another file")
	}
	// The folder alone reveals nothing, and a damaged copy is dropped.
	p := c.sentPath("hub_a", "f_1")
	blob, _ := os.ReadFile(p)
	if string(blob) == string(png) || len(blob) == 0 {
		t.Fatal("the copy is not sealed")
	}
	blob[len(blob)-1] ^= 0xff
	_ = os.WriteFile(p, blob, 0o600)
	if _, _, _, ok := c.Sent("hub_a", "f_1"); ok {
		t.Fatal("a damaged copy was read")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("a damaged copy was kept")
	}
	// A nil cache (the browser console) keeps nothing and says so.
	var none *Cache
	if err := none.PutSent("h", "f", "n", "t", png); err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok := none.Sent("h", "f"); ok {
		t.Fatal("a nil cache answered")
	}
}
