package app

import (
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// A task stuck behind a newer one that finished: the row's Task is the
// newest, Open the one still unfinished, so the app can tell it may be stuck.
func TestRecentRowKeepsAnOlderOpenTask(t *testing.T) {
	now := time.Now()
	stuck := &domain.Message{ID: "old", Type: domain.TypeTask, Status: domain.StatusAcked, From: "@console:glenn", To: "worker", CreatedAt: now.Add(-19 * time.Hour)}
	done := &domain.Message{ID: "new", Type: domain.TypeTask, Status: domain.StatusDone, From: "@console:glenn", To: "worker", CreatedAt: now.Add(-time.Hour)}

	it := threadItem(domain.ThreadRef{ID: "th"}, []*domain.Message{stuck, done}) // oldest first
	if it.Task != done || it.Open != stuck || it.State != "working" {
		t.Fatalf("thread: task=%v open=%v state=%q", it.Task, it.Open, it.State)
	}

	var pair RecentItem
	pair.anyDirection([]*domain.Message{done, stuck}) // newest first
	if pair.Task != done || pair.Open != stuck || pair.State != "working" {
		t.Fatalf("pair: task=%v open=%v state=%q", pair.Task, pair.Open, pair.State)
	}

	stuck.Status = domain.StatusFailed
	if it := threadItem(domain.ThreadRef{ID: "th"}, []*domain.Message{stuck, done}); it.Open != nil {
		t.Fatalf("nothing open, but Open = %v", it.Open)
	}
}
