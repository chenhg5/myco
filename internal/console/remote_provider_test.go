package console_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/e2e"
)

func TestRemoteProviderAppSealsBeforeHub(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	inv, err := r.admin.InviteMember(ctx, api.InviteRequest{Name: "provider-owner"})
	if err != nil {
		t.Fatal(err)
	}
	guest, _ := client.New(r.hubURL, "")
	pair, err := guest.Pair(ctx, strings.Split(inv.Link, "/c/")[1], "owner-device")
	if err != nil {
		t.Fatal(err)
	}
	owner, _ := client.New(r.hubURL, pair.Token)
	reg, err := r.admin.Register(ctx, api.RegisterRequest{Name: "target", Binding: api.Binding{Harness: "claude", SessionID: "remote-provider-test"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	r.admin.SetAgentOwner(ctx, "target", "provider-owner")
	target, _ := client.New(r.hubURL, reg.Token)
	id, _ := e2e.NewIdentity()
	target.Sealer = &client.Sealer{Me: "target", ID: id}
	target.RegisterKeys(ctx)
	options := []api.ProviderOption{}
	if err := target.ReportRuntime(ctx, api.Runtime{CLI: "claude", Features: []string{"remote_provider"}, Providers: &options}); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.SetRunSettings(ctx, "target", api.RunSettings{Model: "kept-model", Effort: "high"}); err != nil {
		t.Fatal(err)
	}
	code, added, raw := r.do("POST", "/app/hubs", map[string]string{"url": r.hubURL, "token": pair.Token, "name": "Workspace"})
	if code != 200 && code != 201 {
		t.Fatalf("add: %d %s", code, raw)
	}
	hubID, _ := added["id"].(string)
	if hubID == "" {
		t.Fatalf("no hub id: %v", added)
	}
	endpoint := "/app/hubs/" + hubID + "/agents/target/provider"
	body := map[string]any{"manual": map[string]string{"base_url": "http://127.0.0.1:3425", "api_key": "app-private-provider-key", "model": "custom-model"}}
	code, m, raw := r.do("PUT", endpoint, body)
	if code != 400 || m["error"] != "e2e_required" || strings.Contains(string(raw), "app-private-provider-key") {
		t.Fatalf("no E2E: %d %s", code, raw)
	}
	if _, err := r.admin.SetHubSettings(ctx, api.HubSettings{E2E: true}); err != nil {
		t.Fatal(err)
	}
	code, _, raw = r.do("PUT", endpoint, body)
	if code != 200 || strings.Contains(string(raw), "app-private-provider-key") {
		t.Fatalf("sealed update: %d %s", code, raw)
	}
	me, err := target.Me(ctx)
	if err != nil || me.RunSettings.ProviderEncrypted == nil {
		t.Fatal("no encrypted provider delivered")
	}
	plain, err := target.OpenProvider(ctx, *me.RunSettings.ProviderEncrypted)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		APIKey string `json:"api_key"`
	}
	if json.Unmarshal(plain, &cfg) != nil || cfg.APIKey != "app-private-provider-key" {
		t.Fatal("local App did not seal the right settings")
	}
	if me.RunSettings.Model != "kept-model" || me.RunSettings.Effort != "high" {
		t.Fatal("custom provider reset model/effort")
	}
	code, _, raw = r.do("PUT", "/app/hubs/"+hubID+"/agents/target/run-settings", map[string]string{"provider": ""})
	if code != 200 {
		t.Fatalf("clear: %d %s", code, raw)
	}
	me, _ = target.Me(ctx)
	if me.RunSettings.ProviderEncrypted != nil || me.RunSettings.Model != "kept-model" || me.RunSettings.Effort != "high" {
		t.Fatal("partial provider update reset other settings")
	}
}
