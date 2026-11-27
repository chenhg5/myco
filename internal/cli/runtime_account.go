package cli

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
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
		if !readAccountJSON(filepath.Join(home, ".claude.json"), &cfg) {
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
