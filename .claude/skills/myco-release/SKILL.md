---
name: myco-release
description: Publish a new myco release (bump VERSION, write the CHANGELOG section, push, wait for GitHub, which signs the Mac app, verify). Use only when a maintainer asks for a release ("发版", "发个新版本", "release vX.Y.Z"), in the chenhg5/myco repository.
---

# Releasing myco

The full manual is `docs/release.md` (Chinese); this is the short path. Release only when a maintainer asks, one version at a time. Never print tokens; the Mac signing identity and certificate stay on the maintainer's Mac and in the repository's secrets, never in the repo, docs or messages. If you were asked through myco, `myco message ack` first and report each step with `myco message progress` (a checklist), then `myco message reply` at the end.

A release takes about 10 minutes after the push, mostly Apple notarizing: GitHub's release workflow builds every platform and, with the signing secrets set, signs, notarizes and packs the DMG on its macOS runner. A Mac is only the fallback.

## 1. No failing test on main

Do not wait for CI to finish: the release workflow builds every platform itself and runs the smoke test on the Linux binary.

```bash
git checkout main && git pull --rebase origin main
scripts/release-status.sh "$(git rev-parse HEAD)" ci     # no --wait
```

A failed test (go, windows, web, a UI smoke check) → stop, report it with its link. Still running, or failed only on infrastructure (a stalled Playwright install, a macOS runner queue) → go on. CI runs the UI smoke only when `web/` changed, and a newer push cancels the older run on the same branch.

## 2. Version and CHANGELOG

- Version: the latest tag plus one patch (`git fetch --tags && git describe --tags --abbrev=0`), unless the maintainer named one.
- What changed: `git log --format='%h %s' <last tag>..HEAD | grep -v ' Merge '`; read bodies with `git show -s <hash>` when the subject is not enough.
- Add `## vX.Y.Z` at the top of `CHANGELOG.md`: user-facing changes in Chinese, one bullet each, bold lead for the important ones, as in the sections below it. Leave out tests, refactors and iOS. Say when the server's hub must be upgraded (`sudo myco upgrade`).
- Preview: `scripts/changelog-section.sh vX.Y.Z`.

## 3. Commit and push

```bash
echo vX.Y.Z > VERSION
git add CHANGELOG.md VERSION && git commit -m "Release vX.Y.Z"   # with the commit trailers you were given
git pull --rebase origin main      # new code came in? wait for its CI first (step 1)
git push origin HEAD:main
sha=$(git rev-parse HEAD)
```

`VERSION` and `CHANGELOG.md` go in the same commit (the section becomes the release notes).

## 4. Wait

```bash
scripts/release-status.sh "$sha" release --wait
```

- `release` failed → the release did not happen; report the step and log. A 403 at "create release": re-run that job once.
- `ci` failed but `release` succeeded → the release is out; fix the test, push the fix, say so in the report.
- The release has no `myco.dmg` (no signing secrets, or that step failed) → sign on the Mac:
  `myco exec --to mac-exec --cmd "~/code/myco-release.sh start vX.Y.Z $sha"`, then
  `myco exec --to mac-exec --cmd "~/code/myco-release.sh status"` every minute until UPLOADED or FAILED.
  If `mac-exec` is offline, ask the maintainer to wake and unlock the Mac. FAILED, or notarization stuck over 30 minutes → stop and report the log's last lines.

## 5. Verify and report

```bash
scripts/release-verify.sh vX.Y.Z
```

Both files `OK` and `notes: the CHANGELOG section` → done. Report: the version, the release link (`https://github.com/chenhg5/myco/releases/tag/vX.Y.Z`), the changelog's main points, CI and signing results, and whether the server's hub needs `sudo myco upgrade`. If you were asked to upgrade your own myco too: `myco upgrade --version vX.Y.Z` and show `myco version`.
