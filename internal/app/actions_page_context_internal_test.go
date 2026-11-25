package app

import (
	"context"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/domain"
)

type contextPageRepo struct {
	domain.PageRepository
	get      func(context.Context) (*domain.Page, error)
	versions func(context.Context) ([]*domain.PageVersion, error)
}

func (r contextPageRepo) Get(ctx context.Context, _ string) (*domain.Page, error) {
	return r.get(ctx)
}
func (r contextPageRepo) Versions(ctx context.Context, _ string) ([]*domain.PageVersion, error) {
	return r.versions(ctx)
}

func TestCancelledHostPollPreservesPageGrantAndArtifact(t *testing.T) {
	for _, phase := range []string{"before lookup", "during page lookup", "during versions lookup", "visibility revoked"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			clock := &actionResourceClock{now: time.Now()}
			expires := clock.now.Add(time.Minute)
			repo := contextPageRepo{
				get: func(ctx context.Context) (*domain.Page, error) {
					if phase == "visibility revoked" {
						return nil, domain.Errf(domain.CodeNotFound, "page is no longer visible")
					}
					if phase == "during page lookup" {
						cancel()
					}
					if err := ctx.Err(); err != nil {
						return nil, err
					}
					return &domain.Page{ID: "pg_fixture", Publisher: "maker"}, nil
				},
				versions: func(ctx context.Context) ([]*domain.PageVersion, error) {
					if phase == "during versions lookup" {
						cancel()
					}
					return nil, ctx.Err()
				},
			}
			grant := &actionGrant{ActionGrant: api.ActionGrant{ID: "grant", Agent: "maker", State: api.GrantActive,
				Target: api.ActionTarget{Kind: "page", ID: "pg_fixture"}, ExpiresAt: &expires}}
			artifact := &actionArtifact{ActionArtifact: api.ActionArtifact{ID: "artifact", ExpiresAt: expires}, grantID: "grant", jobID: "job", data: []byte("<svg/>")}
			s := &ActionService{clock: clock, pages: &PageService{pages: repo}, wake: make(chan struct{}),
				grants: map[string]*actionGrant{"grant": grant}, arts: map[string]*actionArtifact{"artifact": artifact},
				hosts: map[string]*actionHost{"host": {owner: "owner", ActionHost: api.ActionHost{ID: "host", Generation: "g"}}},
				jobs:  map[string]*actionJob{"job": {hostID: "host", ActionJob: api.ActionJob{ID: "job", GrantID: "grant", State: api.JobSucceeded}}}}
			if phase == "before lookup" {
				cancel()
			}
			_, err := s.HostJob(ctx, Actor{Member: &domain.Member{ID: "owner"}}, "job", "host", "g", 0)
			if err != nil {
				t.Fatal(err)
			}
			if phase == "visibility revoked" {
				if grant.State != api.GrantEnded || len(s.arts) != 0 {
					t.Fatal("real visibility loss kept the grant or artifact")
				}
				return
			}
			if grant.State != api.GrantActive || s.arts["artifact"] == nil {
				t.Fatalf("cancelled poll destroyed shared state: grant=%s reason=%s artifact=%v", grant.State, grant.EndReason, s.arts["artifact"])
			}
			_, data, err := s.Artifact(context.Background(), &domain.Agent{Name: "maker"}, "artifact")
			if err != nil || string(data) != "<svg/>" {
				t.Fatalf("completed export is no longer readable: %q %v", data, err)
			}
		})
	}
}
