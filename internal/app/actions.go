package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/domain"
)

// ActionService: agents acting on a target (a computer) through a provider
// the target's own App runs, with its owner's approval. See api/actions.go.
//
// Grants, hosts, jobs and artifacts live in memory only: they are short
// (30 minutes at most) and a hub restart ends every session, so nothing can
// outlive what its owner approved. An approval whose grant is gone cannot be
// approved any more.
type ActionService struct {
	approvals *SuperagentService
	machines  *MachineService
	members   *MemberService
	messages  *MessageService
	pages     *PageService
	e2e       func(context.Context) bool
	audit     *AuditService
	ids       IDGenerator
	clock     Clock

	mu     sync.Mutex
	wake   chan struct{} // closed (and replaced) when anything changes
	grants map[string]*actionGrant
	hosts  map[string]*actionHost
	jobs   map[string]*actionJob
	arts   map[string]*actionArtifact
}

// Limits.
const (
	ActionGrantDefaultTTL = 5 * time.Minute
	ActionGrantMaxTTL     = 30 * time.Minute
	actionHostStale       = 45 * time.Second
	actionHostGone        = 10 * time.Minute // a host not seen this long is dropped
	actionArtifactMax     = 4 << 20
	actionArtifactTTL     = 10 * time.Minute
	actionArgsMax         = 64 << 10
	actionResultMax       = 256 << 10
	actionJobsPerGrant    = 1000
	actionSchemaMax       = 16 << 10
	actionWaitMax         = 60 * time.Second
)

// actionProviders: the providers a trusted App may host, and the
// permissions each defines. A page cannot add one.
var actionProviders = map[string][]string{
	"remote.desktop": {"remote.desktop.view", "remote.desktop.control"},
	// A shared page's own content, rendered by its app (page.export: a
	// file of what the agent can already read).
	"page":       {"page.export"},
	"server.ssh": {"server.ssh.read", "server.ssh.manage"},
}

// providerTarget is the kind of target each provider acts on.
var providerTarget = map[string]string{"remote.desktop": "machine", "page": "page", "server.ssh": "machine"}

// providerOf is the provider a permission or action name belongs to.
func providerOf(name string) string {
	for p := range actionProviders {
		if strings.HasPrefix(name, p+".") {
			return p
		}
	}
	return ""
}

type actionGrant struct {
	api.ActionGrant
	agentOwner string // the agent's owner (member id): page visibility is checked as it
	ttl        time.Duration
	hostSeen   string // the host it ran on (it ends when that host goes quiet)
	idem       map[string]string
	idemHash   map[string]string
	jobsCount  int
}

type actionHost struct {
	api.ActionHost
	owner   string // the member who registered it
	actions []api.ActionDescriptor
}

type actionJob struct {
	api.ActionJob
	agent  string
	args   json.RawMessage
	hostID string
}

type actionArtifact struct {
	api.ActionArtifact
	grantID string
	jobID   string
	hostID  string
	data    []byte
}

// NewActionService wires agent actions. e2e reports whether the workspace
// encrypts end to end (then actions are unsupported: the hub would see
// what they carry).
func NewActionService(approvals *SuperagentService, machines *MachineService, members *MemberService, messages *MessageService,
	e2e func(context.Context) bool, audit *AuditService, ids IDGenerator, clock Clock) *ActionService {
	s := &ActionService{approvals: approvals, machines: machines, members: members, messages: messages, e2e: e2e, audit: audit,
		ids: ids, clock: clock, wake: make(chan struct{}),
		grants: map[string]*actionGrant{}, hosts: map[string]*actionHost{}, jobs: map[string]*actionJob{}, arts: map[string]*actionArtifact{}}
	if approvals != nil {
		approvals.SetTyped(domain.ApprovalKindActionGrant, actionApprovals{s})
	}
	return s
}

// SetPages enables page targets (page.export).
func (s *ActionService) SetPages(p *PageService) { s.pages = p }

// pageFor returns the page if by may see it (not found otherwise).
func (s *ActionService) pageFor(ctx context.Context, by Actor, id string) (*domain.Page, error) {
	if s.pages == nil {
		return nil, domain.Errf(domain.CodeUnsupported, "this hub has no pages")
	}
	v, err := s.pages.Get(ctx, by, id)
	if err != nil {
		return nil, err
	}
	return v.Page, nil
}

// agentSees reports whether the grant's agent may still see its page
// target (a machine target needs no such check).
func (s *ActionService) agentSees(ctx context.Context, g *actionGrant) bool {
	if g.Target.Kind != "page" {
		return true
	}
	_, err := s.pageFor(ctx, Actor{Agent: &domain.Agent{Name: domain.AgentName(g.Agent), Owner: g.agentOwner}}, g.Target.ID)
	return err == nil
}

// memberSees reports whether a member may see a page.
func (s *ActionService) memberSees(ctx context.Context, memberID, pageID string) bool {
	if s.members == nil {
		return false
	}
	m, err := s.members.Get(ctx, memberID)
	if err != nil || !m.Active() {
		return false
	}
	_, err = s.pageFor(ctx, Actor{Member: m}, pageID)
	return err == nil
}

// changed wakes everything waiting (call with mu held).
func (s *ActionService) changed() {
	close(s.wake)
	s.wake = make(chan struct{})
}

func (s *ActionService) enabled(ctx context.Context) error {
	if s.e2e != nil && s.e2e(ctx) {
		return domain.Errf(domain.CodeUnsupported, "agent actions are not available in an end-to-end encrypted workspace (the hub would see what they carry)")
	}
	return nil
}

// ---- grants (the agent) ----

// RequestGrant asks the target's owner to let agent act on it (with dry,
// only checks and returns what would be asked).
func (s *ActionService) RequestGrant(ctx context.Context, agent *domain.Agent, req api.ActionGrantRequest, dry bool) (*api.ActionGrant, error) {
	if err := s.enabled(ctx); err != nil {
		return nil, err
	}
	if req.Target.Kind == "page" {
		if req.Target.Resource != nil {
			return nil, domain.Errf(domain.CodeInvalidArgument, "page targets do not accept resource")
		}
		return s.requestPageGrant(ctx, agent, req, dry)
	}
	if req.Target.Kind != "machine" {
		return nil, domain.Errf(domain.CodeUnsupported, "a target is a computer (kind \"machine\") or a shared page (kind \"page\"), not %q", req.Target.Kind)
	}
	m, err := s.machineByIDOrName(ctx, req.Target.ID)
	if err != nil {
		return nil, err
	}
	req.Target.ID = m.ID
	perms, err := checkPermissions(req.Permissions, "machine")
	if err != nil {
		return nil, err
	}
	targetName := m.Name
	if req.Target.Resource != nil || contains(perms, "server.ssh.read") {
		if req.Target.Resource == nil || req.Target.Resource.Provider != "server.ssh" {
			return nil, domain.Errf(domain.CodeInvalidArgument, "SSH grants require an enabled, bound profile resource")
		}
		for _, p := range perms {
			if providerOf(p) != "server.ssh" {
				return nil, domain.Errf(domain.CodeInvalidArgument, "a resource grant cannot mix providers")
			}
		}
		s.mu.Lock()
		var found *api.ActionResource
		for _, h := range s.hosts {
			if h.MachineID == m.ID && h.Provider == "server.ssh" && sameResource(h.Resource, req.Target.Resource) && s.clock.Now().Sub(h.LastSeenAt) <= actionHostStale {
				cp := *h.Resource
				found = &cp
				break
			}
		}
		s.mu.Unlock()
		if found == nil {
			return nil, domain.Errf(domain.CodeUnsupported, "this SSH profile is not enabled on the controller App, or its binding changed")
		}
		req.Target.Resource = found
		targetName += " / " + found.Name + " / " + found.Summary
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return nil, domain.Errf(domain.CodeInvalidArgument, "say why (reason): its owner reads it before approving")
	}
	ttl := ActionGrantDefaultTTL
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}
	if ttl > ActionGrantMaxTTL {
		ttl = ActionGrantMaxTTL
	}
	if ttl < 30*time.Second {
		ttl = 30 * time.Second
	}
	if req.MessageID != "" {
		msg, err := s.messages.Get(ctx, agent, req.MessageID)
		if err != nil {
			return nil, err
		}
		if msg.To != agent.Name || msg.Type != domain.TypeTask || msg.Status.Terminal() {
			return nil, domain.Errf(domain.CodeInvalidArgument, "message %s is not an open task of yours", req.MessageID)
		}
	}
	owner, err := s.ownerOf(ctx, m)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	g := &actionGrant{agentOwner: agent.Owner, ttl: ttl, idem: map[string]string{}, idemHash: map[string]string{}}
	g.ActionGrant = api.ActionGrant{ID: s.ids.New("agr"), Agent: string(agent.Name), Owner: owner.Name, Target: req.Target, TargetName: targetName,
		Requested: perms, Reason: clipRunes(reason, 500), MessageID: req.MessageID, State: api.GrantPending, CreatedAt: now}
	if dry {
		g.Permissions = perms
		exp := now.Add(ttl)
		g.ExpiresAt = &exp
		out := g.ActionGrant
		return &out, nil
	}
	payload, _ := json.Marshal(map[string]string{"grant_id": g.ID})
	ap, err := s.approvals.AskTyped(ctx, owner, agent.Name, domain.ApprovalKindActionGrant, grantWhat(g.Agent, targetName, perms, ttl), reason, string(payload))
	if err != nil {
		return nil, err
	}
	g.ApprovalID = ap.ID
	s.mu.Lock()
	s.grants[g.ID] = g
	s.changed()
	out := g.ActionGrant
	s.mu.Unlock()
	s.audit.Record(ctx, Actor{Agent: agent}, "action.grant.request", m.Name, fmt.Sprintf("%s (%s)", strings.Join(perms, ","), g.ID))
	return &out, nil
}

// requestPageGrant: an agent that may see a shared page may export it (a
// file of what it can already read), so the grant is active at once, for
// the page's owner to see and revoke. Its visibility is checked again at
// every job and artifact.
func (s *ActionService) requestPageGrant(ctx context.Context, agent *domain.Agent, req api.ActionGrantRequest, dry bool) (*api.ActionGrant, error) {
	p, err := s.pageFor(ctx, Actor{Agent: agent}, req.Target.ID)
	if err != nil {
		return nil, err
	}
	perms, err := checkPermissions(req.Permissions, "page")
	if err != nil {
		return nil, err
	}
	ttl := ActionGrantDefaultTTL
	if req.TTLSeconds > 0 {
		ttl = min(max(time.Duration(req.TTLSeconds)*time.Second, 30*time.Second), ActionGrantMaxTTL)
	}
	if req.MessageID != "" {
		msg, err := s.messages.Get(ctx, agent, req.MessageID)
		if err != nil {
			return nil, err
		}
		if msg.To != agent.Name || msg.Type != domain.TypeTask || msg.Status.Terminal() {
			return nil, domain.Errf(domain.CodeInvalidArgument, "message %s is not an open task of yours", req.MessageID)
		}
	}
	ownerName := ""
	if s.members != nil {
		if p.Owner != "" {
			if mem, err := s.members.Get(ctx, p.Owner); err == nil && mem.Active() {
				ownerName = mem.Name
			}
		}
		if ownerName == "" {
			if o, err := s.members.Owner(ctx); err == nil && o != nil {
				ownerName = o.Name
			}
		}
	}
	now := s.clock.Now()
	exp := now.Add(ttl)
	g := &actionGrant{agentOwner: agent.Owner, ttl: ttl, idem: map[string]string{}, idemHash: map[string]string{}}
	g.ActionGrant = api.ActionGrant{ID: s.ids.New("agr"), Agent: string(agent.Name), Owner: ownerName, Target: api.ActionTarget{Kind: "page", ID: p.ID}, TargetName: p.Name,
		Permissions: perms, Requested: perms, Reason: clipRunes(strings.TrimSpace(req.Reason), 500), MessageID: req.MessageID, State: api.GrantActive, CreatedAt: now, ExpiresAt: &exp}
	out := g.ActionGrant
	if dry {
		return &out, nil
	}
	s.mu.Lock()
	s.grants[g.ID] = g
	s.changed()
	s.mu.Unlock()
	s.audit.Record(ctx, Actor{Agent: agent}, "action.grant.page", p.Name, fmt.Sprintf("%s (%s)", strings.Join(perms, ","), g.ID))
	return &out, nil
}

// machineByIDOrName finds a computer by its id, or by its name when that
// is unique (agents know computers by name).
func (s *ActionService) machineByIDOrName(ctx context.Context, idOrName string) (*domain.Machine, error) {
	m, err := s.machines.Get(ctx, idOrName)
	if err == nil || !domain.IsCode(err, domain.CodeNotFound) {
		return m, err
	}
	views, _, lerr := s.machines.List(ctx)
	if lerr != nil {
		return nil, err
	}
	var found *domain.Machine
	for _, v := range views {
		if !v.Legacy && strings.EqualFold(v.Machine.Name, idOrName) {
			if found != nil {
				return nil, domain.Errf(domain.CodeInvalidArgument, "more than one computer is called %s; use its id", idOrName)
			}
			mm := v.Machine
			found = &mm
		}
	}
	if found == nil {
		return nil, err
	}
	return found, nil
}

// grantWhat says in words what a grant lets its agent do (the approval's
// title).
func grantWhat(agent, machine string, perms []string, ttl time.Duration) string {
	do := "看电脑 " + machine + " 的屏幕"
	for _, p := range perms {
		if p == "server.ssh.read" {
			do = "读取服务器 " + machine + " 的状态与日志"
		}
		if p == "server.ssh.manage" {
			do = "读取服务器 " + machine + " 并启动、停止或重启服务与容器"
		}
		if p == "remote.desktop.control" {
			do = "看并操作电脑 " + machine + " 的屏幕（键盘和鼠标）"
		}
	}
	// Manage implies read, so its more powerful description must win regardless
	// of the normalized permission order.
	if contains(perms, "server.ssh.manage") {
		do = "读取服务器 " + machine + " 并启动、停止或重启服务与容器"
	}
	_ = agent // (the card says who asks)
	return fmt.Sprintf("%s，%d 分钟", do, int(ttl.Round(time.Minute)/time.Minute))
}

// checkPermissions validates and normalizes requested permissions: known
// ones only, no wildcard; control implies view.
func checkPermissions(in []string, kind string) ([]string, error) {
	if len(in) == 0 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "ask for at least one permission (a computer: remote.desktop.view, remote.desktop.control; a page: page.export)")
	}
	set := map[string]bool{}
	for _, p := range in {
		p = strings.TrimSpace(p)
		prov := providerOf(p)
		known := false
		for _, k := range actionProviders[prov] {
			known = known || k == p
		}
		if !known {
			return nil, domain.Errf(domain.CodeInvalidArgument, "unknown permission %q", p).WithSuggestion("a computer: remote.desktop.view or remote.desktop.control; a page: page.export")
		}
		if kind != "" && providerTarget[prov] != kind {
			return nil, domain.Errf(domain.CodeInvalidArgument, "%s is not for a %s target", p, kind)
		}
		set[p] = true
	}
	if set["server.ssh.manage"] {
		set["server.ssh.read"] = true
	}
	if set["remote.desktop.control"] {
		set["remote.desktop.view"] = true
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// ownerOf is the member who decides for a computer: its owner, or the
// hub's owner for one nobody owns.
func (s *ActionService) ownerOf(ctx context.Context, m *domain.Machine) (*domain.Member, error) {
	if s.members == nil {
		return nil, domain.Errf(domain.CodeUnsupported, "this hub has no members to approve agent actions")
	}
	if m.Owner != "" {
		if mem, err := s.members.Get(ctx, m.Owner); err == nil && mem.Active() {
			return mem, nil
		}
	}
	o, err := s.members.Owner(ctx)
	if err != nil || o == nil {
		return nil, domain.Errf(domain.CodeInvalidState, "nobody can approve actions on %s (it has no owner)", m.Name)
	}
	return o, nil
}

// actionApprovals decides action_grant approvals.
type actionApprovals struct{ s *ActionService }

func (h actionApprovals) grantOf(a *domain.Approval) string {
	var p struct {
		GrantID string `json:"grant_id"`
	}
	_ = json.Unmarshal([]byte(a.Payload), &p)
	return p.GrantID
}

func (h actionApprovals) Approve(ctx context.Context, a *domain.Approval, by *domain.Member, narrow json.RawMessage) (string, error) {
	s := h.s
	var n api.ActionGrantNarrow
	if len(bytes.TrimSpace(narrow)) > 0 {
		if err := json.Unmarshal(narrow, &n); err != nil {
			return "", domain.Errf(domain.CodeInvalidArgument, "invalid body: %v", err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.grants[h.grantOf(a)]
	if g == nil || g.State != api.GrantPending {
		return "", domain.Errf(domain.CodeInvalidState, "this request is no longer waiting (its agent ended it, or the hub restarted); ask the agent to request again")
	}
	perms := g.Requested
	if len(n.Permissions) > 0 {
		narrowed, err := checkPermissions(n.Permissions, g.Target.Kind)
		if err != nil {
			return "", err
		}
		for _, p := range narrowed {
			if !contains(g.Requested, p) {
				return "", domain.Errf(domain.CodeInvalidArgument, "%s was not asked for: approve less than asked, never more", p)
			}
		}
		perms = narrowed
	}
	ttl := g.ttl
	if n.TTLSeconds > 0 {
		t := time.Duration(n.TTLSeconds) * time.Second
		if t > g.ttl {
			return "", domain.Errf(domain.CodeInvalidArgument, "%d seconds is longer than asked (%d)", n.TTLSeconds, int(g.ttl/time.Second))
		}
		ttl = t
	}
	now := s.clock.Now()
	exp := now.Add(ttl)
	g.Permissions, g.State, g.ExpiresAt = perms, api.GrantActive, &exp
	s.changed()
	return fmt.Sprintf("授权 %s 生效到 %s（%s）；用 myco action run --grant %s 执行动作。", g.ID, exp.Format(time.RFC3339), strings.Join(perms, "、"), g.ID), nil
}

func (h actionApprovals) Deny(ctx context.Context, a *domain.Approval) {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if g := s.grants[h.grantOf(a)]; g != nil && g.State == api.GrantPending {
		now := s.clock.Now()
		g.State, g.EndedAt, g.EndReason = api.GrantDenied, &now, "denied"
		s.changed()
	}
}

// artifactTypeAllowed: the files a job may make (a screenshot, an export).
func artifactTypeAllowed(t string) bool {
	switch t {
	// (text/html: an app's export of a design; artifacts are served as
	// attachments, nosniff, sandboxed, never as a page)
	case "text/plain", "text/csv", "text/markdown", "text/html", "application/json", "application/pdf":
		return true
	}
	return strings.HasPrefix(t, "image/") || strings.HasPrefix(t, "application/vnd.openxmlformats-officedocument.")
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// refresh ends a grant whose time, host or task is over (mu held).
func (s *ActionService) refresh(ctx context.Context, g *actionGrant) {
	now := s.clock.Now()
	switch g.State {
	case api.GrantPending:
		if now.Sub(g.CreatedAt) > domain.ApprovalTTL {
			s.close(g, api.GrantExpired, "nobody approved it in time")
		}
	case api.GrantActive:
		if g.ExpiresAt != nil && !now.Before(*g.ExpiresAt) {
			s.close(g, api.GrantExpired, "its time ran out")
			return
		}
		if !s.agentSees(ctx, g) {
			s.close(g, api.GrantEnded, "its agent may no longer see the page")
			return
		}
		if g.hostSeen != "" {
			if h := s.hosts[g.hostSeen]; h == nil || now.Sub(h.LastSeenAt) > actionHostStale {
				if g.Target.Kind != "page" {
					s.close(g, api.GrantEnded, "the computer went offline (or its App closed)")
					return
				}
				// A page is rendered by any App of a person who sees it: the
				// grant stays; what that App was running is lost.
				s.failRunning(g.hostSeen, "host_gone", "the App rendering it closed: export again")
				g.hostSeen = ""
			}
		}
		if g.MessageID != "" && s.messages != nil {
			if m, err := s.messages.Get(ctx, &domain.Agent{Name: domain.AgentName(g.Agent)}, g.MessageID); err != nil || m.Status.Terminal() {
				s.close(g, api.GrantEnded, "its task ended")
			}
		}
	}
}

// close ends a grant: its queued jobs are cancelled, running ones can no
// longer report, its artifacts are gone (mu held).
func (s *ActionService) close(g *actionGrant, state, reason string) {
	if g.State != api.GrantPending && g.State != api.GrantActive {
		return
	}
	now := s.clock.Now()
	g.State, g.EndedAt, g.EndReason = state, &now, reason
	for _, j := range s.jobs {
		if j.GrantID != g.ID {
			continue
		}
		switch j.State {
		case api.JobQueued:
			j.State, j.FinishedAt = api.JobCancelled, &now
			j.Error = &api.ActionError{Code: "grant_ended", Message: "the grant ended before it ran: " + reason}
		case api.JobRunning:
			j.CancelRequested = true
		}
	}
	for id, a := range s.arts {
		if a.grantID == g.ID {
			delete(s.arts, id)
		}
	}
	s.changed()
}

// grantOf returns agent's grant (refreshed), or not found (mu held).
func (s *ActionService) grantOf(ctx context.Context, agent domain.AgentName, id string) (*actionGrant, error) {
	g := s.grants[id]
	if g == nil || g.Agent != string(agent) {
		return nil, domain.Errf(domain.CodeNotFound, "grant %s not found", id)
	}
	s.refresh(ctx, g)
	return g, nil
}

// waitFor waits until done reports true (checked under mu), wait runs out
// or ctx ends; it returns what get returns then.
func waitFor[T any](ctx context.Context, s *ActionService, wait time.Duration, get func() (T, bool, error)) (T, error) {
	if wait > actionWaitMax {
		wait = actionWaitMax
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	for {
		s.mu.Lock()
		v, done, err := get()
		ch := s.wake
		s.mu.Unlock()
		if err != nil || done || wait <= 0 {
			return v, err
		}
		select {
		case <-ch:
		case <-deadline.C:
			s.mu.Lock()
			v, _, err = get()
			s.mu.Unlock()
			return v, err
		case <-ctx.Done():
			return v, nil
		case <-time.After(5 * time.Second): // (expiry needs no event)
		}
	}
}

// Grant returns one of agent's grants; with wait, until it is no longer
// pending.
func (s *ActionService) Grant(ctx context.Context, agent *domain.Agent, id string, wait time.Duration) (*api.ActionGrant, error) {
	return waitFor(ctx, s, wait, func() (*api.ActionGrant, bool, error) {
		g, err := s.grantOf(ctx, agent.Name, id)
		if err != nil {
			return nil, true, err
		}
		out := g.ActionGrant
		return &out, g.State != api.GrantPending, nil
	})
}

// Grants lists agent's grants, newest first.
func (s *ActionService) Grants(ctx context.Context, agent *domain.Agent) []api.ActionGrant {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []api.ActionGrant{}
	for _, g := range s.grants {
		if g.Agent == string(agent.Name) {
			s.refresh(ctx, g)
			out = append(out, g.ActionGrant)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// EndGrant ends one of agent's grants (pending or active).
func (s *ActionService) EndGrant(ctx context.Context, agent *domain.Agent, id string) (*api.ActionGrant, error) {
	s.mu.Lock()
	g, err := s.grantOf(ctx, agent.Name, id)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.close(g, api.GrantEnded, "its agent ended it")
	out := g.ActionGrant
	s.mu.Unlock()
	s.audit.Record(ctx, Actor{Agent: agent}, "action.grant.end", out.TargetName, out.ID)
	return &out, nil
}

// Actions lists what agent may do with a grant: the target's actions its
// permissions cover.
func (s *ActionService) Actions(ctx context.Context, agent *domain.Agent, id string) ([]api.ActionDescriptor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.grantOf(ctx, agent.Name, id)
	if err != nil {
		return nil, err
	}
	if g.State != api.GrantActive {
		return nil, domain.Errf(domain.CodeInvalidState, "grant %s is %s", id, g.State)
	}
	out := []api.ActionDescriptor{}
	for _, h := range s.hostsFor(ctx, g) {
		for _, d := range h.actions {
			if contains(g.Permissions, d.Permission) {
				out = append(out, d)
			}
		}
	}
	return out, nil
}

// ---- grants (the owner) ----

// OwnerGrants lists the grants a member decides (all of them for one who
// manages the hub), newest first; state filters ("" for all).
func (s *ActionService) OwnerGrants(ctx context.Context, by Actor, state string) ([]api.ActionGrant, error) {
	m, err := person(by, "see agent grants")
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []api.ActionGrant{}
	for _, g := range s.grants {
		if g.Owner != m.Name && !by.Manages() {
			continue
		}
		s.refresh(ctx, g)
		if state == "" || g.State == state {
			out = append(out, g.ActionGrant)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// Revoke ends a grant its owner (or someone who manages the hub) no longer
// allows.
func (s *ActionService) Revoke(ctx context.Context, by Actor, id string) (*api.ActionGrant, error) {
	m, err := person(by, "revoke an agent's grant")
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	g := s.grants[id]
	if g == nil || (g.Owner != m.Name && !by.Manages()) {
		s.mu.Unlock()
		return nil, domain.Errf(domain.CodeNotFound, "grant %s not found", id)
	}
	s.close(g, api.GrantRevoked, "revoked by "+m.Name)
	out := g.ActionGrant
	s.mu.Unlock()
	s.audit.Record(ctx, by, "action.grant.revoke", out.TargetName, fmt.Sprintf("%s (%s)", out.Agent, out.ID))
	return &out, nil
}

// ---- hosts (the trusted App on the target) ----

// RegisterHost offers a provider's actions on a computer the member owns
// (or manages the hub of). Registering again (a new generation) replaces
// the old host: what it was running fails as outcome unknown.
func (s *ActionService) RegisterHost(ctx context.Context, by Actor, req api.ActionHostRegister) (*api.ActionHost, error) {
	m, err := person(by, "host agent actions")
	if err != nil {
		return nil, err
	}
	if err := s.enabled(ctx); err != nil {
		return nil, err
	}
	perms, ok := actionProviders[req.Provider]
	if !ok {
		return nil, domain.Errf(domain.CodeInvalidArgument, "unknown provider %q", req.Provider)
	}
	// A page renderer belongs to its person (it renders only pages they may
	// see); a computer's provider to that computer.
	where := m.Name
	machineID := ""
	if providerTarget[req.Provider] == "machine" {
		mach, err := s.machines.Get(ctx, req.MachineID)
		if err != nil {
			return nil, err
		}
		if mach.Owner != m.ID && (!by.Manages() || req.Provider == "server.ssh") {
			return nil, domain.Errf(domain.CodePermissionDenied, "%s is not your computer", mach.Name)
		}
		where, machineID = mach.Name, mach.ID
	}
	if req.Provider == "server.ssh" {
		if err := validateSSHResource(req.Resource); err != nil {
			return nil, err
		}
		cp := *req.Resource
		req.Resource = &cp
	} else if req.Resource != nil {
		return nil, domain.Errf(domain.CodeInvalidArgument, "this provider does not accept resource")
	}
	if strings.TrimSpace(req.Generation) == "" {
		return nil, domain.Errf(domain.CodeInvalidArgument, "generation is required (a fresh value each time the host starts)")
	}
	if len(req.Actions) == 0 || len(req.Actions) > 32 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "offer 1 to 32 actions")
	}
	for _, d := range req.Actions {
		if !strings.HasPrefix(d.Name, req.Provider+".") || d.Version < 1 || !contains(perms, d.Permission) {
			return nil, domain.Errf(domain.CodeInvalidArgument, "action %q (v%d, %s) is not one of %s's", d.Name, d.Version, d.Permission, req.Provider)
		}
		if len(d.InputSchema) > actionSchemaMax || len(d.OutputSchema) > actionSchemaMax {
			return nil, domain.Errf(domain.CodeInvalidArgument, "action %s's schema is too large", d.Name)
		}
	}
	now := s.clock.Now()
	h := &actionHost{owner: m.ID, actions: req.Actions}
	h.ActionHost = api.ActionHost{ID: s.ids.New("ahs"), MachineID: machineID, Resource: req.Resource, Provider: req.Provider, Generation: req.Generation, LastSeenAt: now}
	s.mu.Lock()
	for id, old := range s.hosts {
		// A computer has one host per provider: a new registration replaces
		// it. A person's page renderers are their Apps (on several
		// computers, or two Apps on one): each stays, and one of theirs is
		// replaced only once it has stopped polling. Replacing a live one
		// made two Apps take it from each other in a tight loop. Hosts long
		// gone are dropped whoever they belong to.
		gone := now.Sub(old.LastSeenAt) > actionHostGone
		same := old.Provider == h.Provider && old.MachineID == h.MachineID && sameResourceID(old.Resource, h.Resource) &&
			(machineID != "" || (old.owner == h.owner && now.Sub(old.LastSeenAt) > actionHostStale))
		if !gone && !same {
			continue
		}
		s.failRunning(old.ID, "host_restarted", "the computer's App restarted while it ran: whether it happened is unknown; observe before going on")
		delete(s.hosts, id)
		if !same {
			continue // its grants find out in refresh
		}
		for _, g := range s.grants {
			if same && !sameResource(old.Resource, h.Resource) && g.Target.ID == h.MachineID && sameResourceID(g.Target.Resource, old.Resource) {
				s.close(g, api.GrantRevoked, "SSH target binding changed")
			} else if g.hostSeen == old.ID {
				g.hostSeen = h.ID
			}
		}
	}
	s.hosts[h.ID] = h
	s.changed()
	out := h.ActionHost
	s.mu.Unlock()
	s.audit.Record(ctx, by, "action.host.register", where, req.Provider)
	return &out, nil
}

// failRunning fails a host's running jobs as outcome unknown (mu held).
func (s *ActionService) failRunning(hostID, code, msg string) {
	now := s.clock.Now()
	for _, j := range s.jobs {
		if j.hostID == hostID && j.State == api.JobRunning {
			j.State, j.FinishedAt = api.JobFailed, &now
			j.Error = &api.ActionError{Code: code, Message: msg}
		}
	}
}

// hostsFor are the live hosts that may run a grant's jobs (mu held): its
// computer's, or for a page, page renderers whose person may see it.
func (s *ActionService) hostsFor(ctx context.Context, g *actionGrant) []*actionHost {
	var out []*actionHost
	now := s.clock.Now()
	for _, h := range s.hosts {
		if now.Sub(h.LastSeenAt) <= actionHostStale && s.hostServes(ctx, h, g) {
			out = append(out, h)
		}
	}
	return out
}

// hostServes reports whether host h may run grant g's jobs (mu held).
func (s *ActionService) hostServes(ctx context.Context, h *actionHost, g *actionGrant) bool {
	if providerTarget[h.Provider] != g.Target.Kind {
		return false
	}
	if g.Target.Kind == "page" {
		return s.memberSees(ctx, h.owner, g.Target.ID)
	}
	return h.MachineID == g.Target.ID && sameResource(h.Resource, g.Target.Resource)
}

func (s *ActionService) hostOf(by Actor, id, generation string) (*actionHost, error) {
	h := s.hosts[id]
	if h == nil || (by.Member != nil && h.owner != by.Member.ID && (!by.Manages() || h.Provider == "server.ssh")) {
		return nil, domain.Errf(domain.CodeNotFound, "host %s not found", id).WithSuggestion("register it again")
	}
	if generation != "" && generation != h.Generation {
		return nil, domain.Errf(domain.CodeConflict, "host %s is now generation %s (this is %s)", id, h.Generation, generation)
	}
	return h, nil
}

// HostJobs hands a host the queued jobs for its computer and provider (now
// running), waiting up to wait for one.
func (s *ActionService) HostJobs(ctx context.Context, by Actor, hostID string, wait time.Duration) ([]api.ActionHostJob, error) {
	if _, err := person(by, "host agent actions"); err != nil {
		return nil, err
	}
	return waitFor(ctx, s, wait, func() ([]api.ActionHostJob, bool, error) {
		h, err := s.hostOf(by, hostID, "")
		if err != nil {
			return nil, true, err
		}
		now := s.clock.Now()
		h.LastSeenAt = now
		out := []api.ActionHostJob{}
		ids := make([]string, 0, len(s.jobs))
		for id := range s.jobs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			j := s.jobs[id]
			if j.State != api.JobQueued || providerOf(j.Action) != h.Provider {
				continue
			}
			g := s.grants[j.GrantID]
			if g == nil || !s.hostServes(ctx, h, g) {
				continue
			}
			s.refresh(ctx, g)
			if g.State != api.GrantActive {
				continue
			}
			j.State, j.hostID = api.JobRunning, h.ID
			g.hostSeen = h.ID
			out = append(out, api.ActionHostJob{Job: j.ActionJob, Args: j.args, Grant: g.ActionGrant})
			if len(out) >= 8 {
				break
			}
		}
		if len(out) > 0 {
			s.changed()
		}
		return out, len(out) > 0, nil
	})
}

// HostJob is how a job handed to a host stands (GET
// /v1/console/action-jobs/{id}?host_id&generation): Stop says to stop now
// (the agent cancelled it, its grant ended, it is no longer running). With
// wait, it answers when Stop turns true, or after wait.
func (s *ActionService) HostJob(ctx context.Context, by Actor, jobID, hostID, generation string, wait time.Duration) (*api.ActionHostJobState, error) {
	if _, err := person(by, "host agent actions"); err != nil {
		return nil, err
	}
	return waitFor(ctx, s, wait, func() (*api.ActionHostJobState, bool, error) {
		h, err := s.hostOf(by, hostID, generation)
		if err != nil {
			return nil, true, err
		}
		j := s.jobs[jobID]
		if j == nil || j.hostID != h.ID {
			return nil, true, domain.Errf(domain.CodeNotFound, "job %s not found on this host", jobID)
		}
		// A host checking a running job is still alive, including while a batch
		// of bounded SSH jobs is being processed between queue polls.
		h.LastSeenAt = s.clock.Now()
		out := &api.ActionHostJobState{Job: j.ActionJob, GrantState: api.GrantEnded}
		if g := s.grants[j.GrantID]; g != nil {
			s.refresh(ctx, g)
			out.GrantState = g.State
			if g.ExpiresAt != nil {
				t := *g.ExpiresAt
				out.GrantExpiresAt = &t
			}
		}
		out.Stop = j.State != api.JobRunning || j.CancelRequested || out.GrantState != api.GrantActive
		return out, out.Stop, nil
	})
}

// UploadArtifact keeps a file a running job made, for its agent only.
func (s *ActionService) UploadArtifact(ctx context.Context, by Actor, jobID, hostID, generation, mediaType string, data []byte) (*api.ActionArtifact, error) {
	if _, err := person(by, "host agent actions"); err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > actionArtifactMax {
		return nil, domain.Errf(domain.CodeInvalidArgument, "an artifact is 1 byte to %d MB", actionArtifactMax>>20)
	}
	mediaType = strings.TrimSpace(strings.Split(mediaType, ";")[0])
	if !artifactTypeAllowed(mediaType) {
		return nil, domain.Errf(domain.CodeInvalidArgument, "artifact type %q is not allowed (images, PDF, Office documents, text/plain, text/csv, text/markdown, text/html, application/json)", mediaType)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := s.hostOf(by, hostID, generation)
	if err != nil {
		return nil, err
	}
	j := s.jobs[jobID]
	if j == nil || j.hostID != h.ID || j.State != api.JobRunning {
		return nil, domain.Errf(domain.CodeNotFound, "running job %s not found on this host", jobID)
	}
	g := s.grants[j.GrantID]
	if g == nil || g.State != api.GrantActive {
		return nil, domain.Errf(domain.CodeInvalidState, "its grant has ended")
	}
	exp := s.clock.Now().Add(actionArtifactTTL)
	if g.ExpiresAt != nil && g.ExpiresAt.Before(exp) {
		exp = *g.ExpiresAt
	}
	a := &actionArtifact{grantID: g.ID, jobID: j.ID, hostID: h.ID, data: data}
	a.ActionArtifact = api.ActionArtifact{ID: s.ids.New("aar"), MediaType: mediaType, Bytes: len(data), ExpiresAt: exp}
	s.arts[a.ID] = a
	out := a.ActionArtifact
	return &out, nil
}

// CompleteJob records how a running job ended. A job is final once ended;
// a late result (its grant ended meanwhile) is dropped.
func (s *ActionService) CompleteJob(ctx context.Context, by Actor, jobID string, c api.ActionJobComplete) (*api.ActionJob, error) {
	if _, err := person(by, "host agent actions"); err != nil {
		return nil, err
	}
	if !contains([]string{api.JobSucceeded, api.JobFailed, api.JobCancelled}, c.State) {
		return nil, domain.Errf(domain.CodeInvalidArgument, "state is succeeded, failed or cancelled")
	}
	if len(c.Result) > actionResultMax {
		return nil, domain.Errf(domain.CodeInvalidArgument, "the result is over %d KB", actionResultMax>>10)
	}
	if len(c.Result) > 0 && !json.Valid(c.Result) {
		return nil, domain.Errf(domain.CodeInvalidArgument, "the result is not JSON")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := s.hostOf(by, c.HostID, c.Generation)
	if err != nil {
		return nil, err
	}
	j := s.jobs[jobID]
	if j == nil || j.hostID != h.ID {
		return nil, domain.Errf(domain.CodeNotFound, "job %s not found on this host", jobID)
	}
	if j.State != api.JobRunning {
		return nil, domain.Errf(domain.CodeInvalidState, "job %s is %s already", jobID, j.State)
	}
	now := s.clock.Now()
	j.FinishedAt = &now
	g := s.grants[j.GrantID]
	if g != nil {
		s.refresh(ctx, g) // (a page's agent may no longer see it)
	}
	if g == nil || g.State != api.GrantActive {
		j.State = api.JobCancelled
		j.Error = &api.ActionError{Code: "grant_ended", Message: "its grant ended while it ran; whatever it did happened, but its result is not kept"}
		for id, a := range s.arts {
			if a.jobID == j.ID {
				delete(s.arts, id)
			}
		}
		s.changed()
		out := j.ActionJob
		return &out, nil
	}
	j.State, j.Result, j.Error = c.State, c.Result, c.Error
	for _, id := range c.ArtifactIDs {
		a := s.arts[id]
		if a == nil || a.jobID != j.ID {
			return nil, domain.Errf(domain.CodeInvalidArgument, "artifact %s is not this job's", id)
		}
		j.Artifacts = append(j.Artifacts, a.ActionArtifact)
	}
	s.changed()
	out := j.ActionJob
	return &out, nil
}

// ---- jobs (the agent) ----

// RunAction admits an action on a grant as a job for the target to carry
// out. The same idempotency key with the same body returns the first job.
func (s *ActionService) RunAction(ctx context.Context, agent *domain.Agent, inv api.ActionInvocation) (*api.ActionJob, error) {
	key := strings.TrimSpace(inv.IdempotencyKey)
	if key == "" || len(key) > 128 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "idempotency_key is required (up to 128 characters, unique per action within the grant)")
	}
	if len(inv.Args) > actionArgsMax {
		return nil, domain.Errf(domain.CodeInvalidArgument, "args are over %d KB", actionArgsMax>>10)
	}
	args := inv.Args
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage(`{}`)
	}
	if !json.Valid(args) {
		return nil, domain.Errf(domain.CodeInvalidArgument, "args are not JSON")
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", inv.Action, inv.Version, args)))
	hash := hex.EncodeToString(sum[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.grantOf(ctx, agent.Name, inv.GrantID)
	if err != nil {
		return nil, err
	}
	if id, ok := g.idem[key]; ok {
		if g.idemHash[key] != hash {
			return nil, domain.Errf(domain.CodeConflict, "idempotency key %q was used for a different action", key)
		}
		out := s.jobs[id].ActionJob
		return &out, nil
	}
	if g.State != api.GrantActive {
		return nil, domain.Errf(domain.CodeInvalidState, "grant %s is %s", g.ID, g.State).WithSuggestion("request a new grant")
	}
	var desc *api.ActionDescriptor
	for _, h := range s.hostsFor(ctx, g) {
		for i := range h.actions {
			if h.actions[i].Name == inv.Action {
				desc = &h.actions[i]
			}
		}
	}
	if desc == nil {
		return nil, domain.Errf(domain.CodeUnsupported, "%s cannot do %s now (its App is not running it, or is too old)", g.TargetName, inv.Action)
	}
	if desc.Version != inv.Version {
		return nil, domain.Errf(domain.CodeUnsupported, "%s offers %s version %d, not %d", g.TargetName, inv.Action, desc.Version, inv.Version)
	}
	if !contains(g.Permissions, desc.Permission) {
		return nil, domain.Errf(domain.CodePermissionDenied, "%s needs %s, which grant %s does not have", inv.Action, desc.Permission, g.ID)
	}
	if g.jobsCount >= actionJobsPerGrant {
		return nil, domain.Errf(domain.CodeRateLimited, "grant %s has run %d actions; request a new one", g.ID, actionJobsPerGrant)
	}
	j := &actionJob{agent: string(agent.Name), args: args}
	j.ActionJob = api.ActionJob{ID: s.ids.New("ajb"), GrantID: g.ID, Action: inv.Action, Version: inv.Version, State: api.JobQueued, CreatedAt: s.clock.Now()}
	s.jobs[j.ID] = j
	g.idem[key], g.idemHash[key] = j.ID, hash
	g.jobsCount++
	s.changed()
	out := j.ActionJob
	return &out, nil
}

// Job returns one of agent's jobs; with wait, until it ends.
func (s *ActionService) Job(ctx context.Context, agent *domain.Agent, id string, wait time.Duration) (*api.ActionJob, error) {
	return waitFor(ctx, s, wait, func() (*api.ActionJob, bool, error) {
		j := s.jobs[id]
		if j == nil || j.agent != string(agent.Name) {
			return nil, true, domain.Errf(domain.CodeNotFound, "job %s not found", id)
		}
		if g := s.grants[j.GrantID]; g != nil {
			s.refresh(ctx, g)
		}
		out := j.ActionJob
		return &out, j.State != api.JobQueued && j.State != api.JobRunning, nil
	})
}

// CancelJob cancels a queued job; a running one is asked to stop (what it
// already did stays done).
func (s *ActionService) CancelJob(ctx context.Context, agent *domain.Agent, id string) (*api.ActionJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.jobs[id]
	if j == nil || j.agent != string(agent.Name) {
		return nil, domain.Errf(domain.CodeNotFound, "job %s not found", id)
	}
	switch j.State {
	case api.JobQueued:
		now := s.clock.Now()
		j.State, j.FinishedAt = api.JobCancelled, &now
	case api.JobRunning:
		j.CancelRequested = true
	}
	s.changed()
	out := j.ActionJob
	return &out, nil
}

// Artifact returns a file one of agent's jobs made, while its grant lasts.
func (s *ActionService) Artifact(ctx context.Context, agent *domain.Agent, id string) (*api.ActionArtifact, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.arts[id]
	if a == nil {
		return nil, nil, domain.Errf(domain.CodeNotFound, "artifact %s not found (or no longer kept)", id)
	}
	g := s.grants[a.grantID]
	if g == nil || g.Agent != string(agent.Name) {
		return nil, nil, domain.Errf(domain.CodeNotFound, "artifact %s not found (or no longer kept)", id)
	}
	s.refresh(ctx, g)
	if _, still := s.arts[id]; !still || !s.clock.Now().Before(a.ExpiresAt) {
		delete(s.arts, id)
		return nil, nil, domain.Errf(domain.CodeNotFound, "artifact %s is no longer kept", id)
	}
	out := a.ActionArtifact
	return &out, a.data, nil
}
