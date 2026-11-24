package app

import (
	"context"
	"encoding/hex"
	"strings"
	"unicode"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/domain"
)

func sameResourceID(a, b *api.ActionResource) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Provider == b.Provider && a.ID == b.ID
}
func sameResource(a, b *api.ActionResource) bool {
	return sameResourceID(a, b) && (a == nil || a.Binding == b.Binding)
}
func validateSSHResource(r *api.ActionResource) error {
	invalid := func() error {
		return domain.Errf(domain.CodeInvalidArgument, "SSH resource needs a profile id, SHA256 binding and safe display name and endpoint summary")
	}
	if r == nil || r.Provider != "server.ssh" || !strings.HasPrefix(r.ID, "ssh_") || len(r.ID) > 80 || len(r.ID) < 10 || len(r.Binding) != 64 {
		return invalid()
	}
	for _, c := range r.ID {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return invalid()
		}
	}
	if _, err := hex.DecodeString(r.Binding); err != nil {
		return invalid()
	}
	if r.Name == "" || len(r.Name) > 200 || r.Summary == "" || len(r.Summary) > 800 {
		return invalid()
	}
	if strings.IndexFunc(r.Name+r.Summary, unicode.IsControl) >= 0 {
		return invalid()
	}
	return nil
}

// ResourceTargets discloses only resources explicitly enabled in this workspace
// by this agent's own person. Knowing a target does not grant permission to use it.
func (s *ActionService) ResourceTargets(ctx context.Context, a *domain.Agent) ([]api.ActionTarget, error) {
	if err := s.enabled(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []api.ActionTarget{}
	seen := map[string]bool{}
	for _, h := range s.hosts {
		if h.Resource == nil || h.owner != a.Owner || s.clock.Now().Sub(h.LastSeenAt) > actionHostStale {
			continue
		}
		key := h.MachineID + "/" + h.Resource.ID + "/" + h.Resource.Binding
		if seen[key] {
			continue
		}
		seen[key] = true
		r := *h.Resource
		out = append(out, api.ActionTarget{Kind: "machine", ID: h.MachineID, Resource: &r})
	}
	return out, nil
}
