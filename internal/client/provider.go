package client

import (
	"context"
	"crypto/rand"
	"net/http"
	"strings"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/e2e"
)

// SealProvider seals raw local settings only for the agent, never its owner or
// the Hub. Unlike message encryption, missing E2E never falls back to plaintext.
func (c *Client) SealProvider(ctx context.Context, name string, raw []byte) (*api.EncryptedProvider, error) {
	s := c.Sealer
	if s == nil || s.ID == nil || !s.Device || !strings.HasPrefix(s.Me, "@console:") {
		return nil, &Error{Status: 400, Code: "e2e_required", Message: "this device has no encryption identity", Suggestion: "enter the provider in the App or CLI on that computer"}
	}
	keys, err := c.KeysOf(ctx, name)
	if err != nil {
		return nil, err
	}
	if !keys.E2E {
		return nil, &Error{Status: 400, Code: "e2e_required", Message: "remote custom providers require an encrypted workspace", Suggestion: "enter the provider in the App or CLI on that computer"}
	}
	target := keys.Keys[name]
	if len(target) != 1 {
		return nil, &Error{Status: 409, Code: "keys_missing", Message: "the target agent has not registered its encryption key", Suggestion: "start its updated runner first"}
	}
	if err := s.pin(target[0]); err != nil {
		return nil, err
	}
	id := rand.Text()
	body, err := e2e.Seal(s.ID, e2e.Header{From: s.Me, To: name, Type: e2e.ProviderSettingsType, ReplyTo: id}, raw, []string{target[0].EncKey})
	if err != nil {
		return nil, err
	}
	return &api.EncryptedProvider{ID: id, From: s.Me, Body: body}, nil
}

// OpenProvider verifies the owner's device signature and target before decoding
// locally. No plaintext is ever submitted to the Hub by this method.
func (c *Client) OpenProvider(ctx context.Context, p api.EncryptedProvider) ([]byte, error) {
	s := c.Sealer
	if s == nil || s.ID == nil {
		return nil, e2e.ErrNotForMe
	}
	keys, err := c.KeysOf(ctx, p.From)
	if err != nil {
		return nil, err
	}
	if !keys.E2E {
		return nil, &Error{Code: "e2e_required", Message: "workspace encryption is disabled"}
	}
	for _, holder := range keys.Keys[p.From] {
		if e2e.KeyID(holder.SignKey) != e2e.Signer(p.Body) {
			continue
		}
		// Device keys rotate independently; member devices are authenticated by
		// the same directory used for normal e2e messages.
		if err := s.pin(holder); err != nil {
			return nil, err
		}
		h := e2e.Header{From: p.From, To: s.Me, Type: e2e.ProviderSettingsType, ReplyTo: p.ID}
		if err := e2e.VerifyProvider(p.Body, h, holder.SignKey, s.ID.Public().Enc); err != nil {
			return nil, err
		}
		return e2e.Open(s.ID, h, p.Body, holder.SignKey)
	}
	return nil, e2e.ErrUnknownSigner
}

func (c *Client) AckProvider(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/v1/me/provider/ack", nil, map[string]string{"id": id}, nil)
}

func (c *Client) PatchRunSettings(ctx context.Context, name string, rs api.RunSettingsPatch) (*api.Agent, error) {
	var out api.Agent
	return &out, c.do(ctx, http.MethodPut, "/v1/agents/"+esc(name)+"/run-settings", nil, rs, &out)
}
