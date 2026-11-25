package app_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/notify"
	"github.com/myco-ai/myco/internal/infra/security"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

func newNotices(t *testing.T) (*app.NoticeService, *app.EventBus) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "n.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	clock := &stepClock{now: time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)}
	svc := app.NewNoticeService(sqlite.NewNoticeRepo(db), sqlite.NewSettingRepo(db), notify.NewMemory(), security.IDs{}, clock, nil)
	bus := app.NewEventBus()
	svc.Listen(bus)
	return svc, bus
}

type recordingSink struct {
	mu        sync.Mutex
	delivered []*domain.Notice
	changed   []string
}

func (s *recordingSink) Name() string { return "test" }
func (s *recordingSink) Deliver(_ context.Context, n *domain.Notice) {
	s.mu.Lock()
	s.delivered = append(s.delivered, n)
	s.mu.Unlock()
}
func (s *recordingSink) Changed(_ context.Context, who string) {
	s.mu.Lock()
	s.changed = append(s.changed, who)
	s.mu.Unlock()
}

type panickySource struct{}

func (panickySource) Name() string { return "broken" }
func (panickySource) Notices(context.Context, domain.Event) app.NoticeChange {
	panic("a broken source")
}

func person(_ context.Context, name string) string {
	if name == "@console:alice" || name == "alice" {
		return "mem_alice"
	}
	return ""
}

// Messages: only what reaches a person counts; a source that breaks does
// not stop the others; channels get each new notice once, and are told
// when a person's notices change.
func TestNoticeMessagesAndSinks(t *testing.T) {
	svc, bus := newNotices(t)
	sink := &recordingSink{}
	svc.AddSource(panickySource{})
	svc.AddSource(app.MessageNotices{Person: person})
	svc.AddSink(sink)
	c := context.Background()
	msg := func(id, from, to string, typ domain.MessageType, reply string, ask bool) *domain.Message {
		return &domain.Message{ID: id, From: domain.AgentName(from), To: domain.AgentName(to), Type: typ, ReplyTo: reply, Ask: ask, Project: "site"}
	}
	bus.Publish(c, domain.MessageCreated{Message: msg("m1", "codex", "claude", domain.TypeTask, "", false)}) // between agents
	bus.Publish(c, domain.MessageCreated{Message: msg("m2", "@console:alice", "codex", domain.TypeTask, "", false)})
	bus.Publish(c, domain.MessageCreated{Message: msg("m3", "codex", "@console:alice", domain.TypeProgress, "m2", true)})
	bus.Publish(c, domain.MessageCreated{Message: msg("m3", "codex", "@console:alice", domain.TypeProgress, "m2", true)}) // told once
	bus.Publish(c, domain.MessageCreated{Message: msg("m4", "codex", "@console:stranger", domain.TypeReply, "x", false)})
	svc.Drain()
	feed, err := svc.Feed(c, "mem_alice", 0, 0, 0)
	if err != nil || len(feed.Notices) != 1 || feed.Badge != 1 {
		t.Fatalf("one question for alice: %v %+v", err, feed)
	}
	n := feed.Notices[0]
	if n.Kind != domain.NoticeAsking || n.Project != "site" || n.Ref != "task:m2" || !n.Announce || n.MessageID != "m3" {
		t.Fatalf("question: %+v", n)
	}
	if len(sink.delivered) != 1 || len(sink.changed) != 1 || sink.changed[0] != "mem_alice" {
		t.Fatalf("sink: %d delivered, changed %v", len(sink.delivered), sink.changed)
	}
	// The task fails: the question is settled, the failure needs them.
	task := msg("m2", "@console:alice", "codex", domain.TypeTask, "", false)
	task.Status = domain.StatusFailed
	bus.Publish(c, domain.MessageChanged{Message: task})
	bus.Publish(c, domain.MessageCreated{Message: msg("m5", "codex", "@console:alice", domain.TypeEvent, "m2", false)})
	svc.Drain()
	feed, _ = svc.Feed(c, "mem_alice", 0, 0, 0)
	if len(feed.Notices) != 2 || feed.Notices[0].Open() || feed.Notices[1].Kind != domain.NoticeFailed || feed.Badge != 1 {
		t.Fatalf("after the failure: %+v", feed)
	}
	// Reading a failure settles it (it is not actionable).
	if k, err := svc.Read(c, "mem_alice", "agent:codex", 0); err != nil || k != 2 {
		t.Fatalf("read: %v %d", err, k)
	}
	if feed, _ = svc.Feed(c, "mem_alice", 0, 0, 0); feed.Badge != 0 {
		t.Fatalf("badge after reading: %d", feed.Badge)
	}
}

// Approvals and workflow decisions: told while they wait, settled once
// decided; a failed run is told to its owner.
func TestNoticeApprovalsAndDecisions(t *testing.T) {
	svc, bus := newNotices(t)
	runs := fakeRuns{"r1": {ID: "r1", WorkflowID: "wf1", Name: "Release", OwnerID: "mem_owner"}}
	svc.AddSource(app.ApprovalNotices{})
	svc.AddSource(app.WorkflowNotices{Runs: runs, Person: person})
	c := context.Background()

	a := &domain.Approval{ID: "ap1", Owner: "mem_alice", Agent: "shroom", What: "删除项目 old", Status: domain.ApprovalPending}
	bus.Publish(c, domain.ApprovalChanged{Approval: a})
	step := &domain.RunStep{ID: "s1", RunID: "r1", StepID: "review", Status: domain.StepWaiting, Target: "alice"}
	bus.Publish(c, domain.StepChanged{Step: step})
	svc.Drain()
	alice, _ := svc.Feed(c, "mem_alice", 0, 0, 0)
	owner, _ := svc.Feed(c, "mem_owner", 0, 0, 0)
	if len(alice.Notices) != 2 || alice.Badge != 2 || alice.Notices[0].Target != "superagent" || alice.Notices[0].Subject != "删除项目 old" {
		t.Fatalf("alice: %+v", alice)
	}
	if len(owner.Notices) != 1 || owner.Notices[0].Kind != domain.NoticeDecision || owner.Notices[0].Target != "workflow:wf1?run=r1" {
		t.Fatalf("owner: %+v", owner)
	}
	// Reading does not settle what waits for an action.
	if _, err := svc.Read(c, "mem_alice", "", 0); err != nil {
		t.Fatal(err)
	}
	if alice, _ = svc.Feed(c, "mem_alice", 0, 0, 0); alice.Badge != 2 {
		t.Fatalf("still waiting after reading: %d", alice.Badge)
	}
	a.Status = domain.ApprovalDone
	bus.Publish(c, domain.ApprovalChanged{Approval: a})
	step.Status = domain.StepDone
	bus.Publish(c, domain.StepChanged{Step: step})
	bus.Publish(c, domain.RunChanged{Run: &domain.WorkflowRun{ID: "r1", WorkflowID: "wf1", Name: "Release", OwnerID: "mem_owner", Status: domain.FlowFailed}})
	svc.Drain()
	alice, _ = svc.Feed(c, "mem_alice", 0, 0, 0)
	owner, _ = svc.Feed(c, "mem_owner", 0, 0, 0)
	if alice.Badge != 0 || owner.Badge != 1 || owner.Notices[len(owner.Notices)-1].Kind != domain.NoticeRunFail {
		t.Fatalf("after deciding: alice %+v owner %+v", alice, owner)
	}
}

// A waiting read wakes for a new notice.
func TestNoticeFeedWaits(t *testing.T) {
	svc, bus := newNotices(t)
	svc.AddSource(app.ApprovalNotices{})
	c := context.Background()
	go func() {
		time.Sleep(50 * time.Millisecond)
		bus.Publish(c, domain.ApprovalChanged{Approval: &domain.Approval{ID: "ap1", Owner: "mem_alice", Status: domain.ApprovalPending}})
	}()
	start := time.Now()
	feed, err := svc.Feed(c, "mem_alice", 0, 0, 5*time.Second)
	if err != nil || len(feed.Notices) != 1 || time.Since(start) > 4*time.Second {
		t.Fatalf("wait: %v %+v after %v", err, feed, time.Since(start))
	}
	if _, err := svc.Feed(c, "", 0, 0, 0); err == nil {
		t.Fatal("notices are a person's")
	}
}

type fakeRuns map[string]*domain.WorkflowRun

func (f fakeRuns) GetRun(_ context.Context, id string) (*domain.WorkflowRun, error) {
	if r, ok := f[id]; ok {
		return r, nil
	}
	return nil, domain.Errf(domain.CodeNotFound, "no run %s", id)
}
