# Proxy console

`myco/proxy-console` is a monochrome desktop app for an **already running**
Mihomo or sing-box core with the Clash API enabled. It is a controller, not a VPN
or a replacement for the core. Open it from Apps, or create a page:

```sh
myco template use --template myco/proxy-console --name 'Proxy console'
```

Choose **Connect local core**, choose the engine, and enter a loopback controller
address such as `http://127.0.0.1:9090` and its secret. In Mihomo this is
`external-controller` and `secret`; in sing-box it is
`experimental.clash_api.external_controller` and `secret`. Keep the controller
bound to loopback. Use the native desktop App that includes the `proxy.profile`
and `proxy.control` SDK capabilities; updating the template alone cannot add
native capabilities to an older App. Older Apps can still open **Preview**.

## What works

- Overview: cumulative transfer totals, active connections, and rates calculated
  from successive connection snapshots (approximately every two seconds).
  Samples pause in a hidden tab. Counter resets, long gaps and failures discard
  the previous rate sample. A disconnected view labels its retained data as stale.
- Policies: manual **Selector** groups, read-only automatic groups, node delay
  tests using the core's `/delay` endpoint and Google's `generate_204` test URL.
- Connections: search by host, process or policy, then close an individual
  connection. Closing interrupts that connection.
- Rule/global/direct mode switching using the existing core's configuration.
  The core's declared mode list is respected, and the applied mode is read back;
  a successful HTTP response alone is not treated as a successful mode change.
- Read-only configuration summary: ports, LAN access and TUN status when exposed.
- An in-memory action history for this visit, **not the core's raw log stream**.
- Explicit, labelled example data for a tour. Demo controls cannot modify a core.

Mihomo and sing-box share an adapter for their Clash-compatible API; individual
versions expose different subsets. Missing endpoints produce visible warnings or
an `unsupported` error, without pretending that an unsupported operation worked.
A node delay is a probe result, not a connection's measured latency.

The app does not install or launch a core, change system proxy/DNS/TUN settings,
manage subscriptions, write configuration files, or register background services.
Those require further native integration (service lifecycle, proxy ownership and
restore, and a signed macOS Network Extension for an enhanced/TUN mode).

## Credentials and local data

Controller credentials and the connection profile stay on the App's computer,
scoped by workspace and page. They use the App's system keychain; Linux follows
the existing protected-file fallback if configured. They are never returned to
the page, stored in page records, or synchronized to the Hub. The password field
is cleared after saving and closing. Leaving it blank preserves an existing
secret for the same endpoint; changing endpoints clears the old secret. To clear
a saved secret at the same endpoint, forget the connection and reconnect.

Forgetting disconnects this controller and removes its saved profile. It does
not stop the core or change system networking. Each page currently has one local
controller profile; different pages can manage different cores.

The backend accepts literal loopback IPs only (IPv4 or IPv6), no URL userinfo,
query, fragment or path. It bypasses environment/App proxies, refuses redirects,
bounds response sizes and deadlines, and returns only allowlisted configuration
and metadata fields. Upstream error bodies are not exposed. SDK audit logs keep
operation names, argument byte counts and outcome codes, never argument contents.

The SDK currently grants pages broad OS access, including commands and files.
Page scoping avoids accidental mixing; it does not turn an untrusted page into a
security sandbox. Open only pages you trust.

## Verification

```sh
export GOCACHE=/path/to/cache/go-build GOMODCACHE=/path/to/cache/go-mod
make vet
make test
cd templates
npm ci
npm run typecheck
node build.mjs proxy-console
# Set UI_SMOKE_EXECUTABLE when Playwright's default Chromium is unavailable.
node proxy-console.test.mjs
node proxy-console.ui-test.mjs
```

The browser suite covers light/dark layouts at 1200 and 760 px, preview isolation,
connection setup, mode switching, node selection and delay, connection close,
disconnection/recovery, forgetting, and an old App without the bridge. Native
tests cover credential scope, stripping sensitive response fields, redirect and
endpoint restrictions, controller errors and partial API support.

Controller references: [Mihomo API](https://wiki.metacubex.one/api/) and
[sing-box Clash API](https://sing-box.sagernet.org/configuration/experimental/clash-api/).

To exercise real cores without system networking changes, set explicit executable paths:

```sh
PROXY_TEST_CORE=/path/to/mihomo PROXY_TEST_SING_BOX=/path/to/sing-box \
  go test -race ./internal/oscap -run TestProxyRealCores -v
```

These checks use temporary profiles and ports. They are skipped by default.
