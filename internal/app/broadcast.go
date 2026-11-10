package app

import (
	"context"

	"github.com/chenhg5/myco/internal/domain"
)

// MaxBroadcastRecipients bounds one broadcast.
const MaxBroadcastRecipients = 200

// BroadcastInput sends one message to every agent matching Selector.
type BroadcastInput struct {
	Selector       string
	Type           string
	Body           string
	IdempotencyKey string
	// IncludeSelf also delivers to the sender if it matches.
	IncludeSelf bool
	// Attachments are file ids shared with every recipient.
	Attachments []string
	DryRun      bool
}

// Skipped is a matching agent that did not get the message.
type Skipped struct {
	Agent  domain.AgentName
	Reason string
}

// BroadcastOutput lists one message per recipient (each its own thread) and
// the agents that were skipped (e.g. allow-from).
type BroadcastOutput struct {
	ID        string
	Messages  []*domain.Message
	Skipped   []Skipped
	Duplicate bool
}

// Broadcast fans a task or note out to the agents matching a selector.
func (s *MessageService) Broadcast(ctx context.Context, sender *domain.Agent, in BroadcastInput) (*BroadcastOutput, error) {
	sel, err := domain.ParseSelector(in.Selector)
	if err != nil {
		return nil, err
	}
	if sel.Empty() {
		return nil, domain.Errf(domain.CodeInvalidArgument, "broadcast needs a selector").
			WithSuggestion(`pass --selector, e.g. "group=loops" (preview recipients with --dry-run)`)
	}
	typ, err := domain.ParseSendableType(in.Type)
	if err != nil {
		return nil, err
	}
	all, err := s.agents.List(ctx)
	if err != nil {
		return nil, err
	}
	out := &BroadcastOutput{ID: s.ids.New("bc")}
	var recipients []*domain.Agent
	for _, a := range all {
		switch {
		case !sel.Matches(a):
		case a.Name == sender.Name && !in.IncludeSelf:
		case !domain.MaySendTo(sender, a): // private to someone else: not even listed
		case !a.Accepts(sender.Name):
			out.Skipped = append(out.Skipped, Skipped{a.Name, "does not accept messages from " + string(sender.Name)})
		default:
			recipients = append(recipients, a)
		}
	}
	if len(recipients) > MaxBroadcastRecipients {
		return nil, domain.Errf(domain.CodeInvalidArgument, "selector matches %d agents (max %d)", len(recipients), MaxBroadcastRecipients).
			WithSuggestion("narrow the selector")
	}
	if len(recipients) == 0 {
		return nil, domain.Errf(domain.CodeNotFound, "no agent matches %q", in.Selector).
			WithSuggestion("check labels with: myco agent list --selector %q", in.Selector)
	}
	if in.IdempotencyKey != "" {
		// A retried broadcast returns the original one.
		key := "bc:" + in.IdempotencyKey + ":" + string(recipients[0].Name)
		if prev, err := s.messages.FindByIdempotencyKey(ctx, sender.Name, key); err == nil && prev.BroadcastID != "" {
			msgs, err := s.messages.ListBroadcast(ctx, prev.BroadcastID)
			return &BroadcastOutput{ID: prev.BroadcastID, Messages: msgs, Duplicate: true}, err
		}
	}
	atts, err := resolveAttachments(ctx, s.files, s.clock, sender.Name, in.Attachments)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	for _, a := range recipients {
		key := ""
		if in.IdempotencyKey != "" {
			key = "bc:" + in.IdempotencyKey + ":" + string(a.Name)
		}
		m, err := domain.NewMessage(s.ids.New("msg"), sender.Name, a.Name, typ, in.Body, "", "", key, now)
		if err != nil {
			return nil, err
		}
		m.BroadcastID = out.ID
		m.Attachments = atts
		m.FromOwner = s.ownerName(ctx, sender)
		out.Messages = append(out.Messages, m)
	}
	if in.DryRun {
		return out, nil
	}
	if s.files != nil {
		for _, a := range recipients {
			if err := grantAttachments(ctx, s.files, atts, a.Name); err != nil {
				return nil, err
			}
		}
	}
	if err := s.messages.CreateMany(ctx, out.Messages); err != nil {
		return nil, err
	}
	for _, m := range out.Messages {
		s.queued(m.To)
	}
	return out, nil
}

// BroadcastDelivery is one recipient's message and (if any) its answer.
type BroadcastDelivery struct {
	Message *domain.Message
	Reply   *domain.Message
}

// BroadcastStatus returns a broadcast's deliveries; only its sender may read
// it. With consume, returned replies are marked done (like WaitReply) so they
// are not delivered again through the sender's inbox.
func (s *MessageService) BroadcastStatus(ctx context.Context, agent *domain.Agent, id string, consume bool) ([]BroadcastDelivery, error) {
	msgs, err := s.messages.ListBroadcast(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 || msgs[0].From != agent.Name {
		return nil, domain.Errf(domain.CodeNotFound, "broadcast %s not found", id)
	}
	out := make([]BroadcastDelivery, 0, len(msgs))
	for _, m := range msgs {
		d := BroadcastDelivery{Message: m}
		if r, err := s.messages.FindReply(ctx, m.ID); err == nil {
			if consume {
				if c, err := s.consume(ctx, agent, r); err == nil {
					r = c
				}
			}
			d.Reply = r
		}
		out = append(out, d)
	}
	return out, nil
}
