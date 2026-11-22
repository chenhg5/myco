package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/provider"
)

// Applying and acknowledging share a local lock across concurrent sessions.
// A lost ack is retried without reinstalling an older secret over local edits.
func remoteProviderApplier(path string, c *client.Client) func(context.Context, api.RunSettings) error {
	var mu sync.Mutex
	return func(ctx context.Context, rs api.RunSettings) error {
		if rs.Provider == nil && rs.ProviderEncrypted == nil {
			return nil
		}
		mu.Lock()
		defer mu.Unlock()
		cfg, err := config.Load(path)
		if err != nil {
			return err
		}
		if rs.Provider == nil {
			return fmt.Errorf("provider selection missing")
		}
		ref := *rs.Provider
		if ref == "custom" {
			p := rs.ProviderEncrypted
			if p == nil {
				return nil
			} // Already delivered; the settings live only here.
			if cfg.Runner.ProviderApplied != p.ID {
				raw, err := c.OpenProvider(ctx, *p)
				if err != nil {
					return fmt.Errorf("cannot decrypt provider settings")
				}
				var m provider.Manual
				if json.Unmarshal(raw, &m) != nil || m.Validate(profileHarness(cfg)) != nil {
					return fmt.Errorf("invalid decrypted provider settings")
				}
				file := cfg.Runner.EnvFile
				if file == "" {
					file = filepath.Join(filepath.Dir(filepath.Dir(path)), "env", filepath.Base(path)+".env")
				}
				if err := provider.WriteEnv(file, cfg.Agent, m.Variables(profileHarness(cfg))); err != nil {
					return fmt.Errorf("cannot save private provider settings")
				}
				cfg.Runner.EnvFile, cfg.Runner.Provider, cfg.Runner.ProviderApplied = file, "", p.ID
				if err := config.Save(path, cfg); err != nil {
					return fmt.Errorf("cannot save provider profile")
				}
			}
			return c.AckProvider(ctx, p.ID)
		}
		if ref == "clear" {
			ref = ""
		}
		if ref != "" {
			if _, err := provider.Resolve(provider.Dir(), ref, providerCLIs[profileHarness(cfg)]); err != nil {
				return fmt.Errorf("selected provider is not available on this computer")
			}
		}
		if ref == "" && cfg.Runner.EnvFile != "" {
			if err := provider.WriteEnv(cfg.Runner.EnvFile, cfg.Agent, nil); err != nil {
				return fmt.Errorf("cannot clear private provider settings")
			}
		}
		if cfg.Runner.Provider == ref {
			return nil
		}
		cfg.Runner.Provider = ref
		return config.Save(path, cfg)
	}
}

func localProviderOptions(cli string) []api.ProviderOption {
	out := []api.ProviderOption{}
	list, err := provider.List(provider.Dir(), cli)
	if err != nil {
		return out
	}
	for _, p := range list {
		out = append(out, api.ProviderOption{Ref: p.Ref, Name: p.Name, Host: p.Host, Model: orStr(p.Model, p.Env["ANTHROPIC_MODEL"])})
	}
	return out
}

func supportsRemoteProvider(cli string) bool {
	return strings.EqualFold(cli, "claude") || strings.EqualFold(cli, "codex")
}
