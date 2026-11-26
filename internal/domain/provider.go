package domain

import (
	"context"
	"time"
)

// ModelProvider is a model vendor a member uses with their own API key,
// kept in the workspace so every computer they add an agent on can use it
// (i115). The key is never in it: KeySealed is an end-to-end envelope only
// the member's own devices can open, and the hub cannot.
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
	KeySealed  string    `json:"key_sealed"`
	KeyVersion int       `json:"key_version"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
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
}
