// Package security provides credential and identifier generation.
package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"strings"
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

func randomString(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return strings.ToLower(b32.EncodeToString(buf))
}

// IDs generates prefixed random IDs such as msg_k3j2....
type IDs struct{}

func (IDs) New(prefix string) string { return prefix + "_" + randomString(10) }

// Tokens issues agent bearer tokens ("myco_...") stored as SHA-256 hashes.
type Tokens struct{}

func (t Tokens) Issue() (plain, hash string, err error) {
	plain = "myco_" + randomString(24)
	return plain, t.Hash(plain), nil
}

func (Tokens) Hash(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
