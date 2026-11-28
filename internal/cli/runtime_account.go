package cli

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/myco-ai/myco/internal/quota"
)

// runtimeAccount reads identity hints only. It never authenticates, refreshes,
// logs parse errors/credentials, or treats an unverified JWT as authorization.
// Both the environment and home are explicit so tests cannot read user accounts.
func runtimeAccount(cli, home string, getenv func(string) string) string {
	if cli == "claude-cloud" {
		if email := accountEmail(getenv("CLAUDE_CODE_USER_EMAIL")); email != "" {
			return email
		}
	}
	switch cli {
	case "claude", "claude-cloud":
		if home == "" {
			return ""
		}
		var cfg struct {
			OAuthAccount struct {
				Email string `json:"emailAddress"`
			} `json:"oauthAccount"`
		}
		path := filepath.Join(home, ".claude.json")
		if dir := getenv("CLAUDE_CONFIG_DIR"); dir != "" {
			path = filepath.Join(dir, ".claude.json")
		}
		if !readAccountJSON(path, &cfg) {
			return ""
		}
		return accountEmail(cfg.OAuthAccount.Email)
	case "codex":
		dir := strings.TrimSpace(getenv("CODEX_HOME"))
		if dir == "" {
			if home == "" {
				return ""
			}
			dir = filepath.Join(home, ".codex")
		}
		var cfg struct {
			Tokens struct {
				IDToken string `json:"id_token"`
			} `json:"tokens"`
		}
		if !readAccountJSON(filepath.Join(dir, "auth.json"), &cfg) {
			return ""
		}
		parts := strings.Split(cfg.Tokens.IDToken, ".")
		if len(parts) != 3 || len(parts[1]) > 32768 {
			return ""
		}
		payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
		if err != nil {
			return ""
		}
		var claims struct {
			Email string `json:"email"`
		}
		if json.Unmarshal(payload, &claims) != nil {
			return ""
		}
		return accountEmail(claims.Email)
	}
	return ""
}

func accountEmail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 320 || strings.ContainsAny(s, "\x00\r\n\t <>\"") || !strings.Contains(s, "@") {
		return ""
	}
	return s
}

func accountHome(getenv func(string) string) string {
	if home := getenv("HOME"); home != "" {
		return home
	}
	return getenv("USERPROFILE")
}

func readAccountJSON(path string, out any) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	return err == nil && len(b) <= 1<<20 && json.Unmarshal(b, out) == nil
}

// runtimeAccountIDHash reads only identity fields, never credentials/helpers.
// Comparing the email guards an account-file switch between the two reads.
func runtimeAccountIDHash(cli, home string, getenv func(string) string, email string) string {
	provider, id, foundEmail := "", "", ""
	switch cli {
	case "claude", "claude-cloud":
		var cfg struct {
			Account struct {
				ID    string `json:"accountUuid"`
				Email string `json:"emailAddress"`
			} `json:"oauthAccount"`
		}
		path := filepath.Join(home, ".claude.json")
		if dir := getenv("CLAUDE_CONFIG_DIR"); dir != "" {
			path = filepath.Join(dir, ".claude.json")
		}
		if home == "" || !readAccountJSON(path, &cfg) {
			return ""
		}
		provider, id, foundEmail = "claude", cfg.Account.ID, cfg.Account.Email
	case "codex":
		dir := getenv("CODEX_HOME")
		if dir == "" {
			if home == "" {
				return ""
			}
			dir = filepath.Join(home, ".codex")
		}
		var cfg struct {
			Tokens struct {
				ID    string `json:"account_id"`
				Token string `json:"id_token"`
			} `json:"tokens"`
		}
		if !readAccountJSON(filepath.Join(dir, "auth.json"), &cfg) {
			return ""
		}
		provider, id = "codex", cfg.Tokens.ID
		parts := strings.Split(cfg.Tokens.Token, ".")
		if len(parts) == 3 {
			raw, e := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
			if e == nil {
				var claims struct {
					Email string `json:"email"`
				}
				if json.Unmarshal(raw, &claims) == nil {
					foundEmail = claims.Email
				}
			}
		}
	default:
		return ""
	}
	if email != "" && !strings.EqualFold(strings.TrimSpace(email), strings.TrimSpace(foundEmail)) {
		return ""
	}
	return quota.AccountIDHash(provider, id)
}
