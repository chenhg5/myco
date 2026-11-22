package domain

import (
	"testing"
	"time"
	_ "time/tzdata"
)

func TestNoticeTiers(t *testing.T) {
	for k, want := range map[NoticeKind]NoticeTier{
		NoticeAsking: TierNeedsYou, NoticeDecision: TierNeedsYou, NoticeApproval: TierNeedsYou,
		NoticeFailed: TierNeedsYou, NoticeRunFail: TierNeedsYou, NoticeStuck: TierNeedsYou,
		NoticeAnswered: TierResults, NoticeSaid: TierResults, NoticeMention: TierResults,
		NoticeProgress: TierActivity, "something-new": TierActivity,
	} {
		if got := k.Tier(); got != want {
			t.Errorf("%s: tier %s, want %s", k, got, want)
		}
	}
	if !DefaultTierOn(TierNeedsYou) || DefaultTierOn(TierResults) || DefaultTierOn(TierActivity) {
		t.Error("only what needs you is announced by default")
	}
}

func TestQuietHours(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 11, 22, h, m, 0, 0, time.UTC) }
	night := QuietHours{From: 23 * 60, To: 8 * 60}
	for _, c := range []struct {
		t    time.Time
		want bool
	}{{at(23, 0), true}, {at(2, 0), true}, {at(7, 59), true}, {at(8, 0), false}, {at(12, 0), false}, {at(22, 59), false}} {
		if got := night.Covers(c.t); got != c.want {
			t.Errorf("night %v: %v, want %v", c.t, got, c.want)
		}
	}
	noon := QuietHours{From: 13 * 60, To: 14 * 60}
	if !noon.Covers(at(13, 30)) || noon.Covers(at(14, 0)) || noon.Covers(at(12, 59)) {
		t.Error("a span within a day")
	}
	if (QuietHours{From: 600, To: 600}).Covers(at(10, 0)) {
		t.Error("an empty span covers nothing")
	}
	// 15:00 UTC is 23:00 in Shanghai.
	sh := QuietHours{From: 23 * 60, To: 8 * 60, Zone: "Asia/Shanghai"}
	if !sh.Covers(at(15, 0)) || sh.Covers(at(1, 0)) {
		t.Error("hours are in the person's zone")
	}
	if !(QuietHours{From: 23 * 60, To: 8 * 60, Zone: "Not/AZone"}).Covers(at(23, 30)) {
		t.Error("an unknown zone reads as UTC")
	}
}

func TestNoticeVerdict(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	ask := &Notice{Kind: NoticeAsking, Target: "agent:codex", Project: "site", Actor: "codex"}
	reply := &Notice{Kind: NoticeAnswered, Target: "agent:codex"}
	check := func(name string, p *NoticePrefs, n *Notice, want bool, why string) {
		t.Helper()
		got, quiet := p.Verdict(n, now)
		if got != want || quiet != why {
			t.Errorf("%s: %v %q, want %v %q", name, got, quiet, want, why)
		}
	}
	check("no prefs: asks", nil, ask, true, "")
	check("no prefs: replies are quiet", nil, reply, false, "tier")
	check("defaults", &NoticePrefs{}, ask, true, "")
	check("results on", &NoticePrefs{Tiers: map[NoticeTier]bool{TierResults: true}}, reply, true, "")
	check("needs-you off", &NoticePrefs{Tiers: map[NoticeTier]bool{TierNeedsYou: false}}, ask, false, "tier")
	check("all off", &NoticePrefs{Off: true}, ask, false, "off")
	check("conversation muted", &NoticePrefs{Mutes: map[string]time.Time{"agent:codex": {}}}, ask, false, "muted")
	check("project muted", &NoticePrefs{Mutes: map[string]time.Time{"project:site": now.Add(time.Hour)}}, ask, false, "muted")
	check("mute ran out", &NoticePrefs{Mutes: map[string]time.Time{"project:site": now.Add(-time.Minute)}}, ask, true, "")
	check("paused", &NoticePrefs{PausedUntil: now.Add(time.Hour)}, ask, false, "paused")
	check("pause over", &NoticePrefs{PausedUntil: now.Add(-time.Hour)}, ask, true, "")
	check("quiet hours: nothing breaks through", &NoticePrefs{Quiet: &QuietHours{From: 11 * 60, To: 13 * 60}}, ask, false, "quiet_hours")
	check("outside quiet hours", &NoticePrefs{Quiet: &QuietHours{From: 23 * 60, To: 8 * 60}}, ask, true, "")
}

func TestNoticeScopesAndTidy(t *testing.T) {
	n := &Notice{Target: "workflow:wf1?run=r1", Project: "site", Actor: "codex"}
	got := n.Scopes()
	want := []string{"workflow:wf1", "project:site", "agent:codex"}
	if len(got) != len(want) {
		t.Fatalf("scopes %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scopes %v, want %v", got, want)
		}
	}
	if s := (&Notice{Target: "agent:codex", Actor: "codex"}).Scopes(); len(s) != 1 {
		t.Errorf("the actor is not repeated when it is the target: %v", s)
	}
	now := time.Now()
	p := &NoticePrefs{Mutes: map[string]time.Time{"a": {}, "b": now.Add(-time.Second), "c": now.Add(time.Hour)}}
	p.Tidy(now)
	if _, ok := p.Mutes["b"]; ok || len(p.Mutes) != 2 {
		t.Errorf("tidy: %v", p.Mutes)
	}
	if !NoticeAsking.Actionable() || NoticeFailed.Actionable() || NoticeAnswered.Actionable() {
		t.Error("actionable kinds")
	}
}
