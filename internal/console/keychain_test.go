package console

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

// Without a Secret Service (a Linux server, a bare window manager), the
// tokens go to a 0600 file and come back from it; without File set, the
// keychain's refusal stays a refusal.
func TestKeychainFileFallback(t *testing.T) {
	keyring.MockInitWithError(errors.New("org.freedesktop.secrets was not provided"))
	t.Cleanup(keyring.MockInit)
	file := filepath.Join(t.TempDir(), "myco", "console-secrets.json")
	k := Keychain{File: file}

	if err := k.Set("hub_1", "tok-1"); err != nil {
		t.Fatal(err)
	}
	if s, err := k.Get("hub_1"); err != nil || s != "tok-1" {
		t.Fatalf("get: %q %v", s, err)
	}
	if st, err := os.Stat(file); err != nil || (st.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/') {
		t.Fatalf("the file must be private: %v %v", st, err)
	}
	if _, err := k.Get("hub_2"); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("a missing token is not found: %v", err)
	}
	key, err := k.CacheKey()
	if err != nil || len(key) != 32 {
		t.Fatalf("cache key: %v", err)
	}
	if again, _ := k.CacheKey(); string(again) != string(key) {
		t.Fatal("the cache key must stay the same")
	}
	if err := k.Delete("hub_1"); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Get("hub_1"); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("deleted: %v", err)
	}
	if err := (Keychain{}).Set("hub_1", "tok"); err == nil {
		t.Fatal("without File, a keychain error must be returned")
	}

	// The keychain works again: a token set now leaves the file.
	keyring.MockInit()
	k.fileSet("hub_3", "old")
	if err := k.Set("hub_3", "new"); err != nil {
		t.Fatal(err)
	}
	if _, ok := k.fileGet("hub_3"); ok {
		t.Fatal("a token in the keychain must not stay in the file")
	}
	if s, _ := k.Get("hub_3"); s != "new" {
		t.Fatalf("get: %q", s)
	}
}
