#!/bin/sh
# Checks a published release: the signed Mac files are there and match the
# release's SHA256SUMS, and its notes are the CHANGELOG section.
#   scripts/release-verify.sh vX.Y.Z
set -eu
V=${1:?usage: scripts/release-verify.sh vX.Y.Z}
REPO=${MYCO_REPO:-chenhg5/myco}
d=$(mktemp -d)
trap 'rm -rf "$d"' EXIT
for f in SHA256SUMS myco.dmg myco-app-darwin-universal.zip; do
  curl -fsSL --retry 3 -o "$d/$f" "https://github.com/$REPO/releases/download/$V/$f"
done
(cd "$d" && grep -E ' (myco.dmg|myco-app-darwin-universal.zip)$' SHA256SUMS | sha256sum -c)
here=$(cd "$(dirname "$0")" && pwd)
"$here/gh-get.sh" "repos/$REPO/releases/tags/$V" > "$d/release.json"
python3 -c '
import json, sys
r = json.load(open(sys.argv[1]))
print("draft" if r["draft"] else "published", len(r["assets"]), "files:", r["html_url"])
' "$d/release.json"
want=$("$here/changelog-section.sh" "$V" "$here/../CHANGELOG.md" | head -1)
got=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["body"].splitlines()[0])' "$d/release.json")
if [ "$want" != "$got" ]; then echo "notes differ from the CHANGELOG section" >&2; exit 1; fi
echo "notes: the CHANGELOG section"
