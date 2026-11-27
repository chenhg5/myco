package domain

import (
	"context"
	"strconv"
	"time"
)

// ModelProvider is a model vendor a member uses with their own API key,
// kept in the workspace so every computer they add an agent on can use it
// (i115). Who keeps the key is its Custody: with end-to-end encryption on,
// KeySealed is an envelope only the member's own devices can open, and the
// hub cannot (CustodyE2E); with it off, the hub keeps it, encrypted with
// the workspace's own key (KeyHub, i119), and its administrator could read
// it (CustodyHub).
type ModelProvider struct {
	ID string `json:"id"`
	// Owner is the member whose provider it is (their id).
	Owner string `json:"owner"`
	Name  string `json:"name"`
	// Vendor is a catalogue id (provider.Vendors) or "custom".
	Vendor string `json:"vendor"`
	// AnthropicBase and OpenAIBase are its two interfaces' base URLs (one
	// may be empty): Claude Code uses the first, Codex the second.
	AnthropicBase string `json:"anthropic_base,omitempty"`
	OpenAIBase    string `json:"openai_base,omitempty"`
	// SmallModel is for Claude Code's background work ("" = the vendor's,
	// else the chosen model).
	SmallModel string `json:"small_model,omitempty"`
	// Models is the list last read with the key (EncodeModels JSON), at
	// ModelsAt; not secret.
	Models   string    `json:"models,omitempty"`
	ModelsAt time.Time `json:"models_at,omitzero"`
	// KeySealed is the key, sealed for the owner's devices; KeyVersion
	// goes up when the key changes (not when it is sealed again for a new
	// set of devices).
	KeySealed  string `json:"key_sealed"`
	KeyVersion int    `json:"key_version"`
	// Custody is CustodyE2E ("" too, from before i119) or CustodyHub.
	Custody string `json:"custody,omitempty"`
	// KeyHub is the key sealed with the workspace's key (CustodyHub only),
	// bound to the provider, its owner and KeyVersion (HubKeyPurpose).
	KeyHub string `json:"key_hub,omitempty"`
	// KeyHint is the key's last four characters (CustodyHub only: with
	// CustodyE2E the member's App shows them, the hub cannot).
	KeyHint   string    `json:"key_hint,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Who keeps a model provider's key.
const (
	CustodyE2E = "e2e"
	CustodyHub = "hub"
)

// HubHeld reports whether the hub keeps this provider's key.
func (p *ModelProvider) HubHeld() bool { return p.Custody == CustodyHub }

// HubKeyPurpose binds a key sealed by the hub to its provider, owner and
// version: sealed for one, it does not open as another's.
func HubKeyPurpose(providerID, owner string, version int) string {
	return "model-provider-key|" + providerID + "|" + owner + "|" + strconv.Itoa(version)
}

// MaxModelProviders bounds one member's providers.
const MaxModelProviders = 50

// ModelProviderRepository keeps members' model providers.
type ModelProviderRepository interface {
	// List answers the owner's providers, oldest first.
	List(ctx context.Context, owner string) ([]*ModelProvider, error)
	Get(ctx context.Context, id string) (*ModelProvider, error)
	Create(ctx context.Context, p *ModelProvider) error
	Update(ctx context.Context, p *ModelProvider) error
	Delete(ctx context.Context, id string) error
	// AnyHubHeld reports whether any provider's key is kept by the hub
	// (then a missing workspace key is lost, not yet to be made).
	AnyHubHeld(ctx context.Context) (bool, error)
}
