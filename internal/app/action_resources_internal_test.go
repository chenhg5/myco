package app

import (
	"context"
	"strings"
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

func TestBrowserPermissionsAreIndependent(t *testing.T) {
	for _, permission := range []string{"browser.view", "browser.run", "browser.control"} {
		got, err := checkPermissions([]string{permission}, "machine")
		if err != nil || len(got) != 1 || got[0] != permission {
			t.Fatalf("%s: %v %v", permission, got, err)
		}
	}
	if _, err := checkPermissions([]string{"browser.*"}, "machine"); err == nil {
		t.Fatal("wildcard accepted")
	}
	if _, err := checkPermissions([]string{"browser.run"}, "page"); err == nil {
		t.Fatal("page accepted browser permission")
	}
}

func TestBrowserControlWarning(t *testing.T) {
	for _, perms := range [][]string{{"browser.control"}, {"browser.control", "browser.view", "browser.run"}} {
		if !strings.Contains(grantWhat("agent", "profile", perms, time.Minute), "能看到并操作这个 profile 登录的所有网站") {
			t.Fatal("control warning missing")
		}
	}
}

func TestBrowserResourceValidation(t *testing.T) {
	valid := api.ActionResource{Provider: "browser", ID: "profile-one", Binding: strings.Repeat("a", 64), Name: "Work"}
	if err := validateProfileResource("browser", &valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*api.ActionResource){
		func(r *api.ActionResource) { r.Binding = "" }, func(r *api.ActionResource) { r.Provider = "server.ssh" },
		func(r *api.ActionResource) { r.ID = "../profile" }, func(r *api.ActionResource) { r.Name = "profile\nspoof" },
	} {
		r := valid
		mutate(&r)
		if validateProfileResource("browser", &r) == nil {
			t.Fatalf("accepted invalid resource: %+v", r)
		}
	}
}

func TestBrowserTargetsVisibilityAndE2E(t *testing.T) {
	clock := &actionResourceClock{now: time.Now()}
	host := func(owner, provider string, seen time.Time) *actionHost {
		return &actionHost{owner: owner, ActionHost: api.ActionHost{MachineID: "machine", Provider: provider, Resource: &api.ActionResource{Provider: provider, ID: "same-id", Binding: "policy"}, LastSeenAt: seen}}
	}
	s := &ActionService{clock: clock, hosts: map[string]*actionHost{
		"browser": host("me", "browser", clock.now), "ssh": host("me", "server.ssh", clock.now),
		"other": host("other", "browser", clock.now), "stale": host("me", "browser", clock.now.Add(-time.Minute)),
	}}
	targets, err := s.ResourceTargets(context.Background(), &domain.Agent{Owner: "me"})
	if err != nil || len(targets) != 2 {
		t.Fatalf("private/stale/provider dedup: %+v %v", targets, err)
	}
	s.e2e = func(context.Context) bool { return true }
	if _, err := s.ResourceTargets(context.Background(), &domain.Agent{Owner: "me"}); !domain.IsCode(err, domain.CodeUnsupported) {
		t.Fatalf("encrypted discovery: %v", err)
	}
	if _, err := s.RequestGrant(context.Background(), &domain.Agent{Owner: "me"}, api.ActionGrantRequest{Permissions: []string{"browser.run"}}, false); !domain.IsCode(err, domain.CodeUnsupported) {
		t.Fatalf("encrypted grant: %v", err)
	}
	if _, err := s.RegisterHost(context.Background(), Actor{Member: &domain.Member{ID: "me"}}, api.ActionHostRegister{Provider: "browser"}); !domain.IsCode(err, domain.CodeUnsupported) {
		t.Fatalf("encrypted host: %v", err)
	}
}

func TestEncryptionToggleRevokesExistingBrowserGrant(t *testing.T) {
	clock := &actionResourceClock{now: time.Now()}
	expiry := clock.now.Add(time.Minute)
	grant := &actionGrant{ActionGrant: api.ActionGrant{ID: "grant", State: api.GrantActive, ExpiresAt: &expiry}}
	job := &actionJob{ActionJob: api.ActionJob{ID: "job", GrantID: "grant", State: api.JobRunning}}
	s := &ActionService{wake: make(chan struct{}), clock: clock, e2e: func(context.Context) bool { return true }, grants: map[string]*actionGrant{"grant": grant}, jobs: map[string]*actionJob{"job": job}}
	s.refresh(context.Background(), grant)
	if grant.State != api.GrantRevoked || !job.CancelRequested {
		t.Fatal("encryption toggle kept a running action authorized")
	}
}
