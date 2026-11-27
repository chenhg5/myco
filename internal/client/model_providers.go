package client

import (
	"context"
	"net/http"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/e2e"
)

// The member's model providers (i115). Their keys travel sealed for the
// member's own devices: these calls never carry one in the clear.

func (c *Client) ModelProviders(ctx context.Context) (*api.ModelProviderList, error) {
	var out api.ModelProviderList
	return &out, c.do(ctx, http.MethodGet, "/v1/console/providers", nil, nil, &out)
}

func (c *Client) CreateModelProvider(ctx context.Context, in api.ModelProviderRequest) (*api.ModelProvider, error) {
	var out api.ModelProvider
	return &out, c.do(ctx, http.MethodPost, "/v1/console/providers", nil, in, &out)
}

func (c *Client) UpdateModelProvider(ctx context.Context, id string, in api.ModelProviderRequest) (*api.ModelProvider, error) {
	var out api.ModelProvider
	return &out, c.do(ctx, http.MethodPatch, "/v1/console/providers/"+esc(id), nil, in, &out)
}

func (c *Client) DeleteModelProvider(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/console/providers/"+esc(id), nil, nil, nil)
}

// myDevices are the encryption keys of the member's devices (this one
// among them), as the hub lists them, to seal for: with the workspace's
// encryption on only.
func (c *Client) myDevices(ctx context.Context) ([]api.KeyHolder, error) {
	return c.devicesOf(ctx, true)
}

// devicesOf is myDevices; to open a key (forSealing false) the workspace
// may have encryption off by now: a key sealed before is still opened, to
// hand it to the hub (i119).
func (c *Client) devicesOf(ctx context.Context, forSealing bool) ([]api.KeyHolder, error) {
	s := c.Sealer
	if s == nil || s.ID == nil || !s.Device {
		return nil, &Error{Status: 400, Code: "e2e_required", Message: "this device has no encryption identity"}
	}
	// (this device's keys first: the hub accepts only what its devices seal)
	if err := c.registerKeys(ctx); err != nil {
		return nil, err
	}
	keys, err := c.KeysOf(ctx, s.Me)
	if err != nil {
		return nil, err
	}
	if !keys.E2E && forSealing {
		return nil, &Error{Status: 409, Code: "e2e_required", Message: "this workspace has end-to-end encryption off"}
	}
	return keys.Keys[s.Me], nil
}

// SealProviderKey seals a model provider's key (payload: what the member's
// devices need, JSON) for all of the member's devices, signed by this one.
func (c *Client) SealProviderKey(ctx context.Context, member, id string, version int, payload []byte) (string, error) {
	devices, err := c.myDevices(ctx)
	if err != nil {
		return "", err
	}
	var readers []string
	for _, d := range devices {
		readers = append(readers, d.EncKey)
	}
	return e2e.Seal(c.Sealer.ID, e2e.ProviderKeyHeader(member, id, version), payload, readers)
}

// OpenProviderKey opens a model provider's key, checking that one of the
// member's own devices sealed it for this version.
func (c *Client) OpenProviderKey(ctx context.Context, member string, p api.ModelProvider) ([]byte, error) {
	devices, err := c.devicesOf(ctx, false)
	if err != nil {
		return nil, err
	}
	signer := e2e.Signer(p.KeySealed)
	h := e2e.ProviderKeyHeader(member, p.ID, p.KeyVersion)
	for _, d := range devices {
		if e2e.KeyID(d.SignKey) != signer {
			continue
		}
		return e2e.Open(c.Sealer.ID, h, p.KeySealed, d.SignKey)
	}
	return nil, e2e.ErrUnknownSigner
}

// ProviderKeyStale reports whether a provider's key is sealed for another
// set of devices than the member's now (a device added or removed): one
// of their devices that can open it seals it again (rewrap).
func (c *Client) ProviderKeyStale(ctx context.Context, p api.ModelProvider) (bool, error) {
	devices, err := c.myDevices(ctx)
	if err != nil {
		return false, err
	}
	want := map[string]bool{}
	for _, d := range devices {
		want[e2e.KeyID(d.EncKey)] = true
	}
	have := e2e.Readers(p.KeySealed)
	if len(have) != len(want) {
		return true, nil
	}
	for _, id := range have {
		if !want[id] {
			return true, nil
		}
	}
	return false, nil
}

// ModelProviderKey reads the key the hub keeps for one of the member's own
// providers (i119): to seal it for their devices, or to list the vendor's
// models from this computer. Never shown, logged or stored here.
func (c *Client) ModelProviderKey(ctx context.Context, id string) (*api.ProviderKey, error) {
	var out api.ProviderKey
	if err := c.do(ctx, http.MethodGet, "/v1/console/providers/"+esc(id)+"/key", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
