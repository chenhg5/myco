package im

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Config is one bridge: a bot on one chat app, and the myco node it speaks as.
// It holds the app's secrets, so it is written 0600 and never printed.
type Config struct {
	Name     string `json:"name"`
	Platform string `json:"platform"` // feishu, lark, slack
	// Feishu/Lark: the app's credentials.
	AppID     string `json:"app_id,omitempty"`
	AppSecret string `json:"app_secret,omitempty"`
	// Slack: the bot token (xoxb-) and the app-level token for Socket Mode
	// (xapp-).
	BotToken string `json:"bot_token,omitempty"`
	AppToken string `json:"app_token,omitempty"`

	// Owner is the chat-app user who set it up (always allowed); AllowFrom
	// adds others, by their chat-app user id.
	Owner     string   `json:"owner,omitempty"`
	OwnerName string   `json:"owner_name,omitempty"` // how the owner is named to agents
	AllowFrom []string `json:"allow_from,omitempty"`
	// Agent receives what is written when no /agent or /to says otherwise.
	Agent string `json:"agent,omitempty"`
	// Project puts new conversations in a myco project.
	Project string `json:"project,omitempty"`
	// Profile is the myco config file of the node it speaks as.
	Profile string `json:"profile"`
	// Claim, while there is no owner, is the code that makes whoever sends
	// "/claim <code>" to the bot its owner: until ClaimExpires, and only
	// while fewer than MaxClaimFails wrong codes were tried.
	Claim        string    `json:"claim,omitempty"`
	ClaimExpires time.Time `json:"claim_expires,omitempty"`
	ClaimFails   int       `json:"claim_fails,omitempty"`
	// Lang is the language the bot speaks: zh or en (default by platform).
	Lang string `json:"lang,omitempty"`
	// The front desk's model (OpenAI-compatible; the App copies the one set
	// for voice chats). Without one, messages go straight to the agent.
	LLMBaseURL string `json:"llm_base_url,omitempty"`
	LLMModel   string `json:"llm_model,omitempty"`
	LLMKey     string `json:"llm_key,omitempty"`
	// NoDesk: its owner turned the front desk off (the App then leaves it).
	NoDesk bool `json:"no_desk,omitempty"`
	// Prompt is the owner's own instructions for the front desk (this bot
	// only; voice chats have their own): rules for whoever talks to it,
	// such as what it may answer or do. Tasks from people other than the
	// owner carry it too, so the agent keeps to it as well.
	Prompt string `json:"prompt,omitempty"`
}

// MaxPrompt bounds a bot's own instructions.
const MaxPrompt = 8000

// HasDesk says whether the bot has a front-desk model.
func (c Config) HasDesk() bool { return !c.NoDesk && c.LLMModel != "" && c.LLMKey != "" }

// Secret values never leave the file; Redacted is what may be shown.
func (c Config) Redacted() map[string]any {
	set := func(s string) bool { return s != "" }
	return map[string]any{
		"name": c.Name, "platform": c.Platform, "app_id": c.AppID, "owner": c.Owner,
		"allow_from": c.AllowFrom, "agent": c.Agent, "lang": c.Lang, "claim_pending": c.ClaimUsable(time.Now()), "claim_expired": c.Owner == "" && !c.ClaimUsable(time.Now()), "project": c.Project, "profile": c.Profile,
		"has_app_secret": set(c.AppSecret), "has_bot_token": set(c.BotToken), "has_app_token": set(c.AppToken),
		"desk": c.HasDesk(), "llm_model": c.LLMModel, "prompt": c.Prompt,
	}
}

// A claim code lasts a day and allows a few wrong tries.
const (
	ClaimTTL      = 24 * time.Hour
	MaxClaimFails = 5
)

// NewClaim gives the bridge a fresh claim code (no owner yet, or a lost one
// to be claimed again), valid for ClaimTTL.
func (c *Config) NewClaim(now time.Time) string {
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	c.Claim, c.ClaimExpires, c.ClaimFails = hex.EncodeToString(b), now.Add(ClaimTTL), 0
	return c.Claim
}

// ClaimUsable says whether the claim code still works.
func (c Config) ClaimUsable(now time.Time) bool {
	return c.Owner == "" && c.Claim != "" && c.ClaimFails < MaxClaimFails &&
		(c.ClaimExpires.IsZero() || now.Before(c.ClaimExpires))
}

// Allowed says whether a chat-app user may give tasks through the bridge.
func (c Config) Allowed(user string) bool {
	if user == "" {
		return false
	}
	if user == c.Owner {
		return true
	}
	for _, a := range c.AllowFrom {
		if a == user || a == "*" {
			return true
		}
	}
	return false
}

// Validate checks what a bridge needs to run.
func (c Config) Validate() error {
	if !ValidName(c.Name) {
		return fmt.Errorf("invalid bridge name %q: use 1–32 lowercase letters, digits or hyphens", c.Name)
	}
	switch c.Platform {
	case "feishu", "lark":
		if c.AppID == "" || c.AppSecret == "" {
			return errors.New("app_id and app_secret are required")
		}
	case "slack":
		if !strings.HasPrefix(c.BotToken, "xoxb-") || !strings.HasPrefix(c.AppToken, "xapp-") {
			return errors.New("a bot token (xoxb-…) and an app-level token (xapp-…) are required")
		}
	default:
		return fmt.Errorf("unknown platform %q (feishu, lark or slack)", c.Platform)
	}
	if c.Profile == "" {
		return errors.New("no myco node configured (profile)")
	}
	if len([]rune(c.Prompt)) > MaxPrompt {
		return fmt.Errorf("the bot's instructions are too long (at most %d characters)", MaxPrompt)
	}
	return nil
}

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// ValidName says whether name can name a bridge (and its files).
func ValidName(name string) bool { return nameRE.MatchString(name) }

// Dir is where bridges are kept, next to the myco config.
func Dir(mycoDir string) string { return filepath.Join(mycoDir, "im") }

func configPath(dir, name string) string { return filepath.Join(dir, name+".json") }

// Load reads a bridge's config.
func Load(dir, name string) (*Config, error) {
	if !ValidName(name) {
		return nil, fmt.Errorf("invalid bridge name %q", name)
	}
	b, err := os.ReadFile(configPath(dir, name))
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", configPath(dir, name), err)
	}
	return &c, nil
}

// Save writes a bridge's config (0600).
func Save(dir string, c *Config) error {
	if !ValidName(c.Name) {
		return fmt.Errorf("invalid bridge name %q", c.Name)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(configPath(dir, c.Name), b)
}

// Remove deletes a bridge's config and its conversation state.
func Remove(dir, name string) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid bridge name %q", name)
	}
	err := os.Remove(configPath(dir, name))
	_ = os.Remove(statePath(dir, name))
	return err
}

// List names the bridges configured in dir.
func List(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		n, ok := strings.CutSuffix(e.Name(), ".json")
		if ok && !strings.HasSuffix(n, ".state") && ValidName(n) {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out, nil
}

func writeFileAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // CreateTemp makes it 0600
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
