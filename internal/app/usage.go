package app

import (
	"context"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/domain"
)

type UsageService struct {
	repo     domain.UsageRepository
	agents   domain.AgentRepository
	messages *MessageService
	clock    Clock
}

func NewUsageService(repo domain.UsageRepository, agents domain.AgentRepository, messages *MessageService, clock Clock) *UsageService {
	return &UsageService{repo, agents, messages, clock}
}

const UsageValueMax = 1e11

func UsageDay(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil || t.Format("2006-01-02") != s {
		return time.Time{}, domain.Errf(domain.CodeInvalidArgument, "usage day must be YYYY-MM-DD in UTC")
	}
	return t, nil
}
func (s *UsageService) Record(ctx context.Context, a *domain.Agent, req api.UsageRequest) (int, error) {
	if req.Source == "" {
		req.Source = "runner"
	}
	if req.Source != "runner" && req.Source != "session" {
		return 0, domain.Errf(domain.CodeInvalidArgument, "usage source must be runner or session")
	}
	if len(req.Reports) > 100 {
		return 0, domain.Errf(domain.CodeInvalidArgument, "report at most 100 usage entries")
	}
	reports := append([]domain.UsageReport(nil), req.Reports...)
	for i := range reports {
		p := &reports[i]
		if _, err := UsageDay(p.Day); err != nil {
			return 0, err
		}
		for _, v := range []int64{p.Calls, p.InputTokens, p.CacheWriteTokens, p.CacheReadTokens, p.OutputTokens} {
			if v < 0 || v > UsageValueMax {
				return 0, domain.Errf(domain.CodeInvalidArgument, "usage numbers must be between 0 and 1e11")
			}
		}
		if math.IsNaN(p.CostUSD) || math.IsInf(p.CostUSD, 0) || p.CostUSD < 0 || p.CostUSD > UsageValueMax {
			return 0, domain.Errf(domain.CodeInvalidArgument, "invalid usage cost")
		}
		for _, text := range []string{p.Key, p.Model, p.Project, p.MessageID} {
			if len(text) > 256 || strings.IndexFunc(text, unicode.IsControl) >= 0 {
				return 0, domain.Errf(domain.CodeInvalidArgument, "usage identifiers must be at most 256 bytes without control characters")
			}
		}
		if p.Project == "" && p.MessageID != "" {
			m, err := s.messages.Get(ctx, a, p.MessageID)
			if err != nil {
				return 0, err
			}
			p.Project = m.Project
		}
	}
	return s.repo.Record(ctx, string(a.Name), req.Source, reports, s.clock.Now())
}
func (s *UsageService) List(ctx context.Context, by Actor, f domain.UsageFilter) (*api.UsageList, error) {
	if !by.Manages() {
		if by.Member == nil {
			return nil, domain.Errf(domain.CodePermissionDenied, "sign in as a workspace member to read usage")
		}
		f.Owner = by.Member.ID
	} else {
		f.Owner = ""
	}
	now := s.clock.Now().UTC()
	if f.To == "" {
		f.To = now.Format("2006-01-02")
	}
	to, err := UsageDay(f.To)
	if err != nil {
		return nil, err
	}
	if f.From == "" {
		f.From = to.AddDate(0, 0, -6).Format("2006-01-02")
	}
	from, err := UsageDay(f.From)
	if err != nil {
		return nil, err
	}
	if from.After(to) || to.Sub(from) > 365*24*time.Hour {
		return nil, domain.Errf(domain.CodeInvalidArgument, "usage range must cover 1 to 366 inclusive days")
	}
	rows, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, err
	}
	nodes, err := s.agents.List(ctx)
	if err != nil {
		return nil, err
	}
	out := &api.UsageList{From: f.From, To: f.To, Rows: rows, Agents: []api.UsageAgent{}}
	for _, a := range nodes {
		if f.Owner != "" && a.Owner != f.Owner {
			continue
		}
		out.Agents = append(out.Agents, api.UsageAgent{Name: string(a.Name), DisplayName: a.DisplayName, Owner: a.Owner, Harness: a.Binding.Harness, Machine: a.MachineID})
	}
	return out, nil
}
