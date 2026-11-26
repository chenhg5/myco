package console

import (
	"errors"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

func TestBrowserControlGrantVerificationGrace(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	expires := now.Add(time.Hour)
	active := []api.ActionGrant{{ID: "grant", State: api.GrantActive, ExpiresAt: &expires}}
	check := browserControlGrantCheck{}
	transport := errors.New("temporary transport failure")
	verify := func(at time.Time, grants []api.ActionGrant, err error, want string) {
		t.Helper()
		if got := check.reason(at, "grant", expires, grants, err); got != want {
			t.Fatalf("at %s: got %q, want %q", at, got, want)
		}
	}
	verify(now, nil, transport, "")
	verify(now.Add(29*time.Second), nil, transport, "")
	verify(now.Add(30*time.Second), nil, transport, "grant_check_failed")
	verify(now.Add(31*time.Second), active, nil, "")
	if !check.failedSince.IsZero() {
		t.Fatal("successful verification did not reset consecutive failure window")
	}
	// Missing entries and malformed active grants receive the same bounded
	// uncertainty window. Unrelated active grants cannot reset it.
	verify(now.Add(32*time.Second), []api.ActionGrant{{ID: "other", State: api.GrantActive, ExpiresAt: &expires}}, nil, "")
	verify(now.Add(61*time.Second), []api.ActionGrant{{ID: "grant", State: api.GrantActive}}, nil, "")
	verify(now.Add(62*time.Second), nil, nil, "grant_check_failed")
	verify(now.Add(63*time.Second), active, nil, "")
	verify(expires, nil, transport, "grant_expired")
}

func TestBrowserControlGrantTerminalStateImmediatelyStops(t *testing.T) {
	now := time.Now()
	expires := now.Add(time.Hour)
	past := now.Add(-time.Second)
	for _, test := range []struct {
		state string
		end   *time.Time
		want  string
	}{
		{api.GrantActive, &expires, ""},
		{api.GrantRevoked, &expires, "grant_ended"},
		{api.GrantEnded, nil, "grant_ended"},
		{api.GrantDenied, nil, "grant_ended"},
		{api.GrantExpired, nil, "grant_expired"},
		{api.GrantActive, &past, "grant_expired"},
	} {
		t.Run(test.state+test.want, func(t *testing.T) {
			check := browserControlGrantCheck{failedSince: now.Add(-time.Second)}
			grants := []api.ActionGrant{{ID: "unrelated", State: api.GrantRevoked}, {ID: "grant", State: test.state, ExpiresAt: test.end}}
			if got := check.reason(now, "grant", expires, grants, nil); got != test.want {
				t.Fatal(got, test.want)
			}
		})
	}
}
