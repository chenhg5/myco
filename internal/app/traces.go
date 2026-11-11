package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// TraceService keeps the steps of the runs that handled tasks (what the
// agent said, thought and did), uploaded by the recipient's runner, for the
// people concerned to follow in the app. Traces are not messages: they are
// never delivered to agents, and they are dropped after TraceRetention.
//
// Who may read one: the task's sender and its recipient, and the person who
// owns the recipient node. Hub owners and admins do not, unless they are one
// of those.
type TraceService struct {
	traces   domain.TraceRepository
	messages domain.MessageRepository
	agents   domain.AgentRepository
	clock    Clock
	log      *slog.Logger
}

// NewTraceService wires trace use cases.
func NewTraceService(traces domain.TraceRepository, messages domain.MessageRepository, agents domain.AgentRepository, clock Clock, log *slog.Logger) *TraceService {
	if log == nil {
		log = slog.Default()
	}
	return &TraceService{traces: traces, messages: messages, agents: agents, clock: clock, log: log}
}

// TraceInput is one uploaded step.
type TraceInput struct {
	Seq  int
	Kind string
	Data string
}

// Append stores steps of the run handling task msgID, uploaded by its
// recipient.
func (s *TraceService) Append(ctx context.Context, agent *domain.Agent, msgID string, in []TraceInput) error {
	m, err := s.messages.Get(ctx, msgID)
	if err != nil {
		return err
	}
	if m.To != agent.Name {
		return domain.Errf(domain.CodePermissionDenied, "only the recipient of %s uploads its trace", msgID)
	}
	if m.Type != domain.TypeTask {
		return domain.Errf(domain.CodeInvalidArgument, "%s is a %s; only tasks have traces", msgID, m.Type)
	}
	if domain.IsSealed(m.Body) {
		return domain.Errf(domain.CodeInvalidArgument, "%s is end-to-end encrypted; its run is not traced", msgID)
	}
	if len(in) > domain.MaxTraceBatch {
		return domain.Errf(domain.CodeInvalidArgument, "at most %d steps per upload", domain.MaxTraceBatch)
	}
	used, err := s.traces.Bytes(ctx, msgID)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	events := make([]*domain.TraceEvent, 0, len(in))
	for _, e := range in {
		if e.Seq <= 0 || e.Kind == "" {
			return domain.Errf(domain.CodeInvalidArgument, "a step needs a seq (from 1) and a kind")
		}
		if len(e.Data) > domain.MaxTraceEvent {
			return domain.Errf(domain.CodeInvalidArgument, "step %d is %d bytes, limit is %d", e.Seq, len(e.Data), domain.MaxTraceEvent)
		}
		used += int64(len(e.Data))
		if used > domain.MaxTraceBytes {
			return domain.Errf(domain.CodeInvalidState, "the trace of %s is full (%d MB)", msgID, domain.MaxTraceBytes>>20)
		}
		events = append(events, &domain.TraceEvent{Message: msgID, Seq: e.Seq, Kind: e.Kind, Data: e.Data, At: now})
	}
	return s.traces.Append(ctx, events)
}

// TraceReader is who asks for a trace: an agent, or a person in the app
// (their app sender name and member id).
type TraceReader struct {
	Agent    *domain.Agent
	Console  domain.AgentName
	MemberID string
}

// Read returns the steps of task msgID after seq.
func (s *TraceService) Read(ctx context.Context, r TraceReader, msgID string, after int) ([]*domain.TraceEvent, *domain.Message, error) {
	m, err := s.messages.Get(ctx, msgID)
	if err != nil {
		return nil, nil, err
	}
	if !s.mayRead(ctx, r, m) {
		return nil, nil, domain.Errf(domain.CodeNotFound, "message %s not found", msgID)
	}
	events, err := s.traces.List(ctx, msgID, after, 1000)
	return events, m, err
}

func (s *TraceService) mayRead(ctx context.Context, r TraceReader, m *domain.Message) bool {
	switch {
	case r.Agent != nil:
		return m.To == r.Agent.Name || m.From == r.Agent.Name
	case r.Console != "" && m.From == r.Console:
		return true
	case r.MemberID != "":
		a, err := s.agents.Get(ctx, m.To)
		return err == nil && a.Owner == r.MemberID
	}
	return false
}

// Sweep drops traces older than TraceRetention.
func (s *TraceService) Sweep(ctx context.Context) (int, error) {
	return s.traces.DeleteBefore(ctx, s.clock.Now().Add(-domain.TraceRetention))
}

// Run sweeps every interval until ctx ends.
func (s *TraceService) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := s.Sweep(ctx); err != nil {
				s.log.Warn("trace sweep failed", "err", err)
			} else if n > 0 {
				s.log.Info("dropped old run traces", "steps", n)
			}
		}
	}
}
