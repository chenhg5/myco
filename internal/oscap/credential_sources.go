package oscap

// Credential sources are trusted local adapters. They discover identities and
// read secrets, but never query repositories or parse provider business data.
import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"gopkg.in/yaml.v3"
)

func credentialCLIEnv(host string) map[string]string {
	return map[string]string{"GH_TOKEN": "", "GITHUB_TOKEN": "", "GH_ENTERPRISE_TOKEN": "", "GITHUB_ENTERPRISE_TOKEN": "", "GITLAB_TOKEN": "", "GITLAB_ACCESS_TOKEN": "", "GITLAB_OAUTH_TOKEN": "", "GH_DEBUG": "", "DEBUG": "", "GLAB_DEBUG": "", "GITLAB_HOST": host, "GITLAB_API_HOST": host, "GLAB_API_PROTOCOL": "https", "NO_COLOR": "1", "CLICOLOR": "0", "GH_PROMPT_DISABLED": "1", "GIT_TERMINAL_PROMPT": "0"}
}
func credentialCommand(ctx context.Context, e *Env, argv []string, host string, stderr bool) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd, err := e.command(ctx, &execArgs{Argv: argv, Env: credentialCLIEnv(host)})
	if err != nil {
		return "", errf(CodeNoCredentials, "install the CLI and sign in, or paste a credential")
	}
	out := &capped{n: 1 << 20}
	cmd.Stdout = out
	cmd.Stderr = &capped{n: 2048}
	if stderr {
		cmd.Stderr = out
	}
	if cmd.Run() != nil || out.truncated {
		return "", errf(CodeNoCredentials, "the CLI could not provide a credential; sign in again or paste one")
	}
	return out.buf.String(), nil
}

// CredentialSourceTarget is host-owned policy. An app cannot redirect a CLI secret.
func CredentialSourceTarget(cli, host string) (string, string, error) {
	origin, err := CredentialOrigin("https://" + host)
	if err != nil {
		return "", "", err
	}
	u, _ := url.Parse(origin)
	if u.Host != host {
		return "", "", errf(CodeInvalid, "noncanonical login host")
	}
	switch cli {
	case "gh":
		if host == "github.com" {
			return "https://api.github.com", "", nil
		}
		return origin, "/api/v3", nil
	case "glab":
		return origin, "/api/v4", nil
	}
	return "", "", errf(CodeInvalid, "unsupported credential source")
}

var credentialLoginPattern = regexp.MustCompile(`^[a-zA-Z0-9_.@-]{1,200}$`)

func CredentialSources(ctx context.Context, e *Env) []api.CredentialSource {
	sources := []api.CredentialSource{}
	add := func(cli, host, login string) {
		origin, base, err := CredentialSourceTarget(cli, host)
		if err != nil || !credentialLoginPattern.MatchString(login) || strings.HasPrefix(login, "-") {
			return
		}
		sources = append(sources, api.CredentialSource{ID: rand.Text(), CLI: cli, Host: host, Login: login, Origin: origin, BasePath: base})
	}
	if _, err := e.LookPath("gh"); err == nil {
		raw, err := credentialCommand(ctx, e, []string{"gh", "auth", "status", "--json", "hosts"}, "", false)
		var data struct {
			Hosts map[string][]struct {
				Login string `json:"login"`
				State string `json:"state"`
			} `json:"hosts"`
		}
		if err == nil && json.Unmarshal([]byte(raw), &data) == nil {
			for host, accounts := range data.Hosts {
				for _, a := range accounts {
					if a.State == "success" {
						add("gh", host, a.Login)
					}
				}
			}
		}
	}
	if _, err := e.LookPath("glab"); err == nil {
		// Only host/user metadata from glab's own local config; never return tokens.
		dir := os.Getenv("GLAB_CONFIG_DIR")
		if dir == "" {
			dir = os.Getenv("XDG_CONFIG_HOME")
			if dir == "" {
				dir = filepath.Join(e.Home, ".config")
			}
			dir = filepath.Join(dir, "glab-cli")
		}
		f, err := os.Open(filepath.Join(dir, "config.yml"))
		if err == nil {
			defer f.Close()
			var data struct {
				Hosts map[string]struct {
					User string `yaml:"user"`
				} `yaml:"hosts"`
			}
			if yaml.NewDecoder(io.LimitReader(f, 1<<20)).Decode(&data) == nil {
				for host, a := range data.Hosts {
					add("glab", host, a.User)
				}
			}
		}
	}
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].CLI+sources[i].Host+sources[i].Login < sources[j].CLI+sources[j].Host+sources[j].Login
	})
	if len(sources) > 64 {
		sources = sources[:64]
	}
	return sources
}

// CredentialSourceToken accepts only a source retained in a host confirmation.
func CredentialSourceToken(ctx context.Context, e *Env, s api.CredentialSource) (string, error) {
	origin, base, err := CredentialSourceTarget(s.CLI, s.Host)
	if err != nil || origin != s.Origin || base != s.BasePath || !credentialLoginPattern.MatchString(s.Login) {
		return "", errf(CodePermission, "invalid discovered source")
	}
	argv := []string{"gh", "auth", "token", "--hostname", s.Host, "--user", s.Login}
	if s.CLI == "glab" {
		argv = []string{"glab", "auth", "status", "--hostname", s.Host, "--show-token"}
	}
	raw, err := credentialCommand(ctx, e, argv, s.Host, s.CLI == "glab")
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(raw)
	if s.CLI == "glab" {
		identity := regexp.MustCompile(`(?mi)logged in to [^\s]+ as ([a-zA-Z0-9_.@-]+)`).FindStringSubmatch(raw)
		if len(identity) != 2 || identity[1] != s.Login {
			return "", errf(CodeNoCredentials, "the selected CLI identity changed; reconnect")
		}

		matches := regexp.MustCompile(`(?mi)(?:^|\s)(?:token found|token):[ \t]+([^\s*]+)[ \t]*$`).FindAllStringSubmatch(raw, -1)
		if len(matches) != 1 {
			return "", errf(CodeNoCredentials, "the CLI did not provide one usable credential")
		}
		token = matches[0][1]
	}
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return "", errf(CodeNoCredentials, "the CLI did not provide a usable credential")
	}
	return token, nil
}
