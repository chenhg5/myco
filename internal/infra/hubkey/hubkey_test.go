package hubkey

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSealedValuesOpenOnlyForTheirPurposeAndWithTheKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	k := New(path)
	if k.Present() {
		t.Fatal("a key before one was made")
	}
	if _, err := k.Seal("p1", []byte("secret"), false); !errors.Is(err, ErrMissing) {
		t.Fatalf("sealed without making a key: %v", err)
	}
	s, err := k.Seal("p1", []byte("secret"), true)
	if err != nil || !IsSealed(s) {
		t.Fatalf("seal: %q %v", s, err)
	}
	if got, err := k.Open("p1", s); err != nil || string(got) != "secret" {
		t.Fatalf("open: %q %v", got, err)
	}
	if _, err := k.Open("p2", s); !errors.Is(err, ErrOpen) {
		t.Fatalf("opened for another purpose: %v", err)
	}
	if info, err := os.Stat(path); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Fatalf("key file: %v %v", info.Mode(), err)
	}
	// Another hub process on the same files reads the same key.
	if got, err := New(path).Open("p1", s); err != nil || string(got) != "secret" {
		t.Fatalf("open from the file: %q %v", got, err)
	}
	// The key gone: nothing opens, and nothing makes a new one unasked.
	os.Remove(path)
	other := New(path)
	if _, err := other.Open("p1", s); !errors.Is(err, ErrMissing) {
		t.Fatalf("open with the key gone: %v", err)
	}
	if other.Present() {
		t.Fatal("a key appeared")
	}
	// A new key does not open what the old one sealed.
	if _, err := other.Seal("p1", []byte("x"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := other.Open("p1", s); !errors.Is(err, ErrOpen) {
		t.Fatalf("a new key opened the old value: %v", err)
	}
}

func TestAKeyMadeByAnotherProcessIsKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	a, b := New(path), New(path)
	sa, err := a.Seal("x", []byte("1"), true)
	if err != nil {
		t.Fatal(err)
	}
	// b had not loaded yet: it finds a's key, not a new one
	if _, err := b.Seal("x", []byte("2"), true); err != nil {
		t.Fatal(err)
	}
	if got, err := b.Open("x", sa); err != nil || string(got) != "1" {
		t.Fatalf("b made its own key: %v", err)
	}
}

func TestInMemory(t *testing.T) {
	k := New("")
	if _, err := k.Open("x", Prefix+"AAAA"); !errors.Is(err, ErrMissing) {
		t.Fatal(err)
	}
	s, err := k.Seal("x", []byte("y"), true)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := k.Open("x", s); string(got) != "y" {
		t.Fatal("in memory")
	}
}
