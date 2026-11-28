package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
)

func skillsDTO(in []domain.Skill) []api.Skill {
	if len(in) == 0 {
		return nil
	}
	out := make([]api.Skill, len(in))
	for i, s := range in {
		out[i] = api.Skill{Name: s.Name, Summary: s.Summary, Examples: s.Examples}
	}
	return out
}

func skillsFromAPI(in []api.Skill) []domain.Skill {
	out := make([]domain.Skill, len(in))
	for i, s := range in {
		out[i] = domain.Skill{Name: s.Name, Summary: s.Summary, Examples: s.Examples}
	}
	return out
}

func (s *Server) agentDTO(r *http.Request, a *domain.Agent) api.Agent {
	out := api.Agent{
		Updating: a.Runtime.Updating(s.agents.Now()), ID: a.ID, Name: string(a.Name), DisplayName: a.DisplayName, Avatar: a.Avatar, Description: a.Description,
		Capabilities: nonNil(a.Capabilities),
		Skills:       skillsDTO(a.Skills),
		Visibility:   string(domain.VisibilityTeam),
		Labels:       nonNil(a.Labels),
		Binding: api.Binding{Harness: a.Binding.Harness, SessionID: a.Binding.SessionID,
			Environment: a.Binding.Environment, Workdir: a.Binding.Workdir},
		Delivery: api.Delivery{Mode: a.Delivery.Mode, URL: redactURL(a.Delivery.URL),
			HasSecret: a.Delivery.Secret != "", Params: a.Delivery.Params},
		AllowFrom:  make([]string, 0, len(a.AllowFrom)),
		EnrolledBy: a.EnrolledBy, InvitedBy: a.InvitedBy,
		Online:    a.IsOnline(s.agents.Now(), s.agents.OnlineWindow()),
		CreatedAt: a.CreatedAt, LastSeenAt: timePtr(a.LastSeenAt),
		StatusText: a.StatusText, StatusAt: timePtr(a.StatusAt),
	}
	for _, n := range a.AllowFrom {
		out.AllowFrom = append(out.AllowFrom, string(n))
	}
	out.Kind, out.MachineID = a.Kind(), a.MachineID
	out.OwnerID, out.Owner = a.Owner, s.ownerName(a.Owner)
	if rt := a.Runtime; rt != (domain.Runtime{}) {
		out.Runtime = &api.Runtime{Account: s.visibleAccount(r, a), CLI: rt.CLI, CLIVersion: rt.CLIVersion, Model: rt.Model, ModelSource: rt.ModelSource, Access: rt.Access,
			Provider: rt.Provider, ProviderSource: rt.ProviderSource, ProviderHost: rt.ProviderHost, Effort: rt.Effort, EffortSource: rt.EffortSource, MaxAccess: rt.MaxAccess, Quota: s.visibleQuota(r, a),
			RunnerInstance: rt.RunnerInstance, RestartingUntil: rt.RestartingUntil, Features: rt.FeatureList(), Away: rt.Away, Env: rt.Env, Models: modelsDTO(rt.ModelList()), ModelsFrom: rt.ModelsFrom, ModelsAt: timePtr(rt.ModelsAt)}
		if rt.MachineHealth != "" {
			var health api.MachineHealth
			if json.Unmarshal([]byte(rt.MachineHealth), &health) == nil {
				health.Processes = nil
				out.Runtime.MachineHealth = &health
			}
		}
		if rt.RunnerSlots != "" {
			var slots api.RunnerSlots
			if json.Unmarshal([]byte(rt.RunnerSlots), &slots) == nil {
				out.Runtime.RunnerSlots = &slots
			}
		}
		if rt.Tools != "" {
			var tools struct {
				MCP      []api.MCPServer  `json:"mcp"`
				Skills   []api.ToolSkill  `json:"skills"`
				Plugins  []api.ToolPlugin `json:"plugins"`
				Warnings []string         `json:"tool_warnings"`
			}
			if json.Unmarshal([]byte(rt.Tools), &tools) == nil {
				out.Runtime.MCP, out.Runtime.Skills, out.Runtime.Plugins, out.Runtime.ToolWarnings = tools.MCP, tools.Skills, tools.Plugins, tools.Warnings
			}
		}
	}
	if out.Runtime != nil {
		_ = json.Unmarshal([]byte(a.Runtime.Providers), &out.Runtime.Providers)
	}
	if rs := a.RunSettings; rs != (domain.RunSettings{}) {
		out.RunSettings = &api.RunSettings{Model: rs.Model, Effort: rs.Effort, Access: rs.Access, MCPTools: &rs.MCPTools, Provider: rs.Provider, ModelsRefresh: rs.ModelsRefresh}
		if rs.ProviderRef.ID != "" {
			out.RunSettings.ProviderRef = &api.ProviderRef{ID: rs.ProviderRef.ID, KeyVersion: rs.ProviderRef.KeyVersion}
		}
		if rs.ProviderHub != nil {
			// which request, and who set it: never the settings
			out.RunSettings.ProviderHub = &api.HubProviderInfo{ID: rs.ProviderHub.ID, From: rs.ProviderHub.From}
		}
		if rs.ProviderEncrypted != nil {
			e := rs.ProviderEncrypted
			out.RunSettings.ProviderEncrypted = &api.EncryptedProvider{ID: e.ID, From: e.From, Body: e.Body}
		}
	}
	if a.Private() {
		out.Visibility = string(domain.VisibilityPrivate)
	}
	return out
}

func (s *Server) machineDTO(v *app.MachineView) api.Machine {
	out := api.Machine{ID: v.ID, Name: v.Name, Hostname: v.Hostname, OS: v.OS, Arch: v.Arch, MycoVersion: v.MycoVersion,
		Updating: v.Updating, Online: v.Online, LastSeenAt: timePtr(v.LastSeenAt), CanSpawn: v.SpawnNode != "", Node: string(v.SpawnNode), Legacy: v.Legacy, Agents: []string{},
		OwnerID: v.Owner, Owner: s.ownerName(v.Owner), Health: v.Health, CanStopProcess: v.CanStopProcess}
	for _, a := range v.Agents {
		out.Agents = append(out.Agents, string(a))
	}
	return out
}

// enrich fills the aliases and live state of agent DTOs (best effort).
func (s *Server) enrich(ctx context.Context, dtos ...*api.Agent) {
	if m, err := s.agents.AliasMap(ctx); err == nil {
		for _, d := range dtos {
			d.Aliases = m[domain.AgentName(d.Name)]
		}
	}
	if s.overview == nil {
		return
	}
	if states, err := s.overview.States(ctx); err == nil {
		for _, d := range dtos {
			if st, ok := states[domain.AgentName(d.Name)]; ok {
				d.Watching = st.Watching
				d.Goal = goalBrief(st.Goal)
				d.State, d.Queued, d.InProgress, d.Held, d.Asking = string(st.State), st.Queued, st.InProgress, st.Held, st.Asking
			}
		}
	}
}

func aliasDTO(a *domain.Alias) api.Alias {
	return api.Alias{Alias: string(a.Name), Target: string(a.Target), CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt}
}

func enrollmentDTO(e *domain.Enrollment, now time.Time) api.Enrollment {
	out := api.Enrollment{ID: e.ID, NamePrefix: e.NamePrefix, Harness: e.Harness, Capabilities: nonNil(e.Capabilities), Labels: nonNil(e.Labels),
		AllowFrom: make([]string, 0, len(e.AllowFrom)), Note: e.Note, MaxUses: e.MaxUses, Uses: e.Uses,
		ExpiresAt: timePtr(e.ExpiresAt), RevokedAt: timePtr(e.RevokedAt), CreatedAt: e.CreatedAt,
		Active: e.Usable(now, true) == nil, Kind: e.Kind, ParentID: e.ParentID, Machine: e.Machine, OwnerID: e.Owner}
	for _, n := range e.AllowFrom {
		out.AllowFrom = append(out.AllowFrom, string(n))
	}
	return out
}

func fileDTO(f *domain.File) api.File {
	return api.File{ID: f.ID, Name: f.Name, Size: f.Size, SHA256: f.SHA256, ContentType: f.ContentType,
		Owner: string(f.Owner), CreatedAt: f.CreatedAt, ExpiresAt: timePtr(f.ExpiresAt)}
}

// redactURL drops query strings (which may carry tokens) from wake URLs.
func redactURL(raw string) string {
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		return raw[:i] + "?…"
	}
	return raw
}

func messageDTO(m *domain.Message) api.Message {
	out := api.Message{
		ParentID: m.ParentID, ReassignNote: m.ReassignNote,
		ID: m.ID, From: string(m.From), To: string(m.To), ThreadID: m.ThreadID, ReplyTo: m.ReplyTo, After: m.After,
		Type: string(m.Type), Body: m.Body, Status: string(m.Status), Attempts: m.Attempts,
		Error: m.Error, IdempotencyKey: m.IdempotencyKey, BroadcastID: m.BroadcastID, Project: m.Project, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
		FromOwner: m.FromOwner, E2E: domain.IsSealed(m.Body),
		Ask: m.Ask, Asking: m.Asking, Answers: m.Answers, Suggest: m.Suggest, Automation: m.Automation, Workflow: m.Workflow, GrantedBy: m.GrantedBy,
	}
	if m.Status == domain.StatusHeld && m.HoldUntil != "" && m.HoldUntil != domain.ManualHold {
		out.HeldFor = "reply"
	}
	if m.Status == domain.StatusDelivered {
		out.LeaseUntil = timePtr(m.LeaseUntil)
	}
	if m.Result != "" {
		out.Result = json.RawMessage(m.Result)
	}
	for _, a := range m.Attachments {
		out.Attachments = append(out.Attachments, api.Attachment{FileID: a.FileID, Name: a.Name, Size: a.Size, SHA256: a.SHA256, ContentType: a.ContentType})
	}
	return out
}

func messageList(msgs []*domain.Message) api.MessageList {
	out := api.MessageList{Messages: make([]api.Message, 0, len(msgs))}
	for _, m := range msgs {
		out.Messages = append(out.Messages, messageDTO(m))
	}
	return out
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

var statusByCode = map[domain.ErrorCode]int{
	domain.CodeInvalidArgument:  http.StatusBadRequest,
	domain.CodeNotFound:         http.StatusNotFound,
	domain.CodeConflict:         http.StatusConflict,
	domain.CodeInvalidState:     http.StatusConflict,
	domain.CodePermissionDenied: http.StatusForbidden,
	domain.CodeUnauthenticated:  http.StatusUnauthorized,
	domain.CodeInternal:         http.StatusInternalServerError,
	domain.CodeRateLimited:      http.StatusTooManyRequests,
	domain.CodeUnsupported:      http.StatusNotImplemented,
	domain.CodeE2ERequired:      http.StatusConflict,
	domain.CodeReplaced:         http.StatusUnauthorized,
}

func writeError(w http.ResponseWriter, err error) {
	var de *domain.Error
	if !errors.As(err, &de) {
		de = domain.Errf(domain.CodeInternal, "%v", err)
	}
	status, ok := statusByCode[de.Code]
	if !ok {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, api.Error{
		Code: string(de.Code), Message: de.Message, Suggestion: de.Suggestion, ErrorNode: de.ErrorNode,
		Retryable: de.Code == domain.CodeInternal || de.Code == domain.CodeRateLimited,
	})
}

func quotaDTO(q *domain.Quota) *api.Quota {
	if q == nil {
		return nil
	}
	now := time.Now().UTC()
	out := &api.Quota{Source: q.Source, At: q.At, Provider: q.Provider, ProfileRef: q.ProfileRef, MachineID: q.MachineID, SourceKind: q.SourceKind, Status: domain.QuotaStatusAt(q, now), CheckedAt: q.CheckedAt, ErrorCode: q.ErrorCode, Plan: q.Plan, Windows: []api.QuotaWindow{}}
	for _, w := range q.Windows {
		// A window whose reset time has passed is no longer the window these
		// numbers describe: drop it rather than show yesterday's use.
		if w.ResetsAt != nil && !w.ResetsAt.After(now) {
			continue
		}
		out.Windows = append(out.Windows, api.QuotaWindow{Balance: (*api.QuotaBalance)(w.Balance), Name: w.Name, Label: w.Label, Used: w.Used, DurationSec: w.DurationSec, ModelScope: w.ModelScope, ResetsAt: w.ResetsAt, Status: w.Status})
	}
	if out.Status == "ok" && len(out.Windows) == 0 {
		out.Status = "unknown"
	}
	return out
}

func quotaFromAPI(q *api.Quota) *domain.Quota {
	if q == nil {
		return nil
	}
	out := &domain.Quota{Source: q.Source, At: q.At, Provider: q.Provider, ProfileRef: q.ProfileRef, MachineID: q.MachineID, SourceKind: q.SourceKind, Status: q.Status, CheckedAt: q.CheckedAt, ErrorCode: q.ErrorCode, Plan: q.Plan}
	for _, w := range q.Windows {
		out.Windows = append(out.Windows, domain.QuotaWindow{Balance: (*domain.QuotaBalance)(w.Balance), Name: w.Name, Label: w.Label, Used: w.Used, DurationSec: w.DurationSec, ModelScope: w.ModelScope, ResetsAt: w.ResetsAt, Status: w.Status})
	}
	return out
}

func modelsFromAPI(in []api.RuntimeModel) []domain.RuntimeModel {
	var out []domain.RuntimeModel
	for _, m := range in {
		out = append(out, domain.RuntimeModel{ID: m.ID, Name: m.Name, Efforts: m.Efforts})
	}
	return out
}

func modelsDTO(in []domain.RuntimeModel) []api.RuntimeModel {
	var out []api.RuntimeModel
	for _, m := range in {
		out = append(out, api.RuntimeModel{ID: m.ID, Name: m.Name, Efforts: m.Efforts})
	}
	return out
}

func goalBrief(g *domain.Goal) *api.GoalBrief {
	if g == nil {
		return nil
	}
	objective := []rune(g.Objective)
	if len(objective) > 200 {
		objective = objective[:200]
	}
	return &api.GoalBrief{ID: g.ID, Objective: string(objective), Round: g.Round, MaxRounds: g.MaxRounds, NextWakeAt: g.NextWakeAt, State: g.State}
}

// visibleQuota deliberately does not use Actor.Owns: an administrator or
// project coordinator is not automatically the owner of a paid account.
func (s *Server) visibleQuota(r *http.Request, a *domain.Agent) *api.Quota {
	if a.Runtime.Quota == nil {
		return nil
	}
	owner := a.Owner
	if s.machines != nil {
		owner = s.machines.QuotaOwner(r.Context(), a)
	}
	if caller, ok := r.Context().Value(callerAgentKey{}).(*domain.Agent); ok {
		if owner == "" || a.Owner != owner {
			return nil
		}
		// An agent sees its own account, and an agent a member owns sees that
		// member's accounts: their coordinator plans how the quota is spent.
		// Only numbers and window times are kept, never credentials.
		if caller.Name != a.Name && caller.Owner != owner {
			return nil
		}
	} else if s.isMasterAdmin(r) {
		// The explicitly configured master credential retains recovery access.
	} else {
		actor := s.actorOf(r)
		if actor.Member == nil || owner == "" || actor.Member.ID != owner {
			return nil
		}
	}
	q := a.Runtime.Quota
	if s.machines != nil {
		q = s.machines.AccountQuota(r.Context(), a)
	}
	return quotaDTO(q)
}

// modelsFrom keeps a runner's word for where its models came from, when
// it is one the apps know and there are models.
func modelsFrom(from string, models []api.RuntimeModel) string {
	switch from {
	case "provider", "catalog", "cli":
		if len(models) > 0 {
			return from
		}
	}
	return ""
}

// modelsAt is when the provider listed the models (not in the future).
func modelsAt(at *time.Time, models []api.RuntimeModel) time.Time {
	if at == nil || len(models) == 0 || at.After(time.Now().Add(time.Minute)) {
		return time.Time{}
	}
	return at.UTC()
}

// Account identity is visible only to its owner and workspace administrators.
// Ordinary agent credentials cannot read it, even in a runtime-report reply.
func (s *Server) visibleAccount(r *http.Request, a *domain.Agent) string {
	if _, agent := r.Context().Value(callerAgentKey{}).(*domain.Agent); agent && superOf(r) == nil {
		return ""
	}
	actor := s.actorOf(r)
	if actor.Manages() || actor.Member != nil && a.Owner != "" && actor.Member.ID == a.Owner {
		return a.Runtime.Account
	}
	return ""
}

func (s *Server) machineDTOFor(r *http.Request, v *app.MachineView) api.Machine {
	out := s.machineDTO(v)
	actor := s.actorOf(r)
	if !actor.Owns(v.Owner) {
		out.CanStopProcess = false
		if out.Health != nil {
			h := *out.Health
			h.Processes = nil
			out.Health = &h
		}
	}
	return out
}
