package oscap

// Code hosting is a page-scoped, read-only API. Credentials never leave the
// native host. It deliberately does not expose a general URL or shell operation.
import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

func init() {
	register(&Cap{Name: "forge.accounts", About: "Connect this page to GitHub, GitLab or Gitea using local credentials", Fn: forgeAccounts},
		&Cap{Name: "forge.query", About: "Read repositories, pull requests, issues and discussions from a connected account", Fn: forgeQuery})
}

type forgeAccount struct {
	ID           string `json:"id"`
	Provider     string `json:"provider"`
	Host         string `json:"host"`
	Login        string `json:"login"`
	Name         string `json:"name"`
	Source       string `json:"source"`
	AllowPrivate bool   `json:"allow_private"`
}
type forgeConnect struct {
	Action       string `json:"action"`
	ID           string `json:"id"`
	Provider     string `json:"provider"`
	Host         string `json:"host"`
	Name         string `json:"name"`
	Source       string `json:"source"`
	AllowPrivate bool   `json:"allow_private"`
	Token        string `json:"token"`
}

var forgeMu sync.Mutex

func forgeKey(e *Env) (string, error) {
	if e == nil || e.Home == "" || !strings.Contains(e.Scope, "\x00") || strings.HasSuffix(e.Scope, "\x00") {
		return "", errf(CodeInvalid, "code hosting requires a workspace, account and page")
	}
	h := sha256.Sum256([]byte(e.Scope))
	return "app-codehost-" + hex.EncodeToString(h[:16]), nil
}
func forgeFile(e *Env, key string) string {
	return filepath.Join(e.Home, ".myco", "apps", "code-host", key+".json")
}
func forgeLoad(e *Env, key string) ([]forgeAccount, error) {
	raw, err := os.ReadFile(forgeFile(e, key))
	if os.IsNotExist(err) {
		return []forgeAccount{}, nil
	}
	if err != nil {
		return nil, errf(CodeFailed, "could not read local code-host accounts")
	}
	var rows []forgeAccount
	if json.Unmarshal(raw, &rows) != nil {
		return nil, errf(CodeFailed, "local account settings are damaged")
	}
	return rows, nil
}
func forgeSave(e *Env, key string, rows []forgeAccount) error {
	path := forgeFile(e, key)
	if os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return errf(CodeFailed, "could not save accounts")
	}
	raw, _ := json.Marshal(rows)
	f, err := os.CreateTemp(filepath.Dir(path), "accounts-*.tmp")
	if err != nil {
		return errf(CodeFailed, "could not save accounts")
	}
	defer os.Remove(f.Name())
	err = f.Chmod(0600)
	if err == nil {
		_, err = f.Write(raw)
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		return errf(CodeFailed, "could not save accounts")
	}
	return nil
}
func forgeHost(provider, host string) (string, error) {
	if provider != "github" && provider != "gitlab" && provider != "gitea" {
		return "", errf(CodeInvalid, "choose GitHub, GitLab or Gitea")
	}
	host = strings.TrimSpace(host)
	if host == "" {
		if provider == "github" {
			host = "https://github.com"
		} else if provider == "gitlab" {
			host = "https://gitlab.com"
		}
	}
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	u, err := url.Parse(host)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || strings.ContainsAny(u.Host, "\\\r\n\x00") || strings.HasSuffix(u.Hostname(), ".") {
		return "", errf(CodeInvalid, "enter an HTTPS server origin without a path or credentials")
	}
	return "https://" + strings.ToLower(u.Host), nil
}
func forgeCLI(ctx context.Context, e *Env, provider, host string) (string, error) {
	u, _ := url.Parse(host)
	prog := "gh"
	if provider == "gitlab" {
		prog = "glab"
	}
	if provider == "gitea" {
		return "", errf(CodeUnsupported, "use a Gitea access token")
	}
	// Resolve only the explicitly chosen host; environment tokens must not be
	// silently reused for another enterprise host. CLI output is never logged.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	argv := []string{prog, "auth", "token", "--hostname", u.Host}
	if provider == "gitlab" {
		argv = []string{prog, "auth", "status", "--hostname", u.Host, "--show-token"}
	}
	args := execArgs{Argv: argv, Env: map[string]string{"GH_TOKEN": "", "GITHUB_TOKEN": "", "GH_ENTERPRISE_TOKEN": "", "GITHUB_ENTERPRISE_TOKEN": "", "GITLAB_TOKEN": "", "GITLAB_ACCESS_TOKEN": "", "GITLAB_OAUTH_TOKEN": "", "GH_DEBUG": "", "DEBUG": "", "GLAB_DEBUG": "", "NO_COLOR": "1", "CLICOLOR": "0", "GH_PROMPT_DISABLED": "1", "GIT_TERMINAL_PROMPT": "0"}}
	cmd, err := e.command(ctx, &args)
	if err != nil {
		return "", errf(CodeNoCredentials, "install the provider CLI and sign in, or use an access token")
	}
	out := &capped{n: 16384}
	cmd.Stdout = out
	cmd.Stderr = &capped{n: 1024}
	if provider == "gitlab" {
		cmd.Stderr = out
	}
	if cmd.Run() != nil || out.truncated {
		return "", errf(CodeNoCredentials, "the CLI could not provide a credential; sign in for this server or use an access token")
	}
	token := strings.TrimSpace(out.buf.String())
	if provider == "gitlab" {
		token = forgeGitLabToken(token)
	}
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return "", errf(CodeNoCredentials, "the CLI did not provide a usable credential")
	}
	return token, nil
}

// glab has no auth-token command. Its documented --show-token output can be
// on stderr; accept exactly one unmasked token field and keep all text private.
func forgeGitLabToken(text string) string {
	matches := regexp.MustCompile(`(?mi)(?:^|\s)(?:token found|token):[ \t]+([^\s*]+)[ \t]*$`).FindAllStringSubmatch(text, -1)
	if len(matches) != 1 {
		return ""
	}
	return matches[0][1]
}
func forgeAccounts(ctx context.Context, e *Env, args json.RawMessage, _ Emit) (any, error) {
	key, err := forgeKey(e)
	if err != nil {
		return nil, err
	}
	var in forgeConnect
	if err = decode(args, &in); err != nil {
		return nil, err
	}
	if in.Action == "discover" {
		rows := []map[string]any{}
		for _, name := range []string{"gh", "glab"} {
			_, err := e.LookPath(name)
			rows = append(rows, map[string]any{"cli": name, "installed": err == nil})
		}
		return rows, nil
	}
	if in.Action == "list" {
		forgeMu.Lock()
		defer forgeMu.Unlock()
		return forgeLoad(e, key)
	}
	if e.Secrets == nil {
		return nil, errf(CodeUnsupported, "the desktop credential store is unavailable")
	}
	if in.Action == "remove" {
		forgeMu.Lock()
		defer forgeMu.Unlock()
		rows, err := forgeLoad(e, key)
		if err != nil {
			return nil, err
		}
		for i, p := range rows {
			if p.ID == in.ID {
				if err = e.Secrets.Delete(key + "-" + p.ID); err != nil {
					return nil, errf(CodeFailed, "could not remove the saved credential")
				}
				return true, forgeSave(e, key, append(rows[:i], rows[i+1:]...))
			}
		}
		return nil, errf(CodeNotFound, "account not found on this computer")
	}
	if in.Action != "connect" {
		return nil, errf(CodeInvalid, "choose list, discover, connect or remove")
	}
	host, err := forgeHost(in.Provider, in.Host)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(in.Token)
	if in.Source == "cli" {
		token, err = forgeCLI(ctx, e, in.Provider, host)
		if err != nil {
			return nil, err
		}
	} else if in.Source != "token" {
		return nil, errf(CodeInvalid, "choose CLI login or access token")
	}
	if len(token) < 8 || len(token) > 8192 || strings.ContainsAny(token, "\r\n") {
		return nil, errf(CodeInvalid, "enter a valid access token")
	}
	p := forgeAccount{Provider: in.Provider, Host: host, Name: strings.TrimSpace(in.Name), Source: in.Source, AllowPrivate: in.AllowPrivate}
	if len(p.Name) > 100 {
		return nil, errf(CodeInvalid, "account name is too long")
	}
	var user struct {
		Login    string `json:"login"`
		Username string `json:"username"`
	}
	if _, err = forgeGet(ctx, p, token, "/user", nil, &user); err != nil {
		return nil, err
	}
	p.Login = user.Login
	if p.Login == "" {
		p.Login = user.Username
	}
	if p.Login == "" || len(p.Login) > 200 || !regexp.MustCompile(`^[a-zA-Z0-9_.@-]+$`).MatchString(p.Login) {
		return nil, errf("invalid_response", "the server did not return an account identity")
	}
	if p.Name == "" {
		p.Name = p.Login
	}
	forgeMu.Lock()
	defer forgeMu.Unlock()
	rows, err := forgeLoad(e, key)
	if err != nil {
		return nil, err
	}
	// Reconnecting the same identity replaces only its own credential.
	old := ""
	for i, v := range rows {
		if v.Provider == p.Provider && v.Host == p.Host && v.Login == p.Login {
			old = v.ID
			rows = append(rows[:i], rows[i+1:]...)
			break
		}
	}
	if len(rows) >= 24 {
		return nil, errf(CodeTooLarge, "at most 24 accounts can be connected per page")
	}
	var id [12]byte
	rand.Read(id[:])
	p.ID = "a_" + hex.EncodeToString(id[:])
	if e.Secrets.Set(key+"-"+p.ID, forgeSecretValue(p, token)) != nil {
		return nil, errf(CodeFailed, "could not save the credential in the system credential store")
	}
	if err = forgeSave(e, key, append(rows, p)); err != nil {
		_ = e.Secrets.Delete(key + "-" + p.ID)
		return nil, err
	}
	if old != "" {
		_ = e.Secrets.Delete(key + "-" + old)
	}
	return p, nil
}
func forgeCredential(e *Env, id string) (forgeAccount, string, error) {
	key, err := forgeKey(e)
	if err != nil {
		return forgeAccount{}, "", err
	}
	forgeMu.Lock()
	defer forgeMu.Unlock()
	rows, err := forgeLoad(e, key)
	if err != nil {
		return forgeAccount{}, "", err
	}
	for _, p := range rows {
		if p.ID == id {
			if e.Secrets == nil {
				return p, "", errf(CodeUnsupported, "credential store unavailable")
			}
			t, err := e.Secrets.Get(key + "-" + id)
			if err != nil || t == "" {
				return p, "", errf(CodeNoCredentials, "reconnect this account")
			}
			var secret forgeSecret
			if json.Unmarshal([]byte(t), &secret) != nil || secret.Host != p.Host || secret.Provider != p.Provider || secret.API != forgeBase(p) {
				return p, "", errf(CodePermission, "account origin changed; disconnect and reconnect")
			}
			p.AllowPrivate = secret.AllowPrivate
			return p, secret.Token, nil
		}
	}
	return forgeAccount{}, "", errf(CodeNotFound, "account not found on this computer")
}

type forgeSecret struct {
	Token        string `json:"token"`
	Host         string `json:"host"`
	Provider     string `json:"provider"`
	API          string `json:"api"`
	AllowPrivate bool   `json:"allow_private"`
}

func forgeSecretValue(p forgeAccount, token string) string {
	raw, _ := json.Marshal(forgeSecret{Token: token, Host: p.Host, Provider: p.Provider, API: forgeBase(p), AllowPrivate: p.AllowPrivate})
	return string(raw)
}
