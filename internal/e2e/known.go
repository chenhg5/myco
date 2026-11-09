package e2e

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// KeyChangedError: an identity's keys are not the ones seen before (trust on
// first use). The hub may have swapped them, or the identity made new ones.
type KeyChangedError struct{ Who string }

func (e *KeyChangedError) Error() string {
	return fmt.Sprintf("the encryption keys of %s changed since they were first seen", e.Who)
}

// IsKeyChanged reports whether err is a *KeyChangedError.
func IsKeyChanged(err error) bool {
	var k *KeyChangedError
	return errors.As(err, &k)
}

// Known pins identities' public keys the first time they are seen
// (~/.myco/known_keys.json, like SSH's known_hosts), shared by every agent
// on the machine.
type Known struct {
	Path string
	mu   sync.Mutex
}

func (k *Known) load() (map[string]PublicKeys, error) {
	m := map[string]PublicKeys{}
	b, err := os.ReadFile(k.Path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", k.Path, err)
		}
	}
	return m, nil
}

func (k *Known) save(m map[string]PublicKeys) error {
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.MkdirAll(filepath.Dir(k.Path), 0o700); err != nil {
		return err
	}
	tmp := k.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, k.Path)
}

// Check pins who's keys on first sight, and returns a *KeyChangedError when
// they differ from the pinned ones.
func (k *Known) Check(who string, pub PublicKeys) error {
	if k == nil || k.Path == "" {
		return nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	m, err := k.load()
	if err != nil {
		return err
	}
	old, ok := m[who]
	if ok {
		if old != pub {
			return &KeyChangedError{Who: who}
		}
		return nil
	}
	m[who] = pub
	return k.save(m)
}

// Trust forgets who's pinned keys, so the current ones are pinned next time.
func (k *Known) Trust(who string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	m, err := k.load()
	if err != nil {
		return err
	}
	delete(m, who)
	return k.save(m)
}

// Pinned returns who's pinned keys.
func (k *Known) Pinned(who string) (PublicKeys, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	m, err := k.load()
	if err != nil {
		return PublicKeys{}, false
	}
	p, ok := m[who]
	return p, ok
}
