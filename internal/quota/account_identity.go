package quota

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// AccountIDHash is a non-authenticating, domain-separated stable-ID digest.
// The Hub applies its own secret HMAC and owner namespace before persistence.
func AccountIDHash(provider, id string) string {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 512 {
		return ""
	}
	sum := sha256.Sum256([]byte("myco/quota-account-id/v1\x00" + provider + "\x00" + id))
	return "qa_" + hex.EncodeToString(sum[:])
}
func (s Source) AccountIDHash() string {
	if s.Status == "unsupported" || s.Provider != "claude" && s.Provider != "codex" || s.Kind != "native" && s.Kind != "cache" {
		return ""
	}
	return AccountIDHash(s.Provider, s.account)
}
