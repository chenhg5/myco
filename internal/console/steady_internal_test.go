package console

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/client"
)

// One failed check does not turn a hub offline; a hub that keeps failing
// does go offline.
func TestSteadyKeepsLastGoodAnswerBriefly(t *testing.T) {
	var s steady
	t0 := time.Unix(1_800_000_000, 0)
	good := HubStatus{Hub: Hub{ID: "h"}, OK: true, HubVersion: "v1"}
	bad := HubStatus{Hub: Hub{ID: "h"}, Error: "timeout"}
	slow := &client.Error{Code: "timeout", Message: "deadline exceeded"}
	if got := s.hubStatus(good, nil, t0); !got.OK {
		t.Fatal("good answer not passed through")
	}
	if got := s.hubStatus(bad, slow, t0.Add(5*time.Second)); !got.OK || got.HubVersion != "v1" {
		t.Fatalf("one failed check shows the hub offline: %+v", got)
	}
	if got := s.hubStatus(bad, errors.New("connection refused"), t0.Add(steadyFor+time.Second)); got.OK {
		t.Fatalf("a hub failing for longer still shows as online: %+v", got)
	}
	// Another hub has no history: its failure shows.
	if got := s.hubStatus(HubStatus{Hub: Hub{ID: "other"}}, slow, t0); got.OK {
		t.Fatal("an unknown hub shows as online")
	}
	ov := HubOverview{ID: "h", OK: true}
	s.hubOverview(ov, nil, t0)
	if got := s.hubOverview(HubOverview{ID: "h", Error: "x"}, slow, t0.Add(time.Second)); !got.OK {
		t.Fatal("overview: one failure hides the hub")
	}
	// A refused token is an answer, not a hiccup: it shows at once.
	denied := &client.Error{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "revoked"}
	if got := s.hubOverview(HubOverview{ID: "h", Error: "revoked"}, denied, t0.Add(2*time.Second)); got.OK {
		t.Fatal("a revoked token is hidden behind the last good overview")
	}
}
