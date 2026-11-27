package console

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdditionalCLIAuthEvidence(t *testing.T) {
	for _, cli := range []string{"atomcode", "kimi", "copilot", "auggie", "kilo", "cline", "grok", "antigravity", "goose"} {
		t.Run(cli+"_unknown", func(t *testing.T) {
			l := isolatedAuthLocal(t)
			if got := l.signedIn(context.Background(), cli, "must-not-run"); got != "unknown" {
				t.Fatalf("got %s", got)
			}
		})
	}
}

func TestKimiNodeCredentialsAndOverrides(t *testing.T) {
	l := isolatedAuthLocal(t)
	check := func(want bool) {
		t.Helper()
		if l.kimiSignedIn() != want {
			t.Fatalf("want configured=%v", want)
		}
	}
	writeAuthFixture(t, filepath.Join(l.Home, ".kimi", "credentials", "kimi-code.json"), `{"access_token":"fixture"}`)
	check(false) // The Python CLI's home is not the catalog's Node CLI home.
	dir := filepath.Join(t.TempDir(), "[settings]")
	t.Setenv("KIMI_CODE_HOME", dir)
	path := filepath.Join(dir, "credentials", "kimi-code.json")
	writeAuthFixture(t, path, `{"access_token":{"metadata":"fixture"}}`)
	check(false)
	writeAuthFixture(t, path, `{"refresh_token":"fixture"}`)
	check(true)
	writeAuthFixture(t, path, `{}`)
	cfg := filepath.Join(dir, "config.toml")
	writeAuthFixture(t, cfg, "[providers.demo]\ntype='openai'\napi_key_env='MYCO_AUTH_FIXTURE_KEY'\n")
	t.Setenv("MYCO_AUTH_FIXTURE_KEY", "")
	check(false)
	t.Setenv("MYCO_AUTH_FIXTURE_KEY", "fixture")
	check(true)
	writeAuthFixture(t, cfg, "[providers.demo]\napi_key='fixture'\n")
	check(true)
	writeAuthFixture(t, cfg, "[providers.demo]\napi_key=")
	check(false)
}

func TestCopilotEnvironmentWithoutSecretHelpers(t *testing.T) {
	l := isolatedAuthLocal(t)
	writeAuthFixture(t, filepath.Join(l.Home, ".copilot", "config.json"), `{"logged_in_users":[{"login":"demo"}]}`)
	if l.copilotSignedIn() {
		t.Fatal("account metadata is not a credential")
	}
	t.Setenv("GITHUB_TOKEN", "github_pat_fixture")
	if !l.copilotSignedIn() {
		t.Fatal("fine-grained token missing")
	}
	t.Setenv("COPILOT_GITHUB_TOKEN", "ghp_unsupported")
	if l.copilotSignedIn() {
		t.Fatal("must respect highest-precedence unsupported token")
	}
	t.Setenv("COPILOT_PROVIDER_BASE_URL", "https://example.com/v1")
	if l.copilotSignedIn() {
		t.Fatal("endpoint alone is insufficient")
	}
	t.Setenv("COPILOT_PROVIDER_API_KEY", "fixture")
	if !l.copilotSignedIn() {
		t.Fatal("BYOK should not require GitHub sign-in")
	}
}

func TestAuggieSessionLocationAndPairs(t *testing.T) {
	l := isolatedAuthLocal(t)
	other := t.TempDir()
	t.Setenv("AUGMENT_CODING_AGENT_DIR", other)
	writeAuthFixture(t, filepath.Join(other, "session.json"), `{"accessToken":"fixture","tenantURL":"https://example.com"}`)
	if l.auggieSignedIn() {
		t.Fatal("agent settings dir is not session dir")
	}
	t.Setenv("AUGMENT_CACHE_DIR", "~/custom-augment")
	path := filepath.Join(l.Home, "custom-augment", "session.json")
	writeAuthFixture(t, path, `{"accessToken":"fixture"}`)
	if l.auggieSignedIn() {
		t.Fatal("incomplete session")
	}
	writeAuthFixture(t, path, `{"accessToken":"fixture","tenantURL":"https://example.com"}`)
	if !l.auggieSignedIn() {
		t.Fatal("session not recognized")
	}
	t.Setenv("AUGMENT_CACHE_DIR", t.TempDir())
	t.Setenv("AUGMENT_SESSION_AUTH", `{"accessToken":"fixture","tenantURL":"https://example.com"}`)
	if !l.auggieSignedIn() {
		t.Fatal("session environment not recognized")
	}
	t.Setenv("AUGMENT_SESSION_AUTH", "invalid")
	t.Setenv("AUGMENT_API_TOKEN", "fixture")
	t.Setenv("AUGMENT_API_URL", "https://example.com")
	if !l.auggieSignedIn() {
		t.Fatal("API environment pair missing")
	}
}

func TestClineProviderStoreAndOverrides(t *testing.T) {
	for _, variable := range []string{"CLINE_DIR", "CLINE_DATA_DIR", "CLINE_PROVIDER_SETTINGS_PATH"} {
		t.Run(variable, func(t *testing.T) {
			l := isolatedAuthLocal(t)
			dir := t.TempDir()
			path := filepath.Join(dir, "data", "settings", "providers.json")
			if variable == "CLINE_DATA_DIR" {
				path = filepath.Join(dir, "settings", "providers.json")
			}
			if variable == "CLINE_PROVIDER_SETTINGS_PATH" {
				path = filepath.Join(dir, "custom.json")
				t.Setenv(variable, path)
			} else {
				t.Setenv(variable, dir)
			}
			writeAuthFixture(t, path, `{"version":1,"providers":{"cline":{"settings":{"auth":{"accessToken":"fixture"}}}}}`)
			if !l.clineSignedIn() {
				t.Fatal("OAuth credentials missing")
			}
			writeAuthFixture(t, path, `{"version":1,"providers":{"demo":{"settings":{"apiKey":"fixture"}}}}`)
			if !l.clineSignedIn() {
				t.Fatal("API credentials missing")
			}
			writeAuthFixture(t, path, `{"version":1,"providers":{"demo":{"settings":{"auth":{"method":"oauth"}}}}}`)
			if l.clineSignedIn() {
				t.Fatal("auth method alone is not configured")
			}
			writeAuthFixture(t, path, `{"version":2,"providers":{"demo":{"settings":{"apiKey":"fixture"}}}}`)
			if l.clineSignedIn() {
				t.Fatal("unfamiliar version")
			}
		})
	}
}

func TestGrokAPIStoreOverrideAndPolicy(t *testing.T) {
	l := isolatedAuthLocal(t)
	dir := t.TempDir()
	t.Setenv("GROK_HOME", dir)
	path := filepath.Join(dir, "auth.json")
	writeAuthFixture(t, path, `{"other-issuer":{"auth_mode":"oidc","key":"fixture"}}`)
	if l.grokSignedIn() {
		t.Fatal("unrelated OIDC scope")
	}
	path = filepath.Join(t.TempDir(), "custom.json")
	t.Setenv("GROK_AUTH_PATH", path)
	writeAuthFixture(t, path, `{"xai::api_key":{"auth_mode":"api_key","key":"fixture"}}`)
	if !l.grokSignedIn() {
		t.Fatal("API scope not found")
	}
	t.Setenv("GROK_DISABLE_API_KEY_AUTH", "true")
	if l.grokSignedIn() {
		t.Fatal("API auth disabled")
	}
	t.Setenv("GROK_DISABLE_API_KEY_AUTH", "")
	t.Setenv("XAI_API_KEY", "fixture")
	writeAuthFixture(t, filepath.Join(dir, "config.toml"), "[auth]\npreferred_method='oidc'\n")
	if l.grokSignedIn() {
		t.Fatal("OIDC pin must not count an API key")
	}
}

func TestAntigravityRequiresProviderAndKey(t *testing.T) {
	l := isolatedAuthLocal(t)
	t.Setenv("GEMINI_API_KEY", "fixture")
	if l.moreCLISignedIn(context.Background(), "antigravity") {
		t.Fatal("key alone has no effect in agy")
	}
	writeAuthFixture(t, filepath.Join(l.Home, ".gemini", "antigravity-cli", "settings.json"), `{"modelProvider":"gemini"}`)
	if !l.moreCLISignedIn(context.Background(), "antigravity") {
		t.Fatal("configured Gemini provider not found")
	}
	t.Setenv("AGY_PATH", "custom-binary")
	if l.moreCLISignedIn(context.Background(), "antigravity") {
		t.Fatal("custom binary storage is unknown")
	}
}

func TestGoosePlatformDirectories(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	roaming := t.TempDir()
	xdg := t.TempDir()
	for _, tc := range []struct {
		platform string
		env      map[string]string
		want     string
	}{
		{"windows", map[string]string{"APPDATA": roaming}, filepath.Join(roaming, "Block", "goose", "config")},
		{"windows", nil, ""},
		{"darwin", nil, filepath.Join(home, ".config", "goose")},
		{"linux", map[string]string{"XDG_CONFIG_HOME": xdg}, filepath.Join(xdg, "goose")},
		{"linux", map[string]string{"XDG_CONFIG_HOME": "relative"}, filepath.Join(home, ".config", "goose")},
		{"windows", map[string]string{"GOOSE_PATH_ROOT": root, "APPDATA": roaming}, filepath.Join(root, "config")},
	} {
		if got := gooseAuthDir(home, tc.platform, func(k string) string { return tc.env[k] }); got != tc.want {
			t.Fatalf("%s path=%s want=%s", tc.platform, got, tc.want)
		}
	}
}
func TestGooseConfiguredProvider(t *testing.T) {
	l := isolatedAuthLocal(t)
	dir := t.TempDir()
	t.Setenv("GOOSE_PATH_ROOT", dir)
	path := filepath.Join(dir, "config", "config.yaml")
	writeAuthFixture(t, path, "active_provider: demo\nproviders:\n  demo:\n    enabled: true\n    model: example\n    configured: false\n")
	if l.gooseSignedIn() {
		t.Fatal("provider selection alone")
	}
	writeAuthFixture(t, path, "active_provider: demo\nproviders:\n  demo:\n    enabled: true\n    model: example\n    configured: true\n")
	if !l.gooseSignedIn() {
		t.Fatal("configured provider")
	}
	t.Setenv("GOOSE_PROVIDER", "other")
	if l.gooseSignedIn() {
		t.Fatal("process provider override")
	}
	t.Setenv("GOOSE_PROVIDER", "")
	t.Setenv("GOOSE_ADDITIONAL_CONFIG_FILES", "custom.yaml")
	if l.gooseSignedIn() {
		t.Fatal("unresolved layered config must stay unknown")
	}
}

func TestKiloReadOnlyCredentialEvidence(t *testing.T) {
	l := isolatedAuthLocal(t)
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	path := filepath.Join(data, "kilo", "kilo.db")
	if l.kiloSignedIn(context.Background()) {
		t.Fatal("missing database")
	}
	if fileExists(path) {
		t.Fatal("probe created database")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`CREATE TABLE account(id TEXT PRIMARY KEY, access_token TEXT, refresh_token TEXT)`,
		`CREATE TABLE account_state(active_account_id TEXT)`,
		`CREATE TABLE credential(integration_id TEXT, connector_id TEXT, active INTEGER, value TEXT)`,
		`INSERT INTO account VALUES('demo','fixture','')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if l.kiloSignedIn(context.Background()) {
		t.Fatal("inactive account")
	}
	if _, err = db.Exec(`INSERT INTO account_state VALUES('demo')`); err != nil {
		t.Fatal(err)
	}
	if !l.kiloSignedIn(context.Background()) {
		t.Fatal("active account")
	}
	if _, err = db.Exec(`DELETE FROM account_state`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO credential VALUES('demo',NULL,0,'{"type":"key","key":"fixture"}')`); err != nil {
		t.Fatal(err)
	}
	if l.kiloSignedIn(context.Background()) {
		t.Fatal("inactive credential")
	}
	if _, err = db.Exec(`UPDATE credential SET active=1`); err != nil {
		t.Fatal(err)
	}
	if !l.kiloSignedIn(context.Background()) {
		t.Fatal("active API credential")
	}
	if _, err = db.Exec(`UPDATE credential SET connector_id='mcp'`); err != nil {
		t.Fatal(err)
	}
	if l.kiloSignedIn(context.Background()) {
		t.Fatal("MCP credential is not model auth")
	}

	// A custom path must be used literally, with URI metacharacters escaped.
	t.Setenv("KILO_DB", filepath.Join(t.TempDir(), "custom #.db"))
	custom, err := sql.Open("sqlite", os.Getenv("KILO_DB"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = custom.Exec(`CREATE TABLE credential(integration_id TEXT, connector_id TEXT, active INTEGER, value TEXT); INSERT INTO credential VALUES('demo',NULL,1,'{"type":"oauth","access":"fixture"}')`); err != nil {
		t.Fatal(err)
	}
	if err = custom.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if l.kiloSignedIn(ctx) {
		t.Fatal("cancelled probe")
	}
	before, err := os.ReadFile(os.Getenv("KILO_DB"))
	if err != nil {
		t.Fatal(err)
	}
	if !l.kiloSignedIn(context.Background()) {
		t.Fatal("custom credential DB not found")
	}
	after, err := os.ReadFile(os.Getenv("KILO_DB"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("read-only probe changed the database")
	}
	t.Setenv("KILO_DB", ":memory:")
	if l.kiloSignedIn(context.Background()) {
		t.Fatal("in-memory DB cannot be inspected")
	}
	t.Setenv("KILO_DB", filepath.Join(t.TempDir(), "custom #?.db"))
	if l.kiloSignedIn(context.Background()) {
		t.Fatal("override must not fall back to old DB")
	}
}

func TestAuthConfigLimits(t *testing.T) {
	for _, format := range []string{"toml", "yaml"} {
		path := filepath.Join(t.TempDir(), "config")
		writeAuthFixture(t, path, strings.Repeat(" ", 1<<20)+"secret")
		if authConfig(path, format) != nil {
			t.Fatal("oversized configuration accepted")
		}
	}
}

func TestAtomcodeOAuthEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		ready      bool
	}{
		{"access", "access_token = 'fixture'\ntoken_type = 'Bearer'\n[user]\nid = 'demo'\nusername = 'demo'\n", true},
		{"refresh", "refresh_token = 'fixture'\n", true},
		{"empty", "access_token = '  '\nrefresh_token = ''\n", false},
		{"metadata", "token_type = 'Bearer'\n[user]\nid = 'demo'\nusername = 'demo'\n", false},
		{"nested", "[access_token]\nmetadata = 'fixture'\n", false},
		{"number", "access_token = 123\n", false},
		{"invalid", "access_token =", false},
		{"oversize", "access_token = 'fixture'\n#" + strings.Repeat("x", 1<<20), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := isolatedAuthLocal(t)
			path := filepath.Join(l.Home, ".atomcode", "auth.toml")
			writeAuthFixture(t, path, tc.body)
			want := "unknown"
			if tc.ready {
				want = "ready"
			}
			if got := l.signedIn(context.Background(), "atomcode", "must-not-run"); got != want {
				t.Fatalf("state=%s want=%s", got, want)
			}
			body, err := os.ReadFile(path)
			if err != nil || string(body) != tc.body {
				t.Fatal("credential file changed")
			}
		})
	}
}

func TestAtomcodeHomeOverride(t *testing.T) {
	l := isolatedAuthLocal(t)
	writeAuthFixture(t, filepath.Join(l.Home, ".atomcode", "auth.json"), `{"access_token":"fixture"}`)
	if l.atomcodeSignedIn() {
		t.Fatal("unverified JSON store must not count")
	}
	writeAuthFixture(t, filepath.Join(l.Home, ".atomcode", "auth.toml"), "access_token = 'fixture'\n")
	if !l.atomcodeSignedIn() {
		t.Fatal("default store missing")
	}
	dir := filepath.Join(t.TempDir(), "custom home")
	t.Setenv("ATOMCODE_HOME", dir)
	if l.atomcodeSignedIn() {
		t.Fatal("override must not fall back to default home")
	}
	writeAuthFixture(t, filepath.Join(dir, "auth.toml"), "access_token = 'fixture'\n")
	if !l.atomcodeSignedIn() {
		t.Fatal("relocated store missing")
	}
}
