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
```

Before you finish a change, run `make vet` and `make test`. Run `make smoke` when you change CLI or runner behaviour.

## Rules

- **Format.** Run `gofmt` on Go files; CI fails on unformatted code.
- **Web changes.** If you change `web/src`, run `make web` and commit the regenerated `internal/web/dist`. Go builds never run Node, so a stale `dist` ships a stale UI. Never hand-edit `dist`.
- **Releases.** Do not touch `VERSION`. Bumping it on `main` publishes a release. Only maintainers do that, and only when they ask for it.
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
  - `README.md` and `README.zh-CN.md` are kept in sync.
  - Most of `docs/` is in Chinese.
