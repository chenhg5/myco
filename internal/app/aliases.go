package app

import (
	"context"
	"sort"

	"github.com/chenhg5/myco/internal/domain"
)

// aliasResolver resolves agent names and aliases (shared by services).
type aliasResolver struct {
	agents  domain.AgentRepository
	aliases domain.AliasRepository // nil: aliases disabled
}

// resolve returns the agent addressed by name or alias, and the alias used
// ("" if name was a real agent name).
func (r aliasResolver) resolve(ctx context.Context, name domain.AgentName) (*domain.Agent, domain.AgentName, error) {
	a, err := r.agents.Get(ctx, name)
	if err == nil || !domain.IsCode(err, domain.CodeNotFound) || r.aliases == nil {
		return a, "", err
	}
	al, aerr := r.aliases.Get(ctx, name)
	if aerr != nil {
		return nil, "", err // report the original "agent not found"
	}
	a, err = r.agents.Get(ctx, al.Target)
	if domain.IsCode(err, domain.CodeNotFound) {
		return nil, "", domain.Errf(domain.CodeNotFound, "alias %s points to %s, which no longer exists", name, al.Target).
			WithSuggestion("re-point it: myco agent alias set --alias %s --target <agent>", name)
	}
	return a, name, err
}

// taken reports whether name is used by an agent or an alias.
func (r aliasResolver) aliasTaken(ctx context.Context, name domain.AgentName) bool {
	if r.aliases == nil {
		return false
	}
	_, err := r.aliases.Get(ctx, name)
	return err == nil
}

// SetAliases enables aliases for the agent service.
func (s *AgentService) SetAliases(repo domain.AliasRepository) { s.aliases = repo }

func (s *AgentService) resolver() aliasResolver {
	return aliasResolver{agents: s.agents, aliases: s.aliases}
}

// SetAlias creates or re-points alias to target (admin).
func (s *AgentService) SetAlias(ctx context.Context, alias, target string, dryRun bool) (*domain.Alias, error) {
	if s.aliases == nil {
		return nil, domain.Errf(domain.CodeInvalidArgument, "aliases are not enabled on this hub")
	}
	an, err := domain.ParseAgentName(alias)
	if err != nil {
		return nil, err
	}
	tn, err := domain.ParseAgentName(target)
	if err != nil {
		return nil, err
	}
	if _, err := s.agents.Get(ctx, an); err == nil {
		return nil, domain.Errf(domain.CodeConflict, "%s is an agent name; aliases and names share one namespace", an).
			WithSuggestion("pick another alias")
	}
	if _, err := s.agents.Get(ctx, tn); err != nil {
		if domain.IsCode(err, domain.CodeNotFound) {
			return nil, domain.Errf(domain.CodeNotFound, "target agent %s not found", tn).
				WithSuggestion("aliases point at agent names, not at other aliases")
		}
		return nil, err
	}
	now := s.clock.Now()
	al := &domain.Alias{Name: an, Target: tn, CreatedAt: now, UpdatedAt: now}
	if prev, err := s.aliases.Get(ctx, an); err == nil {
		al.CreatedAt = prev.CreatedAt
	}
	if dryRun {
		return al, nil
	}
	return al, s.aliases.Set(ctx, al)
}

// RemoveAlias deletes an alias (admin).
func (s *AgentService) RemoveAlias(ctx context.Context, alias string, dryRun bool) (*domain.Alias, error) {
	if s.aliases == nil {
		return nil, domain.Errf(domain.CodeNotFound, "alias %s not found", alias)
	}
	an, err := domain.ParseAgentName(alias)
	if err != nil {
		return nil, err
	}
	al, err := s.aliases.Get(ctx, an)
	if err != nil || dryRun {
		return al, err
	}
	return al, s.aliases.Delete(ctx, an)
}

// ListAliases returns all aliases.
func (s *AgentService) ListAliases(ctx context.Context) ([]*domain.Alias, error) {
	if s.aliases == nil {
		return nil, nil
	}
	return s.aliases.List(ctx)
}

// AliasMap returns target -> sorted aliases.
func (s *AgentService) AliasMap(ctx context.Context) (map[domain.AgentName][]string, error) {
	list, err := s.ListAliases(ctx)
	if err != nil {
		return nil, err
	}
	m := map[domain.AgentName][]string{}
	for _, a := range list {
		m[a.Target] = append(m[a.Target], string(a.Name))
	}
	for k := range m {
		sort.Strings(m[k])
	}
	return m, nil
}

// UpdateProfile changes display name and/or description (nil = unchanged).
func (s *AgentService) UpdateProfile(ctx context.Context, name string, displayName, description *string) (*domain.Agent, error) {
	a, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := a.SetProfile(displayName, description); err != nil {
		return nil, err
	}
	return a, s.agents.Save(ctx, a)
}
