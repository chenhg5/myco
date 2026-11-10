package integration_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
)

// The recent list grouped by pair: one row per two parties, whatever the
// number of tasks (threads) between them, with the newest task's state.
func TestRecentByPair(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	enroll := func(name string) *client.Client {
		enr, err := admin.CreateEnrollment(c, api.CreateEnrollmentRequest{NamePrefix: name, MaxUses: 1}, false)
		if err != nil {
			t.Fatal(err)
		}
		res, err := h.client(enr.Token).Enroll(c, api.EnrollRequest{Name: name, Binding: api.Binding{Harness: "claude", SessionID: "s-" + name}}, false)
		if err != nil {
			t.Fatal(err)
		}
		return h.client(res.Token)
	}
	lead, worker := enroll("lead"), enroll("worker")
	// Two tasks from the app to lead (two threads), two from lead to worker,
	// and one from worker back to lead.
	for _, b := range []string{"one", "two"} {
		if _, err := admin.ConsoleSend(c, api.SendRequest{To: "lead", Body: b}); err != nil {
			t.Fatal(err)
		}
	}
	for _, b := range []string{"a", "b"} {
		if _, err := lead.Send(c, api.SendRequest{To: "worker", Body: b}, false); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := worker.Send(c, api.SendRequest{To: "lead", Body: "question"}, false); err != nil {
		t.Fatal(err)
	}

	threads, err := admin.Recent(c, "", "", "", 0, 0)
	if err != nil || len(threads.Items) != 5 {
		t.Fatalf("threads: %v %d", err, len(threads.Items))
	}
	pairs, err := admin.RecentGrouped(c, "pair", "", "", "", "", 0, 0)
	if err != nil || len(pairs.Items) != 2 {
		t.Fatalf("pairs: %v %+v", err, pairs)
	}
	// Newest first: lead ↔ worker (3 messages, the newest task queued).
	lw := pairs.Items[0]
	if lw.Kind != "pair" || lw.Other != "lead" || lw.Node != "worker" || lw.Messages != 3 || lw.State != "queued" || lw.Task == nil || lw.Task.Body != "question" {
		t.Fatalf("lead-worker: %+v", lw)
	}
	// The person is always the Other side.
	cl := pairs.Items[1]
	if cl.Node != "lead" || cl.Other[:9] != "@console:" || cl.Messages != 2 {
		t.Fatalf("console-lead: %+v", cl)
	}
	// The pair's messages, every thread.
	ex, err := admin.Exchanges(c, cl.Other, "lead", 0, 0)
	if err != nil || len(ex.Messages) != 2 || ex.Messages[0].ThreadID == ex.Messages[1].ThreadID {
		t.Fatalf("exchanges: %v %+v", err, ex)
	}
}
