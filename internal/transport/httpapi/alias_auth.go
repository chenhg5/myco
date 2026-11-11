package httpapi

import (
	"net/http"

	"github.com/chenhg5/myco/internal/domain"
)

const selfAliasLimit = 3

func (s *Server) maySetAlias(w http.ResponseWriter, r *http.Request, name, target string) bool {
	actor := s.actorOf(r)
	_, isAgent := r.Context().Value(callerAgentKey{}).(*domain.Agent)
	if !isAgent && actor.Manages() {
		return true
	}
	a, err := s.agents.Get(r.Context(), target)
	if err != nil {
		writeError(w, err)
		return false
	}
	if caller, ok := r.Context().Value(callerAgentKey{}).(*domain.Agent); ok {
		if caller.Name != a.Name {
			writeError(w, domain.Errf(domain.CodePermissionDenied, "agents can only set aliases for themselves").WithSuggestion("use --target %s or ask the target's owner", caller.Name))
			return false
		}
		aliases, err := s.agents.ListAliases(r.Context())
		if err != nil {
			writeError(w, err)
			return false
		}
		count := 0
		for _, al := range aliases {
			if string(al.Name) == name {
				if al.Target != a.Name {
					writeError(w, domain.Errf(domain.CodeConflict, "alias %s already belongs to %s", name, al.Target).WithSuggestion("choose a free alias"))
					return false
				}
				return true
			}
			if al.Target == a.Name {
				count++
			}
		}
		if count >= selfAliasLimit {
			writeError(w, domain.Errf(domain.CodeConflict, "agents may set at most %d aliases", selfAliasLimit).WithSuggestion("remove one of your aliases before adding another"))
			return false
		}
		return true
	}
	return s.mayOwn(w, r, a.Owner, "set aliases for")
}

func (s *Server) mayRemoveAlias(w http.ResponseWriter, r *http.Request, name string) bool {
	_, isAgent := r.Context().Value(callerAgentKey{}).(*domain.Agent)
	if !isAgent && s.actorOf(r).Manages() {
		return true
	}
	aliases, err := s.agents.ListAliases(r.Context())
	if err != nil {
		writeError(w, err)
		return false
	}
	for _, al := range aliases {
		if string(al.Name) != name {
			continue
		}
		if caller, ok := r.Context().Value(callerAgentKey{}).(*domain.Agent); ok {
			if caller.Name == al.Target {
				return true
			}
			writeError(w, domain.Errf(domain.CodePermissionDenied, "agents can only remove their own aliases"))
			return false
		}
		a, err := s.agents.Get(r.Context(), string(al.Target))
		if err != nil {
			writeError(w, err)
			return false
		}
		return s.mayOwn(w, r, a.Owner, "remove aliases from")
	}
	writeError(w, domain.Errf(domain.CodeNotFound, "alias %s not found", name))
	return false
}
