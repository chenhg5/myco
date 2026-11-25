package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/domain"
)

// Cancelling a host poll during its visibility read must not end the grant
// or erase an export that has already been uploaded.
type refreshPageRepository struct {
	domain.PageRepository
	read func(context.Context) (*domain.Page, error)
}

func (r refreshPageRepository) Get(ctx context.Context, _ string) (*domain.Page, error) {
	return r.read(ctx)
}
func (r refreshPageRepository) Versions(context.Context, string) ([]*domain.PageVersion, error) {
	return nil, nil
}

func refreshFixture() (*ActionService, *actionGrant, Actor) {
	now := time.Now()
	expires := now.Add(time.Minute)
	g := &actionGrant{ActionGrant: api.ActionGrant{ID: "grant", Agent: "agent", State: api.GrantActive, Target: api.ActionTarget{Kind: "page", ID: "page"}, ExpiresAt: &expires}, agentOwner: "owner"}
	s := &ActionService{clock: &actionResourceClock{now: now}, wake: make(chan struct{}), grants: map[string]*actionGrant{"grant": g},
		hosts: map[string]*actionHost{"host": {owner: "owner", ActionHost: api.ActionHost{ID: "host", Generation: "generation", LastSeenAt: now}}},
		jobs:  map[string]*actionJob{"job": {hostID: "host", ActionJob: api.ActionJob{ID: "job", GrantID: "grant", State: api.JobRunning}}},
		arts:  map[string]*actionArtifact{"artifact": {grantID: "grant", data: []byte("export"), ActionArtifact: api.ActionArtifact{ID: "artifact", ExpiresAt: expires}}}}
	return s, g, Actor{Member: &domain.Member{ID: "owner"}}
}

func TestCancelledHostVisibilityReadKeepsGrantAndArtifact(t *testing.T) {
	for _, phase := range []string{"before read", "during read"} {
		t.Run(phase, func(t *testing.T) {
			s, g, owner := refreshFixture()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if phase == "before read" {
				cancel()
			}
			s.pages = &PageService{pages: refreshPageRepository{read: func(ctx context.Context) (*domain.Page, error) { cancel(); return nil, ctx.Err() }}}
			_, err := s.HostJob(ctx, owner, "job", "host", "generation", 0)
			if g.State != api.GrantActive || s.arts["artifact"] == nil {
				t.Fatalf("cancelled poll revoked grant or deleted export: state=%s reason=%s artifact=%v", g.State, g.EndReason, s.arts["artifact"])
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("poll error = %v, want context.Canceled", err)
			}
			s.pages = &PageService{pages: refreshPageRepository{read: func(context.Context) (*domain.Page, error) { return &domain.Page{ID: "page", Owner: "owner"}, nil }}}
			_, data, err := s.Artifact(context.Background(), &domain.Agent{Name: "agent"}, "artifact")
			if err != nil || string(data) != "export" {
				t.Fatalf("export lost after cancelled poll: %q %v", data, err)
			}
		})
	}
}

func TestRefreshStillClosesInvalidGrants(t *testing.T) {
	for _, reason := range []string{"permission lost", "storage failure", "expired", "encrypted"} {
		t.Run(reason, func(t *testing.T) {
			s, g, owner := refreshFixture()
			s.pages = &PageService{pages: refreshPageRepository{read: func(context.Context) (*domain.Page, error) {
				if reason == "storage failure" {
					return nil, errors.New("storage unavailable")
				}
				owner := "owner"
				if reason == "permission lost" {
					owner = "someone-else"
				}
				return &domain.Page{ID: "page", Owner: owner}, nil
			}}}
			if reason == "expired" {
				past := s.clock.Now().Add(-time.Second)
				g.ExpiresAt = &past
			}
			if reason == "encrypted" {
				s.e2e = func(context.Context) bool { return true }
			}
			state, err := s.HostJob(context.Background(), owner, "job", "host", "generation", 0)
			if err != nil || !state.Stop || g.State == api.GrantActive || s.arts["artifact"] != nil {
				t.Fatalf("invalid grant stayed usable: %+v %v grant=%s artifact=%v", state, err, g.State, s.arts["artifact"])
			}
		})
	}
}

type refreshMessageRepository struct {
	domain.MessageRepository
	read func(context.Context) (*domain.Message, error)
}

func (r refreshMessageRepository) Get(ctx context.Context, _ string) (*domain.Message, error) {
	return r.read(ctx)
}

func TestCancelledTaskReadKeepsGrantAndArtifact(t *testing.T) {
	s, g, owner := refreshFixture()
	g.Target.Kind = "machine"
	g.MessageID = "task"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.messages = &MessageService{messages: refreshMessageRepository{read: func(ctx context.Context) (*domain.Message, error) { cancel(); return nil, ctx.Err() }}}
	_, err := s.HostJob(ctx, owner, "job", "host", "generation", 0)
	if !errors.Is(err, context.Canceled) || g.State != api.GrantActive || s.arts["artifact"] == nil {
		t.Fatalf("cancelled task read ended grant: err=%v state=%s", err, g.State)
	}
}

func TestExpiredRequestCannotDeleteActionArtifact(t *testing.T) {
	s, g, _ := refreshFixture()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, _, err := s.Artifact(ctx, &domain.Agent{Name: "agent"}, "artifact")
	if !errors.Is(err, context.DeadlineExceeded) || g.State != api.GrantActive || s.arts["artifact"] == nil {
		t.Fatalf("expired request ended grant: err=%v state=%s", err, g.State)
	}
}

func TestCancelledCompletionDoesNotFinishJob(t *testing.T) {
	s, g, owner := refreshFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.CompleteJob(ctx, owner, "job", api.ActionJobComplete{HostID: "host", Generation: "generation", State: api.JobSucceeded})
	j := s.jobs["job"]
	if !errors.Is(err, context.Canceled) || j.State != api.JobRunning || j.FinishedAt != nil || g.State != api.GrantActive || s.arts["artifact"] == nil {
		t.Fatalf("cancelled completion changed job/grant: err=%v job=%+v grant=%s", err, j, g.State)
	}
}
