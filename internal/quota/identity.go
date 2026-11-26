package quota

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// credentialRevision is local-only. File generations detect A -> B -> A
// switches even when the final account or credential bytes match again.
func credentialRevision(paths ...string) string {
	h := sha256.New()
	found := false
	for _, path := range paths {
		if st, e := os.Stat(path); e == nil {
			found = true
			fmt.Fprintf(h, "%s:%d:%d\n", canonical(path), st.ModTime().UnixNano(), st.Size())
		}
	}
	if !found {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}
func keychainPaths(home string) []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	root := filepath.Join(home, "Library", "Keychains")
	paths := []string{filepath.Join(root, "login.keychain-db"), filepath.Join(root, "login.keychain")}
	nested, _ := filepath.Glob(filepath.Join(root, "*", "keychain-2.db"))
	return append(paths, nested...)
}
func codexAuth(dir, home, mode string) ([]byte, string) {
	var raw []byte
	var paths []string
	if mode == "keyring" || mode == "auto" {
		raw = codexKeychain(dir)
		if len(raw) > 0 {
			paths = keychainPaths(home)
			if credentialRevision(paths...) == "" {
				return nil, ""
			}
		}
	}
	if len(raw) == 0 && mode != "keyring" {
		path := filepath.Join(dir, "auth.json")
		raw, _ = os.ReadFile(path)
		paths = []string{path}
	}
	if len(raw) == 0 {
		return nil, ""
	}
	paths = append(paths, filepath.Join(dir, "config.toml"))
	h := sha256.New()
	h.Write(raw)
	h.Write([]byte(mode))
	h.Write([]byte(credentialRevision(paths...)))
	return raw, hex.EncodeToString(h.Sum(nil))
}

// SameLogin includes the local generation, not only the logical account.
func (s Source) SameLogin(other Source) bool {
	return s.Ref == other.Ref && s.revision == other.revision
}
func (s Source) CanRecord(other Source) bool { return s.revision != "" && s.SameLogin(other) }
