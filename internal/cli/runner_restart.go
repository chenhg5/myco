package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/domain"
)

func runnerRestartPath(defaultPath string) string {
	return filepath.Join(config.Dir(defaultPath), "runner-restarting.json")
}

func runnerRestartUntil(path string) time.Time {
	var until time.Time
	b, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(b, &until)
	}
	now := time.Now()
	if !until.After(now) || until.After(now.Add(domain.RestartLease)) {
		return time.Time{}
	}
	return until
}

// announceRunnerRestart marks every local profile, including idle command
// and machine nodes on other workspaces. One shared deadline bounds all Hub
// calls; an unavailable/old Hub must never hold up restarting the service.
func (e *env) announceRunnerRestart() {
	until := time.Now().UTC().Add(domain.RestartLease)
	path := runnerRestartPath(e.defaultPath)
	b, _ := json.Marshal(until)
	if os.MkdirAll(filepath.Dir(path), 0o700) == nil {
		_ = os.WriteFile(path, b, 0o600)
	}
	profiles, _ := config.ListProfiles(config.ProfilesDir(e.defaultPath))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, p := range profiles {
		if p.Config.Replaced != "" || p.Config.Harness == "inbox" {
			continue
		}
		wg.Add(1)
		go func(cfg *config.Config) {
			defer wg.Done()
			c, err := client.New(cfg.Hub, cfg.Token)
			if err != nil {
				return
			}
			me, err := c.Me(ctx)
			if err != nil {
				return
			}
			rt := api.Runtime{}
			if me.Runtime != nil {
				rt = *me.Runtime
			}
			rt.Away, rt.RestartingUntil, rt.RunnerSlots = "updating", until, nil
			_ = c.ReportRuntime(ctx, rt)
		}(p.Config)
	}
	wg.Wait()
}

// Windows logon services request graceful shutdown without a console signal.
// The PID scopes the request; startup removes any stale file before watching.
func watchRunnerStop(parent context.Context, path string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	_ = os.Remove(path)
	go func() {
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if _, err := os.Stat(path); err == nil {
					_ = os.Remove(path)
					cancel()
					return
				}
			}
		}
	}()
	return ctx, cancel
}
