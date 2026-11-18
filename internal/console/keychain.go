package console

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/zalando/go-keyring"
)

// keychainService names the console's entries in the system keychain
// (macOS Keychain, Secret Service on Linux, Credential Manager on Windows).
const keychainService = "myco console"

// Keychain stores console tokens in the system keychain.
type Keychain struct {
	// File, when set, keeps the tokens the system keychain cannot: a Linux
	// computer without a Secret Service (no GNOME Keyring or KWallet, a
	// server, a bare window manager). It is written 0600, like the CLI's
	// own token files. Mac and Windows leave it empty: their keychain is
	// always there, and a refusal there must stay a refusal.
	File string
}

func (k Keychain) Get(hubID string) (string, error) {
	s, err := keyring.Get(keychainService, hubID)
	if err == nil || k.File == "" {
		return s, err
	}
	if s, ok := k.fileGet(hubID); ok {
		return s, nil
	}
	return "", keyring.ErrNotFound
}

func (k Keychain) Set(hubID, token string) error {
	err := keyring.Set(keychainService, hubID, token)
	if k.File == "" {
		return err
	}
	if err == nil {
		// The keychain works (again): nothing stays in the file.
		return k.fileSet(hubID, "")
	}
	return k.fileSet(hubID, token)
}

// Delete removes a token; one that is already gone is not an error.
func (k Keychain) Delete(hubID string) error {
	err := keyring.Delete(keychainService, hubID)
	if k.File != "" {
		if ferr := k.fileSet(hubID, ""); ferr != nil {
			return ferr
		}
		return nil
	}
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

// CacheKey returns the page cache's key, made the first time.
func (k Keychain) CacheKey() ([]byte, error) {
	const user = "app-cache-key"
	if s, err := k.Get(user); err == nil {
		if b, err := base64.StdEncoding.DecodeString(s); err == nil && len(b) == 32 {
			return b, nil
		}
	} else if !errors.Is(err, keyring.ErrNotFound) {
		return nil, err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	if err := k.Set(user, base64.StdEncoding.EncodeToString(b)); err != nil {
		return nil, err
	}
	return b, nil
}

var keychainFileMu sync.Mutex

func (k Keychain) fileLoad() map[string]string {
	m := map[string]string{}
	if b, err := os.ReadFile(k.File); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func (k Keychain) fileGet(id string) (string, bool) {
	keychainFileMu.Lock()
	defer keychainFileMu.Unlock()
	s, ok := k.fileLoad()[id]
	return s, ok
}

// fileSet stores token under id ("" removes it).
func (k Keychain) fileSet(id, token string) error {
	keychainFileMu.Lock()
	defer keychainFileMu.Unlock()
	m := k.fileLoad()
	if _, had := m[id]; token == "" && !had {
		return nil
	}
	if token == "" {
		delete(m, id)
	} else {
		m[id] = token
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(k.File), 0o700); err != nil {
		return err
	}
	tmp := k.File + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, k.File)
}
