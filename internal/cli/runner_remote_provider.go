package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/provider"
)

// Applying and acknowledging share a local lock across concurrent sessions.
// A lost ack is retried without reinstalling an older secret over local edits.
func remoteProviderApplier(path string, c *client.Client) func(context.Context, api.RunSettings) error {
	var mu sync.Mutex
	return func(ctx context.Context, rs api.RunSettings) error {
		if rs.Provider == nil && rs.ProviderEncrypted == nil && rs.ProviderHub == nil {
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
		if ref == "custom" && rs.ProviderHub != nil {
			// Kept by the hub for this agent (i119): fetched, once applied
			// acknowledged, and from then on only here.
			p := rs.ProviderHub
			if cfg.Runner.ProviderApplied != p.ID {
				if err := ownerSealed(ctx, c, p.From); err != nil {
					return fmt.Errorf("provider settings refused: %v", err)
				}
				got, err := c.ProviderMaterial(ctx, p.ID, "")
				if err != nil {
					return fmt.Errorf("cannot fetch provider settings: %v", err)
				}
				if got.ID != p.ID || got.From != p.From {
					return fmt.Errorf("provider settings refused: not the ones set")
				}
				m := provider.Manual{BaseURL: got.BaseURL, APIKey: got.APIKey, Model: got.Model, WireAPI: got.WireAPI, SmallModel: got.SmallModel}
				if err := applyManual(path, cfg, m, p.ID); err != nil {
					return err
				}
			}
			return c.AckProvider(ctx, p.ID)
		}
		if ref == "custom" {
			p := rs.ProviderEncrypted
			if p == nil {
				return nil
			} // Already delivered; the settings live only here.
			if cfg.Runner.ProviderApplied != p.ID {
				// From a person's device, the agent's owner's (the hub
				// lets only them set it); never another agent's.
				if err := ownerSealed(ctx, c, p.From); err != nil {
					return fmt.Errorf("provider settings refused: %v", err)
				}
				raw, err := c.OpenProvider(ctx, *p)
				if err != nil {
					return fmt.Errorf("cannot decrypt provider settings")
				}
				var m provider.Manual
				if json.Unmarshal(raw, &m) != nil {
					return fmt.Errorf("invalid decrypted provider settings")
				}
				if err := applyManual(path, cfg, m, p.ID); err != nil {
					return err
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

// applyManual writes a custom provider's settings to the agent's env file
// (for its user only) and records request id as applied.
func applyManual(path string, cfg *config.Config, m provider.Manual, id string) error {
	if m.Validate(profileHarness(cfg)) != nil {
		return fmt.Errorf("invalid provider settings")
	}
	file := cfg.Runner.EnvFile
	if file == "" {
		file = filepath.Join(filepath.Dir(filepath.Dir(path)), "env", filepath.Base(path)+".env")
	}
	if err := provider.WriteEnv(file, cfg.Agent, m.Variables(profileHarness(cfg))); err != nil {
		return fmt.Errorf("cannot save private provider settings")
	}
	cfg.Runner.EnvFile, cfg.Runner.Provider, cfg.Runner.ProviderApplied = file, "", id
	if err := config.Save(path, cfg); err != nil {
		return fmt.Errorf("cannot save provider profile")
	}
	return nil
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

// ownerSealed: a custom provider sent to this agent was sealed by its
// owner's device ("@console:<owner>"); with no owner on record, by a
// person's device (the hub let only its owner set it).
func ownerSealed(ctx context.Context, c *client.Client, sealedBy string) error {
	who, ok := strings.CutPrefix(sealedBy, "@console:")
	if !ok || who == "" {
		return errors.New("not sealed by a person")
	}
	me, err := c.Me(ctx)
	if err != nil {
		return errors.New("its owner could not be checked")
	}
	if me.Owner != "" && who != me.Owner {
		return errors.New("not sealed by its owner")
	}
	return nil
}
