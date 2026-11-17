package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/config"
)

func pickEnv(t *testing.T, def string, vars map[string]string) *env {
	t.Helper()
	cfg, err := config.Load(def)
	if err != nil {
		t.Fatal(err)
	}
	return &env{cfg: cfg, cfgPath: def, defaultPath: def,
		in: &cmdkit.Invocation{Getenv: func(k string) string { return vars[k] }}}
}

func writeTestProfile(t *testing.T, def, agent, harness string, mutate func(*config.Config)) {
	t.Helper()
	c := &config.Config{Hub: "http://hub.test", Agent: agent, Token: "tok-" + agent, Harness: harness, SessionID: "s-" + agent}
	if mutate != nil {
		mutate(c)
	}
	if err := config.Save(config.ProfilePath(def, agent), c); err != nil {
		t.Fatal(err)
	}
}

func TestAdoptProfileUsesTheOnlyAgent(t *testing.T) {
	def := filepath.Join(t.TempDir(), "config.yaml")
	writeTestProfile(t, def, "mac-myco-inbox", "inbox", nil)
	// The machine node and a taken-over node are not agents one means.
	writeTestProfile(t, def, "mac-machine", "machine", nil)
	writeTestProfile(t, def, "old", "claude", func(c *config.Config) { c.Replaced = "x" })

	e := pickEnv(t, def, nil)
	several, err := e.adoptProfile()
	if err != nil || several != nil {
		t.Fatalf("adopt: %v %v", several, err)
	}
	if e.cfg.Agent != "mac-myco-inbox" || e.cfg.Token != "tok-mac-myco-inbox" || !strings.HasSuffix(e.cfgPath, "mac-myco-inbox.yaml") {
		t.Fatalf("picked %s at %s", e.cfg.Agent, e.cfgPath)
	}
}

func TestAdoptProfileSeveralAndMycoAgent(t *testing.T) {
	def := filepath.Join(t.TempDir(), "config.yaml")
	writeTestProfile(t, def, "b-codex", "codex", nil)
	writeTestProfile(t, def, "a-claude", "claude", nil)

	e := pickEnv(t, def, nil)
	several, err := e.adoptProfile()
	if err != nil || strings.Join(several, ",") != "a-claude,b-codex" || e.cfg.Token != "" {
		t.Fatalf("several: %v %v token=%q", several, err, e.cfg.Token)
	}
	if msg := severalAgentsError(several).(*Error).Suggestion; !strings.Contains(msg, "MYCO_AGENT=<name>") || !strings.Contains(msg, "a-claude, b-codex") {
		t.Fatalf("suggestion: %s", msg)
	}

	e = pickEnv(t, def, map[string]string{EnvAgent: "b-codex"})
	if _, err := e.adoptProfile(); err != nil || e.cfg.Agent != "b-codex" {
		t.Fatalf("MYCO_AGENT: %v %s", err, e.cfg.Agent)
	}
	e = pickEnv(t, def, map[string]string{EnvAgent: "nobody"})
	if _, err := e.adoptProfile(); err == nil || !strings.Contains(err.Error(), "nobody") {
		t.Fatalf("unknown MYCO_AGENT: %v", err)
	}
}

func TestAdoptProfileKeepsAnIdentityAlreadyChosen(t *testing.T) {
	def := filepath.Join(t.TempDir(), "config.yaml")
	writeTestProfile(t, def, "only", "inbox", nil)
	if err := config.Save(def, &config.Config{Hub: "http://hub.test", Agent: "main", Token: "tok-main"}); err != nil {
		t.Fatal(err)
	}
	e := pickEnv(t, def, nil)
	if _, err := e.adoptProfile(); err != nil || e.cfg.Agent != "main" {
		t.Fatalf("config.yaml's identity stays: %v %s", err, e.cfg.Agent)
	}
	// --config / MYCO_CONFIG chose the file: never switch.
	e = pickEnv(t, filepath.Join(t.TempDir(), "other.yaml"), nil)
	e.defaultPath, e.explicitConfig = def, true
	if _, err := e.adoptProfile(); err != nil || e.cfg.Token != "" {
		t.Fatalf("explicit config: %v %q", err, e.cfg.Token)
	}
}
