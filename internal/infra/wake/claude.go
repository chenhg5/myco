package wake

import (
	"context"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// Claude Code routine ("scheduled task") with an API trigger. The fire URL
// and token come from the routine's settings page on claude.ai.
const (
	defaultAnthropicVersion = "2023-06-01"
	defaultRoutineBeta      = "experimental-cc-routine-2026-04-01"
)

func fireRoutine(ctx context.Context, h Doer, a *domain.Agent, text string) error {
	p := a.Delivery.Params
	headers := map[string]string{
		"Authorization":     "Bearer " + a.Delivery.Secret,
		"anthropic-version": paramOr(p, "anthropic_version", defaultAnthropicVersion),
	}
	if beta := paramOr(p, "anthropic_beta", defaultRoutineBeta); beta != "none" {
		headers["anthropic-beta"] = beta
	}
	_, err := postJSON(ctx, httpOr(h), a.Delivery.URL, headers, map[string]string{"text": text})
	return err
}

func paramOr(p map[string]string, k, def string) string {
	if v := p[k]; v != "" {
		return v
	}
	return def
}

// ClaudeRoutine fires a routine whose own fresh session *is* the agent
// (stateless worker: every wake starts a new session of that routine).
type ClaudeRoutine struct{ HTTP Doer }

var _ app.Waker = ClaudeRoutine{}

func (ClaudeRoutine) Mode() string   { return "claude_routine" }
func (ClaudeRoutine) Consumes() bool { return false }
func (ClaudeRoutine) Validate(a *domain.Agent) error {
	if err := requireURL(a); err != nil {
		return err
	}
	return requireSecret(a, "the routine API token")
}
func (c ClaudeRoutine) Wake(ctx context.Context, req app.WakeRequest) error {
	return fireRoutine(ctx, c.HTTP, req.Agent, req.Text)
}
