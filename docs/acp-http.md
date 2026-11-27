# CodeBuddy and WorkBuddy over ACP HTTP

The experimental `codebuddy` preset uses CodeBuddy's local streamable-HTTP
transport. Other ACP presets and the default generic `acp` harness keep their
existing stdio transport.

Install CodeBuddy Code, or use the CLI bundled with WorkBuddy. PATH takes
precedence. When no `codebuddy` command is found, myco checks the unpacked CLI in:

- macOS: `~/Applications/WorkBuddy.app` and `/Applications/WorkBuddy.app`, under
  `Contents/Resources/app.asar.unpacked/cli/bin/codebuddy`.
- Windows: `WorkBuddy/resources/app.asar.unpacked/cli/bin/codebuddy.cmd` under
  LocalAppData (including `Programs`) and Program Files. Its sibling Node script
  is executed through Node, preserving argument boundaries without cmd.exe
  interpolation. Node must be available unless the bundle includes `node.exe`.
- Linux: `/opt/WorkBuddy/resources/app.asar.unpacked/cli/bin/codebuddy`
  (case-sensitive), verified in the official deb 5.11.6.44396970 from Tencent's
  [Linux update channel](https://copilot.tencent.com/v2/update?platform=workbuddy-linux-x64-deb).
  myco also checks the same resource suffix under `/opt/apps/workbuddy`, the
  installation root documented in the [official Linux FAQ](https://www.workbuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/FAQ).
  WorkBuddy supports [UOS](https://www.workbuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Installation-UOS-Guide)
  and [Kylin](https://www.workbuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Installation-Kylin-Guide)
  through their software stores. The bundled CLI must exist and be executable,
  and its Node runtime must be available. Package contents have been inspected;
  an end-to-end Linux login/ACP session has not been tested. Custom installations
  can set `runner.binary`.

For WorkBuddy, sign in through WorkBuddy on that computer before starting an
agent. For standalone CodeBuddy Code, sign in through its CLI. A successful
ACP initialization does not imply the account is signed in: `session/new` can
still return an authentication-required error. myco does not copy credentials
between CodeBuddy and WorkBuddy accounts or regions.

```sh
myco agent spawn --cli codebuddy --workdir /path/to/project
```

For a generic CodeBuddy-compatible HTTP agent, configure:

```yaml
runner:
  harness: acp
  binary: /path/to/codebuddy
  args: [--acp]
  acp_transport: streamable-http
```

`runner.acp_transport` accepts `stdio` (the default for generic ACP) and
`streamable-http`. The latter is CodeBuddy's connection-bootstrap protocol,
not an arbitrary remote ACP URL. It starts one child per run, appends the selected transport and loopback
host/automatic-port options, and reads the child's announced local HTTP origin
from stdout/stderr. Some WorkBuddy builds ignore the requested port; myco does
not assume it was honored or scan for other local servers.

The client creates a connection with `POST /api/v1/acp/connect`, keeps the
returned connection ID and optional session token only in memory, and sends
`x-codebuddy-request`, `acp-connection-id` and `acp-session-token` headers.
JSON-RPC requests and permission replies use POST; responses may be JSON or SSE.
GET SSE carries asynchronous updates and server requests. The same ACP session,
resume, permission policy and live-step handling are used for both transports.
myco bypasses HTTP proxies for this loopback connection, refuses redirects and
non-loopback endpoints, bounds frames, and sends DELETE before stopping the
child. Cancellation uses `session/cancel` before process-group cleanup.

Older bundled builds have been observed to start HTTP even when asked for
stdio. Their stdio flag is not used by the CodeBuddy preset. Other agents'
stdio behavior is unchanged.

## AtomCode sign-in detection

The `atomcode` preset uses `atomcode acp`. The npm package
`@atomgit.com/atomcode` launches the native Rust CLI. myco detects local OAuth
credential evidence from top-level `access_token` or `refresh_token` strings in
`~/.atomcode/auth.toml`, or `auth.toml` under `ATOMCODE_HOME` when overridden.
This matches the [AtomCode OAuth store](https://github.com/atomgit-atomcode/atomcode/blob/a507ec7666cf44f5abd294448291e5e14013e2e7/crates/atomcode-auth/src/oauth.rs)
in version 5.2.8. Account metadata alone does not count. Missing, unreadable or
malformed stores keep the App status `unknown`; other authentication methods
also remain `unknown`. This is a read-only presence check, not remote validation
or a guarantee that an expired credential can still be refreshed.
