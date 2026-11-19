#!/bin/bash
# Packs a built myco.app into a DMG whose window says what to do: the app
# on the left, Applications on the right, an arrow between them and one
# line under them, on a background image (cmd/myco-app/build/darwin/dmg).
# The window's layout (.DS_Store) comes from dmgbuild, which writes it
# without a Finder, so it works on a CI runner too. Without dmgbuild (no
# network for pip) it falls back to a plain DMG, as before.
#
#   scripts/mac-dmg.sh dist/myco.app dist/myco.dmg
#
# macOS only (hdiutil, tiffutil). Signing and notarizing are the caller's
# (scripts/mac-sign.sh).
set -euo pipefail

app=${1:?usage: mac-dmg.sh path/to/myco.app out.dmg}
out=${2:?usage: mac-dmg.sh path/to/myco.app out.dmg}
here=$(cd "$(dirname "$0")/.." && pwd)
art="$here/cmd/myco-app/build/darwin/dmg"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

plain() {
  echo "==> plain DMG (no dmgbuild)"
  stage="$work/stage"
  mkdir -p "$stage"
  ditto "$app" "$stage/myco.app"
  ln -s /Applications "$stage/Applications"
  hdiutil create -quiet -volname myco -srcfolder "$stage" -ov -format UDZO "$out"
}

# One TIFF holding both resolutions, so the window is sharp on Retina.
tiffutil -cathidpicheck "$art/background.png" "$art/background@2x.png" -out "$work/background.tiff" >/dev/null

if ! python3 -m venv "$work/venv" >/dev/null 2>&1 \
  || ! "$work/venv/bin/pip" install --quiet --disable-pip-version-check "dmgbuild>=1.6,<2" >/dev/null 2>&1; then
  plain
  exit 0
fi

rm -f "$out"
"$work/venv/bin/python" -m dmgbuild -s "$here/scripts/mac-dmg-settings.py" \
  -D app="$app" -D background="$work/background.tiff" myco "$out" \
  || { echo "dmgbuild failed; making a plain DMG" >&2; plain; }
