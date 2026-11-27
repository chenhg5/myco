package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
)

// SetModelProviders enables /v1/console/providers (members' model
// providers, i115).
func (s *Server) SetModelProviders(p *app.ModelProviderService) { s.modelProviders = p }

// providerMember is the member a model provider request is for: a person
// at one of their own devices (not an agent, nor a superagent for them).
func (s *Server) providerMember(w http.ResponseWriter, r *http.Request) (string, bool) {
	if s.modelProviders == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "this hub does not keep model providers"))
		return "", false
	}
	a := s.actorOf(r)
	if a.Agent != nil || a.Via != nil {
		writeError(w, domain.Errf(domain.CodePermissionDenied, "model providers are managed by their member only"))
		return "", false
	}
	// A member's own device only: not the hub's administrator token, nor a
	// device token that belongs to no member (they would stand for the
	// owner, whose keys they cannot open anyway).
	if a.Member == nil {
		writeError(w, domain.Errf(domain.CodePermissionDenied, "model providers belong to members: use one of your own devices"))
		return "", false
	}
	return a.Member.ID, true
}

func (s *Server) listModelProviders(w http.ResponseWriter, r *http.Request) {
	member, ok := s.providerMember(w, r)
	if !ok {
		return
	}
	list, err := s.modelProviders.List(r.Context(), member)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.ModelProviderList{E2E: s.modelProviders.E2E(r.Context()), Providers: []api.ModelProvider{}}
	for _, p := range list {
		out.Providers = append(out.Providers, modelProviderDTO(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createModelProvider(w http.ResponseWriter, r *http.Request) {
	member, ok := s.providerMember(w, r)
	if !ok {
		return
	}
	var in api.ModelProviderRequest
	if !decode(w, r, &in) {
		return
	}
	p, err := s.modelProviders.Create(r.Context(), member, in)
	if err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "provider.create", p.ID, "vendor="+p.Vendor)
	writeJSON(w, http.StatusCreated, modelProviderDTO(p))
}

func (s *Server) updateModelProvider(w http.ResponseWriter, r *http.Request) {
	member, ok := s.providerMember(w, r)
	if !ok {
		return
	}
	var in api.ModelProviderRequest
	if !decode(w, r, &in) {
		return
	}
	p, err := s.modelProviders.Update(r.Context(), member, r.PathValue("id"), in)
	if err != nil {
		writeError(w, err)
		return
	}
	if in.KeySealed != nil {
		// (never the key: only that it changed, or was sealed again)
		s.record(r, "provider.key", p.ID, "key_version="+strconv.Itoa(p.KeyVersion))
	}
	writeJSON(w, http.StatusOK, modelProviderDTO(p))
}

func (s *Server) deleteModelProvider(w http.ResponseWriter, r *http.Request) {
	member, ok := s.providerMember(w, r)
	if !ok {
		return
	}
	if err := s.modelProviders.Delete(r.Context(), member, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "provider.delete", r.PathValue("id"), "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func modelProviderDTO(p *domain.ModelProvider) api.ModelProvider {
	return api.ModelProvider{
		ID: p.ID, Name: p.Name, Vendor: p.Vendor, AnthropicBase: p.AnthropicBase, OpenAIBase: p.OpenAIBase, SmallModel: p.SmallModel,
		Models: modelsDTO(domain.Runtime{Models: p.Models}.ModelList()), ModelsAt: timePtr(p.ModelsAt),
		KeySealed: p.KeySealed, KeyVersion: p.KeyVersion, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

// checkSpawnProvider: a new agent's provider sealed for the machine node
// (i115), by the person's own device; only for starting a session, and
// only with end-to-end encryption on. ProviderRef must be their own.
func (s *Server) checkSpawnProvider(w http.ResponseWriter, r *http.Request, in *api.SpawnRequest) bool {
	if in.CLI != "claude" && in.CLI != "codex" {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "a provider goes only with a Claude Code or Codex session"))
		return false
	}
	actor := s.actorOf(r)
	if actor.Agent != nil || actor.Via != nil || actor.Member == nil {
		writeError(w, domain.Errf(domain.CodePermissionDenied, "only a person, from one of their own devices, may send a provider with a new session"))
		return false
	}
	name, member := actor.Member.Name, actor.Member.ID
	// The key ends up on that computer: its owner's call only (as for an
	// agent's provider), not an administrator's.
	m, err := s.machines.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return false
	}
	if !(m.Owner == member || (m.Owner == "" && actor.IsOwner())) {
		writeError(w, domain.Errf(domain.CodePermissionDenied, "only the owner of this computer may send it a provider"))
		return false
	}
	if in.ProviderRef != nil && in.ProviderRef.ID != "" {
		if s.modelProviders == nil {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "unknown model provider"))
			return false
		}
		p, err := s.modelProviders.Owned(r.Context(), member, in.ProviderRef.ID)
		if err != nil {
			writeError(w, err)
			return false
		}
		if in.ProviderRef.KeyVersion < 1 || in.ProviderRef.KeyVersion > p.KeyVersion {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "provider_ref.key_version must be 1 to %d", p.KeyVersion))
			return false
		}
	}
	if in.ProviderEncrypted == nil {
		return true
	}
	if s.e2e == nil || !s.e2e.Enabled(r.Context()) {
		writeError(w, domain.Errf(domain.CodeE2ERequired, "a provider's key goes to a computer end-to-end encrypted: this workspace has encryption off").
			WithSuggestion("turn on end-to-end encryption, or enter the provider on that computer"))
		return false
	}
	node, err := s.machines.SpawnNode(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return false
	}
	p := in.ProviderEncrypted
	if p.From != app.ConsoleSenderPrefix+name {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "invalid encrypted provider sender"))
		return false
	}
	if err := verifyEncryptedProvider(r.Context(), s.e2e, string(node), &domain.EncryptedProvider{ID: p.ID, From: p.From, Body: p.Body}); err != nil {
		writeError(w, err)
		return false
	}
	return true
}

// markSpawnedProvider records on a new agent that its provider is the
// custom one written on its computer, from this model provider. Only the
// agent this spawn made: the machine's answer names it, and is not taken
// on its word (spawnedHere).
func (s *Server) markSpawnedProvider(r *http.Request, name domain.AgentName, ref *api.ProviderRef, res api.SpawnResult, since time.Time) {
	cur, err := s.agents.Get(r.Context(), string(name))
	if err != nil {
		return
	}
	m, err := s.machines.Get(r.Context(), r.PathValue("id"))
	if err != nil || !spawnedHere(cur, res, since, m.Owner) {
		return
	}
	rs := cur.RunSettings
	custom := "custom"
	rs.Provider, rs.ProviderEncrypted = &custom, nil
	if ref != nil {
		rs.ProviderRef = domain.ProviderRef{ID: ref.ID, KeyVersion: ref.KeyVersion}
	}
	_, _ = s.agents.SetRunSettings(r.Context(), name, rs)
}

// spawnedHere: a is the agent a spawn just made, as its machine answered:
// joined since the request, with the session the machine started, and the
// computer's owner's.
func spawnedHere(a *domain.Agent, res api.SpawnResult, since time.Time, machineOwner string) bool {
	return a != nil && string(a.Name) == res.Agent && !a.CreatedAt.Before(since) &&
		res.SessionID != "" && a.Binding.SessionID == res.SessionID && a.Owner == machineOwner
}
