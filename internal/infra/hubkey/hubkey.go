// Package hubkey keeps a workspace's own secret key (i119): a random 32-byte
// key in a file next to its database, with which the hub encrypts what it
// keeps for members (a model provider's API key, when the workspace has
// end-to-end encryption off). A copy of the database alone does not open
// them; the key file and the database together do, so a backup holds both
// and is kept like a secret.
package hubkey

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileName is the key file's name, next to the workspace's hub.db.
const FileName = "provider-secret.key"

// Prefix starts every sealed value.
const Prefix = "myco-hub:v1:"

// ErrMissing: there is no key (the file is gone, or was never made).
var ErrMissing = errors.New("the hub's secret key is missing")

// ErrOpen: the value is not one this key sealed for that purpose.
var ErrOpen = errors.New("cannot open a value sealed by the hub")

// Keeper seals and opens with the key in one file. With no path it keeps
// a key in memory only (an in-memory hub, tests).
type Keeper struct {
	path string
	mu   sync.Mutex
	key  []byte
}

// New answers a keeper for the key file at path ("" = in memory).
func New(path string) *Keeper { return &Keeper{path: path} }

// Path is where the key is kept ("" in memory).
func (k *Keeper) Path() string { return k.path }

// Present reports whether there is a key to open with.
func (k *Keeper) Present() bool {
	_, err := k.load(false)
	return err == nil
}

// Seal encrypts plain for purpose (bound to it: a value sealed for one
// provider and version does not open as another's). With create, a
// missing key is made first; without it, a missing key is ErrMissing.
func (k *Keeper) Seal(purpose string, plain []byte, create bool) (string, error) {
	key, err := k.load(create)
	if err != nil {
		return "", err
	}
	aead, err := gcm(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := aead.Seal(nonce, nonce, plain, []byte(purpose))
	return Prefix + base64.RawURLEncoding.EncodeToString(out), nil
}

// Open decrypts a value sealed for purpose.
func (k *Keeper) Open(purpose, sealed string) ([]byte, error) {
	raw, ok := strings.CutPrefix(sealed, Prefix)
	if !ok {
		return nil, ErrOpen
	}
	key, err := k.load(false)
	if err != nil {
		return nil, err
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, ErrOpen
	}
	aead, err := gcm(key)
	if err != nil {
		return nil, err
	}
	if len(b) < aead.NonceSize() {
		return nil, ErrOpen
	}
	plain, err := aead.Open(nil, b[:aead.NonceSize()], b[aead.NonceSize():], []byte(purpose))
	if err != nil {
		return nil, ErrOpen
	}
	return plain, nil
}

// IsSealed reports whether s looks like a value the hub sealed.
func IsSealed(s string) bool { return strings.HasPrefix(s, Prefix) }

func gcm(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (k *Keeper) load(create bool) ([]byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.key != nil {
		return k.key, nil
	}
	if k.path == "" {
		if !create {
			return nil, ErrMissing
		}
		k.key = make([]byte, 32)
		if _, err := rand.Read(k.key); err != nil {
			k.key = nil
			return nil, err
		}
		return k.key, nil
	}
	b, err := os.ReadFile(k.path)
	switch {
	case err == nil:
		if len(b) != 32 {
			return nil, fmt.Errorf("the hub's secret key %s is damaged", k.path)
		}
		k.key = b
		return b, nil
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("reading the hub's secret key: %w", err)
	case !create:
		return nil, ErrMissing
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := write(k.path, key); errors.Is(err, errExists) {
		// made by another process meanwhile: that one is the key
		b, err := os.ReadFile(k.path)
		if err != nil || len(b) != 32 {
			return nil, fmt.Errorf("reading the hub's secret key: %v", err)
		}
		key = b
	} else if err != nil {
		return nil, err
	}
	k.key = key
	return key, nil
}

var errExists = errors.New("the key file exists")

// write puts the key in place whole (a temporary file, then a rename), for
// the hub's user only; it never replaces a key already there.
func write(path string, key []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".provider-secret-*")
	if err != nil {
		return fmt.Errorf("saving the hub's secret key: %w", err)
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		f.Close()
		return fmt.Errorf("saving the hub's secret key: %w", err)
	}
	if _, err := f.Write(key); err != nil {
		f.Close()
		return fmt.Errorf("saving the hub's secret key: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("saving the hub's secret key: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("saving the hub's secret key: %w", err)
	}
	// Link, not rename: if another process made a key meanwhile, keep it.
	if err := os.Link(tmp, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return errExists
		}
		// Filesystems without hard links: a rename, after checking again.
		if _, serr := os.Stat(path); serr == nil {
			return errExists
		}
		if rerr := os.Rename(tmp, path); rerr != nil {
			return fmt.Errorf("saving the hub's secret key: %w", rerr)
		}
	}
	return nil
}
