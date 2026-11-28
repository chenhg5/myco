# Building myco with official applications

Application source, the page SDK, JavaScript checks and application releases live
in [myco-ai/apps](https://github.com/myco-ai/apps). The host repository keeps Go
services, native bridges, the console UI and a small application lock file.

## Production builds

Use Go (the version in `go.mod`), Python 3 and Make. The committed console UI
needs no Node unless you change it. macOS desktop builds also need Xcode tools.

```sh
make build                 # CLI and Hub
make cross                 # CLI for supported platforms
make app                   # macOS desktop
```

These targets prepare `internal/templates/builtin.zip` and its Go embed file
before compiling. Both are ignored, generated files. `apps.lock.json` records
the source repository, full commit, immutable `apps-<commit>` release, builtin
membership, and expected SHA256 of `builtin.zip`, `builtin.json`,
`app-locales.json` and `apps.zip`. Builds never resolve the moving `apps` tag.
Downloads, cache entries and ZIP paths/membership are checked before embedding.
A missing or corrupt bundle fails preparation; test fixtures are never used.

Before a direct desktop `go build`, run `make prepare-apps` too. A raw Go build
from an unprepared checkout succeeds for development with no bundled apps;
its Hub can still fetch the live catalog when online. It is not a distribution
build. Build every release with the preparation step.

## Offline builds

Populate the content-addressed cache while online, then use the same lock offline:

```sh
make prepare-apps
make build APPS_PREPARE_FLAGS=--offline
```

The default cache is `.cache/apps/<sha256>/<artifact>`. Copy that directory to an
offline builder (and prefetch Go modules), or transfer the exact release assets:

```sh
python3 scripts/prepare-apps.py --offline --artifacts-dir /path/to/release-assets
make build APPS_PREPARE_FLAGS=--offline
```

`--cache-dir` selects another cache. Offline preparation never accesses the
network and refuses missing or changed assets. `--download-base` selects a
public release mirror while preserving the committed tag and hash checks.

## Update or roll back a pin

Choose an immutable release in myco-ai/apps and review its source commit,
`SHA256SUMS` and builtin membership. Update all four hashes and the matching
commit/tag in `apps.lock.json`; do not generate a pin from a moving tag during
build or release. Run:

```sh
python3 scripts/prepare-apps.py --update-catalog
make web                   # only if web/src/lib/app-locales.json changed
make vet
make test
make smoke
```

Commit the lock and, if changed, the host's fallback localization metadata and
rebuilt `internal/web/dist`. Default preparation checks the committed metadata
and fails if it needs updating; it never rewrites tracked files implicitly.

Rolling back an embedded pack means restoring a previously reviewed lock and
its matching fallback metadata, then preparing and rebuilding. Verified cached
old packs work offline. This does not downgrade pages already created or remove
newer apps stored by a running Hub. Use an isolated fresh Hub and disable live
catalog checks when validating the embedded pack alone.

## Tests and application contracts

`make test` runs no-network bundle-preparation checks and Go race tests. Ordinary
service tests explicitly load the tiny fixtures in
`internal/templates/testdata/official`; these are not production applications.
Go tests work in a fresh checkout without preparing a bundle or installing Node.
After preparation, a Go test also checks embedded bytes against the lock.

CI divides the Linux race suite into three package shards using the measured
weights in `scripts/ci-race-weights.json`. Every worker runs `go list -race -json ./...` on
the exact source, then `go test -race -count=1 -json` for its assigned packages.
New packages and race-only build-tag files enter the plan automatically. A
second plain `go list` rejects packages excluded by race build tags rather than
silently omitting them. Packages
without test files are explicitly reported as compile-only success. A skipped
package with test files, a failed test or missing/duplicate terminal result fails
the shard. Vet, Python regressions and the real-binary smoke still run once.

The required `go` check is now an aggregate gate. It independently lists all
packages again and requires complete, disjoint successful results from all three
shards, bound to the source, toolchain, CI helper commit and workflow run.
`go-race-<source>-<shard>` artifacts retain JSON events, the complete manifest
and per-package outcomes for seven days; the gate summary lists every result.
Failed/cancelled jobs or stale/incomplete artifacts cannot satisfy the gate.

The CI helper is checked out from the workflow definition's immutable commit
separately from the tested source. The release caller uses the same-repository
relative workflow reference, so older frozen sources need not contain the new
helper. The CI workflow's manual entry can check an exact frozen source without
publishing a release; it defaults to the full UI matrix. Such a manual run does
not masquerade as main-push proof. Package weights are scheduling hints only,
not proof or a prediction obtained by adding overlapping package times.

For real application/native bridge acceptance, use a separate apps checkout at
the locked source commit, install its template dependencies and Chromium, then
unpack the exact released apps (not a locally rebuilt substitute):

```sh
npm --prefix /path/to/apps/templates ci
python3 scripts/prepare-apps.py --apps-dir /path/to/apps/dist/official
APPS_TEST_ROOT=/path/to/apps go test -count=1 ./internal/integration ./internal/console ./internal/sshcap -run 'Test(AgentActionsSDK|PageExportApplications|WhiteboardSharedExport|PaletteAgentToPage|ShoppingComparisonAgentToPage|DatabaseNativeBridgeAndProductionUI|RealSSHOverviewTemplateParser)$'
```

Host UI smoke, `npm --prefix web run dev:app` and promotional screenshot scenes
also read that apps checkout and its released bundles. Set `APPS_TEST_ROOT`
explicitly, or place the locked checkout at `.cache/apps-source`. They never read
retired main-repository app copies; incomplete resources fail with setup guidance.
The host UI CI prepares its own exact pinned checkout and release artifacts.

The `app-contract` CI job compiles the same seven Go fixtures on Ubuntu 24.04,
then executes the six integration/console fixtures with Node, Chromium and
CJK fonts inside the same digest-pinned Playwright 1.57.0 image used by UI smoke.
The real SSH parser fixture stays on the Ubuntu host with its installed sshd
and Node; it does not need a browser. The job installs no apt packages and
downloads no browser at runtime. The current fixture set does not
invoke external ffmpeg. Each named fixture must execute and pass; missing or
skipped cases fail the job. Source and app pins remain unchanged, and the helper
comes from the immutable workflow commit for older frozen sources. Raw JSON
outcomes and a seven-case result map are retained as `app-contract-results-*`
artifacts for seven days.

For local execution outside that image, install Playwright 1.57.0 and its
Chromium/system dependencies yourself. Without `APPS_TEST_ROOT`, only these
optional cross-repository tests skip;
an explicitly supplied incomplete checkout fails. Application business, typing,
localization and browser regression suites run in the apps repository's CI.

## Running multiple desktop App copies

Desktop App copies using the same profile share the saved local addresses for
pages. If a page says it is open in another myco App, quit that other App, then
close and reopen the page in the App you want to use. Closing only its page or
window may leave the local address in use.

Do not delete the address registry or assign a different port as a workaround:
the address is part of the browser origin used by the page's local storage.
A different address can make existing local files and settings inaccessible.
Preview builds using the same bundle identity do not guarantee a separate
WebKit data store; changing their addresses requires a storage migration.

## Runtime downloads and older Hubs

New Hubs fetch the daily rolling catalog from
`https://github.com/myco-ai/apps/releases/download/apps/`. Set
`MYCO_APPS_DOWNLOAD` to a public release download base to override application
fetches independently of binary updates. An explicitly configured legacy
`MYCO_RELEASE_DOWNLOAD` remains a fallback override. The build lock controls
bundled startup apps; it does not freeze a Hub's online catalog refreshes.

For explicit public network acceptance, run
`APPS_LIVE_ACCEPTANCE=1 go test -v ./internal/templates -run TestLiveCatalogSources`.
It downloads and compares the new and historical URLs; ordinary tests skip it.

The main repository's `apps-mirror.yml` verifies and mirrors public assets to
its `apps` prerelease, preserving old Hub URLs including the former owner URL's
GitHub redirect. It needs only the main repository's workflow token. Keep it
for at least 90 days after cutover and two stable host releases; stopping it
requires a maintenance decision.


## Version labels from source

The default Make version uses the committed `VERSION` as its release base.
An exact matching tag produces that version; other Git checkouts append a
commit count and hash, plus `-dirty` for tracked changes. This also works when
the published tag is on a release branch rather than an ancestor of main.
A source archive without Git uses `VERSION` directly. Explicit `make ...
VERSION=vX.Y.Z` overrides this label, as the publishing workflow does.

## Required language-switching gate

The release-required `app-macos` CI job runs the focused language gate on macOS
with Playwright WebKit before packaging the App. It always checks seven languages
at both window widths and in both themes, without retrying a failed step. It uses
mouse coordinates and keyboard events, checks switching back to Simplified Chinese,
reload persistence, System selection, failed-pack recovery, native-menu language
reporting and unchanged original messages. A missing matrix or step fails the gate.

The picker must use the App menu rather than an HTML `select`: macOS system WebKit
can crash when a native popup reports a selection after the language change has
removed its control. Programmatic `selectOption` bypasses that native-popup path.
The gate therefore also rejects a native language `select` before interacting.
Playwright WebKit is a browser-engine check, not a replacement for testing the
signed Wails preview on the current system WebKit after picker or native-menu changes.

To run the same gate on a Mac after preparing the pinned app fixtures:

```sh
cd web
npm ci
PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 npm install --no-save playwright@1.57.0
npx playwright install webkit
node scripts/ui-smoke.mjs --language-gate --browser webkit
```

`web/ui-smoke/language-gate.json` records the source SHA, engine, complete matrix
and outcome; CI uploads it alongside timings and the first failing screenshot.
The gate cannot be reduced with `--only`, `--combo` or `--shard`. Other local UI
checks can select `--browser webkit` or disable retry with `--no-retry`.
