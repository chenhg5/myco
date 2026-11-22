package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// NoticeService keeps what each person should hear about (domain.Notice).
//
// It is built from three kinds of parts, wired at the composition root:
//
//   - sources (NoticeSource) turn the hub's events into notices: one per
//     feature (messages, workflows, approvals). A new feature that has
//     something to tell people adds a source; nothing else changes.
//   - the person's preferences (domain.NoticePrefs) decide, once, whether
//     a notice is announced; every device and channel follows that.
//   - sinks (NoticeSink) take notices to people outside the App's own feed:
//     a push relay for phones, later chat apps or mail. A channel is a
//     sink; adding one changes nothing else.
//
// The App's feed is the service's own: List and Wait read what is stored,
// and every change wakes the person's waiters.
type NoticeService struct {
	repo     domain.NoticeRepository
	settings domain.SettingRepository
	ids      IDGenerator
	clock    Clock
	log      *slog.Logger
	wake     Notifier

	mu      sync.Mutex
	sources []NoticeSource
	sinks   []NoticeSink

	queue   chan queued
	pending sync.WaitGroup
	once    sync.Once
	pruned  time.Time
}

// NoticeSource turns one of the hub's events into what changes for people.
// It must be quick and must not fail the event: what it cannot work out is
// left out.
type NoticeSource interface {
	Name() string
	Notices(ctx context.Context, e domain.Event) NoticeChange
}

// NoticeChange is what a source found in an event: notices to add (their
// Recipient, Key, Kind and where they lead; the service fills in the
// rest), and matters now settled.
type NoticeChange struct {
	Add     []*domain.Notice
	Resolve []NoticeSettled
}

// NoticeSettled names notices to settle by key, by ref, or both.
type NoticeSettled struct{ Key, Ref string }

// NoticeSink takes notices to people by some channel (a push relay, a chat
// app). Deliver gets each new notice once it is stored, with its verdict
// (Announce, Quiet) decided: a sink that only announces skips quiet ones.
// Changed tells that a person's notices were read or settled (to update a
// badge, or withdraw a banner). Both run in the service's own goroutine,
// one at a time: a slow channel queues its own work.
type NoticeSink interface {
	Name() string
	Deliver(ctx context.Context, n *domain.Notice)
	Changed(ctx context.Context, recipient string)
}

type queued struct {
	ctx context.Context
	e   domain.Event
}

// NoticeRetention is how long notices are kept.
const NoticeRetention = 30 * 24 * time.Hour

// NewNoticeService returns a service with no sources or sinks.
func NewNoticeService(repo domain.NoticeRepository, settings domain.SettingRepository, wake Notifier, ids IDGenerator, clock Clock, log *slog.Logger) *NoticeService {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &NoticeService{repo: repo, settings: settings, wake: wake, ids: ids, clock: clock, log: log, queue: make(chan queued, 4096)}
}

// AddSource adds a source of notices.
func (s *NoticeService) AddSource(src NoticeSource) {
	s.mu.Lock()
	s.sources = append(s.sources, src)
	s.mu.Unlock()
}

// AddSink adds a channel.
func (s *NoticeService) AddSink(k NoticeSink) {
	s.mu.Lock()
	s.sinks = append(s.sinks, k)
	s.mu.Unlock()
}

// Listen subscribes the service to the hub's events. They are handled in
// order, in one goroutine of the service's own, so the hub's writes never
// wait on notices.
func (s *NoticeService) Listen(bus *EventBus) {
	bus.Subscribe(func(ctx context.Context, e domain.Event) {
		s.once.Do(func() { go s.loop() })
		s.pending.Add(1)
		select {
		case s.queue <- queued{context.WithoutCancel(ctx), e}:
		default:
			s.pending.Done()
			s.log.Warn("notices: queue full, an event was dropped", "event", e.EventName())
		}
	})
}

// Drain waits until every event published so far is handled (tests).
func (s *NoticeService) Drain() { s.pending.Wait() }

func (s *NoticeService) loop() {
	for q := range s.queue {
		s.Handle(q.ctx, q.e)
		s.pending.Done()
	}
}

// Handle works one event through every source (Listen calls it; tests may
// call it directly).
func (s *NoticeService) Handle(ctx context.Context, e domain.Event) {
	s.mu.Lock()
	sources, sinks := s.sources, s.sinks
	s.mu.Unlock()
	changed := map[string]bool{}
	for _, src := range sources {
		ch := s.safe(src.Name(), func() NoticeChange { return src.Notices(ctx, e) })
		// Settle first: an answer that comes with a new notice (a reply)
		// settles the question it answers before the reply is told.
		for _, r := range ch.Resolve {
			who, err := s.repo.Resolve(ctx, r.Key, r.Ref, s.clock.Now())
			if err != nil {
				s.log.Warn("notices: resolve", "source", src.Name(), "err", err)
			}
			for _, w := range who {
				changed[w] = true
			}
		}
		for _, n := range ch.Add {
			if s.add(ctx, n) {
				changed[n.Recipient] = true
				for _, k := range sinks {
					s.safe(k.Name(), func() NoticeChange { k.Deliver(ctx, n); return NoticeChange{} })
				}
			}
		}
	}
	for who := range changed {
		s.wake.Publish(noticeTopic(who))
		for _, k := range sinks {
			s.safe(k.Name(), func() NoticeChange { k.Changed(ctx, who); return NoticeChange{} })
		}
	}
	s.prune(ctx)
}

// safe runs a part of the pipeline so that one broken source or sink does
// not stop the others.
func (s *NoticeService) safe(part string, fn func() NoticeChange) (out NoticeChange) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("notices: a part failed", "part", part, "panic", r)
			out = NoticeChange{}
		}
	}()
	return fn()
}

func (s *NoticeService) add(ctx context.Context, n *domain.Notice) bool {
	if n == nil || n.Recipient == "" || n.Key == "" || n.Kind == "" {
		return false
	}
	now := s.clock.Now()
	n.ID, n.CreatedAt = s.ids.New("ntc"), now
	p, err := s.Prefs(ctx, n.Recipient)
	if err != nil {
		s.log.Warn("notices: prefs", "err", err)
	}
	n.Announce, n.Quiet = p.Verdict(n, now)
	ok, err := s.repo.Add(ctx, n)
	if err != nil {
		s.log.Warn("notices: store", "err", err)
		return false
	}
	return ok
}

func (s *NoticeService) prune(ctx context.Context) {
	now := s.clock.Now()
	if now.Sub(s.pruned) < 24*time.Hour {
		return
	}
	s.pruned = now
	if err := s.repo.Prune(ctx, now.Add(-NoticeRetention)); err != nil {
		s.log.Warn("notices: prune", "err", err)
	}
}

func noticeTopic(recipient string) string { return "notices:" + recipient }

// --- the feed: what devices read ---

// NoticeFeed is a page of a person's notices.
type NoticeFeed struct {
	Notices []*domain.Notice
	// Last is the Seq to read after next time (the newest stored, even
	// when this page is empty).
	Last int64
	// Badge counts what still needs the person.
	Badge int
}

// MaxNoticeWait is the longest a device waits for news in one request.
const MaxNoticeWait = 55 * time.Second

// Feed returns a person's notices after after. With wait > 0 and nothing
// new, it waits up to wait for the next one (or a change: read, settled).
func (s *NoticeService) Feed(ctx context.Context, recipient string, after int64, limit int, wait time.Duration) (*NoticeFeed, error) {
	if recipient == "" {
		return nil, domain.Errf(domain.CodePermissionDenied, "notices are a person's: sign in as a member")
	}
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	wait = min(wait, MaxNoticeWait)
	// Subscribed before reading, so nothing stored in between is missed.
	woke, cancel := s.wake.Subscribe(noticeTopic(recipient))
	defer cancel()
	f, err := s.page(ctx, recipient, after, limit)
	if err != nil || len(f.Notices) > 0 || wait <= 0 {
		return f, err
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-woke:
		// Something changed (maybe only the badge): read once more.
		return s.page(ctx, recipient, after, limit)
	case <-t.C:
	case <-ctx.Done():
	}
	return f, nil
}

func (s *NoticeService) page(ctx context.Context, recipient string, after int64, limit int) (*NoticeFeed, error) {
	ns, err := s.repo.List(ctx, domain.NoticeFilter{Recipient: recipient, After: after, Limit: limit})
	if err != nil {
		return nil, err
	}
	last, err := s.repo.Last(ctx, recipient)
	if err != nil {
		return nil, err
	}
	badge, err := s.repo.OpenCount(ctx, recipient, domain.TierNeedsYou)
	if err != nil {
		return nil, err
	}
	return &NoticeFeed{Notices: ns, Last: last, Badge: badge}, nil
}

// Read marks a person's notices as seen: those leading to target ("": all)
// up to through (0: all). Seen elsewhere, they are not told again there.
func (s *NoticeService) Read(ctx context.Context, recipient, target string, through int64) (int, error) {
	if recipient == "" {
		return 0, domain.Errf(domain.CodePermissionDenied, "notices are a person's: sign in as a member")
	}
	n, err := s.repo.Read(ctx, recipient, target, through, s.clock.Now())
	if err != nil || n == 0 {
		return n, err
	}
	s.wake.Publish(noticeTopic(recipient))
	s.mu.Lock()
	sinks := s.sinks
	s.mu.Unlock()
	for _, k := range sinks {
		s.safe(k.Name(), func() NoticeChange { k.Changed(ctx, recipient); return NoticeChange{} })
	}
	return n, nil
}

// --- preferences ---

func noticePrefsKey(recipient string) string { return "notice-prefs/" + recipient }

// Prefs returns what a person chose (the defaults when nothing).
func (s *NoticeService) Prefs(ctx context.Context, recipient string) (*domain.NoticePrefs, error) {
	p := &domain.NoticePrefs{}
	if recipient == "" {
		return p, nil
	}
	raw, err := s.settings.Get(ctx, noticePrefsKey(recipient))
	if err != nil || raw == "" {
		return p, err
	}
	if err := json.Unmarshal([]byte(raw), p); err != nil {
		s.log.Warn("notices: prefs unreadable, using the defaults", "err", err)
		return &domain.NoticePrefs{}, nil
	}
	return p, nil
}

// SetPrefs stores what a person chose.
func (s *NoticeService) SetPrefs(ctx context.Context, recipient string, p *domain.NoticePrefs) (*domain.NoticePrefs, error) {
	if recipient == "" {
		return nil, domain.Errf(domain.CodePermissionDenied, "notices are a person's: sign in as a member")
	}
	if p == nil {
		p = &domain.NoticePrefs{}
	}
	if q := p.Quiet; q != nil {
		if q.From < 0 || q.From >= 24*60 || q.To < 0 || q.To >= 24*60 {
			return nil, domain.Errf(domain.CodeInvalidArgument, "quiet hours are minutes from midnight (0-1439)")
		}
		if q.Zone != "" {
			if _, err := time.LoadLocation(q.Zone); err != nil {
				return nil, domain.Errf(domain.CodeInvalidArgument, "unknown time zone %q", q.Zone).WithSuggestion("use an IANA zone, like Asia/Shanghai")
			}
		}
	}
	for t := range p.Tiers {
		if !validTier(t) {
			return nil, domain.Errf(domain.CodeInvalidArgument, "unknown tier %q", t)
		}
	}
	if len(p.Mutes) > 500 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "too many mutes (500 at most)")
	}
	now := s.clock.Now()
	p.Tidy(now)
	p.UpdatedAt = now
	raw, _ := json.Marshal(p)
	if err := s.settings.Set(ctx, noticePrefsKey(recipient), string(raw)); err != nil {
		return nil, err
	}
	return p, nil
}

func validTier(t domain.NoticeTier) bool {
	for _, x := range domain.NoticeTiers {
		if x == t {
			return true
		}
	}
	return false
}
