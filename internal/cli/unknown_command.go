package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/release"
)

// unknownCommandUpgradeHint makes a best-effort, bounded health check only
// after command resolution has already failed. An unavailable or older hub
// never changes the original usage error.
func unknownCommandUpgradeHint(ctx context.Context, args []string, getenv func(string) string) string {
	hub := configuredHubForUnknown(args, getenv)
	if hub == "" {
		return ""
	}
	c, err := client.New(hub, "")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	health, err := c.Health(ctx)
	if err != nil || !release.Newer(health.ServerVersion, Version) {
		return ""
	}
	return fmt.Sprintf("this command may require a newer myco: local %s, Hub %s; run: myco upgrade", Version, health.ServerVersion)
}

func configuredHubForUnknown(args []string, getenv func(string) string) string {
	defaultPath := config.DefaultPath()
	path := commandFlagValue(args, "config")
	if path == "" && getenv != nil {
		path = getenv("MYCO_CONFIG")
	}
	if path == "" {
		path = defaultPath
	}
	cfg, err := config.Load(path)
	if err != nil {
		return ""
	}
	if getenv != nil {
		if h := getenv(config.EnvHub); h != "" {
			cfg.Hub = h
		}
	}
	if h := commandFlagValue(args, "hub"); h != "" {
		return h
	}
	if cfg.Hub != "" {
		return cfg.Hub
	}
	if machine, err := config.LoadMachine(defaultPath); err == nil && machine != nil {
		return machine.Hub
	}
	return ""
}

func commandFlagValue(args []string, name string) string {
	flag := "--" + name
	for i := 0; i < len(args); i++ {
		if args[i] == flag && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(args[i], flag+"="); ok {
			return v
		}
	}
	return ""
}
