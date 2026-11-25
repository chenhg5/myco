package console

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/runner"
)

// These probes inspect local credential fields only. They do not decrypt,
// refresh, validate remotely, execute helpers, or expose credential values.
func authObject(path string) map[string]json.RawMessage {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out map[string]json.RawMessage
	decoder := json.NewDecoder(io.LimitReader(f, 1<<20))
	if decoder.Decode(&out) != nil {
		return nil
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil
	}
	return out
}

func authFields(raw json.RawMessage) map[string]json.RawMessage {
	var out map[string]json.RawMessage
	_ = json.Unmarshal(raw, &out)
	return out
}

func credentialPresent(raw json.RawMessage) bool {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text) != ""
	}
	// CodeBuddy protects individual credential fields with encrypted objects.
	return len(authFields(raw)) > 0
}

func (l *Local) authDir(variable, fallback string) string {
	if dir := strings.TrimSpace(l.userGetenv(variable)); dir != "" {
		return filepath.Clean(dir)
	}
	return filepath.Join(l.Home, fallback)
}

func (l *Local) codebuddyAuthState() string {
	keyEnabled := l.userGetenv("CODEBUDDY_API_KEY_DISABLED") == ""
	helperEnabled := l.userGetenv("CODEBUDDY_API_KEY_HELPER_DISABLED") == ""
	if strings.TrimSpace(l.userGetenv("CODEBUDDY_AUTH_TOKEN")) != "" || (keyEnabled && strings.TrimSpace(l.userGetenv("CODEBUDDY_API_KEY")) != "") {
		return "ready"
	}
	settings := authObject(filepath.Join(l.authDir("CODEBUDDY_CONFIG_DIR", ".codebuddy"), "settings.json"))
	if helperEnabled && credentialPresent(settings["apiKeyHelper"]) {
		return "ready"
	}
	env := authFields(settings["env"])
	if credentialPresent(env["CODEBUDDY_AUTH_TOKEN"]) || (keyEnabled && credentialPresent(env["CODEBUDDY_API_KEY"])) {
		return "ready"
	}
	var files []string
	if path := strings.Trim(strings.TrimSpace(l.userGetenv("CODEBUDDY_AUTH_FILE_PATH")), `"`); filepath.IsAbs(path) {
		files = []string{path}
	} else {
		dir := filepath.Join(codebuddyDataDir(l.Home, runtime.GOOS), "Data", "Public", "auth")
		files, _ = filepath.Glob(filepath.Join(dir, "*.info"))
		markers, _ := filepath.Glob(filepath.Join(dir, "*.info.logged-out"))
		for _, marker := range markers {
			files = append(files, strings.TrimSuffix(marker, ".logged-out"))
		}
	}
	loggedOut := false
	for _, path := range files {
		if fileExists(path + ".logged-out") {
			loggedOut = true
			continue
		}
		record := authObject(path)
		auth := authFields(record["auth"])
		if len(authFields(record["account"])) > 0 && (credentialPresent(auth["accessToken"]) || credentialPresent(auth["refreshToken"])) {
			return "ready"
		}
	}
	if loggedOut {
		return "signed_out"
	}
	return "unknown"
}

func codebuddyDataDir(home, platform string) string {
	switch platform {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "CodeBuddyExtension")
	case "windows":
		return filepath.Join(home, "AppData", "Local", "CodeBuddyExtension")
	default:
		return filepath.Join(home, ".local", "share", "CodeBuddyExtension")
	}
}

func (l *Local) qoderSignedIn() bool {
	if strings.TrimSpace(l.userGetenv("QODER_PERSONAL_ACCESS_TOKEN")) != "" {
		return true
	}
	record := authObject(filepath.Join(l.authDir("QODER_CONFIG_DIR", ".qoder"), "qoder_oauth_creds.json"))
	// Qoder's version-1 store holds one account per sign-in region.
	var version int
	if json.Unmarshal(record["version"], &version) != nil || version != 1 {
		return false
	}
	for _, raw := range authFields(record["accounts"]) {
		account := authFields(raw)
		if credentialPresent(account["accessToken"]) || credentialPresent(account["refreshToken"]) {
			return true
		}
	}
	return false
}

func (l *Local) qwenSignedIn() bool {
	record := authObject(filepath.Join(l.Home, ".qwen", "oauth_creds.json"))
	return credentialPresent(record["access_token"]) || credentialPresent(record["refresh_token"])
}

// A successful task is positive evidence when the CLI's auth is opaque. Use
// only this computer's active profiles and a bounded, non-future timestamp.
func (l *Local) recentAgentSuccess(cli string, now time.Time) bool {
	profiles, _ := config.ListProfiles(config.ProfilesDir(l.defaultConfig()))
	for _, profile := range profiles {
		c := profile.Config
		name := c.Runner.Harness
		if name == "" {
			name = c.Harness
		}
		if (name != "" && name != "auto" && name != cli) || c.Replaced != "" || filepath.Base(c.Agent) != c.Agent || c.Agent == "" {
			continue
		}
		success := runner.ReadSuccess(filepath.Join(config.Dir(l.defaultConfig()), "state", c.Agent+".json"))
		if success != nil && success.CLI == cli && !success.At.After(now) && now.Sub(success.At) <= 24*time.Hour {
			return true
		}
	}
	return false
}
