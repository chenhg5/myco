package app

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// AutomationService keeps automations and runs them: on their schedule
// (RunDue, from the hub's loop), when their webhook is called, or by hand.
// A run sends the automation's message as an ordinary task from its owner.
type AutomationService struct {
	repo     domain.AutomationRepository
	messages *MessageService
	msgRepo  domain.MessageRepository
	tokens   TokenIssuer
	ids      IDGenerator
	clock    Clock
	log      *slog.Logger
	// One run at a time per automation (a webhook racing the schedule).
	mu sync.Mutex
}

// WebhookRate is how many webhook calls an automation takes per hour.
const WebhookRate = 30

func NewAutomationService(repo domain.AutomationRepository, messages *MessageService, msgRepo domain.MessageRepository,
	tokens TokenIssuer, ids IDGenerator, clock Clock, log *slog.Logger) *AutomationService {
	if log == nil {
		log = slog.Default()
	}
	return &AutomationService{repo: repo, messages: messages, msgRepo: msgRepo, tokens: tokens, ids: ids, clock: clock, log: log}
}

// Caller is who asks: the member (or the admin's device) using the app,
// and whether they manage the hub.
type Caller struct {
	Name     string // member name, or the console device's
	MemberID string
	Manages  bool
}

func (c Caller) owns(a *domain.Automation) bool {
	if c.Manages {
		return true
	}
	if a.OwnerID != "" {
		return a.OwnerID == c.MemberID
	}
	return a.Owner == c.Name
}

// AutomationInput is what people set (nil: unchanged on update).
type AutomationInput struct {
	Name          *string
	To            *string
	Project       *string
	Body          *string
	Cron          *string
	TZ            *string
	ClearSchedule bool
	Webhook       *bool
	RotateSecret  bool
	SkipIfBusy    *bool
	Enabled       *bool
}

// AutomationOut is an automation and, when one was just made, its webhook
// secret (shown once).
type AutomationOut struct {
	Automation *domain.Automation
	Secret     string
	LastRun    *domain.AutomationRun
}

func (s *AutomationService) apply(a *domain.Automation, in AutomationInput) (secret string, err error) {
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	set(&a.Name, in.Name)
	set(&a.Body, in.Body)
	set(&a.Project, in.Project)
	if in.To != nil {
		to, err := domain.ParseAgentName(strings.TrimSpace(*in.To))
		if err != nil {
			return "", err
		}
		a.To = to
	}
	if in.ClearSchedule {
		a.Cron, a.TZ = "", ""
	}
	if in.Cron != nil {
		a.Cron = strings.Join(strings.Fields(*in.Cron), " ")
		a.TZ = ""
		if in.TZ != nil {
			a.TZ = strings.TrimSpace(*in.TZ)
		}
	} else if in.TZ != nil && a.Cron != "" {
		a.TZ = strings.TrimSpace(*in.TZ)
	}
	if in.SkipIfBusy != nil {
		a.SkipIfBusy = *in.SkipIfBusy
	}
	if in.Enabled != nil {
		a.Enabled = *in.Enabled
	}
	newSecret := in.RotateSecret && a.WebhookHash != ""
	if in.Webhook != nil {
		switch {
		case *in.Webhook && a.WebhookHash == "":
			newSecret = true
		case !*in.Webhook:
			a.WebhookHash, newSecret = "", false
		}
	}
	if newSecret {
		plain, hash, err := s.tokens.Issue()
		if err != nil {
			return "", err
		}
		a.WebhookHash, secret = hash, "whk_"+plain
	}
	return secret, nil
}

// schedule sets the next run from now (none when disabled or unscheduled).
func (s *AutomationService) schedule(a *domain.Automation, now time.Time) {
	a.NextRunAt = time.Time{}
	if !a.Enabled || a.Cron == "" {
		return
	}
	if sc, err := domain.ParseSchedule(a.Cron, a.TZ); err == nil {
		a.NextRunAt = sc.Next(now)
	}
}

// Create makes an automation owned by the caller, sending from their
// console address.
func (s *AutomationService) Create(ctx context.Context, c Caller, in AutomationInput) (*AutomationOut, error) {
	if n, err := s.repo.Count(ctx); err != nil {
		return nil, err
	} else if n >= domain.MaxAutomations {
		return nil, domain.Errf(domain.CodeInvalidArgument, "this workspace has %d automations, the most it can have", domain.MaxAutomations).
			WithSuggestion("remove ones no longer needed: myco automation remove --id <id>")
	}
	now := s.clock.Now()
	a := &domain.Automation{ID: s.ids.New("aut"), Owner: c.Name, OwnerID: c.MemberID, From: ConsoleSender(c.Name),
		Enabled: true, CreatedAt: now, UpdatedAt: now}
	secret, err := s.apply(a, in)
	if err != nil {
		return nil, err
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	if err := s.checkTarget(ctx, a); err != nil {
		return nil, err
	}
	s.schedule(a, now)
	if err := s.repo.Create(ctx, a); err != nil {
		return nil, err
	}
	return &AutomationOut{Automation: a, Secret: secret}, nil
}

// checkTarget: the agent exists (or is an alias of one).
func (s *AutomationService) checkTarget(ctx context.Context, a *domain.Automation) error {
	if _, _, err := (aliasResolver{agents: s.messages.agents, aliases: s.messages.aliases}).resolve(ctx, a.To); err != nil {
		if domain.IsCode(err, domain.CodeNotFound) {
			return domain.Errf(domain.CodeNotFound, "agent %s not found", a.To).WithSuggestion("list agents with: myco agent list")
		}
		return err
	}
	return nil
}

// get loads one the caller may see and change (others: not found).
func (s *AutomationService) get(ctx context.Context, c Caller, id string) (*domain.Automation, error) {
	a, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !c.owns(a) {
		return nil, domain.Errf(domain.CodeNotFound, "automation %s not found", id)
	}
	return a, nil
}

// List: everything for those who manage the hub, else the caller's own.
func (s *AutomationService) List(ctx context.Context, c Caller) ([]*AutomationOut, error) {
	all, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := []*AutomationOut{}
	for _, a := range all {
		if !c.owns(a) {
			continue
		}
		o := &AutomationOut{Automation: a}
		if runs, err := s.repo.Runs(ctx, a.ID, 1); err == nil && len(runs) > 0 {
			o.LastRun = runs[0]
		}
		out = append(out, o)
	}
	return out, nil
}

// Get returns one with its newest runs.
func (s *AutomationService) Get(ctx context.Context, c Caller, id string, runs int) (*AutomationOut, []*domain.AutomationRun, error) {
	a, err := s.get(ctx, c, id)
	if err != nil {
		return nil, nil, err
	}
	rs, err := s.repo.Runs(ctx, id, runs)
	if err != nil {
		return nil, nil, err
	}
	o := &AutomationOut{Automation: a}
	if len(rs) > 0 {
		o.LastRun = rs[0]
	}
	return o, rs, nil
}

// Update changes what is set in in.
func (s *AutomationService) Update(ctx context.Context, c Caller, id string, in AutomationInput) (*AutomationOut, error) {
	a, err := s.get(ctx, c, id)
	if err != nil {
		return nil, err
	}
	was := *a
	secret, err := s.apply(a, in)
	if err != nil {
		return nil, err
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	if a.To != was.To {
		if err := s.checkTarget(ctx, a); err != nil {
			return nil, err
		}
	}
	now := s.clock.Now()
	a.UpdatedAt = now
	if a.Cron != was.Cron || a.TZ != was.TZ || a.Enabled != was.Enabled || (a.Enabled && a.Cron != "" && a.NextRunAt.IsZero()) {
		s.schedule(a, now)
	}
	if err := s.repo.Save(ctx, a); err != nil {
		return nil, err
	}
	o := &AutomationOut{Automation: a, Secret: secret}
	if runs, err := s.repo.Runs(ctx, a.ID, 1); err == nil && len(runs) > 0 {
		o.LastRun = runs[0]
	}
	return o, nil
}

// Delete removes one and its runs (the messages it sent stay).
func (s *AutomationService) Delete(ctx context.Context, c Caller, id string) error {
	if _, err := s.get(ctx, c, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

// RunNow runs one by hand (also when it is turned off).
func (s *AutomationService) RunNow(ctx context.Context, c Caller, id string) (*domain.AutomationRun, error) {
	a, err := s.get(ctx, c, id)
	if err != nil {
		return nil, err
	}
	return s.run(ctx, a, domain.TriggerManual, nil)
}

// Webhook runs one whose webhook was called with secret and what the call
// carried (its body goes into the message; all of it is kept with the
// run). Any reason not to (no such automation, wrong secret, turned off,
// no webhook) is the same not_found, so a caller learns nothing.
func (s *AutomationService) Webhook(ctx context.Context, id, secret string, call *domain.WebhookRequest) (*domain.AutomationRun, error) {
	notFound := domain.Errf(domain.CodeNotFound, "not found")
	a, err := s.repo.Get(ctx, id)
	if err != nil || !a.Enabled || a.WebhookHash == "" || secret == "" {
		return nil, notFound
	}
	if s.tokens.Hash(strings.TrimPrefix(secret, "whk_")) != a.WebhookHash {
		return nil, notFound
	}
	now := s.clock.Now()
	n, err := s.repo.RunsSince(ctx, id, domain.TriggerWebhook, now.Add(-time.Hour))
	if err != nil {
		return nil, err
	}
	if n >= WebhookRate {
		return nil, domain.Errf(domain.CodeRateLimited, "this webhook was called %d times in the last hour, the most it takes", WebhookRate).
			WithSuggestion("call it less often; it takes %d calls an hour", WebhookRate)
	}
	if call == nil {
		call = &domain.WebhookRequest{}
	}
	return s.run(ctx, a, domain.TriggerWebhook, call)
}

// GetRun is one run of one the caller may see, with what its webhook call
// carried.
func (s *AutomationService) GetRun(ctx context.Context, c Caller, id, runID string) (*domain.AutomationRun, error) {
	if _, err := s.get(ctx, c, id); err != nil {
		return nil, err
	}
	return s.repo.Run(ctx, id, runID)
}

// RetryAfter is how long a rate-limited webhook should wait.
func (s *AutomationService) RetryAfter(ctx context.Context, id string) time.Duration {
	runs, err := s.repo.Runs(ctx, id, domain.MaxAutomationRuns)
	if err != nil {
		return time.Minute
	}
	cut := s.clock.Now().Add(-time.Hour)
	var oldest time.Time
	for _, r := range runs {
		if r.Trigger == domain.TriggerWebhook && r.At.After(cut) {
			oldest = r.At // newest first: the last one seen is the oldest
		}
	}
	if oldest.IsZero() {
		return time.Minute
	}
	return max(oldest.Add(time.Hour).Sub(s.clock.Now()), time.Second)
}

// RunDue runs the scheduled automations whose time has come. One missed
// while the hub was down runs once, then its schedule goes on from now.
func (s *AutomationService) RunDue(ctx context.Context) {
	now := s.clock.Now()
	due, err := s.repo.Due(ctx, now)
	if err != nil {
		s.log.Warn("automations: reading due ones failed", "err", err)
		return
	}
	for _, a := range due {
		if ctx.Err() != nil {
			return
		}
		// Next first: a run that fails never makes it fire again at once.
		s.schedule(a, now)
		a.UpdatedAt = a.UpdatedAt.UTC()
		if err := s.repo.Save(ctx, a); err != nil {
			s.log.Warn("automations: rescheduling failed", "automation", a.ID, "err", err)
			continue
		}
		if _, err := s.run(ctx, a, domain.TriggerSchedule, nil); err != nil {
			s.log.Warn("automations: run failed", "automation", a.ID, "err", err)
		}
	}
}

// Run calls RunDue every tick until ctx is done.
func (s *AutomationService) Run(ctx context.Context, tick time.Duration) {
	t := time.NewTicker(tick)
	defer t.Stop()
	s.RunDue(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.RunDue(ctx)
		}
	}
}

// run sends the message (or records why not) and the run.
func (s *AutomationService) run(ctx context.Context, a *domain.Automation, trigger string, call *domain.WebhookRequest) (*domain.AutomationRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fresh, err := s.repo.Get(ctx, a.ID); err == nil {
		a.LastTask = fresh.LastTask
	}
	r := &domain.AutomationRun{ID: s.ids.New("arun"), AutomationID: a.ID, At: s.clock.Now(), Trigger: trigger, Request: call}
	if a.SkipIfBusy && a.LastTask != "" {
		if m, err := s.msgRepo.Get(ctx, a.LastTask); err == nil && !m.Status.Terminal() {
			r.Outcome, r.Error = domain.RunSkipped, "the last task it sent is not finished"
		}
	}
	if r.Outcome == "" {
		body := a.Body
		if call != nil {
			body = domain.WithPayload(a.Body, call.Body)
		}
		out, err := s.messages.SendFromMember(ctx, a.Owner, a.OwnerID, SendInput{
			To: string(a.To), Type: string(domain.TypeTask), Body: body, Project: a.Project, Automation: a.ID,
		})
		if err != nil {
			r.Outcome, r.Error = domain.RunFailed, errText(err)
		} else {
			r.Outcome, r.MessageID = domain.RunSent, out.Message.ID
			a.LastTask = out.Message.ID
			if fresh, err := s.repo.Get(ctx, a.ID); err == nil {
				fresh.LastTask = a.LastTask
				if err := s.repo.Save(ctx, fresh); err != nil {
					s.log.Warn("automations: recording the last task failed", "automation", a.ID, "err", err)
				}
			}
		}
	}
	if err := s.repo.AddRun(ctx, r, domain.MaxAutomationRuns); err != nil {
		return nil, err
	}
	return r, nil
}

// errText is an error's message without its suggestion, short.
func errText(err error) string {
	msg := err.Error()
	if e, ok := err.(*domain.Error); ok {
		msg = e.Message
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}
