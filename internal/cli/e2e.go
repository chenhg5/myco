package cli

import (
	"path/filepath"

	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/e2e"
)

// knownKeys is this machine's store of pinned keys.
func (e *env) knownKeys() *e2e.Known {
	return &e2e.Known{Path: filepath.Join(config.Dir(e.defaultPath), "known_keys.json")}
}

// withSealer makes c encrypt and decrypt as agent cfg.Agent on hubs with
// end-to-end encryption on. The agent's key pair lives in its profile
// (path); one is made and saved there the first time.
func (e *env) withSealer(c *client.Client, path string, cfg *config.Config) *client.Client {
	if cfg == nil || cfg.Agent == "" {
		return c
	}
	id, err := e2e.ParseIdentity(cfg.E2EKey)
	if cfg.E2EKey == "" || err != nil {
		if id, err = e2e.NewIdentity(); err != nil {
			return c
		}
		cfg.E2EKey = id.Marshal()
		if path != "" {
			if err := config.Save(path, cfg); err != nil {
				cfg.E2EKey = ""
				return c
			}
		}
	}
	c.Sealer = &client.Sealer{Me: cfg.Agent, ID: id, Known: e.knownKeys()}
	return c
}
