# AGENTS.md

Guidance for coding agents (Claude Code, Codex, and others) working in this repository.
Humans: see [CONTRIBUTING.md](CONTRIBUTING.md).

## What this is

myco lets AI agents (Claude Code, Codex, shell nodes, ...) message each other through a small hub.
One Go module ships three things:

- **Hub**: an HTTP server with SQLite storage (`myco hub serve`).
- **CLI and runner**: `myco` commands, plus the runner that wakes local agent sessions when messages arrive.
- **App**: a React console (`web/`), wrapped as a desktop app with Wails (`cmd/myco-app`, macOS and Windows).

Read [docs/architecture.md](docs/architecture.md) for concepts and layering, [docs/api.md](docs/api.md) for the HTTP API, [docs/delivery.md](docs/delivery.md) for wake modes, and [INSTALL.md](INSTALL.md) for setup.
Before changing the App's interface, read [docs/design/app-ui.md](docs/design/app-ui.md): layout, colours, type, components, interaction and copy rules, with a checklist.

## Layout

```
cmd/myco                    CLI entry point
cmd/myco-app                desktop app (Wails v3; cgo + system WebKit on macOS)
internal/domain             entities, state machines, repository interfaces (no outside deps)
internal/app                use cases (services) behind ports
internal/infra/...          adapters: sqlite, notify, security, wake, blob, tunnel
internal/transport/httpapi  HTTP API (DTO mapping, error codes)
internal/hubserver          composition root for the hub
internal/api                JSON contract shared by client and server
internal/client             Go client for the hub
internal/cli                command tree, output, exit codes
internal/runner             runner loop
internal/harness            Harness interface; claude/, codex/, shell/, execnode/, machinenode/ implementations
internal/provider           provider settings (cc-switch, captured env)
internal/console            local backend the App talks to
internal/web/dist           built web UI, embedded in the binary (generated, committed)
internal/integration        hub + HTTP + client + CLI wired in one process
web/                        React + Vite + Tailwind source of the UI
ios/                        iOS app (SwiftUI; see ios/README.md). Built with Xcode on a Mac, not by `make`
skill/myco/SKILL.md         skill that teaches an agent to use myco
scripts/smoke.sh            drives the real binary as separate processes
```

Dependencies point inward: `domain` must not import anything from the outer layers.

## Commands

```bash
make build              # bin/myco
make vet                # gofmt check + go vet
make test               # go test -race ./... (includes integration tests)
make smoke              # build, then scripts/smoke.sh against the real binary
make check              # vet + test + smoke: what CI runs on Linux
make web                # rebuild web/ into internal/web/dist (Node 18+)
cd web && npm run typecheck
cd web && npm run dev:app   # UI against the mock App backend (web/mock/app-server.mjs)
make app                # macOS only: bin/myco.app
scripts/ship.sh         # push to main: rebuilds dist if web/ changed, rebases (dist-only conflicts rebuilt), pushes; --check runs vet+test first
```

Before you finish a change, run `make vet` and `make test`. Run `make smoke` when you change CLI or runner behaviour.

## Rules

- **Format.** Run `gofmt` on Go files; CI fails on unformatted code.
- **Web changes.** If you change `web/src`, run `make web` and commit the regenerated `internal/web/dist` (`scripts/ship.sh` does both, and rebuilds dist when a rebase conflicts on it). Go builds never run Node, so a stale `dist` ships a stale UI. Never hand-edit `dist`.
- **App UI.** New pages, components and dialogs in `web/src/app` follow `docs/design/app-ui.md`: use the existing classes and components it lists, and run its checklist (`npm run test:ui`, light and dark, two widths) before you finish.
- **Releases.** Do not touch `VERSION`. Bumping it on `main` publishes a release. Only maintainers do that, and only when they ask for it.
  When a release is asked for, add its section to `CHANGELOG.md` (`## vX.Y.Z`, newest first, user-facing changes in English, then the same in Chinese under a `**中文**` line) in the same commit as `VERSION`: the release workflow publishes that section as the release notes.
  Speed matters: a release reaches GitHub within about ten minutes of pushing `VERSION`. Push it as soon as `main`'s CI is green; do not run the UI smoke, Go tests or other checks in front of it (CI and the release workflow's own build cover them). Write the CHANGELOG section while CI runs, commit it, and let `scripts/release-wait.sh` wait: it follows main when other people push and returns as soon as go, windows, web and app-macos pass. The ui jobs are advisory, not a gate: their failures are usually the runner's Playwright install, and the App's checks run on the developer's machine. The Gitee mirror is separate and must never hold the release open: start `scripts/gitee-sync.sh vX.Y.Z` from the release machine as soon as GitHub has the release, in parallel with checking it on GitHub, not from a GitHub-hosted runner (Gitee throttles those uploads to a few files an hour).
  The steps (and the Mac preview build) are in the maintainers' internal repository, chenhg5/myco-harness: its `myco-release` and `myco-preview` skills and `context/runbooks/release.md`.
- **Windows.** CI builds and tests on native Windows too. Keep OS-specific code in `_unix.go`, `_windows.go` or `_other.go` files. Tests that need a POSIX shell must skip themselves on Windows.
- **CLI design.**
  - Commands are `noun verb` with long flags.
  - Output is JSON when stdout is not a terminal.
  - Errors are structured, with a code, a suggestion and `retryable`.
  - Exit codes are distinct per error type.
  - Side-effecting commands support `--dry-run`.
  - Keep `myco schema` and help accurate; both come from the same command definitions.
- **API changes.**
  - Update `internal/api` (the shared contract), `docs/api.md`, and the web types in `web/src/lib`.
  - Keep old clients working: runners and hubs are upgraded at different times.
- **Schema changes.** Add migrations in `internal/infra/sqlite`. Existing hub databases must upgrade in place.
- **Tests.**
  - Prefer unit tests next to the code.
  - Use `internal/integration` for flows across hub, client and CLI.
  - Tests must not depend on the caller's `MYCO_*` or `CLAUDE*` environment variables, or on `~/.myco`.
- **Secrets.** Never log or print tokens, API keys or provider settings. Tokens and pairing codes appear in URLs only where the design requires it.
- **Docs.**
  - User-facing behaviour changes go into `INSTALL.md` or `docs/`.
  - Write docs, comments and commit messages in English: most users read English.
  - `README.md`/`README.zh-CN.md` and `INSTALL.md`/`INSTALL.zh-CN.md` are kept in sync (English first).
  - `docs/design/app-ui.md` is the one design doc kept here (the UI spec); the team's other design notes live in the maintainers' internal repository.

## Official applications and locked bundles

Official app sources, `_lib` SDK, app builds, browser tests and releases live in [myco-ai/apps](https://github.com/myco-ai/apps). The three app agents each use an independent apps clone. Host bridges, Go services and `web/` stay here; do not recreate `templates/` or generated application trees in this repository.

`apps.lock.json` pins immutable release artifacts and their SHA256. Production `make build`, `make cross` and `make app` first run `make prepare-apps` (Python 3), producing ignored `internal/templates/builtin.zip` and `builtin_generated.go`. Never commit these outputs. Offline builds use `APPS_PREPARE_FLAGS=--offline` with the verified cache or explicit local artifacts. Plain Go tests use tiny fixed fixtures, do not download applications, and need no Node. A raw Go build without preparation has no bundled applications; use the Make targets for distribution. See [docs/building.md](docs/building.md) for preparation, pin updates, offline builds and cross-repository contract tests.

The apps repository publishes the rolling catalog and immutable artifacts. `.github/workflows/apps-mirror.yml` verifies and mirrors public assets to this repository's `apps` prerelease for older Hubs. Keep the compatibility mirror for at least 90 days after cutover and two stable host releases; removal requires a maintenance decision. New Hubs fetch from myco-ai/apps; runtime catalog updates remain independent of the builtin build lock.
