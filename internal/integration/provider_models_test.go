package integration_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
)

// A runner on another vendor reports that vendor's models and where they
// came from; the App asks it to list them again without touching the
// model it chose.
func TestProviderModelsAndTheirRefreshGoThroughTheHub(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	agent := h.client(h.register(api.RegisterRequest{Name: "mac-claude", Binding: api.Binding{Harness: "claude", Environment: "mac"}}))
	if _, err := h.admin().PatchRunSettings(c, "mac-claude", api.RunSettingsPatch{Model: ptr("kimi-k3")}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 26, 15, 0, 0, 0, time.UTC)
	if err := agent.ReportRuntime(c, api.Runtime{CLI: "claude", Features: []string{"models_refresh"}, ModelsFrom: "provider", ModelsAt: &at,
		Models: []api.RuntimeModel{{ID: "deepseek-v4.1-flash", Name: "DeepSeek V4.1 Flash"}, {ID: "kimi-k3"}}}); err != nil {
		t.Fatal(err)
	}
	a, err := h.admin().GetAgent(c, "mac-claude")
	if err != nil {
		t.Fatal(err)
	}
	rt := a.Runtime
	if rt == nil || rt.ModelsFrom != "provider" || rt.ModelsAt == nil || !rt.ModelsAt.Equal(at) || len(rt.Models) != 2 {
		t.Fatalf("runtime = %+v", rt)
	}
	// A source the apps do not know, or no models: not passed on.
	if err := agent.ReportRuntime(c, api.Runtime{CLI: "claude", ModelsFrom: "somewhere", ModelsAt: &at}); err != nil {
		t.Fatal(err)
	}
	if a, _ = h.admin().GetAgent(c, "mac-claude"); a.Runtime.ModelsFrom != "" || a.Runtime.ModelsAt != nil {
		t.Fatalf("runtime = %+v", a.Runtime)
	}

	if _, err := h.admin().PatchRunSettings(c, "mac-claude", api.RunSettingsPatch{ModelsRefresh: ptr("2026-11-26T15:01:00Z")}); err != nil {
		t.Fatal(err)
	}
	me, err := agent.Me(c)
	if err != nil {
		t.Fatal(err)
	}
	if me.RunSettings == nil || me.RunSettings.ModelsRefresh != "2026-11-26T15:01:00Z" || me.RunSettings.Model != "kimi-k3" {
		t.Fatalf("run settings = %+v", me.RunSettings)
	}
	if _, err := h.admin().PatchRunSettings(c, "mac-claude", api.RunSettingsPatch{ModelsRefresh: ptr("x\ny")}); err == nil {
		t.Fatal("a refresh with a newline is taken")
	}
}
