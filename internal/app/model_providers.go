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
	if in.Name == nil || in.Vendor == nil || in.KeySealed == nil || in.KeyVersion == nil || *in.KeyVersion != 1 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "name, vendor, key_sealed and key_version 1 are required")
	}
	have, err := s.repo.List(ctx, member)
	if err != nil {
		return nil, err
	}
	if len(have) >= domain.MaxModelProviders {
		return nil, domain.Errf(domain.CodeInvalidArgument, "at most %d model providers", domain.MaxModelProviders)
	}
	now := s.clock.Now().UTC()
	p := &domain.ModelProvider{ID: in.ID, Owner: member, CreatedAt: now, UpdatedAt: now}
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
	if in.KeySealed != nil && in.KeyVersion == nil {
		return nil, domain.Errf(domain.CodeInvalidArgument, "key_sealed comes with key_version")
	}
	if in.KeyVersion != nil && *in.KeyVersion != p.KeyVersion && *in.KeyVersion != p.KeyVersion+1 {
		return nil, domain.Errf(domain.CodeConflict, "key_version must be %d (a new key) or %d (sealed again)", p.KeyVersion+1, p.KeyVersion).
			WithSuggestion("read the provider again: another device may have changed it")
	}
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
	if !s.E2E(ctx) {
		return domain.Errf(domain.CodeE2ERequired, "model providers keep their keys end-to-end encrypted: this workspace has encryption off").
			WithSuggestion("turn on end-to-end encryption in the workspace settings, or enter the provider on the agent's computer")
	}
	return nil
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
	}
	if p.KeySealed == "" {
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
