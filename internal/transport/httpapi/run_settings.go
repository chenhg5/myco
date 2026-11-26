package httpapi

import (
	"context"
	"encoding/json"
	"github.com/myco-ai/myco/internal/e2e"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
)

// setAgentRunSettings: PUT /v1/agents/{name}/run-settings from the console.
func (s *Server) setAgentRunSettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.mayOwnAgent(w, r, name, "choose the model of") {
		return
	}
	s.setRunSettings(w, r, domain.AgentName(name), s.actorOf(r))
}

// setMyRunSettings: PUT /v1/me/run-settings, the agent itself.
func (s *Server) setMyRunSettings(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	s.setRunSettings(w, r, a.Name, app.Actor{Agent: a})
}

func (s *Server) setRunSettings(w http.ResponseWriter, r *http.Request, name domain.AgentName, actor app.Actor) {
	s.runSettingsMu.Lock()
	defer s.runSettingsMu.Unlock()
	// Access is told apart from absent: older apps and the agent itself
	// leave it out, and it stays as it is.
	var in struct {
		Model             *string                   `json:"model"`
		Effort            *string                   `json:"effort"`
		Access            *string                   `json:"access"`
		MCPTools          *bool                     `json:"mcp_tools"`
		Provider          *string                   `json:"provider"`
		ProviderEncrypted *domain.EncryptedProvider `json:"provider_encrypted"`
	}
	if !decode(w, r, &in) {
		return
	}
	cur, err := s.agents.Get(r.Context(), string(name))
	if err != nil {
		writeError(w, err)
		return
	}
	rs := cur.RunSettings
	if in.Model != nil {
		rs.Model = strings.TrimSpace(*in.Model)
	}
	if in.Effort != nil {
		rs.Effort = strings.TrimSpace(*in.Effort)
	}
	providerChanged := in.Provider != nil || in.ProviderEncrypted != nil
	// A model editor may echo the current selection. An unchanged custom
	// marker without a new envelope must not erase a pending delivery.
	if in.Provider != nil && rs.Provider != nil && *in.Provider == *rs.Provider {
		if in.ProviderEncrypted == nil || (rs.ProviderEncrypted != nil && *in.ProviderEncrypted == *rs.ProviderEncrypted) {
			providerChanged = false
		}
	}
	if providerChanged {
		if actor.Via != nil || !ownsComputer(actor, cur) {
			writeError(w, domain.Errf(domain.CodePermissionDenied, "only the agent owner may change its provider"))
			return
		}
		if !slices.Contains(strings.Fields(cur.Runtime.Features), "remote_provider") {
			writeError(w, domain.Errf(domain.CodeUnsupported, "this runner does not support remote providers").WithSuggestion("update the runner on that computer"))
			return
		}
		if in.Provider != nil && strings.HasPrefix(*in.Provider, "cc-switch:") {
			var options []struct {
				Ref string `json:"ref"`
			}
			_ = json.Unmarshal([]byte(cur.Runtime.Providers), &options)
			found := false
			for _, option := range options {
				if option.Ref == *in.Provider {
					found = true
					break
				}
			}
			if !found {
				writeError(w, domain.Errf(domain.CodeInvalidArgument, "provider is not advertised by this runner").WithSuggestion("refresh the runner's available providers"))
				return
			}
		}
		if in.ProviderEncrypted != nil {
			if s.e2e == nil || !s.e2e.Enabled(r.Context()) {
				writeError(w, domain.Errf(domain.CodePermissionDenied, "remote custom providers require an encrypted workspace").WithSuggestion("enter the provider in the App or CLI on that computer"))
				return
			}
			if in.Provider == nil || *in.Provider != "custom" || actor.Member == nil || in.ProviderEncrypted.From != "@console:"+actor.Member.Name {
				writeError(w, domain.Errf(domain.CodeInvalidArgument, "invalid encrypted provider sender or selection"))
				return
			}
			if err := verifyEncryptedProvider(r.Context(), s.e2e, string(name), in.ProviderEncrypted); err != nil {
				writeError(w, err)
				return
			}
			if rs.ProviderEncrypted != nil && rs.ProviderEncrypted.ID == in.ProviderEncrypted.ID {
				writeError(w, domain.Errf(domain.CodeConflict, "use a new provider request id"))
				return
			}
		} else if in.Provider != nil && *in.Provider == "custom" {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "custom provider requires an encrypted payload"))
			return
		}
		rs.Provider, rs.ProviderEncrypted = in.Provider, in.ProviderEncrypted
	}

	if in.Access != nil && strings.TrimSpace(*in.Access) != cur.RunSettings.Access {
		// What an agent may do on someone's computer is that person's call:
		// not the agent's own, and not a hub admin's.
		if !ownsComputer(actor, cur) {
			writeError(w, domain.Errf(domain.CodePermissionDenied, "only the owner of %s can change its access", name).
				WithSuggestion("ask its owner; the machine itself also caps it (spawn.max_access in ~/.myco/machine.yaml)"))
			return
		}
		rs.Access = strings.TrimSpace(*in.Access)
	}
	if in.MCPTools != nil && *in.MCPTools != cur.RunSettings.MCPTools {
		if !ownsComputer(actor, cur) {
			writeError(w, domain.Errf(domain.CodePermissionDenied, "only the owner of %s can change its MCP tools access", name).
				WithSuggestion("ask the agent's owner to change this run setting"))
			return
		}
		rs.MCPTools = *in.MCPTools
	}
	a, err := s.agents.SetRunSettings(r.Context(), name, rs)
	if err != nil {
		writeError(w, err)
		return
	}
	detail := "model=" + orDash(rs.Model) + " effort=" + orDash(rs.Effort) + " access=" + orDash(rs.Access) + " mcp_tools=" + strconv.FormatBool(rs.MCPTools)
	if providerChanged {
		detail += " provider=updated"
	}
	s.recordAs(r, actor, "agent.run_settings", string(name), detail)
	writeJSON(w, http.StatusOK, s.agentDTO(r, a))
}

// ownsComputer: the member who owns agent a (or, for a node nobody owns,
// the hub's owner).
func ownsComputer(actor app.Actor, a *domain.Agent) bool {
	if actor.Agent != nil {
		return false
	}
	if a.Owner == "" {
		return actor.IsOwner()
	}
	return actor.MemberID() == a.Owner
}

func (s *Server) ackMyProvider(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	var in struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.ID) < 16 || len(in.ID) > 80 {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "invalid provider request id"))
		return
	}
	s.runSettingsMu.Lock()
	defer s.runSettingsMu.Unlock()
	if err := s.agents.AckProvider(r.Context(), a.Name, in.ID); err != nil {
		writeError(w, err)
		return
	}
	s.recordAs(r, app.Actor{Agent: a}, "agent.provider.applied", string(a.Name), "request acknowledged")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func verifyEncryptedProvider(ctx context.Context, service *app.E2EService, name string, p *domain.EncryptedProvider) error {
	target, err := service.KeysOf(ctx, name)
	if err != nil || len(target) != 1 {
		return domain.Errf(domain.CodeInvalidArgument, "target agent has no encryption key")
	}
	signers, err := service.KeysOf(ctx, p.From)
	if err != nil {
		return domain.Errf(domain.CodeInvalidArgument, "sender encryption key unavailable")
	}
	for _, signer := range signers {
		if e2e.VerifyProvider(p.Body, e2e.Header{From: p.From, To: name, Type: e2e.ProviderSettingsType, ReplyTo: p.ID}, signer.Key.Sign, target[0].Key.Enc) == nil {
			return nil
		}
	}
	return domain.Errf(domain.CodeInvalidArgument, "invalid provider envelope or signature")
}
