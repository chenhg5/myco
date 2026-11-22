package e2e

import (
	"encoding/json"
	"testing"
)

func TestProviderEnvelopeTargetAndSignature(t *testing.T) {
	sender, _ := NewIdentity()
	target, _ := NewIdentity()
	other, _ := NewIdentity()
	h := Header{From: "@console:owner", To: "agent", Type: ProviderSettingsType, ReplyTo: "request-identifier"}
	body, err := Seal(sender, h, []byte(`{"api_key":"secret-only-at-endpoints"}`), []string{target.Public().Enc})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyProvider(body, h, sender.Public().Sign, target.Public().Enc); err != nil {
		t.Fatal(err)
	}
	wrong := h
	wrong.To = "other-agent"
	if VerifyProvider(body, wrong, sender.Public().Sign, target.Public().Enc) == nil {
		t.Fatal("rerouted envelope accepted")
	}
	if VerifyProvider(body, h, other.Public().Sign, target.Public().Enc) == nil {
		t.Fatal("wrong signer accepted")
	}
	if VerifyProvider(body, h, sender.Public().Sign, other.Public().Enc) == nil {
		t.Fatal("wrong recipient accepted")
	}
	multi, _ := Seal(sender, h, []byte("secret"), []string{target.Public().Enc, other.Public().Enc})
	if VerifyProvider(multi, h, sender.Public().Sign, target.Public().Enc) == nil {
		t.Fatal("extra recipient accepted")
	}
	raw, _ := b64.DecodeString(body[len(Prefix):])
	var doc map[string]any
	json.Unmarshal(raw, &doc)
	doc["api_key"] = "plaintext-side-channel"
	raw, _ = json.Marshal(doc)
	if VerifyProvider(Prefix+b64.EncodeToString(raw), h, sender.Public().Sign, target.Public().Enc) == nil {
		t.Fatal("unrecognized plaintext envelope fields accepted")
	}
}
