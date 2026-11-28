# Account quota sources

Myco reads only the agent's effective connection: Claude's configured
`ANTHROPIC_BASE_URL` and auth token/key, Codex's selected `model_providers`
base URL and `env_key`, or OpenCode's selected provider and credentials.
A key merely being present does not opt another connection into collection.
Pinned and project-specific configurations retain the same discovery rules.
No inference request, login, token refresh or helper is used for these API reads.

| Source | Recognized connection | Read-only endpoint and auth |
| --- | --- | --- |
| Codex subscription | Native Codex account | Native account protocol |
| Claude subscription | Native Claude account | Local cache/events/status line |
| OpenCode Go | Explicit Go connection | `https://opencode.ai/inference/go/v1/usage`, Bearer |
| MiniMax Coding Plan | Existing supported MiniMax origins | `/v1/token_plan/remains`, Bearer |
| GLM Coding Plan | HTTPS `open.bigmodel.cn` or `api.z.ai`, path `/api/anthropic`, `/api/anthropic/v1` or `/api/coding/paas/v4` | Same host, `/api/monitor/usage/quota/limit`, raw key in `Authorization` (no Bearer prefix) |
| Kimi Coding Plan | HTTPS `api.kimi.com`, path `/coding` or below it (`/coding/`, `/coding/v1`) | `https://api.kimi.com/coding/v1/usages`, Bearer |
| OpenRouter | HTTPS `openrouter.ai/api/v1` | `https://openrouter.ai/api/v1/key`, Bearer |

Trailing slashes are accepted. For the three new sources, HTTP, explicit ports,
URL user information, query/fragment overrides, encoded paths, other paths and
look-alike/gateway domains are not supported. GLM keys stay on the selected
region's host. Redirects are never followed, including same-provider redirects.
No raw response, credential, account name or local path is included in reports.
Only normalized quota numbers and an opaque machine-local reference reach the Hub.

## Interpreting the three plan APIs

GLM uses `data.limits[]`: `TOKENS_LIMIT/unit=3/number=5` is five hours;
`TOKENS_LIMIT/unit=6/number=1` is weekly; `TIME_LIMIT/unit=5/number=1` is
monthly MCP usage (`monthly_mcp`), separate from per-model buckets. `percentage` is percent used.
`nextResetTime` is optional (epoch milliseconds or an RFC 3339 timestamp).
A missing period is omitted, never fabricated. Unknown types/periods, including
unqualified `CREDIT_LIMIT` variants, invalidate the read instead of silently
hiding a possibly tighter limit. Additional unrelated object fields are ignored.
See the [official plugin](https://github.com/zai-org/zai-coding-plugins/blob/main/plugins/glm-plan-usage/skills/usage-query-skill/scripts/query-usage.mjs)
and [vendor response contract](https://github.com/zai-org/ZCode/blob/main/docs/bigmodel-coding-plan-usage-quota.md).
This monitor API is not a stable public API.

Kimi uses the count-based shape consumed by the
[official CLI](https://github.com/MoonshotAI/kimi-cli/blob/main/src/kimi_cli/ui/shell/usage.py):
top-level `usage` is the weekly allowance, while `limits[].detail` describes
each additional window (`window.duration` and `window.timeUnit`, such as 300
minutes). Counts may be JSON numbers or decimal strings. A positive `limit`
and at least one of `used`/`remaining` are required; if both are present they
must agree. Reset timestamps are optional. Ratio-only OAuth/wrapped responses
are unsupported: they are not interpreted as zero use. The endpoint is experimental.

OpenRouter's [current-key endpoint](https://openrouter.ai/docs/api/api-reference/api-keys/get-current-api-key)
reports the key's spending allowance, **not the account's total prepaid credit**.
`effective_budget.limit_usd/remaining_usd`, when supplied, take precedence over
`limit/limit_remaining`. Explicit null remaining means unlimited; a missing
field means an unreadable response. Finite allowances show remaining/limit in
USD; an overspend is shown as zero remaining. A zero limit and zero remaining
means exhausted. These are balance rows, without a period or reset countdown,
even when the provider also includes budget-reset metadata. Unlimited is shown
as “Unlimited” / “不限额”, never “Usage unknown”.

## Failures and compatibility

No key is `no_credentials`; 401/403 on these three APIs is `auth_required`;
429 is `throttled` and honors `Retry-After`. Network/HTTP failures are
`transient_error`; unrecognized formats are `schema_changed`. Both are failed
reads, never fresh zero-use observations. The shared cache keeps previous
numbers and their observation time, with the failure status. The default
successful sampling interval is five minutes, failed reads fifteen minutes;
a longer Retry-After extends the wait.

Unsupported gateways/general API connections and unrelated CLI credentials
are not probed. There is no cross-region fallback. New runners omit this quota
on an older strict Hub's precise unsupported-field/provider validation error;
other runtime fields still report normally. An older App can ignore the
optional balance object, but displaying USD/unlimited requires an updated App.
