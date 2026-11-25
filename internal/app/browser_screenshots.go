package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/domain"
)

const browserScreenshotTTL = 7 * 24 * time.Hour

func (s *ActionService) SetBrowserScreenshots(repo domain.BrowserScreenshotRepository) {
	s.browserScreenshots = repo
}

func (s *ActionService) RunBrowserScreenshotPrune(ctx context.Context, log *slog.Logger) {
	if s.browserScreenshots == nil {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := s.browserScreenshots.Prune(ctx, s.clock.Now()); err != nil && ctx.Err() == nil {
			log.Warn("prune expired browser screenshots", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func screenshotDTO(a domain.BrowserScreenshot) *api.ActionArtifact {
	return &api.ActionArtifact{ID: a.ID, MediaType: a.MediaType, Bytes: a.Bytes, ExpiresAt: a.ExpiresAt}
}

func (s *ActionService) browserScreenshot(ctx context.Context, id, agent, owner string) (*api.ActionArtifact, []byte, error) {
	if s.browserScreenshots == nil {
		return nil, nil, domain.Errf(domain.CodeNotFound, "browser screenshot not found")
	}
	if err := s.enabled(ctx); err != nil {
		return nil, nil, err
	}
	a, err := s.browserScreenshots.Get(ctx, id, agent, owner, s.clock.Now())
	if err != nil {
		return nil, nil, err
	}
	return screenshotDTO(*a), a.Data, nil
}

func (s *ActionService) OwnerBrowserScreenshot(ctx context.Context, by Actor, id string) (*api.ActionArtifact, []byte, error) {
	m, err := person(by, "read browser screenshots")
	if err != nil {
		return nil, nil, err
	}
	return s.browserScreenshot(ctx, id, "", m.ID)
}

func (s *ActionService) OwnerBrowserScreenshots(ctx context.Context, by Actor) ([]*api.ActionArtifact, error) {
	m, err := person(by, "list browser screenshots")
	if err != nil {
		return nil, err
	}
	if err := s.enabled(ctx); err != nil {
		return nil, err
	}
	out := []*api.ActionArtifact{}
	if s.browserScreenshots == nil {
		return out, nil
	}
	rows, err := s.browserScreenshots.List(ctx, m.ID, s.clock.Now())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out = append(out, screenshotDTO(row))
	}
	return out, nil
}
