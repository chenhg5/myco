package app

import (
	"context"

	"github.com/myco-ai/myco/internal/domain"
)

// The sources shipped with the hub: one per feature that has something to
// tell people. Each only reads its own events; none knows how notices are
// stored or delivered.

// PersonOf names the person behind a console sender or a member's handle
// (their member id), or "" when there is none.
type PersonOf func(ctx context.Context, name string) string

// MessageNotices tells a person what agents send them: answers, failures,
// questions, messages on their own, progress. Only messages to a person
// (a console sender) count; what agents send each other never does.
type MessageNotices struct{ Person PersonOf }

func (MessageNotices) Name() string { return "messages" }

func (s MessageNotices) Notices(ctx context.Context, e domain.Event) NoticeChange {
	switch ev := e.(type) {
	case domain.MessageCreated:
		return s.created(ctx, ev.Message)
	case domain.MessageChanged:
		m := ev.Message
		// A task the person sent is over: its questions no longer wait.
		if m.Type == domain.TypeTask && IsConsoleSender(m.From) && m.Status.Terminal() {
			return NoticeChange{Resolve: []NoticeSettled{{Ref: askRef(m.ID)}}}
		}
	}
	return NoticeChange{}
}

func askRef(task string) string { return "task:" + task }

func (s MessageNotices) created(ctx context.Context, m *domain.Message) NoticeChange {
	if m == nil {
		return NoticeChange{}
	}
	if IsConsoleSender(m.From) {
		// The person answered a question (a steer that answers it).
		if m.Type == domain.TypeSteer && m.Answers != "" {
			return NoticeChange{Resolve: []NoticeSettled{{Key: "msg:" + m.Answers}}}
		}
		return NoticeChange{}
	}
	if !IsConsoleSender(m.To) || s.Person == nil {
		return NoticeChange{}
	}
	who := s.Person(ctx, string(m.To))
	if who == "" {
		return NoticeChange{}
	}
	n := &domain.Notice{
		Recipient: who, Key: "msg:" + m.ID, Target: "agent:" + string(m.From), Project: m.Project,
		Actor: string(m.From), MessageID: m.ID, Ref: askRef(m.ReplyTo),
	}
	var out NoticeChange
	switch m.Type {
	case domain.TypeReply:
		n.Kind = domain.NoticeAnswered
		// The task is answered: what it asked no longer waits.
		out.Resolve = append(out.Resolve, NoticeSettled{Ref: askRef(m.ReplyTo)})
	case domain.TypeEvent:
		n.Kind = domain.NoticeFailed
		out.Resolve = append(out.Resolve, NoticeSettled{Ref: askRef(m.ReplyTo)})
	case domain.TypeProgress:
		n.Kind = domain.NoticeProgress
		if m.Ask {
			n.Kind = domain.NoticeAsking
		}
	case domain.TypeTask, domain.TypeNote:
		n.Kind, n.Ref = domain.NoticeSaid, "msg:"+m.ID
	default:
		return NoticeChange{}
	}
	if m.ReplyTo == "" && n.Kind != domain.NoticeSaid {
		n.Ref = "msg:" + m.ID
	}
	out.Add = append(out.Add, n)
	return out
}

// ApprovalNotices tells a person their superagent asks for their OK, and
// settles it once decided (or expired, or withdrawn).
type ApprovalNotices struct{}

func (ApprovalNotices) Name() string { return "approvals" }

func (ApprovalNotices) Notices(_ context.Context, e domain.Event) NoticeChange {
	ev, ok := e.(domain.ApprovalChanged)
	if !ok || ev.Approval == nil {
		return NoticeChange{}
	}
	a := ev.Approval
	ref := "approval:" + a.ID
	if a.Status != domain.ApprovalPending {
		return NoticeChange{Resolve: []NoticeSettled{{Ref: ref}}}
	}
	if a.Owner == "" {
		return NoticeChange{}
	}
	return NoticeChange{Add: []*domain.Notice{{
		Recipient: a.Owner, Key: ref, Ref: ref, Kind: domain.NoticeApproval,
		Target: "superagent", Actor: string(a.Agent), Subject: a.What,
	}}}
}

// WorkflowNotices tells people a workflow waits on their decision (the
// run's owner, and the person the step names), and its owner that a run
// failed.
type WorkflowNotices struct {
	Runs interface {
		GetRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
	}
	Person PersonOf
}

func (WorkflowNotices) Name() string { return "workflows" }

func (s WorkflowNotices) Notices(ctx context.Context, e domain.Event) NoticeChange {
	switch ev := e.(type) {
	case domain.StepChanged:
		st := ev.Step
		if st == nil {
			return NoticeChange{}
		}
		ref := "decision:" + st.ID
		if st.Status != domain.StepWaiting {
			return NoticeChange{Resolve: []NoticeSettled{{Ref: ref}}}
		}
		if s.Runs == nil {
			return NoticeChange{}
		}
		run, err := s.Runs.GetRun(ctx, st.RunID)
		if err != nil {
			return NoticeChange{}
		}
		var out NoticeChange
		seen := map[string]bool{}
		for _, who := range []string{run.OwnerID, s.person(ctx, st.Target)} {
			if who == "" || seen[who] {
				continue
			}
			seen[who] = true
			out.Add = append(out.Add, &domain.Notice{
				Recipient: who, Key: ref, Ref: ref, Kind: domain.NoticeDecision,
				Target: runTarget(run), Subject: run.Name,
			})
		}
		return out
	case domain.RunChanged:
		r := ev.Run
		if r == nil || r.Status != domain.FlowFailed || r.OwnerID == "" {
			return NoticeChange{}
		}
		return NoticeChange{Add: []*domain.Notice{{
			Recipient: r.OwnerID, Key: "run:" + r.ID + ":failed", Ref: "run:" + r.ID, Kind: domain.NoticeRunFail,
			Target: runTarget(r), Subject: r.Name,
		}}}
	}
	return NoticeChange{}
}

func (s WorkflowNotices) person(ctx context.Context, name string) string {
	if name == "" || s.Person == nil {
		return ""
	}
	return s.Person(ctx, name)
}

func runTarget(r *domain.WorkflowRun) string {
	return "workflow:" + r.WorkflowID + "?run=" + r.ID
}
