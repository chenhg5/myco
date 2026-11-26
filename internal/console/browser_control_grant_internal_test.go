package console

import (
	"errors"
	"github.com/myco-ai/myco/internal/api"
	"testing"
	"time"
)

func TestBrowserControlGrantIsIndependentOfCompletedJob(t *testing.T) {
	future := time.Now().Add(time.Minute)
	past := time.Now().Add(-time.Second)
	for _, test := range []struct {
		name  string
		state *api.ActionHostJobState
		err   error
		want  string
	}{
		{"completed-open", &api.ActionHostJobState{Stop: true, Job: api.ActionJob{State: api.JobSucceeded}, GrantState: api.GrantActive, GrantExpiresAt: &future}, nil, ""},
		{"completed-run", &api.ActionHostJobState{Stop: true, Job: api.ActionJob{State: api.JobFailed}, GrantState: api.GrantActive, GrantExpiresAt: &future}, nil, ""},
		{"revoked", &api.ActionHostJobState{GrantState: api.GrantRevoked}, nil, "grant_ended"},
		{"expired", &api.ActionHostJobState{GrantState: api.GrantActive, GrantExpiresAt: &past}, nil, "grant_expired"},
		{"unknown", nil, errors.New("transport failed"), "grant_check_failed"},
		{"missing-expiry", &api.ActionHostJobState{GrantState: api.GrantActive}, nil, "grant_check_failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := browserControlGrantReason(test.state, test.err); got != test.want {
				t.Fatal(got, test.want)
			}
		})
	}
}
