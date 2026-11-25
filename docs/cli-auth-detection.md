# Local CLI sign-in detection

Settings → Agents checks installed CLIs without sending a model request. A
credential file or API configuration is evidence that the CLI is configured;
it does not prove the account still has quota or that a remote token is valid.
Checks never refresh tokens, decrypt protected fields, run API-key helpers, or
return credential contents to the interface.

An installed CLI whose sign-in cannot be determined shows **Installed**, with
an optional **Go to sign in** link. If it already works, no action is needed.
Only an explicit signed-out result shows the normal **Sign in** button.

## CodeBuddy Code

Verified against the installed official `@tencent-ai/codebuddy-code` package
2.186.0. Its platform sign-in store is separate from the CLI settings directory:

| Platform | Platform sign-in files |
| --- | --- |
| macOS | `~/Library/Application Support/CodeBuddyExtension/Data/Public/auth/*.info` |
| Windows | `%USERPROFILE%\AppData\Local\CodeBuddyExtension\Data\Public\auth\*.info` |
| Linux | `~/.local/share/CodeBuddyExtension/Data/Public/auth/*.info` |

These paths come from the CLI's home-directory-based `FilePathService`, rather
than assuming the roaming `%APPDATA%` npm installation folder also holds auth.
The default product file is `Tencent-Cloud.coding-copilot.info`; regional or
custom products can have another name. An `account` object and nonempty
`auth.accessToken` or `auth.refreshToken` fields count as configured. Protected
credential objects are recognized without decryption. A sibling `.logged-out`
marker prevents that file from counting. An absolute `CODEBUDDY_AUTH_FILE_PATH`
overrides the platform file location.

The App also recognizes `CODEBUDDY_API_KEY` / `CODEBUDDY_AUTH_TOKEN` in its
inherited or login-shell environment, or the `env` object in
`~/.codebuddy/settings.json`. A configured `apiKeyHelper` also counts, without
executing it. `CODEBUDDY_CONFIG_DIR` overrides the settings directory; it does
not relocate the platform sign-in store. MCP's `.credentials.json` is not
account sign-in evidence.

See the official [authentication guide](https://www.codebuddy.ai/docs/cli/iam)
and [environment reference](https://www.codebuddy.ai/docs/cli/env-vars).

## Qoder CLI

Verified against the installed official `@qoder-ai/qodercli` package 1.1.97.
The default store is `~/.qoder/qoder_oauth_creds.json` on macOS/Linux and
`%USERPROFILE%\.qoder\qoder_oauth_creds.json` on Windows. `QODER_CONFIG_DIR`
overrides the directory. The version-1 store has regional entries under
`accounts`; a nonempty `accessToken` or `refreshToken` is recognized. An inherited
or login-shell `QODER_PERSONAL_ACCESS_TOKEN` also counts. Missing, empty,
unreadable or unfamiliar file formats leave the state unknown.

These checks are entirely local. The CLI's interactive `/status` or
`qodercli status --output json` is not needed. See the official
[authentication guide](https://docs.qoder.com/cli/authentication).

## Qwen Code and other CLIs

Qwen's legacy `~/.qwen/oauth_creds.json` is recognized through nonempty
`access_token` or `refresh_token` fields, including the corresponding
`%USERPROFILE%\.qwen` directory on Windows. Other Qwen credential formats and
CLIs without a verified matching probe remain unknown when installed.
They can still be used. See Qwen's
[authentication documentation](https://github.com/QwenLM/qwen-code/blob/main/docs/users/configuration/auth.md).

## Recent local success

When a direct probe cannot determine sign-in, the App also checks active myco
profiles on this computer. A matching CLI's successful task run within the last
24 hours counts as ready. Future or older timestamps, replaced profiles, CLI
mismatches, failed runs, held tasks and ordinary session activity do not supply
this evidence. An explicit signed-out result is not overridden.

New runners retain only the CLI name and success timestamp in the existing
local runner state file. Older state files remain readable and simply have no
success evidence until the upgraded runner successfully runs a task. No Hub
API, network history query or task-body copy is needed for this fallback.

## Additional CLI audit

The following official packages were installed in an isolated macOS prefix and
checked with version/help commands: Kimi Code **2.7.3**, GitHub Copilot
**1.0.115**, Auggie **1.2.3**, Kilo **7.8.32**, Cline **3.0.75**, Grok Build
**1.0.92**, Google Antigravity CLI **1.5.17**, and Goose **1.61.0**. No sign-in
or model request was made. Paths and formats were checked against the installed
packages and their first-party documentation/source. This is a format audit,
not a live Windows or Linux account test.

In this section, `~` means the user's home directory on macOS/Linux and
`%USERPROFILE%` on Windows. Environment overrides come from the App's inherited
or login-shell environment. An override supplied only as a terminal flag or to
one runner process is not visible to the App; an unresolved configuration stays
unknown. Missing, unreadable, malformed, or unfamiliar stores also stay unknown.
TOML/YAML/JSON reads are bounded; errors and credential values are not logged.

| CLI | macOS / Linux store | Windows store | Overrides and detected evidence |
| --- | --- | --- | --- |
| Kimi Code (Node) | `~/.kimi-code/credentials/kimi-code*.json`; `config.toml` in the same home | `%USERPROFILE%\.kimi-code\credentials\kimi-code*.json`; sibling `config.toml` | `KIMI_CODE_HOME`; OAuth `access_token`/`refresh_token`, or provider `api_key` / a populated named `api_key_env` in TOML |
| GitHub Copilot | Native secure store; `~/.copilot/config.json` is application state | Windows Credential Manager; `%USERPROFILE%\.copilot\config.json` | `COPILOT_HOME` relocates state. Recognized environment tokens: `COPILOT_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN` in that order; BYOK requires both `COPILOT_PROVIDER_BASE_URL` and `COPILOT_PROVIDER_API_KEY` |
| Auggie | `~/.augment/session.json` | `%USERPROFILE%\.augment\session.json` | `AUGMENT_CACHE_DIR` (including `~/`); `accessToken` **and** `tenantURL`; alternatively paired fields in `AUGMENT_SESSION_AUTH`, or `AUGMENT_API_TOKEN` **and** `AUGMENT_API_URL` |
| Kilo | `~/.local/share/kilo/kilo.db` by default, including on macOS | `%USERPROFILE%\.local\share\kilo\kilo.db` | `XDG_DATA_HOME` changes the base; `KILO_DB` changes the DB filename (relative to data dir) or absolute path. Read-only SQL checks active account token presence or an active model integration credential |
| Cline | `~/.cline/data/settings/providers.json` | `%USERPROFILE%\.cline\data\settings\providers.json` | `CLINE_DIR`, then `CLINE_DATA_DIR`, then `CLINE_PROVIDER_SETTINGS_PATH`; version 1 `providers.*.settings.apiKey` or `auth.apiKey`/`accessToken`/`refreshToken` |
| Grok Build | `~/.grok/auth.json`; sibling `config.toml` | `%USERPROFILE%\.grok\auth.json`; sibling `config.toml` | `GROK_HOME`, `GROK_AUTH_PATH`; `xai::api_key` scope with `auth_mode=api_key` and a nonempty `key`, or `XAI_API_KEY`, respecting detected API-key disable/team/OIDC pins |
| Antigravity | Native secure store; `~/.gemini/antigravity-cli/settings.json` | Windows Credential Manager; `%USERPROFILE%\.gemini\antigravity-cli\settings.json` | A populated `GEMINI_API_KEY` counts only with `modelProvider: "gemini"`. No verified settings-directory override. Wrapper `AGY_PATH` selects an executable, not a credential directory |
| Goose | `${XDG_CONFIG_HOME:-~/.config}/goose/config.yaml` on both macOS/Linux | `%APPDATA%\Block\goose\config\config.yaml` (roaming) | Absolute `GOOSE_PATH_ROOT` changes this to `<root>/config/config.yaml`; selected provider must have `enabled: true`, `configured: true`, and a model. `GOOSE_PROVIDER` overrides the selection |

### Deliberately unresolved cases

- **Kimi:** The catalog installs the Node `@moonshot-ai/kimi-code` product.
  Python `kimi-cli` uses a different `~/.kimi` home; its mere presence does not
  prove that the Node CLI is configured. Custom OAuth storage names and explicit
  config-path flags are not guessed.
- **Copilot:** Secure-store tokens and `gh auth` fallback remain unknown. The
  probe never invokes `gh auth token`, decrypts a keyring, or infers a token from
  `logged_in_users`. The documented plaintext `config.json` fallback has no
  verified stable token-field schema in this audit, so it is not parsed as a
  credential. Classic `ghp_` PATs are unsupported by Copilot CLI; supported
  fine-grained/OAuth prefixes count as configuration evidence. Named BYOK
  provider files, callback-only providers, and Entra auth are not resolved.
- **Auggie:** `AUGMENT_CODING_AGENT_DIR` relocates agent settings and `auth.json`,
  not `session.json`. MCP/provider metadata in those files is insufficient.
- **Kilo:** This version stores credentials in SQLite. The probe opens only an
  existing file with `mode=ro` and `query_only`, uses a one-second deadline, and
  returns SQL `EXISTS` booleans; token columns never enter the Go result. It
  does not migrate, refresh, or start the CLI. Inactive accounts, MCP-only
  credentials, old `auth.json`/legacy account files, `KILO_AUTH_CONTENT`,
  `KILO_DB=:memory:`, nonstandard release-channel databases, and unfamiliar DB
  schemas remain unknown. Old JSON files can outlive migration/logout and must
  not override the current DB's state.
- **Cline:** Old `data/secrets.json` / `globalState.json`, provider helper
  commands, AWS profiles, ADC, and other ambient SDK credentials remain unknown.
  The current version uses a versioned provider-settings file; selecting a
  provider without credential fields is insufficient.
- **Grok:** OIDC issuer/client/team-scoped entries, `GROK_AUTH` process snapshots,
  deployment credentials, external auth-provider commands, managed policy
  layers, and per-model BYOK are not resolved. Finding a token for a different
  issuer is not proof that the selected configuration can use it. The check
  never calls `grok login` or its auth-manager/refresh functions.
- **Antigravity:** The catalog's `google-antigravity-acp` **2.0.1** is a
  third-party wrapper, not Google's own package. Its source was inspected
  without running it; it delegates to `agy` (or `AGY_PATH`) and can download a
  binary on first run. The separately installed Google CLI's version/help were
  verified. Account keyring profiles, ADC/workforce identity, custom binaries,
  and session-only `--settings` overrides remain unknown. A settings file alone
  is not account sign-in evidence.
- **Goose:** A selected, explicitly configured provider is configuration
  evidence, including local providers that need no key. This does not decrypt
  or validate its secret. Older flat-key configs, a keyring item alone,
  `secrets.yaml` alone, custom layered `GOOSE_ADDITIONAL_CONFIG_FILES`, and
  system-only settings, and Windows without `APPDATA` remain unknown. Naming a provider/model without its
  configured flag is insufficient.

### Windows path audit

The current catalog has 15 CLIs. The rows below distinguish possible path
misses from opaque authentication; **unknown does not mean signed out**. The
24-hour local-success fallback described above applies to all of them.

| CLI | Windows path risk / current behavior |
| --- | --- |
| Claude | Existing home credential/settings and directory probes; macOS keychain lookup is platform-only. Unrecognized Windows credential arrangements can still yield signed out in that existing detector. This audit did not replace it. |
| Codex | Uses the resolved CLI's `login status` and `CODEX_HOME` handling, rather than guessing `%APPDATA%`. Provider configurations can remain unknown. |
| Gemini | Existing home-based `.gemini` probe; custom homes/ambient API credentials can be missed. This audit leaves that existing detector unchanged. |
| OpenCode | Existing `%USERPROFILE%\.local\share\opencode\auth.json` probe; an `XDG_DATA_HOME` override can be missed. Existing detector unchanged. |
| CodeBuddy | Fixed: platform auth is under home-based `AppData\Local\CodeBuddyExtension`, not roaming `%APPDATA%`; auth-file override supported. |
| Qoder | Fixed: home `.qoder`, not `%APPDATA%`; `QODER_CONFIG_DIR` supported. |
| Qwen | Legacy home `.qwen` OAuth probe; other credential formats remain unknown. Existing detector unchanged. |
| Kimi | Home `.kimi-code`, not `.kimi` or `%APPDATA%`; `KIMI_CODE_HOME` supported. Python CLI remains a separate unresolved case. |
| Copilot | Home `.copilot` / `COPILOT_HOME` is state, not proof of the Credential Manager token. Opaque secure-store sign-in remains unknown, rather than reported as signed out. |
| Auggie | Home `.augment` / `AUGMENT_CACHE_DIR`, not `%APPDATA%`. `AUGMENT_CODING_AGENT_DIR` must not be mistaken for the session directory. |
| Kilo | Uses XDG-style home `.local\share\kilo` even on Windows; `XDG_DATA_HOME` and `KILO_DB` supported. Merely using `%APPDATA%` would miss it. |
| Cline | Home `.cline\data`, not `%APPDATA%`; all three directory/file overrides supported. Old formats/helpers remain unknown. |
| Grok | Home `.grok`, not `%APPDATA%`; `GROK_HOME` and `GROK_AUTH_PATH` supported. Other scoped/policy auth remains unknown. |
| Antigravity | Home `.gemini\antigravity-cli` stores settings; real account credentials use Credential Manager. Settings/API pair detected; opaque accounts stay unknown. |
| Goose | Uses roaming `%APPDATA%\Block\goose\config`; respects redirected `%APPDATA%`, `GOOSE_PATH_ROOT`, and provider selection. Assuming home `.config` would miss it. |

Sources: [Kimi configuration](https://github.com/MoonshotAI/kimi-code/blob/main/docs/en/configuration/config-files.md),
[Copilot authentication](https://docs.github.com/en/copilot/how-tos/copilot-cli/set-up-copilot-cli/authenticate-copilot-cli),
[Auggie authentication](https://docs.augmentcode.com/cli/setup-auggie/authentication),
[Kilo credential storage](https://github.com/Kilo-Org/kilocode/blob/main/packages/core/src/credential.ts),
[Cline provider storage](https://github.com/cline/cline/blob/main/sdk/packages/core/src/services/storage/provider-settings-manager.ts),
[Grok login storage](https://github.com/xai-org/grok-build/blob/main/crates/codegen/xai-grok-login/src/storage.rs),
[Antigravity authentication](https://antigravity.google/docs/install?tab=cli),
[Goose configuration](https://block.github.io/goose/docs/guides/config-files/).
