package console

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// keychainService names the console's entries in the system keychain
// (macOS Keychain, Secret Service on Linux, Credential Manager on Windows).
const keychainService = "myco console"

// Keychain stores console tokens in the system keychain.
type Keychain struct{}

func (Keychain) Get(hubID string) (string, error) { return keyring.Get(keychainService, hubID) }
func (Keychain) Set(hubID, token string) error    { return keyring.Set(keychainService, hubID, token) }

// Delete removes a token; one that is already gone is not an error.
func (Keychain) Delete(hubID string) error {
	if err := keyring.Delete(keychainService, hubID); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}
