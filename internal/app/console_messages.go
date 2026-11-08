package app

import (
	"context"
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

// Conversations lists the console's threads, newest first; agent and query
// narrow them, before (unix ms) pages back.
func (s *MessageService) Conversations(ctx context.Context, agent, query string, before int64, limit int) ([]Conversation, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var who domain.AgentName
	if agent != "" {
		n, err := domain.ParseAgentName(agent)
		if err != nil {
			return nil, err
		}
		who = n
	}
	refs, err := s.messages.ListThreads(ctx, ConsoleSenderPrefix, who, strings.TrimSpace(query), before, limit)
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

// Peers lists the nodes agent exchanged messages with (not the console),
// most recent first.
func (s *MessageService) Peers(ctx context.Context, agent domain.AgentName, limit int) ([]domain.PeerRef, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	return s.messages.ListPeers(ctx, agent, limit)
}

// Between pages through the messages between nodes a and b (either
// direction), newest first; before is unix ms (0 = now).
func (s *MessageService) Between(ctx context.Context, a, b domain.AgentName, before int64, limit int) ([]*domain.Message, error) {
	if a == "" || b == "" {
		return nil, domain.Errf(domain.CodeInvalidArgument, "name both nodes (a and b)")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.messages.ListBetween(ctx, a, b, before, limit)
}
