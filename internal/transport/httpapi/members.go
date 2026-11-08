package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// SetMembers enables members (people, roles and ownership) and the audit
// log. Without it every console token acts as the hub's administrator.
func (s *Server) SetMembers(m *app.MemberService, audit *app.AuditService) {
	s.members, s.audit = m, audit
}

// actorOf is who makes an administrative request: the hub's admin token
// (acting as the owner), or the member behind a console token.
func (s *Server) actorOf(r *http.Request) app.Actor {
	if s.isMasterAdmin(r) || s.console == nil {
		return app.Actor{Admin: true}
	}
	t, ok := s.console.Authenticate(r.Context(), bearer(r))
	if !ok {
		return app.Actor{}
	}
	if s.members == nil || t.MemberID == "" {
		return app.Actor{Admin: true}
	}
	m, err := s.members.Get(r.Context(), t.MemberID)
	if err != nil || !m.Active() {
		return app.Actor{}
	}
	return app.Actor{Member: m}
}

// ownerIDFor is the member that new things created by actor belong to.
func (s *Server) ownerIDFor(ctx context.Context, a app.Actor) string {
	if a.Member != nil {
		return a.Member.ID
	}
	if s.members != nil {
		if o, err := s.members.Owner(ctx); err == nil {
			return o.ID
		}
	}
	return ""
}

// record appends to the audit log (no-op without members).
func (s *Server) record(r *http.Request, action, target, detail string) {
	if s.audit != nil {
		s.audit.Record(r.Context(), s.actorOf(r), action, target, detail)
	}
}

// mayManage answers 403 unless the actor is the owner or an admin.
func (s *Server) mayManage(w http.ResponseWriter, r *http.Request, what string) bool {
	if s.actorOf(r).Manages() {
		return true
	}
	writeError(w, domain.Errf(domain.CodePermissionDenied, "only the hub's owner or an admin can %s", what))
	return false
}

// mayOwn answers 403 unless the actor manages the hub or owns the thing.
func (s *Server) mayOwn(w http.ResponseWriter, r *http.Request, owner, what string) bool {
	if s.actorOf(r).Owns(owner) {
		return true
	}
	who := "its owner"
	if n := s.memberName(r.Context(), owner); n != "" {
		who = n
	}
	writeError(w, domain.Errf(domain.CodePermissionDenied, "you can only %s your own nodes and machines; this one belongs to %s", what, who).
		WithSuggestion("ask %s or a hub admin", who))
	return false
}

// mayOwnAgent checks the actor may manage agent name (404s pass through).
func (s *Server) mayOwnAgent(w http.ResponseWriter, r *http.Request, name, what string) bool {
	if s.actorOf(r).Manages() {
		return true
	}
	a, err := s.agents.Get(r.Context(), name)
	if err != nil {
		writeError(w, err)
		return false
	}
	return s.mayOwn(w, r, a.Owner, what)
}

// memberName resolves a member id to their name ("" if unknown).
func (s *Server) memberName(ctx context.Context, id string) string {
	if s.members == nil || id == "" {
		return ""
	}
	m, err := s.members.Get(ctx, id)
	if err != nil {
		return ""
	}
	return m.Name
}

// memberNames maps member ids to names (best effort).
func (s *Server) memberNames(ctx context.Context) map[string]string {
	out := map[string]string{}
	if s.members == nil {
		return out
	}
	list, err := s.members.List(ctx)
	if err != nil {
		return out
	}
	for _, m := range list {
		out[m.ID] = m.Name
	}
	return out
}

func memberDTO(m *domain.Member) api.Member {
	return api.Member{ID: m.ID, Name: m.Name, DisplayName: m.DisplayName, Role: string(m.Role), CreatedAt: m.CreatedAt, Removed: !m.Active()}
}

func (s *Server) membersEnabled(w http.ResponseWriter) bool {
	if s.members == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "members are not enabled on this hub"))
		return false
	}
	return true
}

func (s *Server) whoami(w http.ResponseWriter, r *http.Request) {
	a := s.actorOf(r)
	out := api.Whoami{Admin: a.Admin, CanManage: a.Manages()}
	if t := s.currentDevice(r); t != nil {
		d := consoleDTO(t)
		out.Device = &d
	}
	if a.Member != nil {
		d := memberDTO(a.Member)
		out.Member = &d
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	if !s.membersEnabled(w) {
		return
	}
	list, err := s.members.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.MemberList{Members: make([]api.Member, 0, len(list))}
	for _, m := range list {
		if m.Active() || r.URL.Query().Get("all") == "1" {
			out.Members = append(out.Members, memberDTO(m))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) inviteMember(w http.ResponseWriter, r *http.Request) {
	if !s.membersEnabled(w) {
		return
	}
	defer s.forgetNames()
	var in api.InviteRequest
	if !decode(w, r, &in) {
		return
	}
	m, plain, err := s.members.Invite(r.Context(), s.actorOf(r), app.InviteInput{Name: in.Name, DisplayName: in.DisplayName, Role: in.Role})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, api.InviteResponse{Member: memberDTO(m), Link: s.publicBase(r) + "/c/" + plain})
}

func (s *Server) updateMember(w http.ResponseWriter, r *http.Request) {
	if !s.membersEnabled(w) {
		return
	}
	defer s.forgetNames()
	var in api.MemberUpdateRequest
	if !decode(w, r, &in) {
		return
	}
	m, err := s.members.Update(r.Context(), s.actorOf(r), r.PathValue("member"), app.UpdateInput{Name: in.Name, DisplayName: in.DisplayName, Role: in.Role})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, memberDTO(m))
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	if !s.membersEnabled(w) {
		return
	}
	defer s.forgetNames()
	m, err := s.members.Remove(r.Context(), s.actorOf(r), r.PathValue("member"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, memberDTO(m))
}

// newOwner resolves the member an owner change names.
func (s *Server) newOwner(w http.ResponseWriter, r *http.Request) (*domain.Member, bool) {
	if !s.membersEnabled(w) {
		return nil, false
	}
	var in api.OwnerRequest
	if !decode(w, r, &in) {
		return nil, false
	}
	m, err := s.members.Find(r.Context(), in.Owner)
	if err == nil && !m.Active() {
		err = domain.Errf(domain.CodeInvalidState, "member %s was removed", m.Name)
	}
	if err != nil {
		writeError(w, err)
		return nil, false
	}
	return m, true
}

func (s *Server) setAgentOwner(w http.ResponseWriter, r *http.Request) {
	if !s.mayManage(w, r, "change who owns a node") {
		return
	}
	m, ok := s.newOwner(w, r)
	if !ok {
		return
	}
	a, err := s.agents.SetOwner(r.Context(), r.PathValue("name"), m.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "agent.owner", string(a.Name), "now owned by "+m.Name)
	writeJSON(w, http.StatusOK, s.agentDTO(a))
}

func (s *Server) setMachineOwner(w http.ResponseWriter, r *http.Request) {
	if !s.machinesEnabled(w) || !s.mayManage(w, r, "change who owns a machine") {
		return
	}
	m, ok := s.newOwner(w, r)
	if !ok {
		return
	}
	mc, err := s.machines.SetOwner(r.Context(), r.PathValue("id"), m.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "machine.owner", mc.Name, "now owned by "+m.Name)
	writeJSON(w, http.StatusOK, map[string]string{"id": mc.ID, "name": mc.Name, "owner": m.Name, "owner_id": m.ID})
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	if !s.membersEnabled(w) || !s.mayManage(w, r, "read the audit log") {
		return
	}
	q := r.URL.Query()
	f := domain.AuditFilter{Actor: q.Get("actor"), Target: q.Get("target")}
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	if b, err := strconv.ParseInt(q.Get("before"), 10, 64); err == nil && b > 0 {
		f.Before = time.UnixMilli(b)
	}
	list, err := s.audit.List(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.AuditList{Entries: make([]api.AuditEntry, 0, len(list))}
	for _, e := range list {
		out.Entries = append(out.Entries, api.AuditEntry{ID: e.ID, At: e.At, Actor: e.ActorName, ActorKind: e.ActorKind,
			Action: e.Action, Target: e.Target, Detail: e.Detail})
	}
	writeJSON(w, http.StatusOK, out)
}

// mayOwnMachine checks the actor may manage machine id.
func (s *Server) mayOwnMachine(w http.ResponseWriter, r *http.Request, id, what string) bool {
	if s.actorOf(r).Manages() {
		return true
	}
	m, err := s.machines.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return false
	}
	return s.mayOwn(w, r, m.Owner, what)
}

func orNone(s string) string {
	if s == "" {
		return "anyone"
	}
	return s
}

// ownerName resolves a member id for DTOs, from a cache refreshed at most
// every few seconds ("" if unknown or members are off).
func (s *Server) ownerName(id string) string {
	if s.members == nil || id == "" {
		return ""
	}
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	if time.Since(s.namesAt) > 3*time.Second {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		s.names = s.memberNames(ctx)
		cancel()
		s.namesAt = time.Now()
	}
	return s.names[id]
}

// forgetNames drops the member name cache (after member changes).
func (s *Server) forgetNames() {
	s.namesMu.Lock()
	s.namesAt = time.Time{}
	s.namesMu.Unlock()
}

// currentDevice is the console token of the request (nil for the admin token).
func (s *Server) currentDevice(r *http.Request) *domain.ConsoleToken {
	if s.console == nil || s.isMasterAdmin(r) {
		return nil
	}
	t, ok := s.console.Authenticate(r.Context(), bearer(r))
	if !ok {
		return nil
	}
	return t
}

// pair trades a one-time pairing code for a device's own console token
// (no other credential needed: the code is the proof).
func (s *Server) pair(w http.ResponseWriter, r *http.Request) {
	if s.console == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "console tokens are not enabled on this hub"))
		return
	}
	var in api.PairRequest
	if !decode(w, r, &in) {
		return
	}
	t, plain, err := s.console.Pair(r.Context(), strings.TrimSpace(in.Code), in.Device)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.PairResponse{Token: plain, Device: consoleDTO(t)}
	if s.members != nil {
		if m, err := s.members.Get(r.Context(), t.MemberID); err == nil {
			d := memberDTO(m)
			out.Member = &d
			if s.audit != nil {
				s.audit.Record(r.Context(), app.Actor{Member: m}, "device.add", t.Name, "")
			}
		}
	}
	writeJSON(w, http.StatusCreated, out)
}

// createPairing makes a one-time link for the caller to add another device.
func (s *Server) createPairing(w http.ResponseWriter, r *http.Request) {
	owner := s.ownerIDFor(r.Context(), s.actorOf(r))
	plain, exp, err := s.console.CreatePairing(r.Context(), owner)
	if err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "device.link", "", "valid until "+exp.UTC().Format(time.RFC3339))
	writeJSON(w, http.StatusCreated, api.PairingLink{Link: s.publicBase(r) + "/c/" + plain, ExpiresAt: exp})
}

func (s *Server) myDevices(w http.ResponseWriter, r *http.Request) {
	list, err := s.console.Devices(r.Context(), s.ownerIDFor(r.Context(), s.actorOf(r)))
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.DeviceList{Devices: make([]api.ConsoleToken, 0, len(list))}
	for _, t := range list {
		out.Devices = append(out.Devices, consoleDTO(t))
	}
	writeJSON(w, http.StatusOK, out)
}

// removeMyDevice revokes one of the caller's devices.
func (s *Server) removeMyDevice(w http.ResponseWriter, r *http.Request) {
	list, err := s.console.Devices(r.Context(), s.ownerIDFor(r.Context(), s.actorOf(r)))
	if err != nil {
		writeError(w, err)
		return
	}
	for _, t := range list {
		if t.ID == r.PathValue("id") {
			got, err := s.console.Revoke(r.Context(), t.ID)
			if err != nil {
				writeError(w, err)
				return
			}
			s.record(r, "device.remove", t.Name, "")
			writeJSON(w, http.StatusOK, consoleDTO(got))
			return
		}
	}
	writeError(w, domain.Errf(domain.CodeNotFound, "no such device of yours"))
}
