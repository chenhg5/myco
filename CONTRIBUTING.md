# Contributing to myco

Thanks for helping. Bug reports, fixes, docs and new harnesses are all welcome.

## Reporting bugs and asking for features

- Search [existing issues](https://github.com/chenhg5/myco/issues) first.
- For a bug, include:
  - `myco version` for the CLI, and the hub's version (`/v1/health`).
  - Your OS.
  - Which CLI the agent runs (Claude Code, Codex, ...).
  - The command and its output.
- Remove tokens, hub URLs you consider private, and pairing links before posting.
- Security problems go through [SECURITY.md](SECURITY.md), not public issues.

## Development setup

You need:

- Go (the version in `go.mod`).
- Node 18+, only if you change the web UI.
- macOS with Xcode command line tools, only to build the macOS app.

```bash
git clone https://github.com/chenhg5/myco && cd myco
make build      # bin/myco
make check      # gofmt + vet, unit and integration tests, smoke test
```

Web UI:

```bash
cd web && npm ci
npm run dev:app      # UI with a mock backend, http://localhost:5173
cd .. && make web    # rebuild internal/web/dist (commit the result)
```

[AGENTS.md](AGENTS.md) describes the layout, commands and conventions in more detail. It is written for coding agents but applies to humans too.

## Pull requests

- Keep each PR focused on one change. Open an issue first for larger features or API changes.
- Add or update tests, and run `make check` (plus `npm run typecheck` in `web/` for UI changes).
- If you changed `web/src`, include the rebuilt `internal/web/dist`.
- Update `INSTALL.md` or `docs/` when user-visible behaviour changes.
- Do not change `VERSION`. Maintainers cut releases.

By contributing you agree that your contributions are licensed under the [MIT License](LICENSE).

## Download counts (maintainers)

myco has no telemetry. To see how many people download it, use the release download counts:

```bash
python3 scripts/downloads.py                      # totals by kind, platform and version
git fetch origin stats && git show origin/stats:stats/downloads.csv > /tmp/downloads.csv
python3 scripts/downloads.py --trend /tmp/downloads.csv 30   # downloads per day
```

The `downloads` workflow records a snapshot every day on the `stats` branch. The counts include updates and our own installs (CI, servers, checks after a release); the `.dmg` is the only file that is mostly first installs.
