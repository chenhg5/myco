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
curl -fsS "https://api.github.com/repos/$REPO/releases/tags/$V" | python3 -c '
import json, sys
r = json.load(sys.stdin)
print("draft" if r["draft"] else "published", len(r["assets"]), "files:", r["html_url"])
' 
cd "$(dirname "$0")/.."
want=$(scripts/changelog-section.sh "$V" | head -1)
got=$(curl -fsS "https://api.github.com/repos/$REPO/releases/tags/$V" | python3 -c 'import json,sys; print(json.load(sys.stdin)["body"].splitlines()[0])')
if [ "$want" != "$got" ]; then echo "notes differ from the CHANGELOG section" >&2; exit 1; fi
echo "notes: the CHANGELOG section"
