package e2e

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
)

const ProviderSettingsType = "provider.settings"

// VerifyProvider checks the opaque envelope without decrypting it. Only the
// target agent may be a recipient; unknown JSON fields are rejected so the
// envelope cannot carry a second, unencrypted settings object alongside it.
func VerifyProvider(body string, h Header, signKey, recipient string) error {
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
	if d.Decode(&e) != nil || len(e.K) != 1 || e.K[0].ID != KeyID(recipient) || e.From != KeyID(signKey) {
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
	wk, err := b64.DecodeString(e.K[0].WK)
	if err != nil || len(wk) != 48 {
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
