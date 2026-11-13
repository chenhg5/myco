package app

import (
	"context"
	"strings"

	"github.com/chenhg5/myco/internal/domain"
)

const ResumeTokenPrefix = "myco_resume_"

type ResumeService struct {
	repo   domain.ResumeRepository
	agents domain.AgentRepository
	tokens TokenIssuer
	clock  Clock
}

func NewResumeService(repo domain.ResumeRepository, agents domain.AgentRepository, tokens TokenIssuer, clock Clock) *ResumeService {
	return &ResumeService{repo: repo, agents: agents, tokens: tokens, clock: clock}
}

func (s *ResumeService) Get(ctx context.Context, name domain.AgentName) (*domain.ResumeCredential, error) {
	return s.repo.Get(ctx, name)
}

func (s *ResumeService) Peek(ctx context.Context, plain string) (*domain.ResumeCredential, error) {
	if !strings.HasPrefix(plain, ResumeTokenPrefix) {
		return nil, domain.Errf(domain.CodeNotFound, "resume link not found")
	}
	return s.repo.GetByTokenHash(ctx, s.tokens.Hash(plain))
}

func (s *ResumeService) Create(ctx context.Context, name domain.AgentName, ifMissing bool) (*domain.ResumeCredential, string, error) {
	if _, err := s.agents.Get(ctx, name); err != nil {
		return nil, "", err
	}
	if ifMissing {
		if c, err := s.repo.Get(ctx, name); err == nil {
			return c, "", nil
		} else if !domain.IsCode(err, domain.CodeNotFound) {
			return nil, "", err
		}
	}
	plain, _, err := s.tokens.Issue()
	if err != nil {
		return nil, "", err
	}
	plain = ResumeTokenPrefix + strings.TrimPrefix(plain, "myco_")
	c := &domain.ResumeCredential{Agent: name, TokenHash: s.tokens.Hash(plain), CreatedAt: s.clock.Now()}
	if err := s.repo.Put(ctx, c); err != nil {
		return nil, "", err
	}
	return c, plain, nil
}

func (s *ResumeService) Revoke(ctx context.Context, name domain.AgentName) error {
	return s.repo.Delete(ctx, name)
}

func (s *ResumeService) Use(ctx context.Context, plain string, b domain.Binding) (*domain.Agent, string, error) {
	if !strings.HasPrefix(plain, ResumeTokenPrefix) {
		return nil, "", domain.Errf(domain.CodeUnauthenticated, "invalid resume token")
	}
	if err := b.Validate(); err != nil {
		return nil, "", err
	}
	if b.Harness == "" {
		return nil, "", domain.Errf(domain.CodeInvalidArgument, "binding harness is required")
	}
	newToken, hash, err := s.tokens.Issue()
	if err != nil {
		return nil, "", err
	}
	name, err := s.repo.Use(ctx, s.tokens.Hash(plain), b, hash, s.clock.Now())
	if err != nil {
		return nil, "", err
	}
	a, err := s.agents.Get(ctx, name)
	return a, newToken, err
}
