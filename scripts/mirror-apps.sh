#!/usr/bin/env bash
# Copy verified app releases for Hubs that still use the main repository URL.
# Run in GitHub Actions with this repository's GITHUB_TOKEN, or with gh auth.
set -euo pipefail
source_repo=myco-ai/apps
target_repo=${GITHUB_REPOSITORY:-myco-ai/myco}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
assets=(apps.zip builtin.zip app-locales.json builtin.json)
verified=0
for attempt in 1 2 3; do
  mkdir -p "$work/$attempt"
  if gh release download apps --repo "$source_repo" --dir "$work/$attempt" \
      --pattern apps.zip --pattern builtin.zip --pattern app-locales.json --pattern builtin.json --pattern SHA256SUMS &&
    python3 - "$work/$attempt" <<'PY'
from pathlib import Path
import hashlib
import re
import sys
root = Path(sys.argv[1])
expected = {'apps.zip', 'builtin.zip', 'app-locales.json', 'builtin.json'}
lines = (root / 'SHA256SUMS').read_text().splitlines()
seen = set()
for line in lines:
    match = re.fullmatch(r'([0-9a-f]{64})  ([a-zA-Z0-9_.-]+)', line)
    if not match or match[2] not in expected or match[2] in seen:
        raise SystemExit('Invalid or unexpected checksum entry')
    name = match[2]
    seen.add(name)
    if hashlib.sha256((root / name).read_bytes()).hexdigest() != match[1]:
        raise SystemExit('Checksum mismatch: ' + name)
if seen != expected:
    raise SystemExit('Incomplete checksum manifest')
print('Verified apps.zip, builtin.zip and catalog metadata')
PY
  then
    verified=1
    work_assets="$work/$attempt"
    break
  fi
  # A rolling release may change between requests. Never publish mixed bytes.
  if [ "$attempt" -lt 3 ]; then sleep 5; fi
done
[ "$verified" = 1 ] || { echo 'Source release was not verified; old release left untouched' >&2; exit 1; }
if gh release view apps --repo "$target_repo" >/dev/null 2>&1; then
  mkdir -p "$work/old"
  if gh release download apps --repo "$target_repo" --pattern SHA256SUMS --dir "$work/old" &&
      cmp -s "$work/old/SHA256SUMS" "$work_assets/SHA256SUMS"; then
    echo 'Legacy catalog is already current'
    exit 0
  fi
else
  gh release create apps --repo "$target_repo" --title 'Official myco apps (compatibility mirror)' \
    --notes 'Exact verified mirror of myco-ai/apps for older Hubs.' --prerelease --latest=false
fi
paths=()
for name in "${assets[@]}"; do paths+=("$work_assets/$name"); done
# Existing Hubs retry checksum mismatch during this brief update window.
gh release upload apps "${paths[@]}" --repo "$target_repo" --clobber
gh release upload apps "$work_assets/SHA256SUMS" --repo "$target_repo" --clobber
gh release edit apps --repo "$target_repo" --prerelease --latest=false \
  --notes "Compatibility mirror of https://github.com/myco-ai/apps/releases/tag/apps. All four artifacts were SHA256 verified before upload."
echo "Mirrored verified assets to https://github.com/$target_repo/releases/tag/apps"
