package cli

import (
	"strings"
	"testing"
)

// The runner service must reach model APIs through the user's proxy (a
// service does not read ~/.zshrc), without leaking proxy passwords or
// tokens into the world-readable service definition.
func TestServiceEnvCopiesProxiesButNotSecrets(t *testing.T) {
	vars := map[string]string{
		"PATH":              "/usr/bin",
		"HTTPS_PROXY":       "http://127.0.0.1:7890",
		"https_proxy":       "http://127.0.0.1:7890",
		"ALL_PROXY":         "socks5h://127.0.0.1:7890",
		"NO_PROXY":          "localhost,127.0.0.1",
		"HTTP_PROXY":        "http://alice:s3cret@proxy.corp:3128",
		"MYCO_TOKEN":        "myco_secret",
		"MYCO_ENROLL_TOKEN": "myco_enroll_secret",
		"OPENAI_API_KEY":    "sk-secret",
	}
	env, notes := serviceEnv(func(k string) string { return vars[k] })
	for _, k := range []string{"PATH", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "NO_PROXY"} {
		if env[k] != vars[k] {
			t.Errorf("%s = %q, want %q", k, env[k], vars[k])
		}
	}
	for _, k := range []string{"HTTP_PROXY", "MYCO_TOKEN", "MYCO_ENROLL_TOKEN", "OPENAI_API_KEY"} {
		if _, ok := env[k]; ok {
			t.Errorf("%s must not be written into the service definition", k)
		}
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "HTTP_PROXY") || strings.Contains(notes[0], "s3cret") {
		t.Errorf("notes = %q, want one note naming HTTP_PROXY without its password", notes)
	}
}

func TestProxyHasCredentials(t *testing.T) {
	for v, want := range map[string]bool{
		"http://127.0.0.1:7890":       false,
		"socks5h://127.0.0.1:7890":    false,
		"http://u:p@proxy:3128":       true,
		"u:p@proxy:3128":              true,
		"http://proxy:3128/path@x":    false,
		"localhost,127.0.0.1,*.local": false,
	} {
		if got := proxyHasCredentials(v); got != want {
			t.Errorf("proxyHasCredentials(%q) = %v, want %v", v, got, want)
		}
	}
}
