package domain

import (
	"context"
	"strings"
	"time"
)

// SealedPrefix starts an end-to-end encrypted body (internal/e2e.Prefix):
// the hub stores and relays such bodies without reading them.
const SealedPrefix = "myco-e2e:"

// IsSealed reports whether s is an end-to-end encrypted body.
func IsSealed(s string) bool { return strings.HasPrefix(s, SealedPrefix) }

// Key kinds: whose public keys these are.
const (
	KeyAgent  = "agent"  // id: the agent's name
	KeyDevice = "device" // id: the console token's id (an app or CLI device of a member)
)

// PublicKey is an identity's public keys for end-to-end encryption
// (X25519 to receive, Ed25519 to sign; base64url). The hub only relays them.
type PublicKey struct {
	Kind, ID  string
	Enc, Sign string
	UpdatedAt time.Time
}

// KeyRepository stores public keys.
type KeyRepository interface {
	Put(ctx context.Context, k *PublicKey) error
	// Get returns CodeNotFound when the identity has no key.
	Get(ctx context.Context, kind, id string) (*PublicKey, error)
}

// SettingRepository stores hub-wide settings.
type SettingRepository interface {
	// Get returns "" when the setting was never set.
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}
