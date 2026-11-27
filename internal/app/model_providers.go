package app

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/e2e"
)

// Members' model providers (i115): a vendor, its addresses and the
// member's API key, kept in the workspace so every computer they add an
// agent on can use it. The key is an end-to-end envelope sealed for the
// member's own devices: the hub only checks that it is one (signed by one
// of those devices, readable by them alone) and cannot open it. A member
// sees and changes only their own providers; shared ones are not offered
// (see docs/api.md).
type ModelProviderService struct {
	repo  domain.ModelProviderRepository
	e2e   *E2EService
	clock Clock
	// vendor reports whether a catalogue vendor id is known.
	vendor func(string) bool
	// box is the workspace's own key, for keys the hub keeps (i119);
	// heldElsewhere reports other values it sealed (agents' providers).
	box           SecretBox
	heldElsewhere func(context.Context) (bool, error)
	// manual makes a provider's settings for a CLI and model.
	manual func(ProviderSettings, string, string) (api.ProviderMaterial, error)
}

// SecretBox is the workspace's own key (infra/hubkey): what the hub keeps
// for members is sealed with it, bound to a purpose.
type SecretBox interface {
	Present() bool
	Seal(purpose string, plain []byte, create bool) (string, error)
	Open(purpose, sealed string) ([]byte, error)
}

// ProviderSettings are a model provider's, with its key, for a CLI.
type ProviderSettings struct {
	Vendor, AnthropicBase, OpenAIBase, SmallModel, APIKey string
}

// SetHubCustody lets the hub keep keys (i119): box seals them; held
// reports values it sealed outside providers; manual makes settings.
func (s *ModelProviderService) SetHubCustody(box SecretBox, held func(context.Context) (bool, error), manual func(ProviderSettings, string, string) (api.ProviderMaterial, error)) {
	s.box, s.heldElsewhere, s.manual = box, held, manual
}

func NewModelProviderService(repo domain.ModelProviderRepository, e2e *E2EService, clock Clock, vendor func(string) bool) *ModelProviderService {
	return &ModelProviderService{repo: repo, e2e: e2e, clock: clock, vendor: vendor}
}

var (
	modelProviderID = regexp.MustCompile(`^prv_[a-z0-9]{8,40}$`)
	vendorID        = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)
)

// Limits of a model provider.
const (
	maxProviderName  = 80
	maxProviderURL   = 300
	maxProviderModel = 80
	maxProviderKey   = 16 << 10 // the sealed key
	maxPlainKey      = 4 << 10  // the key itself
)

// E2E reports whether the workspace has end-to-end encryption on.
func (s *ModelProviderService) E2E(ctx context.Context) bool {
	return s.e2e != nil && s.e2e.Enabled(ctx)
}

// List answers the member's providers.
func (s *ModelProviderService) List(ctx context.Context, member string) ([]*domain.ModelProvider, error) {
	if member == "" {
		return nil, domain.Errf(domain.CodePermissionDenied, "model providers belong to members: sign in as one")
	}
	return s.repo.List(ctx, member)
}

// Owned answers the member's provider id (not found for anyone else's).
func (s *ModelProviderService) Owned(ctx context.Context, member, id string) (*domain.ModelProvider, error) {
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if member == "" || p.Owner != member {
		return nil, domain.Errf(domain.CodeNotFound, "model provider %s not found", id)
	}
	return p, nil
}

// Create adds a provider of the member's.
func (s *ModelProviderService) Create(ctx context.Context, member string, in api.ModelProviderRequest) (*domain.ModelProvider, error) {
	if err := s.writable(ctx, member); err != nil {
		return nil, err
	}
	if !modelProviderID.MatchString(in.ID) {
		return nil, domain.Errf(domain.CodeInvalidArgument, "id must be prv_ and 8 to 40 lowercase letters or digits")
	}
	custody, err := s.custodyFor(ctx, in.Custody)
	if err != nil {
		return nil, err
	}
	if custody == domain.CustodyHub {
		if in.Name == nil || in.Vendor == nil || in.APIKey == nil || in.KeySealed != nil || (in.KeyVersion != nil && *in.KeyVersion != 1) {
			return nil, domain.Errf(domain.CodeInvalidArgument, "name, vendor and api_key are required (the hub keeps the key: this workspace has end-to-end encryption off)")
		}
		one := 1
		in.KeyVersion = &one
	} else if in.Name == nil || in.Vendor == nil || in.KeySealed == nil || in.KeyVersion == nil || *in.KeyVersion != 1 || in.APIKey != nil {
		return nil, domain.Errf(domain.CodeInvalidArgument, "name, vendor, key_sealed and key_version 1 are required (this workspace has end-to-end encryption on: the key is sealed for your devices)")
	}
	have, err := s.repo.List(ctx, member)
	if err != nil {
		return nil, err
	}
	if len(have) >= domain.MaxModelProviders {
		return nil, domain.Errf(domain.CodeInvalidArgument, "at most %d model providers", domain.MaxModelProviders)
	}
	now := s.clock.Now().UTC()
	p := &domain.ModelProvider{ID: in.ID, Owner: member, Custody: custody, CreatedAt: now, UpdatedAt: now}
	if err := s.apply(ctx, p, in); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Update changes what is set in the request.
func (s *ModelProviderService) Update(ctx context.Context, member, id string, in api.ModelProviderRequest) (*domain.ModelProvider, error) {
	if err := s.writable(ctx, member); err != nil {
		return nil, err
	}
	p, err := s.Owned(ctx, member, id)
	if err != nil {
		return nil, err
	}
	if in.ID != "" && in.ID != id {
		return nil, domain.Errf(domain.CodeInvalidArgument, "the id cannot change")
	}
	custody, err := s.custodyFor(ctx, in.Custody)
	if err != nil {
		return nil, err
	}
	moving := custody != p.Custody && !(custody == domain.CustodyE2E && p.Custody == "")
	if custody == domain.CustodyHub {
		if in.KeySealed != nil {
			return nil, domain.Errf(domain.CodeInvalidArgument, "the hub keeps this provider's key: send api_key, not key_sealed")
		}
		if moving && in.APIKey == nil {
			return nil, domain.Errf(domain.CodeInvalidArgument, "moving the key to the hub: send it as api_key")
		}
		if in.KeyVersion != nil && *in.KeyVersion != p.KeyVersion && *in.KeyVersion != p.KeyVersion+1 {
			return nil, domain.Errf(domain.CodeConflict, "key_version must be %d or %d", p.KeyVersion, p.KeyVersion+1).
				WithSuggestion("read the provider again: another device may have changed it")
		}
		if in.APIKey != nil {
			// The version goes up for a new key only: the same key moved
			// here, or entered again, keeps it (agents are not behind).
			version := p.KeyVersion
			if !moving && !s.sameKey(p, *in.APIKey) {
				version++
			}
			if in.KeyVersion != nil {
				version = *in.KeyVersion
			}
			in.KeyVersion = &version
		}
	} else {
		if in.APIKey != nil {
			return nil, domain.Errf(domain.CodeInvalidArgument, "this workspace has end-to-end encryption on: send the key sealed for your devices (key_sealed), not api_key")
		}
		if in.KeySealed != nil && in.KeyVersion == nil {
			return nil, domain.Errf(domain.CodeInvalidArgument, "key_sealed comes with key_version")
		}
		if moving && in.KeySealed == nil {
			return nil, domain.Errf(domain.CodeInvalidArgument, "moving the key off the hub: send it sealed for your devices (key_sealed at key_version %d)", p.KeyVersion)
		}
		// Where the key goes is sealed with it: a new vendor, address or
		// small model comes with the key sealed again, at the next version
		// (the member's App uses only what it opens; this keeps the hub's
		// copy, which is shown, from being changed alone).
		changes := func(v *string, cur string) bool { return v != nil && strings.TrimSpace(*v) != cur }
		if !moving && (changes(in.Vendor, p.Vendor) || changes(in.AnthropicBase, p.AnthropicBase) || changes(in.OpenAIBase, p.OpenAIBase) || changes(in.SmallModel, p.SmallModel)) {
			if in.KeySealed == nil || in.KeyVersion == nil || *in.KeyVersion != p.KeyVersion+1 {
				return nil, domain.Errf(domain.CodeInvalidArgument, "a new vendor, address or small model is sealed with the key: send key_sealed at key_version %d", p.KeyVersion+1)
			}
		}
		if in.KeyVersion != nil && *in.KeyVersion != p.KeyVersion && *in.KeyVersion != p.KeyVersion+1 {
			return nil, domain.Errf(domain.CodeConflict, "key_version must be %d (a new key) or %d (sealed again)", p.KeyVersion+1, p.KeyVersion).
				WithSuggestion("read the provider again: another device may have changed it")
		}
	}
	p.Custody = custody
	if err := s.apply(ctx, p, in); err != nil {
		return nil, err
	}
	p.UpdatedAt = s.clock.Now().UTC()
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Delete removes the member's provider. Agents it was sent to keep what
// was written on their computers.
func (s *ModelProviderService) Delete(ctx context.Context, member, id string) error {
	if _, err := s.Owned(ctx, member, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func (s *ModelProviderService) writable(ctx context.Context, member string) error {
	if member == "" {
		return domain.Errf(domain.CodePermissionDenied, "model providers belong to members: sign in as one")
	}
	return nil
}

// custodyFor is who keeps keys now: the member's devices with end-to-end
// encryption on, the hub with it off (i119). A request may name it, and
// must name that one.
func (s *ModelProviderService) custodyFor(ctx context.Context, asked *string) (string, error) {
	custody := domain.CustodyE2E
	if !s.E2E(ctx) {
		if s.box == nil {
			return "", domain.Errf(domain.CodeE2ERequired, "this hub keeps model providers' keys end-to-end encrypted only: this workspace has encryption off").
				WithSuggestion("turn on end-to-end encryption in the workspace settings, or update the hub")
		}
		custody = domain.CustodyHub
	}
	if asked != nil && *asked != custody {
		if custody == domain.CustodyHub {
			return "", domain.Errf(domain.CodeE2ERequired, "this workspace has end-to-end encryption off: the hub keeps the key").
				WithSuggestion("turn on end-to-end encryption in the workspace settings to keep keys on your devices")
		}
		return "", domain.Errf(domain.CodeInvalidArgument, "this workspace has end-to-end encryption on: keys are sealed for your devices, not kept by the hub")
	}
	return custody, nil
}

// sameKey reports whether key is the one the hub keeps for p now.
func (s *ModelProviderService) sameKey(p *domain.ModelProvider, key string) bool {
	if !p.HubHeld() || s.box == nil || p.KeyHub == "" {
		return false
	}
	got, err := s.box.Open(domain.HubKeyPurpose(p.ID, p.Owner, p.KeyVersion), p.KeyHub)
	return err == nil && string(got) == strings.TrimSpace(key)
}

// KeyLost reports whether the hub keeps p's key but cannot open it now
// (the workspace's key file is gone): it must be entered again.
func (s *ModelProviderService) KeyLost(p *domain.ModelProvider) bool {
	if !p.HubHeld() {
		return false
	}
	if s.box == nil || p.KeyHub == "" || !s.box.Present() {
		return true
	}
	_, err := s.box.Open(domain.HubKeyPurpose(p.ID, p.Owner, p.KeyVersion), p.KeyHub)
	return err != nil
}

// Key answers the key the hub keeps for the member's provider: for its
// owner only, to seal it for their devices or to list the vendor's
// models from their computer.
func (s *ModelProviderService) Key(ctx context.Context, member, id string) (string, int, error) {
	p, err := s.Owned(ctx, member, id)
	if err != nil {
		return "", 0, err
	}
	key, err := s.openKey(p)
	return key, p.KeyVersion, err
}

func (s *ModelProviderService) openKey(p *domain.ModelProvider) (string, error) {
	if !p.HubHeld() {
		return "", domain.Errf(domain.CodeInvalidArgument, "this provider's key is sealed for its owner's devices: the hub cannot open it")
	}
	if s.box == nil || p.KeyHub == "" || !s.box.Present() {
		return "", domain.Errf(domain.CodeInvalidState, "this provider's key is lost: the hub's key file is gone").
			WithSuggestion("enter the key again in Settings › Model providers")
	}
	got, err := s.box.Open(domain.HubKeyPurpose(p.ID, p.Owner, p.KeyVersion), p.KeyHub)
	if err != nil {
		return "", domain.Errf(domain.CodeInvalidState, "this provider's key cannot be opened").
			WithSuggestion("enter the key again in Settings › Model providers")
	}
	return string(got), nil
}

// Material is a provider's settings for an agent on cli with model, its
// key included (the hub keeps it): for the one runner or node it goes to.
func (s *ModelProviderService) Material(ctx context.Context, id, owner, cli, model string) (api.ProviderMaterial, int, error) {
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return api.ProviderMaterial{}, 0, err
	}
	if p.Owner != owner {
		return api.ProviderMaterial{}, 0, domain.Errf(domain.CodePermissionDenied, "this provider is not its owner's")
	}
	key, err := s.openKey(p)
	if err != nil {
		return api.ProviderMaterial{}, 0, err
	}
	if s.manual == nil {
		return api.ProviderMaterial{}, 0, domain.Errf(domain.CodeUnsupported, "this hub cannot send providers")
	}
	m, err := s.manual(ProviderSettings{Vendor: p.Vendor, AnthropicBase: p.AnthropicBase, OpenAIBase: p.OpenAIBase, SmallModel: p.SmallModel, APIKey: key}, cli, model)
	if err != nil {
		return api.ProviderMaterial{}, 0, domain.Errf(domain.CodeInvalidArgument, "%v", err)
	}
	return m, p.KeyVersion, nil
}

// SealForAgent keeps an agent's own custom settings (an address and key
// given for it alone), sealed with the workspace's key for that agent and
// request.
func (s *ModelProviderService) SealForAgent(ctx context.Context, agent, id string, plain []byte) (string, error) {
	if s.box == nil {
		return "", domain.Errf(domain.CodeUnsupported, "this hub cannot keep providers")
	}
	create, err := s.mayMakeKey(ctx)
	if err != nil {
		return "", err
	}
	out, err := s.box.Seal(domain.HubProviderPurpose(agent, id), plain, create)
	if err != nil {
		return "", keyErr(err)
	}
	return out, nil
}

// OpenForAgent opens what SealForAgent kept.
func (s *ModelProviderService) OpenForAgent(agent, id, sealed string) ([]byte, error) {
	if s.box == nil || !s.box.Present() {
		return nil, domain.Errf(domain.CodeInvalidState, "the provider the hub kept for this agent is lost: the hub's key file is gone").
			WithSuggestion("set the agent's provider again")
	}
	out, err := s.box.Open(domain.HubProviderPurpose(agent, id), sealed)
	if err != nil {
		return nil, domain.Errf(domain.CodeInvalidState, "the provider the hub kept for this agent cannot be opened")
	}
	return out, nil
}

// mayMakeKey: the workspace's key may be made now only if nothing sealed
// with one is kept (else it was lost, and a new one would hide that).
func (s *ModelProviderService) mayMakeKey(ctx context.Context) (bool, error) {
	if s.box.Present() {
		return false, nil
	}
	held, err := s.repo.AnyHubHeld(ctx)
	if err != nil || held {
		return false, err
	}
	if s.heldElsewhere != nil {
		if held, err := s.heldElsewhere(ctx); err != nil || held {
			return false, err
		}
	}
	return true, nil
}

// HubKeyLost reports whether values sealed with the workspace's key are
// kept while the key is gone (for a warning when the hub starts).
func (s *ModelProviderService) HubKeyLost(ctx context.Context) bool {
	if s.box == nil || s.box.Present() {
		return false
	}
	create, err := s.mayMakeKey(ctx)
	return err == nil && !create
}

func keyErr(err error) error {
	return domain.Errf(domain.CodeInvalidState, "the hub's key for secrets is missing: keys it kept are lost").
		WithSuggestion("restore the hub's provider-secret.key from a backup, or enter the keys again")
}

// apply checks and sets what the request carries.
func (s *ModelProviderService) apply(ctx context.Context, p *domain.ModelProvider, in api.ModelProviderRequest) error {
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || len(name) > maxProviderName || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			return domain.Errf(domain.CodeInvalidArgument, "name must be 1 to %d characters on one line", maxProviderName)
		}
		p.Name = name
	}
	if in.Vendor != nil {
		v := strings.TrimSpace(*in.Vendor)
		if !vendorID.MatchString(v) || (v != "custom" && (s.vendor == nil || !s.vendor(v))) {
			return domain.Errf(domain.CodeInvalidArgument, "vendor must be one of the catalogue's or custom")
		}
		p.Vendor = v
	}
	for _, f := range []struct {
		in  *string
		out *string
	}{{in.AnthropicBase, &p.AnthropicBase}, {in.OpenAIBase, &p.OpenAIBase}} {
		if f.in == nil {
			continue
		}
		v := strings.TrimSpace(*f.in)
		if v != "" && !baseURL(v) {
			return domain.Errf(domain.CodeInvalidArgument, "addresses must be http(s) URLs without credentials, query or fragment")
		}
		*f.out = v
	}
	if p.AnthropicBase == "" && p.OpenAIBase == "" {
		return domain.Errf(domain.CodeInvalidArgument, "an Anthropic or an OpenAI address is required")
	}
	if in.SmallModel != nil {
		v := strings.TrimSpace(*in.SmallModel)
		if len(v) > maxProviderModel || strings.ContainsAny(v, " \t\r\n\"'`") {
			return domain.Errf(domain.CodeInvalidArgument, "small_model must be a model id")
		}
		p.SmallModel = v
	}
	if in.Models != nil {
		var ms []domain.RuntimeModel
		for _, m := range *in.Models {
			ms = append(ms, domain.RuntimeModel{ID: m.ID, Name: m.Name, Efforts: m.Efforts})
		}
		p.Models = domain.EncodeModels(ms)
		p.ModelsAt = time.Time{}
		if in.ModelsAt != nil && p.Models != "" && !in.ModelsAt.After(s.clock.Now().Add(time.Minute)) {
			p.ModelsAt = in.ModelsAt.UTC()
		}
	}
	if in.KeySealed != nil {
		version := *in.KeyVersion
		if err := s.checkKey(ctx, p.Owner, p.ID, version, *in.KeySealed); err != nil {
			return err
		}
		p.KeySealed, p.KeyVersion = *in.KeySealed, version
		p.KeyHub, p.KeyHint = "", ""
	}
	if in.APIKey != nil {
		key := strings.TrimSpace(*in.APIKey)
		if key == "" || len(key) > maxPlainKey || strings.IndexFunc(key, unicode.IsControl) >= 0 || strings.ContainsAny(key, " \t") {
			return domain.Errf(domain.CodeInvalidArgument, "api_key must be one word on one line")
		}
		create, err := s.mayMakeKey(ctx)
		if err != nil {
			return err
		}
		version := *in.KeyVersion
		sealed, err := s.box.Seal(domain.HubKeyPurpose(p.ID, p.Owner, version), []byte(key), create)
		if err != nil {
			return keyErr(err)
		}
		p.KeyHub, p.KeyVersion, p.KeySealed = sealed, version, ""
		p.KeyHint = key[max(0, len(key)-4):]
	}
	if p.HubHeld() {
		if p.KeyHub == "" {
			return domain.Errf(domain.CodeInvalidArgument, "api_key is required")
		}
	} else if p.KeySealed == "" {
		return domain.Errf(domain.CodeInvalidArgument, "key_sealed is required")
	}
	return nil
}

// checkKey: the envelope is signed by one of the member's devices and
// readable by those devices only.
func (s *ModelProviderService) checkKey(ctx context.Context, member, id string, version int, body string) error {
	if len(body) > maxProviderKey || !e2e.IsSealed(body) {
		return domain.Errf(domain.CodeInvalidArgument, "key_sealed must be an end-to-end envelope")
	}
	devices, err := s.e2e.memberKeys(ctx, member, "", "")
	if err != nil {
		return err
	}
	var readers []string
	for _, d := range devices {
		readers = append(readers, d.Key.Enc)
	}
	h := e2e.ProviderKeyHeader(member, id, version)
	for _, d := range devices {
		if e2e.VerifySealedFor(body, h, d.Key.Sign, readers) == nil {
			return nil
		}
	}
	return domain.Errf(domain.CodeInvalidArgument, "key_sealed must be signed by one of your devices and sealed for your devices only")
}

func baseURL(v string) bool {
	if len(v) > maxProviderURL {
		return false
	}
	u, err := url.Parse(v)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
