package provider

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/myco-ai/myco/internal/osutil"
)

// Manual is a local-only endpoint configuration. APIKey must never be returned
// to clients, sent to a hub, or included in a command line.
type Manual struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key,omitempty"`
	Model   string `json:"model,omitempty"`
	WireAPI string `json:"wire_api,omitempty"`
}

func (m *Manual) Validate(app string) error {
	m.BaseURL = strings.TrimSpace(m.BaseURL)
	m.APIKey = strings.TrimSpace(m.APIKey)
	m.Model = strings.TrimSpace(m.Model)
	u, err := url.Parse(m.BaseURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("enter an http(s) base URL without credentials, query or fragment")
	}
	if app != "claude" && app != "codex" {
		return fmt.Errorf("custom providers require Claude Code or Codex")
	}
	if m.APIKey == "" || strings.ContainsAny(m.APIKey+m.Model+m.BaseURL, "\r\n\x00") {
		return fmt.Errorf("enter a nonempty single-line key and model without control characters")
	}
	if app == "codex" {
		if m.WireAPI == "" {
			m.WireAPI = "responses"
		}
		if m.WireAPI != "chat" && m.WireAPI != "responses" {
			return fmt.Errorf("wire_api must be chat or responses")
		}
	} else if m.WireAPI != "" {
		return fmt.Errorf("wire_api applies to Codex only")
	}
	return nil
}

func (m Manual) Variables(app string) map[string]string {
	if app == "codex" {
		return map[string]string{"OPENAI_BASE_URL": m.BaseURL, "OPENAI_API_KEY": m.APIKey, "OPENAI_MODEL": m.Model, "OPENAI_WIRE_API": m.WireAPI}
	}
	vars := map[string]string{"ANTHROPIC_BASE_URL": m.BaseURL, "ANTHROPIC_AUTH_TOKEN": m.APIKey}
	if m.Model != "" {
		vars["ANTHROPIC_MODEL"] = m.Model
	}
	return vars
}

// CustomCodex pins a separate named provider so global model_provider and
// authentication settings cannot redirect this agent. Secrets are environment only.
func CustomCodex(base, key, model, wire string) (args []string, env map[string]string) {
	if wire == "" {
		wire = "responses"
	}
	args = []string{"-c", `model_provider="myco_custom"`, "-c", `model_providers.myco_custom={name="Custom",base_url=` + strconv.Quote(base) + `,env_key="` + CodexKeyEnv + `",wire_api=` + strconv.Quote(wire) + `,requires_openai_auth=false}`}
	if model != "" {
		args = append(args, "-c", "model="+strconv.Quote(model))
	}
	return args, map[string]string{CodexKeyEnv: key}
}

func IsProviderVar(k string) bool {
	return strings.HasPrefix(k, "ANTHROPIC_") || strings.HasPrefix(k, "CLAUDE_CODE_USE_") || k == "CLAUDE_CODE_OAUTH_TOKEN" || strings.HasPrefix(k, "OPENAI_")
}

// WriteEnv replaces only provider variables, preserving unrelated entries. It
// atomically replaces the file with a fresh 0600 file (including on Windows).
func WriteEnv(path, agent string, vars map[string]string) error {
	if _, ok := osutil.HomeRelative(path); ok {
		home, _ := os.UserHomeDir()
		path = osutil.ExpandHome(path, home)
	}
	var lines []string
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read provider env file: %w", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "export "), "=")
		if strings.TrimSpace(line) != "" && !IsProviderVar(strings.TrimSpace(k)) {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		lines = []string{"# private provider settings for " + agent}
	}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := vars[k]
		if strings.ContainsAny(v, "\r\n\x00") {
			return fmt.Errorf("provider values must be single-line")
		}
		// ReadEnvFile removes the outer quotes without shell evaluation.
		if strings.ContainsAny(v, " \t#\"'") {
			v = "'" + v + "'"
		}
		lines = append(lines, k+"="+v)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".provider-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
