package console

import (
	"context"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

// fakeNotices pages a fixed feed the way the hub does: after a seq, at
// most limit, Last being the newest overall.
type fakeNotices struct {
	all   []api.Notice
	badge int
	calls int
}

func (f *fakeNotices) Notices(_ context.Context, after int64, limit int, _ time.Duration) (*api.NoticeFeed, error) {
	f.calls++
	out := &api.NoticeFeed{Badge: f.badge}
	for _, n := range f.all {
		if n.Seq > after && len(out.Notices) < limit {
			out.Notices = append(out.Notices, n)
		}
	}
	if len(f.all) > 0 {
		out.Last = f.all[len(f.all)-1].Seq
	}
	return out, nil
}

func TestOpenNoticesReadsThroughAndKeepsOpenNeedsYou(t *testing.T) {
	now := time.Now()
	f := &fakeNotices{badge: 2}
	for i := int64(1); i <= 450; i++ {
		n := api.Notice{Seq: i, ID: "n", Tier: "results"}
		switch i {
		case 3: // open, needs the person
			n.Tier, n.Kind, n.ID = "needs_you", "asking", "ask"
		case 200: // needs the person, but settled
			n.Tier, n.Kind, n.ResolvedAt = "needs_you", "failed", &now
		case 449: // open, needs the person (on the third page)
			n.Tier, n.Kind, n.ID = "needs_you", "stuck", "stuck"
		}
		f.all = append(f.all, n)
	}
	open, badge, err := openNotices(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if badge != 2 {
		t.Fatalf("badge = %d, want 2", badge)
	}
	if len(open) != 2 || open[0].ID != "stuck" || open[1].ID != "ask" {
		t.Fatalf("open = %+v, want stuck then ask (newest first)", open)
	}
	if f.calls != 3 {
		t.Fatalf("read the feed in %d pages, want 3", f.calls)
	}
}

func TestOpenNoticesEmptyFeed(t *testing.T) {
	open, badge, err := openNotices(context.Background(), &fakeNotices{})
	if err != nil || len(open) != 0 || badge != 0 {
		t.Fatalf("open=%v badge=%d err=%v", open, badge, err)
	}
}
