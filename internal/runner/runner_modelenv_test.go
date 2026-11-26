package runner

import (
	"reflect"
	"testing"

	"github.com/myco-ai/myco/internal/harness"
)

func TestModelEnvGoesUnderTheProvidersOwnVariables(t *testing.T) {
	vars := map[string]string{"ANTHROPIC_DEFAULT_HAIKU_MODEL": "small", "ANTHROPIC_DEFAULT_OPUS_MODEL": "main"}
	p := &harness.ProviderSettings{Env: map[string]string{"ANTHROPIC_BASE_URL": "https://x", "ANTHROPIC_DEFAULT_HAIKU_MODEL": "mine"}}
	req := harness.Request{Provider: p, Env: []string{"A=1"}}
	withModelEnv(&req, vars)
	want := map[string]string{"ANTHROPIC_BASE_URL": "https://x", "ANTHROPIC_DEFAULT_HAIKU_MODEL": "mine", "ANTHROPIC_DEFAULT_OPUS_MODEL": "main"}
	if !reflect.DeepEqual(req.Provider.Env, want) || len(p.Env) != 2 {
		t.Fatalf("provider env = %v (original %v)", req.Provider.Env, p.Env)
	}
	// No provider: before the environment, whose values win.
	req = harness.Request{Env: []string{"ANTHROPIC_DEFAULT_OPUS_MODEL=env"}}
	withModelEnv(&req, vars)
	if !reflect.DeepEqual(req.Env, []string{"ANTHROPIC_DEFAULT_HAIKU_MODEL=small", "ANTHROPIC_DEFAULT_OPUS_MODEL=main", "ANTHROPIC_DEFAULT_OPUS_MODEL=env"}) {
		t.Fatalf("env = %v", req.Env)
	}
}
