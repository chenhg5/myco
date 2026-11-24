package app

import (
	"context"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/domain"
)

type actionResourceClock struct{ now time.Time }

func (c *actionResourceClock) Now() time.Time { return c.now }

func TestRunningJobPollKeepsBoundHostAlive(t *testing.T) {
	clock := &actionResourceClock{now: time.Now()}
	expires := clock.now.Add(10 * time.Minute)
	r := &api.ActionResource{Provider: "server.ssh", ID: "ssh_testprofile", Binding: "binding"}
	host := &actionHost{owner: "owner", ActionHost: api.ActionHost{ID: "host", MachineID: "machine", Provider: "server.ssh", Resource: r, Generation: "g", LastSeenAt: clock.now}}
	grant := &actionGrant{ActionGrant: api.ActionGrant{ID: "grant", State: api.GrantActive, Target: api.ActionTarget{Kind: "machine", ID: "machine", Resource: r}, ExpiresAt: &expires}, hostSeen: "host"}
	job := &actionJob{ActionJob: api.ActionJob{ID: "job", GrantID: "grant", State: api.JobRunning}, hostID: "host"}
	s := &ActionService{clock: clock, hosts: map[string]*actionHost{"host": host}, grants: map[string]*actionGrant{"grant": grant}, jobs: map[string]*actionJob{"job": job}}
	for range 4 {
		clock.now = clock.now.Add(30 * time.Second)
		state, err := s.HostJob(context.Background(), Actor{Member: &domain.Member{ID: "owner"}}, "job", "host", "g", 0)
		if err != nil || state.Stop || state.GrantState != api.GrantActive {
			t.Fatalf("live host ended between queue polls: %+v %v", state, err)
		}
	}
}

func TestSSHManageApprovalDoesNotUnderstatePermission(t *testing.T) {
	got := grantWhat("agent", "server", []string{"server.ssh.manage", "server.ssh.read"}, time.Minute)
	if got == grantWhat("agent", "server", []string{"server.ssh.read"}, time.Minute) {
		t.Fatal("manage approval described only read access")
	}
}
