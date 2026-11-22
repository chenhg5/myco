package domain

import (
	"context"
	"strings"
	"time"
)

// Notices: what the hub thinks a person should hear about (通知), kept per
// person so every device of theirs agrees and the phone can be woken while
// it is locked. A notice says what happened and where it leads; whether it
// is announced (a banner, a sound, a push) is decided once, here, from the
// person's preferences, and carried with it. Devices only add what they
// alone know: that the person is looking at that very conversation.
//
// The notices themselves come from the hub's own events (a message, a
// workflow step, an approval) through sources; how they reach people
// (the App's feed, a push relay, later chat apps) is a sink. Neither knows
// the other: see app.NoticeService.

// NoticeKind is what a notice is about.
type NoticeKind string

const (
	// Needs you: something waits for the person.
	NoticeAsking   NoticeKind = "asking"   // an agent set a task aside to ask them
	NoticeDecision NoticeKind = "decision" // a workflow waits on their decision
	NoticeApproval NoticeKind = "approval" // their superagent asks for their OK
	NoticeFailed   NoticeKind = "failed"   // a task they sent did not get done
	NoticeRunFail  NoticeKind = "run_failed"
	NoticeStuck    NoticeKind = "stuck" // a task they sent seems stuck
	// Results: something came back.
	NoticeAnswered NoticeKind = "answered" // a task they sent was answered
	NoticeSaid     NoticeKind = "said"     // an agent wrote to them on its own
	NoticeMention  NoticeKind = "mention"  // named in a project's group
	// Activity: worth seeing, not worth a banner.
	NoticeProgress NoticeKind = "progress"
)

// NoticeKinds are every kind, needs-you first.
var NoticeKinds = []NoticeKind{NoticeAsking, NoticeDecision, NoticeApproval, NoticeFailed, NoticeRunFail, NoticeStuck,
	NoticeAnswered, NoticeSaid, NoticeMention, NoticeProgress}

// NoticeTier groups kinds by how much they matter, and so by default
// whether they are announced.
type NoticeTier string

const (
	TierNeedsYou NoticeTier = "needs_you"
	TierResults  NoticeTier = "results"
	TierActivity NoticeTier = "activity"
)

// Tiers in the order the App lists them.
var NoticeTiers = []NoticeTier{TierNeedsYou, TierResults, TierActivity}

// Tier is the kind's tier.
func (k NoticeKind) Tier() NoticeTier {
	switch k {
	case NoticeAsking, NoticeDecision, NoticeApproval, NoticeFailed, NoticeRunFail, NoticeStuck:
		return TierNeedsYou
	case NoticeAnswered, NoticeSaid, NoticeMention:
		return TierResults
	default:
		return TierActivity
	}
}

// Actionable kinds are settled by doing something (answering, deciding,
// approving), not by looking: reading one leaves it open.
func (k NoticeKind) Actionable() bool {
	return k == NoticeAsking || k == NoticeDecision || k == NoticeApproval
}

// DefaultTierOn is whether a tier is announced when the person never chose:
// only what needs them (the maintainers' call, 2026-11-22: a reply does
// not pop up by default).
func DefaultTierOn(t NoticeTier) bool { return t == TierNeedsYou }

// Notice is one thing a person should hear about.
type Notice struct {
	// Seq orders a person's notices; a device reads those after the last
	// it saw.
	Seq int64
	ID  string
	// Recipient is the person: a member's id, or, on a hub without
	// members, the console sender it was addressed to.
	Recipient string
	// Key says what it is about, so it is told once ("msg:<id>",
	// "decision:<step>", "approval:<id>"): a second notice with the same
	// key for the same person is not stored.
	Key  string
	Kind NoticeKind
	// Ref is what settles it: the task an ask or a failure is about, the
	// step a decision is for. Resolving a ref settles every notice on it.
	Ref string
	// Target is where it leads, as the App and the phone open places:
	// "agent:<name>", "project:<name>", "workflow:<id>?run=<run>",
	// "superagent".
	Target string
	// Project is the project it belongs to ("" for none): muting a project
	// mutes it.
	Project string
	// Actor is who it is from (an agent's name; "" for the hub itself).
	Actor string
	// Subject is a few words for it when the hub may know them (a step's
	// title, what an approval would do); a message's text is never copied
	// here (it may be sealed): MessageID points at it.
	Subject   string
	MessageID string
	// Announce is whether it is announced (banner, sound, push); Quiet,
	// when not, says why ("tier", "muted", "paused", "quiet_hours", "off").
	Announce  bool
	Quiet     string
	CreatedAt time.Time
	// ReadAt: the person saw it. ResolvedAt: it no longer waits for them
	// (answered, decided, or read, for kinds that are not actionable).
	ReadAt     time.Time
	ResolvedAt time.Time
}

// Tier is the notice's tier.
func (n *Notice) Tier() NoticeTier { return n.Kind.Tier() }

// Open reports whether it still waits for the person.
func (n *Notice) Open() bool { return n.ResolvedAt.IsZero() }

// Scopes are the mute keys that cover it: where it leads, its project,
// and its actor.
func (n *Notice) Scopes() []string {
	var out []string
	if n.Target != "" {
		t := n.Target
		if i := strings.IndexByte(t, '?'); i >= 0 {
			t = t[:i]
		}
		out = append(out, t)
	}
	if n.Project != "" {
		out = append(out, "project:"+n.Project)
	}
	if n.Actor != "" && "agent:"+n.Actor != n.Target {
		out = append(out, "agent:"+n.Actor)
	}
	return out
}

// QuietHours is a time of day nothing is announced, in the person's zone.
type QuietHours struct {
	// From and To are minutes from midnight; From later than To spans the
	// night (23:00 to 08:00).
	From int `json:"from"`
	To   int `json:"to"`
	// Zone is an IANA zone ("Asia/Shanghai"); "" or unknown: UTC.
	Zone string `json:"zone,omitempty"`
}

// Covers reports whether t falls in the quiet hours.
func (q QuietHours) Covers(t time.Time) bool {
	if q.From == q.To {
		return false
	}
	loc := time.UTC
	if q.Zone != "" {
		if l, err := time.LoadLocation(q.Zone); err == nil {
			loc = l
		}
	}
	lt := t.In(loc)
	m := lt.Hour()*60 + lt.Minute()
	if q.From < q.To {
		return m >= q.From && m < q.To
	}
	return m >= q.From || m < q.To
}

// NoticePrefs is what a person chose about being told, kept on the hub so
// every device and the push relay agree.
type NoticePrefs struct {
	// Off turns every announcement off (notices are still kept: the App
	// shows them as unread).
	Off bool `json:"off,omitempty"`
	// Tiers overrides the default per tier (DefaultTierOn).
	Tiers map[NoticeTier]bool `json:"tiers,omitempty"`
	// Quiet: nothing is announced in these hours (nil: none). Nothing
	// breaks through (the maintainers' call, 2026-11-22).
	Quiet *QuietHours `json:"quiet,omitempty"`
	// PausedUntil: nothing is announced before then.
	PausedUntil time.Time `json:"paused_until,omitzero"`
	// Mutes: scopes (Notice.Scopes) never announced, until when (zero:
	// until unmuted).
	Mutes     map[string]time.Time `json:"mutes,omitempty"`
	UpdatedAt time.Time            `json:"updated_at,omitzero"`
}

// TierOn reports whether tier t is announced.
func (p *NoticePrefs) TierOn(t NoticeTier) bool {
	if on, ok := p.Tiers[t]; ok {
		return on
	}
	return DefaultTierOn(t)
}

// Muted reports whether scope is muted at now.
func (p *NoticePrefs) Muted(scope string, now time.Time) bool {
	until, ok := p.Mutes[scope]
	return ok && (until.IsZero() || now.Before(until))
}

// Verdict is whether to announce n at now, and if not, why. The checks go
// from the broadest to the narrowest, so the reason is the one a person
// would look for first.
func (p *NoticePrefs) Verdict(n *Notice, now time.Time) (announce bool, quiet string) {
	switch {
	case p == nil:
		return DefaultTierOn(n.Tier()), tierReason(DefaultTierOn(n.Tier()))
	case p.Off:
		return false, "off"
	case !p.TierOn(n.Tier()):
		return false, "tier"
	}
	for _, s := range n.Scopes() {
		if p.Muted(s, now) {
			return false, "muted"
		}
	}
	if now.Before(p.PausedUntil) {
		return false, "paused"
	}
	if p.Quiet != nil && p.Quiet.Covers(now) {
		return false, "quiet_hours"
	}
	return true, ""
}

func tierReason(on bool) string {
	if on {
		return ""
	}
	return "tier"
}

// Tidy drops mutes that have run out.
func (p *NoticePrefs) Tidy(now time.Time) {
	for s, until := range p.Mutes {
		if !until.IsZero() && !now.Before(until) {
			delete(p.Mutes, s)
		}
	}
}

// NoticeFilter selects a person's notices.
type NoticeFilter struct {
	Recipient string
	// After: only those with a larger Seq.
	After int64
	// Open: only those still waiting.
	Open  bool
	Limit int
}

// NoticeRepository stores notices.
type NoticeRepository interface {
	// Add stores n and sets its Seq; it returns false (and stores nothing)
	// when the person already has a notice with its key.
	Add(ctx context.Context, n *Notice) (bool, error)
	// List returns a person's notices, oldest first.
	List(ctx context.Context, f NoticeFilter) ([]*Notice, error)
	// Last is the Seq of a person's newest notice (0: none).
	Last(ctx context.Context, recipient string) (int64, error)
	// Resolve settles every open notice with key or ref (either may be
	// ""), for everyone, and returns the people whose notices changed.
	Resolve(ctx context.Context, key, ref string, at time.Time) ([]string, error)
	// Read marks a person's notices up to through (0: all) as read, only
	// those leading to target when it is not "", and settles those that
	// are not actionable. It returns how many changed.
	Read(ctx context.Context, recipient, target string, through int64, at time.Time) (int, error)
	// OpenCount counts a person's open notices of a tier.
	OpenCount(ctx context.Context, recipient string, tier NoticeTier) (int, error)
	// Prune deletes notices created before then.
	Prune(ctx context.Context, before time.Time) error
}
