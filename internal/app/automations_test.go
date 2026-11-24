package app_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/notify"
	"github.com/myco-ai/myco/internal/infra/security"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

func automations(t *testing.T, clock app.Clock) (*app.AutomationService, *app.AgentService, domain.AutomationRepository) {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ar, mr, rr := sqlite.NewAgentRepo(db), sqlite.NewMessageRepo(db), sqlite.NewAutomationRepo(db)
	msgs := app.NewMessageService(ar, mr, notify.NewMemory(), security.IDs{}, clock, app.DefaultMessagePolicy())
	agents := app.NewAgentService(ar, security.Tokens{}, security.IDs{}, clock, time.Minute, nil)
	svc := app.NewAutomationService(rr, msgs, mr, security.Tokens{}, security.IDs{}, clock, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return svc, agents, rr
}

func TestAutomationScheduleRunsOnTimeAndOnceAfterDowntime(t *testing.T) {
	ctx := context.Background()
	clock := &fakeClock{t: time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC)} // a Monday
	svc, agents, repo := automations(t, clock)
	if _, err := agents.Register(ctx, app.RegisterInput{Name: "pm"}); err != nil {
		t.Fatal(err)
	}
	glenn := app.Caller{Name: "glenn", MemberID: "mem_1"}
	s := func(v string) *string { return &v }
	out, err := svc.Create(ctx, glenn, app.AutomationInput{Name: s("standup"), To: s("pm"), Body: s("standup notes"), Cron: s("0 9 * * 1-5"), TZ: s("UTC")})
	if err != nil {
		t.Fatal(err)
	}
	id := out.Automation.ID
	if want := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC); !out.Automation.NextRunAt.Equal(want) {
		t.Fatalf("next: %v", out.Automation.NextRunAt)
	}
	runs := func() []*domain.AutomationRun {
		r, err := repo.Runs(ctx, id, 50)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	clock.t = clock.t.Add(30 * time.Minute) // 08:30: not yet
	svc.RunDue(ctx)
	if n := len(runs()); n != 0 {
		t.Fatalf("early: %d runs", n)
	}
	clock.t = time.Date(2026, 11, 16, 9, 0, 20, 0, time.UTC)
	svc.RunDue(ctx)
	svc.RunDue(ctx) // the next tick in the same minute: not again
	if r := runs(); len(r) != 1 || r[0].Outcome != domain.RunSent || r[0].Trigger != domain.TriggerSchedule {
		t.Fatalf("at 9:00: %+v", r)
	}
	a, _ := repo.Get(ctx, id)
	if want := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC); !a.NextRunAt.Equal(want) {
		t.Fatalf("next after the run: %v", a.NextRunAt)
	}

	// The hub was down from Monday to Friday noon: one run, not four.
	clock.t = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	svc.RunDue(ctx)
	if n := len(runs()); n != 2 {
		t.Fatalf("after downtime: %d runs", n)
	}
	a, _ = repo.Get(ctx, id)
	if want := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC); !a.NextRunAt.Equal(want) { // skips the weekend
		t.Fatalf("next after downtime: %v", a.NextRunAt)
	}

	// Turned off: nothing due, no next run.
	off := false
	if o, err := svc.Update(ctx, glenn, id, app.AutomationInput{Enabled: &off}); err != nil || !o.Automation.NextRunAt.IsZero() {
		t.Fatalf("off: %v %+v", err, o)
	}
	clock.t = time.Date(2026, 11, 23, 9, 1, 0, 0, time.UTC)
	svc.RunDue(ctx)
	if n := len(runs()); n != 2 {
		t.Fatalf("ran while off: %d runs", n)
	}

	// Someone else sees nothing of it; whoever manages the hub does.
	other := app.Caller{Name: "alex", MemberID: "mem_2"}
	if _, _, err := svc.Get(ctx, other, id, 10); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("another member: %v", err)
	}
	if l, _ := svc.List(ctx, other); len(l) != 0 {
		t.Fatalf("another member's list: %d", len(l))
	}
	if err := svc.Delete(ctx, other, id); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("another member deletes: %v", err)
	}
	if l, _ := svc.List(ctx, app.Caller{Name: "admin", Manages: true}); len(l) != 1 || l[0].LastRun == nil {
		t.Fatalf("admin's list: %+v", l)
	}
}

func TestAutomationWebhookRateWindow(t *testing.T) {
	ctx := context.Background()
	clock := &fakeClock{t: time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC)}
	svc, agents, _ := automations(t, clock)
	if _, err := agents.Register(ctx, app.RegisterInput{Name: "ci"}); err != nil {
		t.Fatal(err)
	}
	on, s := true, func(v string) *string { return &v }
	out, err := svc.Create(ctx, app.Caller{Name: "glenn"}, app.AutomationInput{Name: s("ci"), To: s("ci"), Body: s("{{payload}}"), Webhook: &on})
	if err != nil || out.Secret == "" {
		t.Fatalf("create: %v %+v", err, out)
	}
	id := out.Automation.ID
	if _, err := svc.Webhook(ctx, "aut_nope", out.Secret, &domain.WebhookRequest{Body: "x"}); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
	if _, err := svc.Webhook(ctx, id, out.Secret+"x", &domain.WebhookRequest{Body: "x"}); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("wrong secret: %v", err)
	}
	for i := 0; i < app.WebhookRate; i++ {
		if _, err := svc.Webhook(ctx, id, out.Secret, &domain.WebhookRequest{Body: "x"}); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
		clock.t = clock.t.Add(time.Minute)
	}
	// 08:30: the first call was at 08:00, so the window opens at 09:00.
	if _, err := svc.Webhook(ctx, id, out.Secret, &domain.WebhookRequest{Body: "x"}); !domain.IsCode(err, domain.CodeRateLimited) {
		t.Fatalf("over the rate: %v", err)
	}
	if d := svc.RetryAfter(ctx, id); d != 30*time.Minute {
		t.Fatalf("retry after: %v", d)
	}
	clock.t = time.Date(2026, 11, 16, 9, 0, 30, 0, time.UTC)
	if _, err := svc.Webhook(ctx, id, out.Secret, &domain.WebhookRequest{Body: "x"}); err != nil {
		t.Fatalf("an hour later: %v", err)
	}
}

func TestAutomationWindowOnceAndExpiration(t *testing.T) {
	ctx := context.Background()
	clock := &fakeClock{t: time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)}
	svc, agents, repo := automations(t, clock)
	if _, e := agents.Register(ctx, app.RegisterInput{Name: "pm"}); e != nil {
		t.Fatal(e)
	}
	owner := app.Caller{Name: "owner", MemberID: "owner"}
	str := func(s string) *string { return &s }
	yes := true
	start, end := "2026-11-26T09:00:00Z", "2026-11-29T00:00:00Z"
	in := app.AutomationInput{Name: str("Three days"), To: str("pm"), Body: str("Daily check"), Cron: str("0 9 * * *"), StartsAt: &start, EndsAt: &end, Once: &yes}
	out, e := svc.Create(ctx, owner, in)
	if e != nil {
		t.Fatal(e)
	}
	want, _ := time.Parse(time.RFC3339, start)
	if !out.Automation.NextRunAt.Equal(want) {
		t.Fatalf("first run %s", out.Automation.NextRunAt)
	}
	if r, e := svc.RunNow(ctx, owner, out.Automation.ID); e != nil || r.Outcome != domain.RunSkipped {
		t.Fatal("ran before start", r, e)
	}
	clock.t = want
	svc.RunDue(ctx)
	svc.RunDue(ctx)
	stored, _ := repo.Get(ctx, out.Automation.ID)
	if stored.Enabled || stored.DisabledReason != "once" || !stored.NextRunAt.IsZero() {
		t.Fatalf("once not consumed %+v", stored)
	}
	if r, e := svc.RunNow(ctx, owner, stored.ID); e != nil || r.Outcome != domain.RunSkipped {
		t.Fatal("manual run repeated once", r, e)
	}
	runs, _ := repo.Runs(ctx, stored.ID, 50)
	sent := 0
	for _, r := range runs {
		if r.Outcome == domain.RunSent {
			sent++
		}
	}
	if sent != 1 {
		t.Fatalf("sent %d", sent)
	}
	// A window missed while the Hub is down must never dispatch late.
	in.Once = nil
	out, e = svc.Create(ctx, owner, in)
	if e != nil {
		t.Fatal(e)
	}
	clock.t = time.Date(2026, 11, 29, 0, 0, 0, 0, time.UTC)
	svc.RunDue(ctx)
	stored, _ = repo.Get(ctx, out.Automation.ID)
	if stored.Enabled || stored.DisabledReason != "expired" {
		t.Fatal("end did not disable", stored)
	}
	runs, _ = repo.Runs(ctx, stored.ID, 10)
	if len(runs) != 1 || runs[0].Outcome != domain.RunSkipped {
		t.Fatal("missed window sent work", runs)
	}
	// Expiration also covers webhook-only tasks with no next cron run.
	future := "2026-11-30T00:00:00Z"
	hook, e := svc.Create(ctx, owner, app.AutomationInput{Name: str("Hook"), To: str("pm"), Body: str("Hook task"), Webhook: &yes, EndsAt: &future})
	if e != nil {
		t.Fatal(e)
	}
	clock.t = time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	svc.RunDue(ctx)
	stored, _ = repo.Get(ctx, hook.Automation.ID)
	if stored.Enabled || stored.DisabledReason != "expired" {
		t.Fatal("webhook did not expire")
	}
	invalid := "2026-11-20T00:00:00Z"
	in.EndsAt = &invalid
	if _, e = svc.Create(ctx, owner, in); e == nil {
		t.Fatal("reversed dates accepted")
	}
	// Empty strings clear bounds, and explicit enable resets the reason.
	cleared, e := svc.Update(ctx, owner, hook.Automation.ID, app.AutomationInput{StartsAt: str(""), EndsAt: str(""), Enabled: &yes})
	if e != nil || !cleared.Automation.Enabled || cleared.Automation.DisabledReason != "" {
		t.Fatal("clear/re-enable", e)
	}
}

func TestAutomationOnceBusySkipAndConcurrentDispatch(t *testing.T) {
	ctx := context.Background()
	clock := &fakeClock{t: time.Now().UTC()}
	svc, agents, repo := automations(t, clock)
	if _, e := agents.Register(ctx, app.RegisterInput{Name: "worker"}); e != nil {
		t.Fatal(e)
	}
	str := func(s string) *string { return &s }
	yes, no := true, false
	owner := app.Caller{Name: "owner", MemberID: "member"}
	out, e := svc.Create(ctx, owner, app.AutomationInput{Name: str("Check"), To: str("worker"), Body: str("Check"), Cron: str("0 9 * * *"), SkipIfBusy: &yes})
	if e != nil {
		t.Fatal(e)
	}
	if r, e := svc.RunNow(ctx, owner, out.Automation.ID); e != nil || r.Outcome != domain.RunSent {
		t.Fatal(r, e)
	}
	if _, e = svc.Update(ctx, owner, out.Automation.ID, app.AutomationInput{Once: &yes}); e != nil {
		t.Fatal(e)
	}
	if r, e := svc.RunNow(ctx, owner, out.Automation.ID); e != nil || r.Outcome != domain.RunSkipped {
		t.Fatal("busy was not skipped", r, e)
	}
	stored, _ := repo.Get(ctx, out.Automation.ID)
	if !stored.Enabled || stored.DisabledReason != "" {
		t.Fatal("busy skip consumed once")
	}
	if _, e = svc.Update(ctx, owner, out.Automation.ID, app.AutomationInput{SkipIfBusy: &no}); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := svc.RunNow(ctx, owner, out.Automation.ID); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	runs, _ := repo.Runs(ctx, out.Automation.ID, 50)
	sent := 0
	for _, r := range runs {
		if r.Outcome == domain.RunSent {
			sent++
		}
	}
	if sent != 2 {
		t.Fatalf("concurrent once dispatch: sent %d (one before enabling once)", sent)
	}
}
