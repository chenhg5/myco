package app

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// AdminSender is the sender of messages the hub sends on an administrator's
// behalf (starting a session on a machine from the console). Agent names
// cannot contain "@", so no agent can send as it.
const AdminSender domain.AgentName = "@admin"

// MachineService tracks the machines agents run on, from what their
// runners report, and starts sessions on them for administrators.
type MachineService struct {
	machines     domain.MachineRepository
	agents       domain.AgentRepository
	agentService *AgentService
	enroll       *EnrollmentService
	messages     *MessageService
	ids          IDGenerator
	clock        Clock
	window       time.Duration
}

// SetRemovalDeps wires the existing agent cleanup and enrollment revocation paths.
func (s *MachineService) SetRemovalDeps(agents *AgentService, enroll *EnrollmentService) {
	s.agentService, s.enroll = agents, enroll
}

// Remove revokes the machine enrollment before deleting its nodes, then the
// machine record. Retrying after a partial failure is safe.
func (s *MachineService) Remove(ctx context.Context, id string, dry bool) (*domain.Machine, []string, error) {
	m, err := s.machines.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	all, err := s.agents.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	enrollments, err := s.enroll.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	byID := map[string]*domain.Enrollment{}
	for _, e := range enrollments {
		byID[e.ID] = e
	}
	ownedEnrollment := map[string]bool{}
	for _, a := range all {
		if a.MachineID == id {
			if e := byID[a.EnrolledBy]; e != nil && e.Kind == domain.EnrollmentMachine {
				ownedEnrollment[e.ID] = true
			}
		}
	}
	// A machine whose node was already removed can still have its enrollment.
	if len(ownedEnrollment) == 0 {
		linkedElsewhere := map[string]bool{}
		for _, a := range all {
			if a.MachineID != "" && a.MachineID != id {
				linkedElsewhere[a.EnrolledBy] = true
			}
		}
		for _, e := range enrollments {
			if e.Kind == domain.EnrollmentMachine && e.Owner == m.Owner && !linkedElsewhere[e.ID] &&
				(e.Machine == m.Name || e.Machine == hostKey(m.Hostname)) {
				ownedEnrollment[e.ID] = true
			}
		}
	}
	names := make([]string, 0)
	for _, a := range all {
		if a.MachineID == id || ownedEnrollment[a.EnrolledBy] {
			names = append(names, string(a.Name))
		}
	}
	sort.Strings(names)
	if dry {
		return m, names, nil
	}
	for eid := range ownedEnrollment {
		if _, err := s.enroll.Revoke(ctx, eid, false); err != nil {
			return nil, nil, err
		}
	}
	for _, name := range names {
		if _, err := s.agentService.Remove(ctx, name, false); err != nil {
			return nil, nil, err
		}
	}
	if err := s.machines.Delete(ctx, id); err != nil {
		return nil, nil, err
	}
	return m, names, nil
}

// NewMachineService wires the machine use cases.
func NewMachineService(machines domain.MachineRepository, agents domain.AgentRepository, messages *MessageService, ids IDGenerator, clock Clock, onlineWindow time.Duration) *MachineService {
	return &MachineService{machines: machines, agents: agents, messages: messages, ids: ids, clock: clock, window: onlineWindow}
}

// Report records the machine an agent's runner runs on: the machine with
// that fingerprint, created on first report, and links the agent to it.
func (s *MachineService) Report(ctx context.Context, a *domain.Agent, r domain.MachineReport) (*domain.Machine, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	if a.InCloud() {
		return nil, domain.Errf(domain.CodeInvalidArgument, "cloud sessions are not on a machine")
	}
	now := s.clock.Now()
	m, err := s.machines.GetByFingerprint(ctx, r.Fingerprint)
	switch {
	case domain.IsCode(err, domain.CodeNotFound):
		m = &domain.Machine{ID: s.ids.New("mch"), FingerprintHash: r.Fingerprint, Name: defaultMachineName(r),
			Hostname: r.Hostname, OS: r.OS, Arch: r.Arch, MycoVersion: r.MycoVersion, CreatedAt: now, LastSeenAt: now, Owner: a.Owner}
		if err := s.machines.Create(ctx, m); err != nil {
			if !domain.IsCode(err, domain.CodeConflict) {
				return nil, err
			}
			// Another of its agents reported it first.
			if m, err = s.machines.GetByFingerprint(ctx, r.Fingerprint); err != nil {
				return nil, err
			}
		}
	case err != nil:
		return nil, err
	default:
		m.Hostname, m.OS, m.Arch, m.MycoVersion, m.LastSeenAt = r.Hostname, r.OS, r.Arch, r.MycoVersion, now
		if m.Name == "" {
			m.Name = defaultMachineName(r)
		}
		if m.Owner == "" {
			m.Owner = a.Owner
		}
		if err := s.machines.Save(ctx, m); err != nil {
			return nil, err
		}
	}
	if a.MachineID != m.ID {
		if err := s.agents.SetMachine(ctx, a.Name, m.ID); err != nil {
			return nil, err
		}
		a.MachineID = m.ID
	}
	return m, nil
}

func defaultMachineName(r domain.MachineReport) string {
	if n := strings.TrimSpace(r.Name); n != "" {
		return n
	}
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(r.Hostname), ".local"))
}

// SetRuntime records an agent's CLI and model as its runner reports them.
func (s *MachineService) SetRuntime(ctx context.Context, a *domain.Agent, rt domain.Runtime) error {
	if err := rt.Validate(); err != nil {
		return err
	}
	a.Runtime = rt
	return s.agents.SetRuntime(ctx, a.Name, rt)
}

// MachineView is a machine with its agents, for lists and the overview.
// Machines whose agents' runners have not reported yet (older myco) are
// grouped by the agents' environment (host name) and marked Legacy.
type MachineView struct {
	domain.Machine
	Legacy bool
	// Agents are the names of the nodes on it (machine nodes excluded).
	Agents []domain.AgentName
	Online bool
	// SpawnNode is its online machine node ("" = cannot start sessions).
	SpawnNode domain.AgentName
}

// List returns the machines with their agents, and which machine each
// agent is on (by agent name).
func (s *MachineService) List(ctx context.Context) ([]*MachineView, map[domain.AgentName]string, error) {
	machines, err := s.machines.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	agents, err := s.agents.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	now := s.clock.Now()
	views := map[string]*MachineView{}
	var order []string
	byHost := map[string]string{} // lower host name -> machine id
	for _, m := range machines {
		views[m.ID] = &MachineView{Machine: *m}
		order = append(order, m.ID)
		if h := hostKey(m.Hostname); h != "" {
			byHost[h] = m.ID
		}
	}
	where := map[domain.AgentName]string{}
	for _, a := range agents {
		if a.Kind() == domain.KindHuman || a.InCloud() {
			continue // people and cloud sandboxes are not on machines
		}
		id := a.MachineID
		if _, ok := views[id]; !ok || id == "" {
			// Not reported yet: the machine with that host name, or a
			// legacy machine for the environment.
			env := strings.TrimSpace(a.Binding.Environment)
			if env == "" {
				continue
			}
			if mid, ok := byHost[hostKey(env)]; ok {
				id = mid
			} else {
				id = "env:" + hostKey(env)
				if _, ok := views[id]; !ok {
					views[id] = &MachineView{Legacy: true, Machine: domain.Machine{ID: id, Name: env, Hostname: env}}
					order = append(order, id)
				}
			}
		}
		v := views[id]
		online := a.IsOnline(now, s.window)
		if online {
			v.Online = true
			if a.LastSeenAt.After(v.LastSeenAt) {
				v.LastSeenAt = a.LastSeenAt
			}
		}
		if a.Kind() == domain.KindMachine {
			if online {
				v.SpawnNode = a.Name
			}
			continue
		}
		v.Agents = append(v.Agents, a.Name)
		where[a.Name] = id
	}
	out := make([]*MachineView, 0, len(order))
	for _, id := range order {
		v := views[id]
		if v.Legacy && len(v.Agents) == 0 && v.SpawnNode == "" {
			continue
		}
		sort.Slice(v.Agents, func(i, j int) bool { return v.Agents[i] < v.Agents[j] })
		out = append(out, v)
	}
	return out, where, nil
}

func hostKey(h string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(h), ".local"))
}

// Rename changes a machine's display name.
func (s *MachineService) Rename(ctx context.Context, id, name string) (*domain.Machine, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 || strings.ContainsAny(name, "\x00\n\r") {
		return nil, domain.Errf(domain.CodeInvalidArgument, "machine name must be 1-64 characters on one line")
	}
	if strings.HasPrefix(id, "env:") {
		return nil, domain.Errf(domain.CodeInvalidState, "this machine's myco is too old to be identified").
			WithSuggestion("upgrade myco on it (myco upgrade) and restart its runner; it then shows up as a real machine")
	}
	m, err := s.machines.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	m.Name = name
	return m, s.machines.Save(ctx, m)
}

// Spawn starts a session on machine id for an administrator: the request
// goes to the machine's node from AdminSender (not subject to the node's
// allow_from; the machine's own directory and access rules still apply),
// and Spawn waits for the answer.
func (s *MachineService) Spawn(ctx context.Context, id string, body []byte, wait time.Duration) (*domain.Message, error) {
	node, err := s.SpawnNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if !json.Valid(body) {
		return nil, domain.Errf(domain.CodeInvalidArgument, "spawn request must be JSON")
	}
	admin := &domain.Agent{Name: AdminSender}
	out, err := s.messages.send(ctx, admin, SendInput{To: string(node), Type: string(domain.TypeTask), Body: string(body)}, true)
	if err != nil {
		return nil, err
	}
	return s.messages.WaitReply(ctx, admin, out.Message.ID, wait)
}

// SpawnNode returns the machine node that starts sessions on machine id.
func (s *MachineService) SpawnNode(ctx context.Context, id string) (domain.AgentName, error) {
	views, _, err := s.List(ctx)
	if err != nil {
		return "", err
	}
	var node domain.AgentName
	found := false
	for _, v := range views {
		if v.ID == id {
			found, node = true, v.SpawnNode
		}
	}
	if !found {
		return "", domain.Errf(domain.CodeNotFound, "machine %s not found", id)
	}
	if node == "" {
		return "", domain.Errf(domain.CodeInvalidState, "machine %s cannot start sessions now", id).
			WithSuggestion("its myco must be v0.16.5 or newer, joined with a link, and its runner running (myco runner service status)")
	}
	return node, nil
}

// Get returns a machine by id.
func (s *MachineService) Get(ctx context.Context, id string) (*domain.Machine, error) {
	return s.machines.Get(ctx, id)
}

// SetOwner gives a machine to a member.
func (s *MachineService) SetOwner(ctx context.Context, id, memberID string) (*domain.Machine, error) {
	m, err := s.machines.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	m.Owner = memberID
	return m, s.machines.Save(ctx, m)
}
