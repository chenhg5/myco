package app

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// ConsoleSenderPrefix marks messages the hub's administrator sends from the
// console app: "@console:<device>". Agent names cannot contain "@", so no
// agent can send as one.
const ConsoleSenderPrefix = "@console:"

// ConsoleSender is the sender for a console (device) name.
func ConsoleSender(device string) domain.AgentName {
	device = strings.TrimSpace(device)
	if device == "" {
		device = "admin"
	}
	return domain.AgentName(ConsoleSenderPrefix + device)
}

// IsConsoleSender reports whether name is a console sender.
func IsConsoleSender(name domain.AgentName) bool {
	return strings.HasPrefix(string(name), ConsoleSenderPrefix)
}

// SendFromConsole sends a message as the administrator using the console:
// the owner of the network, so the recipient's allow_from (which keeps
// other agents out) does not apply. Exec nodes still check their senders
// on their own machine.
func (s *MessageService) SendFromConsole(ctx context.Context, device string, in SendInput) (*SendOutput, error) {
	return s.SendFromMember(ctx, device, "", in)
}

// SendFromMember sends as a member writing in the app ("@console:<name>").
func (s *MessageService) SendFromMember(ctx context.Context, name, memberID string, in SendInput) (*SendOutput, error) {
	return s.send(ctx, &domain.Agent{Name: ConsoleSender(name), Owner: memberID}, in, true)
}

// SteerFromConsole corrects a task sent from the app: by the member who sent
// it (any of their devices), or by someone who manages the hub.
func (s *MessageService) SteerFromConsole(ctx context.Context, name string, manages bool, id, body string) (*domain.Message, error) {
	m, err := s.messages.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !IsConsoleSender(m.From) {
		return nil, domain.Errf(domain.CodePermissionDenied, "only tasks sent from the console can be corrected here")
	}
	if !manages && m.From != ConsoleSender(name) {
		return nil, domain.Errf(domain.CodePermissionDenied, "only the person who sent this task can correct it")
	}
	// Any of the administrator's devices may correct the conversation.
	return s.Steer(ctx, &domain.Agent{Name: m.From}, id, body, false)
}

// DismissFromConsole closes an open message on its recipient's behalf
// without replying (a stale answer nobody will pull, a task that no longer
// matters). allowed decides whether the caller may act for the recipient.
func (s *MessageService) DismissFromConsole(ctx context.Context, id string, allowed func(to domain.AgentName) error) (*domain.Message, error) {
	m, err := s.messages.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := allowed(m.To); err != nil {
		return nil, err
	}
	return s.Transition(ctx, &domain.Agent{Name: m.To}, id, TransitionComplete, false)
}

// Scope is what someone reading from the console may see: the whole hub, or
// only the threads and exchanges one of their own names takes part in.
type Scope struct {
	// All: the whole hub (the admin token, owners and admins, hubs without
	// members).
	All bool
	// Names are the reader's own: their console sender and the nodes they
	// own. Without All, only what one of these takes part in is readable.
	Names []domain.AgentName
}

// involve is the RecentFilter.Involve for the scope.
func (sc Scope) involve() []domain.AgentName {
	if sc.All {
		return nil
	}
	if sc.Names == nil {
		return []domain.AgentName{}
	}
	return sc.Names
}

// Allows reports whether something with these participants is readable.
func (sc Scope) Allows(names ...domain.AgentName) bool {
	if sc.All {
		return true
	}
	for _, n := range names {
		if slices.Contains(sc.Names, n) {
			return true
		}
	}
	return false
}

// Readable reports whether a list of messages (a thread) is readable.
func (sc Scope) Readable(msgs []*domain.Message) bool {
	if sc.All {
		return true
	}
	for _, m := range msgs {
		if sc.Allows(m.From, m.To) {
			return true
		}
	}
	return false
}

// Conversation is a thread the console takes part in.
type Conversation struct {
	ThreadID  string
	UpdatedAt time.Time
	// Agent is the other party (the first non-console participant).
	Agent    domain.AgentName
	First    *domain.Message
	Last     *domain.Message
	Messages int
}

// Conversations lists the console's threads the scope may read, newest
// first; agent and query narrow them, before (unix ms) pages back.
func (s *MessageService) Conversations(ctx context.Context, sc Scope, agent, query string, before int64, limit int) ([]Conversation, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	f := domain.RecentFilter{SenderPrefix: ConsoleSenderPrefix, Involve: sc.involve(), Query: strings.TrimSpace(query), Before: before, Limit: limit}
	if agent != "" {
		n, err := domain.ParseAgentName(agent)
		if err != nil {
			return nil, err
		}
		f.Match = []domain.AgentName{n}
	}
	refs, err := s.messages.ListRecentThreads(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]Conversation, 0, len(refs))
	for _, ref := range refs {
		msgs, err := s.messages.ListThread(ctx, ref.ID)
		if err != nil || len(msgs) == 0 {
			continue
		}
		c := Conversation{ThreadID: ref.ID, UpdatedAt: ref.UpdatedAt, First: msgs[0], Last: msgs[len(msgs)-1], Messages: len(msgs)}
		for _, m := range msgs {
			for _, n := range []domain.AgentName{m.From, m.To} {
				if c.Agent == "" && !IsConsoleSender(n) && n != AdminSender {
					c.Agent = n
				}
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// RecentInput asks for the recent list: threads, and exchanges with shell
// nodes per (other party, node).
type RecentInput struct {
	Scope Scope
	// Mine keeps only what the reader's own names take part in, even when
	// they may read the whole hub.
	Mine bool
	// Label keeps what a node with the label takes part in: "group:<name>",
	// "group:" (no group) or "tag:<name>" (see domain.MatchesLabelFilter).
	Label string
	Query string
	// Before (unix ms, 0 = now) pages back.
	Before int64
	Limit  int
}

// RecentItem is one row of the recent list.
type RecentItem struct {
	// Kind is "thread" or "pair" (the exchange between Other and the shell
	// node Node).
	Kind     string
	ThreadID string
	Other    domain.AgentName
	Node     domain.AgentName
	// Participants in order of first appearance (threads).
	Participants []domain.AgentName
	UpdatedAt    time.Time
	Messages     int
	First, Last  *domain.Message
	// Task is the newest task, Answer its reply or failure event (if any).
	Task, Answer *domain.Message
	// State: "queued", "working", "done", "failed", or "" when there is no
	// task. A shell command that exited non-zero is "done" here: its exit
	// code is in Answer's body (possibly encrypted).
	State string
}

// Recent lists the threads and shell-node exchanges the input selects,
// newest first; more reports whether older rows exist.
func (s *MessageService) Recent(ctx context.Context, agents []*domain.Agent, in RecentInput) (items []RecentItem, more bool, err error) {
	limit := in.Limit
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	f := domain.RecentFilter{Involve: in.Scope.involve(), Query: strings.TrimSpace(in.Query), Before: in.Before, Limit: limit + 1}
	if in.Mine && in.Scope.Names != nil {
		f.Involve = Scope{Names: in.Scope.Names}.involve()
	}
	if in.Label != "" && in.Label != "all" {
		f.Match = []domain.AgentName{}
	}
	for _, a := range agents {
		switch a.Binding.Harness {
		case "exec":
			f.Nodes = append(f.Nodes, a.Name)
		case "machine":
			f.Skip = append(f.Skip, a.Name)
		}
		if f.Match != nil && domain.MatchesLabelFilter(a, in.Label) {
			f.Match = append(f.Match, a.Name)
		}
	}
	threads, err := s.messages.ListRecentThreads(ctx, f)
	if err != nil {
		return nil, false, err
	}
	pairs, err := s.messages.ListRecentPairs(ctx, f)
	if err != nil {
		return nil, false, err
	}
	for _, t := range threads {
		msgs, err := s.messages.ListThread(ctx, t.ID)
		if err != nil || len(msgs) == 0 {
			continue
		}
		items = append(items, threadItem(t, msgs))
	}
	for _, p := range pairs {
		msgs, err := s.messages.ListBetween(ctx, p.Other, p.Node, 0, 20)
		if err != nil || len(msgs) == 0 {
			continue
		}
		items = append(items, pairItem(p, msgs))
	}
	slices.SortStableFunc(items, func(a, b RecentItem) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	if len(items) > limit {
		items, more = items[:limit], true
	}
	return items, more, nil
}

// threadItem summarizes a thread (msgs oldest first).
func threadItem(t domain.ThreadRef, msgs []*domain.Message) RecentItem {
	it := RecentItem{Kind: "thread", ThreadID: t.ID, UpdatedAt: t.UpdatedAt, Messages: len(msgs), First: msgs[0], Last: msgs[len(msgs)-1]}
	for _, m := range msgs {
		for _, n := range []domain.AgentName{m.From, m.To} {
			if n != AdminSender && !slices.Contains(it.Participants, n) {
				it.Participants = append(it.Participants, n)
			}
		}
	}
	var open *domain.Message
	for _, m := range msgs {
		if m.Type != domain.TypeTask {
			continue
		}
		it.Task = m
		if !m.Status.Terminal() {
			open = m
		}
	}
	if it.Task != nil {
		for _, m := range msgs {
			if m.ReplyTo == it.Task.ID && (m.Type == domain.TypeReply || m.Type == domain.TypeEvent) {
				it.Answer = m
			}
		}
	}
	it.State = taskState(it.Task, it.Answer, open)
	return it
}

// pairItem summarizes an exchange with a shell node (msgs newest first).
func pairItem(p domain.PairRef, msgs []*domain.Message) RecentItem {
	it := RecentItem{Kind: "pair", ThreadID: msgs[0].ThreadID, Other: p.Other, Node: p.Node, Participants: []domain.AgentName{p.Other, p.Node},
		UpdatedAt: p.UpdatedAt, Messages: p.Messages, First: msgs[len(msgs)-1], Last: msgs[0]}
	var open *domain.Message
	for _, m := range msgs {
		if m.Type == domain.TypeTask && m.To == p.Node {
			if it.Task == nil {
				it.Task = m
			}
			if !m.Status.Terminal() {
				open = m
			}
		}
	}
	if it.Task != nil {
		for _, m := range msgs {
			if m.ReplyTo == it.Task.ID && (m.Type == domain.TypeReply || m.Type == domain.TypeEvent) {
				it.Answer = m
				break
			}
		}
	}
	it.State = taskState(it.Task, it.Answer, open)
	return it
}

// taskState is the row's state from its newest task and any still open.
func taskState(task, answer, open *domain.Message) string {
	switch {
	case open != nil && open.Status == domain.StatusQueued:
		return "queued"
	case open != nil:
		return "working"
	case task == nil:
		return ""
	case task.Status == domain.StatusFailed || (answer != nil && answer.Type == domain.TypeEvent):
		return "failed"
	}
	return "done"
}

// Peers lists the nodes agent exchanged messages with (not the console),
// most recent first; only the exchanges the scope may read.
func (s *MessageService) Peers(ctx context.Context, sc Scope, agent domain.AgentName, limit int) ([]domain.PeerRef, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	ps, err := s.messages.ListPeers(ctx, agent, limit)
	if err != nil || sc.Allows(agent) {
		return ps, err
	}
	out := ps[:0]
	for _, p := range ps {
		if sc.Allows(p.Agent) {
			out = append(out, p)
		}
	}
	return out, nil
}

// Between pages through the messages between nodes a and b (either
// direction), newest first; before is unix ms (0 = now).
func (s *MessageService) Between(ctx context.Context, sc Scope, a, b domain.AgentName, before int64, limit int) ([]*domain.Message, error) {
	if a == "" || b == "" {
		return nil, domain.Errf(domain.CodeInvalidArgument, "name both nodes (a and b)")
	}
	if !sc.Allows(a, b) {
		return nil, domain.Errf(domain.CodePermissionDenied, "you can only read exchanges your own nodes take part in")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.messages.ListBetween(ctx, a, b, before, limit)
}
