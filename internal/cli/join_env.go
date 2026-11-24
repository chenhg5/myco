package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/provider"
)

// environ is the environment join inherits provider variables from (tests
// replace it).
var environ = os.Environ

// inheritEnv gives a joined agent the variables its CLI needs to reach its
// model as this session does (provider.Inherit): the runner that wakes it
// later is a background service without the shell profile. They go to the
// agent's private env file (0600; merged with what is already there, which
// `myco runner env capture` may have written) and runner.env_file points at
// it. It returns the variables' names, never values.
func inheritEnv(profilePath, defaultPath, harnessName string) (names []string, file string, err error) {
	home, _ := os.UserHomeDir()
	vars := provider.Inherit(harnessName, home, environ())
	if len(vars) == 0 {
		return nil, "", nil
	}
	cfg, err := config.Load(profilePath)
	if err != nil {
		return nil, "", err
	}
	file = cfg.Runner.EnvFile
	if file == "" {
		file = filepath.Join(config.Dir(defaultPath), "env", cfg.Agent+".env")
	}
	merged := map[string]string{}
	if old, err := config.ReadEnvFile(file); err == nil {
		for _, kv := range old {
			k, v, _ := strings.Cut(kv, "=")
			merged[k] = v
		}
	}
	changed := false
	for k, v := range vars {
		if merged[k] != v {
			merged[k], changed = v, true
		}
		names = append(names, k)
	}
	sort.Strings(names)
	if changed {
		keys := make([]string, 0, len(merged))
		for k := range merged {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString("# passed only to " + cfg.Agent + "'s CLI: written by `myco join` (from the session it joined) and `myco runner env capture`\n")
		for _, k := range keys {
			b.WriteString(k + "=" + quoteEnv(merged[k]) + "\n")
		}
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			return nil, "", err
		}
		if err := os.WriteFile(file, []byte(b.String()), 0o600); err != nil {
			return nil, "", err
		}
		if err := os.Chmod(file, 0o600); err != nil {
			return nil, "", err
		}
	}
	if cfg.Runner.EnvFile != file {
		cfg.Runner.EnvFile = file
		if err := config.Save(profilePath, cfg); err != nil {
			return nil, "", err
		}
	}
	return names, file, nil
}
