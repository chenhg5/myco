package oscap

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
)

func TestCredentialCLISourceUsesDiscoveredHostAndIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX CLI fixture")
	}
	e := credentialTestEnv(t)
	bin := t.TempDir()
	record := filepath.Join(bin, "argv.txt")
	script := `#!/bin/sh
if [ "$2" = "status" ]; then
 printf '%s' '{"hosts":{"github.com":[{"login":"demo","state":"success"}]}}'
else
 [ -z "$GH_TOKEN" ] || exit 1
 printf '%s\n' "$@" > "` + record + `"
 printf '%s' 'fixture-cli-credential'
fi
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	e.Path = func() string { return bin }
	t.Setenv("GH_TOKEN", "ambient-token-must-not-be-used")
	t.Setenv("GLAB_CONFIG_DIR", t.TempDir())
	sources := CredentialSources(context.Background(), e)
	if len(sources) != 1 || sources[0].Origin != "https://api.github.com" || sources[0].Login != "demo" {
		t.Fatal("wrong discovered source", sources)
	}
	token, err := CredentialSourceToken(context.Background(), e, sources[0])
	if err != nil || token != "fixture-cli-credential" {
		t.Fatal("could not read selected source", err)
	}
	raw, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "auth\ntoken\n--hostname\ngithub.com\n--user\ndemo\n" {
		t.Fatalf("unexpected fixed argv %q", raw)
	}
	tampered := sources[0]
	tampered.Origin = "https://attacker.example"
	if _, err := CredentialSourceToken(context.Background(), e, tampered); err == nil {
		t.Fatal("CLI destination could be changed")
	}
	tampered = sources[0]
	tampered.Login = "--other"
	if _, err := CredentialSourceToken(context.Background(), e, tampered); err == nil {
		t.Fatal("option-like identity accepted")
	}
}
func TestCredentialSourceMappingsNeverUseAppAuthenticationRules(t *testing.T) {
	e := credentialTestEnv(t)
	d := api.CredentialDescriptor{Name: "Custom", Origin: "https://custom.example", Authentication: "X-Arbitrary-Header", Paths: []api.CredentialPath{{Path: "/user"}}}
	if _, err := CredentialStore(e, d, "fixture-token", "token", "", false); err == nil {
		t.Fatal("arbitrary authentication header accepted")
	}
	for _, origin := range []string{"https://github.com.evil.example", "https://api.github.com"} {
		real, _, err := CredentialSourceTarget("gh", "github.com")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(origin, "evil") && real == origin {
			t.Fatal("app destination overrode fixed source mapping")
		}
	}
}
