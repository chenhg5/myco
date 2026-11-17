---
name: myco-release
description: Publish a new myco release (bump VERSION, write the CHANGELOG section, push, wait for GitHub, sign the Mac app, verify). Use only when a maintainer asks for a release ("发版", "发个新版本", "release vX.Y.Z"), in the chenhg5/myco repository.
---

# Releasing myco

The full manual is `docs/release.md` (Chinese); this is the short path. Release only when a maintainer asks, one version at a time. Never print tokens; the Mac signing identity stays on the Mac, never in the repo, docs or messages. If you were asked through myco, `myco message ack` first and report each step with `myco message progress` (a checklist), then `myco message reply` at the end.

A release takes about 5–6 minutes: GitHub and the Mac sign at the same time.

## 1. main's CI is green

```bash
git checkout main && git pull --rebase origin main
scripts/release-status.sh "$(git rev-parse HEAD)" ci --wait
```

Exit 0 → go on. Exit 1 → stop, report the failing job and its link. Do not re-run the checks locally: CI already ran Go (vet, race tests, smoke), Windows, the Mac app, the web dist check and the UI smoke on that commit.

## 2. Version and CHANGELOG

- Version: the latest tag plus one patch (`git fetch --tags && git describe --tags --abbrev=0`), unless the maintainer named one.
- What changed: `git log --format='%h %s' <last tag>..HEAD | grep -v ' Merge '`; read bodies with `git show -s <hash>` when the subject is not enough.
- Add `## vX.Y.Z` at the top of `CHANGELOG.md`: user-facing changes in Chinese, one bullet each, bold lead for the important ones, as in the sections below it. Leave out tests, refactors and iOS. Say when the server's hub must be upgraded (`sudo myco upgrade`).
- Preview: `scripts/changelog-section.sh vX.Y.Z`.

## 3. Commit, push, start the Mac signing

```bash
echo vX.Y.Z > VERSION
git add CHANGELOG.md VERSION && git commit -m "Release vX.Y.Z"   # with the commit trailers you were given
git pull --rebase origin main      # new code came in? wait for its CI first (step 1)
git push origin HEAD:main
sha=$(git rev-parse HEAD)
myco exec --to mac-exec --cmd "~/code/myco-release.sh start vX.Y.Z $sha"
```

`VERSION` and `CHANGELOG.md` go in the same commit (the section becomes the release notes). The Mac script builds, signs and notarizes right away and uploads once GitHub's release exists. If `mac-exec` is offline, ask the maintainer to wake and unlock the Mac, then run the last line.

## 4. Wait

```bash
scripts/release-status.sh "$sha" all --wait          # ci and release
myco exec --to mac-exec --cmd "~/code/myco-release.sh status"   # every minute until UPLOADED or FAILED
```

- `release` failed → the release did not happen; report the step and log.
- `ci` failed but `release` succeeded → the release is out; fix the test, push the fix, say so in the report.
- Mac `FAILED`, or notarization stuck over 30 minutes → stop and report the log's last lines.

## 5. Verify and report

```bash
scripts/release-verify.sh vX.Y.Z
```

Both files `OK` and `notes: the CHANGELOG section` → done. Report: the version, the release link (`https://github.com/chenhg5/myco/releases/tag/vX.Y.Z`), the changelog's main points, CI and Mac results, and whether the server's hub needs `sudo myco upgrade`. If you were asked to upgrade your own myco too: `myco upgrade --version vX.Y.Z` and show `myco version`.
