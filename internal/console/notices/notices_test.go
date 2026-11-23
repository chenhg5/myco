package notices

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
)

// fakeFeed is a hub's feed: notices appended by the test, read like the
// hub's long-poll (returns at once when there is news, else after a short
// wait).
type fakeFeed struct {
	mu      sync.Mutex
	list    []api.Notice
	badge   int
	reads   []string
	old     bool
	changed chan struct{}
}

func newFeed() *fakeFeed { return &fakeFeed{changed: make(chan struct{}, 16)} }

func (f *fakeFeed) add(n api.Notice) {
	f.mu.Lock()
	n.Seq = int64(len(f.list) + 1)
	f.list = append(f.list, n)
	f.mu.Unlock()
	f.changed <- struct{}{}
}

func (f *fakeFeed) Notices(ctx context.Context, after int64, limit int, wait time.Duration) (*api.NoticeFeed, error) {
	if f.old {
		return nil, &client.Error{Status: 404, Code: "not_found", Message: "404 page not found"}
	}
	for {
		f.mu.Lock()
		out := &api.NoticeFeed{Last: int64(len(f.list)), Badge: f.badge}
		for _, n := range f.list {
			if n.Seq > after {
				out.Notices = append(out.Notices, n)
			}
		}
		f.mu.Unlock()
		if limit > 0 && len(out.Notices) > limit {
			out.Notices = out.Notices[:limit]
		}
		if len(out.Notices) > 0 || wait == 0 {
			return out, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-f.changed:
		case <-time.After(wait):
			return out, nil
		}
	}
}

func (f *fakeFeed) ReadNotices(_ context.Context, target string, _ int64) (int, error) {
	f.mu.Lock()
	f.reads = append(f.reads, target)
	f.mu.Unlock()
	return 1, nil
}

type fakePresenter struct {
	mu        sync.Mutex
	shown     []Toast
	withdrawn []string
	badge     int
}

func (p *fakePresenter) Show(t Toast) error {
	p.mu.Lock()
	p.shown = append(p.shown, t)
	p.mu.Unlock()
	return nil
}
func (p *fakePresenter) Withdraw(ids ...string) {
	p.mu.Lock()
	p.withdrawn = append(p.withdrawn, ids...)
	p.mu.Unlock()
}
func (p *fakePresenter) Badge(n int) {
	p.mu.Lock()
	p.badge = n
	p.mu.Unlock()
}

func (p *fakePresenter) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.shown)
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func newWatcher(t *testing.T, hubs []Hub, p Presenter, state string) *Watcher {
	w := New(Options{Hubs: func() []Hub { return hubs }, Present: p, Lang: func() string { return "zh" }, StatePath: state})
	w.wait, w.retryMin, w.oldRetry, w.rescan = 200*time.Millisecond, 10*time.Millisecond, time.Hour, time.Hour
	t.Cleanup(w.Stop)
	return w
}

func hubOf(id string, f Feed) Hub {
	return Hub{ID: id, Name: "工作区 " + id, Feed: func(context.Context) (Feed, error) { return f, nil }}
}

func ask(actor, body string) api.Notice {
	return api.Notice{Kind: "asking", Tier: "needs_you", Target: "agent:" + actor, Actor: actor, Announce: true,
		Message: &api.Message{Body: body + "\n1. 是\n2. 否", FromDisplay: "小 " + actor}}
}

func TestWatcherAnnouncesWhatIsNew(t *testing.T) {
	feed := newFeed()
	feed.add(ask("codex", "旧问题")) // there before the App: not news
	p := &fakePresenter{}
	state := filepath.Join(t.TempDir(), "notices.json")
	w := newWatcher(t, []Hub{hubOf("h1", feed)}, p, state)
	w.Start()
	eventually(t, "first read", func() bool { return w.State().Hubs["h1"].Supported })

	feed.mu.Lock()
	feed.badge = 1
	feed.mu.Unlock()
	feed.add(ask("codex", "要删旧页面吗？"))
	eventually(t, "a banner", func() bool { return p.count() == 1 })
	got := p.shown[0]
	if got.Title != "小 codex" || got.Body != "问你：要删旧页面吗？" || !got.Sound || got.ID != "h1|agent:codex" || got.Subtitle != "" {
		t.Fatalf("banner: %+v", got)
	}

	// Kept but not announced (the hub decided), and a second question in
	// the same conversation merges into its banner.
	quiet := ask("codex", "x")
	quiet.Announce, quiet.Kind, quiet.Tier = false, "answered", "results"
	feed.add(quiet)
	feed.add(ask("codex", "第二个问题"))
	eventually(t, "merged banner", func() bool { return p.count() == 2 })
	if b := p.shown[1].Body; b != "2 条新消息 · 问你：第二个问题" || p.shown[1].ID != p.shown[0].ID {
		t.Fatalf("merged: %+v", p.shown[1])
	}
	eventually(t, "badge", func() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.badge == 1 })

	// Looking at the conversation: its banner goes, it is read on the hub,
	// and what comes there while it is on screen is not announced.
	w.SetFocus(context.Background(), Focus{Front: true, Hub: "h1", Target: "agent:codex"})
	feed.add(ask("codex", "又一个"))
	eventually(t, "read on the hub", func() bool { feed.mu.Lock(); defer feed.mu.Unlock(); return len(feed.reads) >= 2 })
	time.Sleep(50 * time.Millisecond)
	if p.count() != 2 || len(p.withdrawn) != 1 {
		t.Fatalf("on screen: %d shown, withdrawn %v", p.count(), p.withdrawn)
	}
	// Another conversation still is.
	w.SetFocus(context.Background(), Focus{Front: true, Hub: "h1", Target: "agent:claude"})
	feed.add(ask("codex", "窗口在前面，但不是这个会话"))
	eventually(t, "another conversation", func() bool { return p.count() == 3 })
	if b := p.shown[2].Body; b != "问你：窗口在前面，但不是这个会话" {
		t.Fatalf("a new burst after reading starts over: %q", b)
	}

	// After a restart, what came meanwhile is told, and nothing twice.
	w.Stop()
	feed.add(ask("claude", "重启期间来的"))
	p2 := &fakePresenter{}
	w2 := newWatcher(t, []Hub{hubOf("h1", feed)}, p2, state)
	w2.Start()
	eventually(t, "after restart", func() bool { return p2.count() == 1 })
	time.Sleep(50 * time.Millisecond)
	if p2.count() != 1 || p2.shown[0].Body != "问你：重启期间来的" {
		t.Fatalf("after restart: %+v", p2.shown)
	}
}

func TestWatcherOlderHubAndSeveral(t *testing.T) {
	old := newFeed()
	old.old = true
	feed := newFeed()
	p := &fakePresenter{}
	w := newWatcher(t, []Hub{hubOf("old", old), hubOf("new", feed)}, p, "")
	w.Start()
	eventually(t, "states", func() bool {
		s := w.State()
		return s.Hubs["new"].Supported && !s.Hubs["old"].Supported && len(s.Hubs) == 2
	})
	n := api.Notice{Kind: "decision", Tier: "needs_you", Target: "workflow:wf1?run=r1", Subject: "发布审核", Announce: true}
	feed.add(n)
	eventually(t, "a banner", func() bool { return p.count() == 1 })
	if got := p.shown[0]; got.Title != "工作区 new" || got.Subtitle != "工作区 new" || got.Body != "等你决定：发布审核" || got.ID != "new|workflow:wf1" {
		t.Fatalf("with two workspaces: %+v", got)
	}
}

func TestWords(t *testing.T) {
	for _, c := range []struct {
		n    api.Notice
		zh   bool
		want string
	}{
		{api.Notice{Kind: "answered", Message: &api.Message{Body: "  done\nmore"}}, false, "Replied: done"},
		{api.Notice{Kind: "said", Message: &api.Message{Body: "myco-e2e:abc"}}, true, "有一条新消息"},
		{api.Notice{Kind: "approval", Subject: "删除项目 old"}, true, "请你批准：删除项目 old"},
		{api.Notice{Kind: "failed", Message: &api.Message{Body: string(make([]rune, 0)) + longText()}}, false, "Didn't get done: " + longText()[:139*1] + "…"},
	} {
		if got := Words(&c.n, c.zh); got != c.want {
			t.Errorf("%s: %q, want %q", c.n.Kind, got, c.want)
		}
	}
}

func longText() string {
	b := make([]byte, 200)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}

func (p *fakePresenter) badgeNow() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.badge
}

// withBadge sets the hub's count without waking a reader (the next notice does).
func (f *fakeFeed) withBadge(n int) {
	f.mu.Lock()
	f.badge = n
	f.mu.Unlock()
}

func (f *fakeFeed) setBadge(n int) {
	f.mu.Lock()
	f.badge = n
	f.mu.Unlock()
	f.changed <- struct{}{}
}

// The Dock counts what came since the App was last in front: bringing it
// to the front clears the number; what comes after shows again.
func TestWatcherBadgeClearsWhenTheAppIsInFront(t *testing.T) {
	f := newFeed()
	f.badge = 2
	p := &fakePresenter{}
	w := newWatcher(t, []Hub{hubOf("h1", f)}, p, filepath.Join(t.TempDir(), "state.json"))
	w.Start()
	eventually(t, "the badge shows what waits", func() bool { return p.badgeNow() == 2 })

	w.SetFocus(context.Background(), Focus{Front: true})
	if p.badgeNow() != 0 {
		t.Fatalf("badge = %d with the App in front, want 0", p.badgeNow())
	}
	// Still in front: a new one does not show.
	f.withBadge(3)
	f.add(ask("codex", "要不要发版？"))
	time.Sleep(50 * time.Millisecond)
	if p.badgeNow() != 0 {
		t.Fatalf("badge = %d while in front, want 0", p.badgeNow())
	}
	// Away: what comes now counts.
	w.SetFocus(context.Background(), Focus{Front: false})
	f.withBadge(4)
	f.add(ask("codex", "再确认一下？"))
	eventually(t, "a new one after leaving shows", func() bool { return p.badgeNow() == 1 })
	// One settled elsewhere (fewer than were seen): never below zero.
	f.setBadge(1)
	eventually(t, "settled ones do not go negative", func() bool { return p.badgeNow() == 0 })
	f.withBadge(2)
	f.add(ask("codex", "第三个问题"))
	eventually(t, "the next one shows as one", func() bool { return p.badgeNow() == 1 })
}
