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
CLIs without a verified probe, including Kimi, remain unknown when installed.
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
