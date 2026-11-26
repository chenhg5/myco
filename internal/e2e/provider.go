package e2e

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"strconv"
)

const ProviderSettingsType = "provider.settings"

// ProviderKeyType is a model provider's key, sealed for its owner's own
// devices and kept in the workspace (i115).
const ProviderKeyType = "provider.key"

// ProviderKeyHeader binds a provider key envelope to its owner, the
// provider and the key's version: one cannot be passed off as another's,
// nor an older key as the current one.
func ProviderKeyHeader(owner, providerID string, version int) Header {
	return Header{From: owner, To: owner, Type: ProviderKeyType, ReplyTo: providerID + "@" + strconv.Itoa(version)}
}

// VerifySealedFor checks an envelope without opening it: it is signed with
// signKey, and each of its readers is one of allowed (X25519 public keys),
// at least one. Unknown JSON fields are refused.
func VerifySealedFor(body string, h Header, signKey string, allowed []string) error {
	ok := map[string]bool{}
	for _, a := range allowed {
		ok[KeyID(a)] = true
	}
	return verifyEnvelope(body, h, signKey, func(ids []string) bool {
		if len(ids) == 0 {
			return false
		}
		for _, id := range ids {
			if !ok[id] {
				return false
			}
		}
		return true
	})
}

// VerifyProvider checks the opaque envelope without decrypting it. Only the
// target agent may be a recipient; unknown JSON fields are rejected so the
// envelope cannot carry a second, unencrypted settings object alongside it.
func VerifyProvider(body string, h Header, signKey, recipient string) error {
	return verifyEnvelope(body, h, signKey, func(ids []string) bool { return len(ids) == 1 && ids[0] == KeyID(recipient) })
}

func verifyEnvelope(body string, h Header, signKey string, readersOK func([]string) bool) error {
	if !IsSealed(body) {
		return ErrMalformed
	}
	raw, err := b64.DecodeString(body[len(Prefix):])
	if err != nil {
		return ErrMalformed
	}
	var e envelope
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&e) != nil || e.From != KeyID(signKey) {
		return ErrMalformed
	}
	ids := make([]string, 0, len(e.K))
	for _, k := range e.K {
		wk, err := b64.DecodeString(k.WK)
		if err != nil || len(wk) != 48 {
			return ErrMalformed
		}
		ids = append(ids, k.ID)
	}
	if !readersOK(ids) {
		return ErrMalformed
	}
	epk, err := b64.DecodeString(e.EPK)
	if err != nil || len(epk) != 32 {
		return ErrMalformed
	}
	nonce, err := b64.DecodeString(e.N)
	if err != nil || len(nonce) != 12 {
		return ErrMalformed
	}
	ct, err := b64.DecodeString(e.CT)
	if err != nil || len(ct) < 16 {
		return ErrMalformed
	}
	pub, err := b64.DecodeString(signKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return ErrUnknownSigner
	}
	sig, err := b64.DecodeString(e.Sig)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(pub), signed(h, e), sig) {
		return ErrBadSignature
	}
	return nil
}
