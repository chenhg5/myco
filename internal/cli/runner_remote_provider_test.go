package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/e2e"
	"github.com/myco-ai/myco/internal/hubserver"
	"github.com/myco-ai/myco/internal/provider"
)

type providerLog struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *providerLog) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(b)
}
func (l *providerLog) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func TestRemoteProviderRealHubPrivateDelivery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CC_SWITCH_DIR", filepath.Join(home, "cc"))
	ctx := context.Background()
	db := filepath.Join(home, "hub.db")
	logs := &providerLog{}
	hub, err := hubserver.Build(hubserver.Options{DBPath: db, AdminToken: "admin-test-token", Policy: app.DefaultMessagePolicy(), Log: slog.New(slog.NewTextHandler(logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(hub.Handler)
	defer func() { srv.Close(); hub.Close() }()
	mk := func(token string) *client.Client {
		c, err := client.New(srv.URL, token)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	admin := mk("admin-test-token")
	device := func(name string) *client.Client {
		inv, err := admin.InviteMember(ctx, api.InviteRequest{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		pair, err := mk("").Pair(ctx, strings.Split(inv.Link, "/c/")[1], name+"-device")
		if err != nil {
			t.Fatal(err)
		}
		c := mk(pair.Token)
		id, _ := e2e.NewIdentity()
		c.Sealer = &client.Sealer{Me: "@console:" + name, ID: id, Device: true}
		if err := c.RegisterKeys(ctx); err != nil {
			t.Fatal(err)
		}
		return c
	}
	owner, other := device("alice"), device("bob")
	if _, err := admin.UpdateMember(ctx, "bob", api.MemberUpdateRequest{Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	reg, err := admin.Register(ctx, api.RegisterRequest{Name: "target", Binding: api.Binding{Harness: "codex", SessionID: "target-session"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.SetAgentOwner(ctx, "target", "alice"); err != nil {
		t.Fatal(err)
	}
	agent := mk(reg.Token)
	id, _ := e2e.NewIdentity()
	agent.Sealer = &client.Sealer{Me: "target", ID: id}
	options := []api.ProviderOption{{Ref: "cc-switch:local", Name: "Local provider", Host: "local.test", Model: "m"}}
	if err := agent.ReportRuntime(ctx, api.Runtime{CLI: "codex", Features: []string{"remote_provider"}, Providers: &options}); err != nil {
		t.Fatal(err)
	}
	ref := "cc-switch:local"
	for _, c := range []*client.Client{other, admin, agent} {
		if _, err := c.PatchRunSettings(ctx, "target", api.RunSettingsPatch{Provider: &ref}); err == nil {
			t.Fatal("non-owner changed provider")
		} else {
			var apiErr *client.Error
			if !errors.As(err, &apiErr) || apiErr.Code != "permission_denied" {
				t.Fatalf("expected owner denial, got %v", err)
			}
		}
	}
	if _, err := owner.SetRunSettings(ctx, "target", api.RunSettings{Model: "owner-model", Effort: "high"}); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.PatchRunSettings(ctx, "target", api.RunSettingsPatch{Provider: &ref}); err != nil {
		t.Fatal(err)
	}
	me, err := agent.Me(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if me.RunSettings.Model != "owner-model" || me.RunSettings.Effort != "high" {
		t.Fatal("provider update reset model/effort")
	}
	// No encryption means no plaintext fallback, even with keys registered.
	secret := "remote-super-private-provider-key"
	m := provider.Manual{BaseURL: "https://gateway.test/v1", APIKey: secret, Model: "custom-model", WireAPI: "responses"}
	raw, _ := json.Marshal(m)
	if _, err := owner.SealProvider(ctx, "target", raw); err == nil {
		t.Fatal("sealed on unencrypted workspace")
	}
	offID := "off-request-identifier"
	offBody, _ := e2e.Seal(owner.Sealer.ID, e2e.Header{From: owner.Sealer.Me, To: "target", Type: e2e.ProviderSettingsType, ReplyTo: offID}, raw, []string{id.Public().Enc})
	customOff := "custom"
	if _, err := owner.PatchRunSettings(ctx, "target", api.RunSettingsPatch{Provider: &customOff, ProviderEncrypted: &api.EncryptedProvider{ID: offID, From: owner.Sealer.Me, Body: offBody}}); err == nil {
		t.Fatal("Hub accepted encrypted provider while E2E off")
	}
	if _, err := admin.SetHubSettings(ctx, api.HubSettings{E2E: true}); err != nil {
		t.Fatal(err)
	}
	sealed, err := owner.SealProvider(ctx, "target", raw)
	if err != nil {
		t.Fatal(err)
	}
	custom := "custom"
	bad := *sealed
	bad.From = "@console:bob"
	if _, err := owner.PatchRunSettings(ctx, "target", api.RunSettingsPatch{Provider: &custom, ProviderEncrypted: &bad}); err == nil {
		t.Fatal("wrong sender accepted")
	}
	if _, err := owner.PatchRunSettings(ctx, "target", api.RunSettingsPatch{Provider: &custom, ProviderEncrypted: sealed}); err != nil {
		t.Fatal(err)
	}
	me, err = agent.Me(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.OpenProvider(ctx, *me.RunSettings.ProviderEncrypted); err == nil {
		t.Fatal("other device decrypted target settings")
	}
	// The Hub's on-disk DB, WAL and audit log must have no plaintext key.
	for _, f := range []string{db, db + "-wal", db + "-shm"} {
		b, _ := os.ReadFile(f)
		if bytes.Contains(b, []byte(secret)) {
			t.Fatal("plaintext key on Hub disk")
		}
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatal("plaintext key in Hub log")
	}
	// Lose the first receipt: a retry must not reinstall an older key over a local edit.
	u, _ := url.Parse(srv.URL)
	proxy := httputil.NewSingleHostReverseProxy(u)
	var acks atomic.Int32
	proxied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/me/provider/ack" && acks.Add(1) == 1 {
			w.WriteHeader(503)
			_, _ = io.WriteString(w, `{"error":"unavailable","message":"retry"}`)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	defer proxied.Close()
	remote, _ := client.New(proxied.URL, reg.Token)
	remote.Sealer = agent.Sealer
	path := config.ProfilePath(filepath.Join(home, "config.yaml"), "target")
	if err := config.Save(path, &config.Config{Hub: srv.URL, Agent: "target", Token: reg.Token, Harness: "codex"}); err != nil {
		t.Fatal(err)
	}
	apply := remoteProviderApplier(path, remote)
	if err := apply(ctx, *me.RunSettings); err == nil {
		t.Fatal("expected first receipt failure")
	}
	cfg, _ := config.Load(path)
	vals, _ := config.ReadEnvFile(cfg.Runner.EnvFile)
	if !strings.Contains(strings.Join(vals, "\n"), secret) || cfg.Runner.ProviderApplied != sealed.ID {
		t.Fatal("private provider not installed")
	}
	if st, _ := os.Stat(cfg.Runner.EnvFile); os.PathSeparator == '/' && st.Mode().Perm() != 0600 {
		t.Fatal("secret file permissions")
	}
	m.APIKey = "local-edit-after-install"
	if err := provider.WriteEnv(cfg.Runner.EnvFile, cfg.Agent, m.Variables("codex")); err != nil {
		t.Fatal(err)
	}
	newModel := "new-owner-model"
	if _, err := owner.PatchRunSettings(ctx, "target", api.RunSettingsPatch{Model: &newModel}); err != nil {
		t.Fatal(err)
	}
	if err := apply(ctx, *me.RunSettings); err != nil {
		t.Fatal(err)
	}
	vals, _ = config.ReadEnvFile(cfg.Runner.EnvFile)
	if !strings.Contains(strings.Join(vals, "\n"), m.APIKey) {
		t.Fatal("receipt retry overwrote local edit")
	}
	now, _ := agent.Me(ctx)
	if now.RunSettings.ProviderEncrypted != nil || now.RunSettings.Model != newModel {
		t.Fatal("receipt did not preserve other settings")
	}
	// An old receipt cannot consume a newer pending provider.
	next, err := owner.SealProvider(ctx, "target", raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.PatchRunSettings(ctx, "target", api.RunSettingsPatch{Provider: &custom, ProviderEncrypted: next}); err != nil {
		t.Fatal(err)
	}
	if err := agent.AckProvider(ctx, sealed.ID); err == nil {
		t.Fatal("stale receipt accepted")
	}
	now, _ = agent.Me(ctx)
	if now.RunSettings.ProviderEncrypted == nil || now.RunSettings.ProviderEncrypted.ID != next.ID {
		t.Fatal("stale receipt erased newer payload")
	}
	clear := "clear"
	if _, err := owner.PatchRunSettings(ctx, "target", api.RunSettingsPatch{Provider: &clear}); err != nil {
		t.Fatal(err)
	}
	now, _ = agent.Me(ctx)
	if err := apply(ctx, *now.RunSettings); err != nil {
		t.Fatal(err)
	}
	vals, _ = config.ReadEnvFile(cfg.Runner.EnvFile)
	if len(vals) != 0 {
		t.Fatal("remote clear left secret variables")
	}
	// Old runners remain readable and retain old model settings; new controls are refused.
	old, err := admin.Register(ctx, api.RegisterRequest{Name: "old", Binding: api.Binding{Harness: "codex", SessionID: "old-session"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = old
	admin.SetAgentOwner(ctx, "old", "alice")
	if _, err := owner.PatchRunSettings(ctx, "old", api.RunSettingsPatch{Provider: &ref}); err == nil {
		t.Fatal("old runner accepted remote provider")
	}
	if _, err := owner.SetRunSettings(ctx, "old", api.RunSettings{Model: "still-works"}); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteProviderSelectionApplier(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CC_SWITCH_DIR", dir)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"claude":{"providers":{"one":{"name":"One","settingsConfig":{"env":{"ANTHROPIC_BASE_URL":"https://one.test","ANTHROPIC_AUTH_TOKEN":"cc-local-key"}}}}}}`), 0600)
	path := config.ProfilePath(filepath.Join(dir, "config.yaml"), "agent")
	env := filepath.Join(dir, "agent.env")
	os.WriteFile(env, []byte("ANTHROPIC_AUTH_TOKEN=custom-local-key\nOTHER=keep\n"), 0600)
	config.Save(path, &config.Config{Agent: "agent", Harness: "claude", Runner: config.Runner{EnvFile: env}})
	apply := remoteProviderApplier(path, nil)
	p := "cc-switch:one"
	if err := apply(context.Background(), api.RunSettings{Provider: &p}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load(path)
	if cfg.Runner.Provider != p {
		t.Fatal("provider pin not saved")
	}
	p = "cc-switch:gone"
	if err := apply(context.Background(), api.RunSettings{Provider: &p}); err == nil {
		t.Fatal("missing provider applied")
	}
	cfg, _ = config.Load(path)
	if cfg.Runner.Provider != "cc-switch:one" {
		t.Fatal("bad provider changed the pin")
	}
	p = ""
	if err := apply(context.Background(), api.RunSettings{Provider: &p}); err != nil {
		t.Fatal(err)
	}
	cfg, _ = config.Load(path)
	vals, _ := config.ReadEnvFile(env)
	if cfg.Runner.Provider != "" || len(vals) != 1 || vals[0] != "OTHER=keep" {
		t.Fatal("follow machine did not clear provider variables safely")
	}
}
