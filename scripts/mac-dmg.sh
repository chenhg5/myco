#!/bin/sh
# Packs an app into the DMG people download: a 640x400 window with myco on
# the left, Applications on the right, the background (an arrow between
# them and one line saying what to do), no toolbar or sidebar, and the
# app's icon as the volume's.
#
#   scripts/mac-dmg.sh dist/myco.app dist/myco.dmg
#
# Finder lays the window out (AppleScript), so it needs a Mac session that
# may script Finder. Where it cannot (a Mac that says no, a runner with no
# Finder), it says why and makes the same DMG without the layout: a plain
# window still installs.
# The background is drawn by scripts/dmg-background.swift; the icon
# positions here match it.
set -eu
app=${1:?usage: scripts/mac-dmg.sh <app> <dmg>}
dmg=${2:?usage: scripts/mac-dmg.sh <app> <dmg>}
here=$(cd "$(dirname "$0")/.." && pwd)
art=$here/cmd/myco-app/build/darwin
name=myco

work=$(mktemp -d)
cleanup() {
  [ -n "${dev:-}" ] && hdiutil detach -quiet "$dev" -force 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT

stage=$work/stage
mkdir -p "$stage/.background"
ditto "$app" "$stage/$name.app"
ln -s /Applications "$stage/Applications"
# One picture with both sizes, so Retina screens get the sharp one.
tiffutil -cathidpicheck "$art/dmg-background.png" "$art/dmg-background@2x.png" -out "$stage/.background/background.tiff" >/dev/null 2>&1 ||
  cp "$art/dmg-background.png" "$stage/.background/background.tiff"
icns="$app/Contents/Resources/$name.icns"
[ -f "$icns" ] && cp "$icns" "$stage/.VolumeIcon.icns"

# A writable image first, laid out while mounted, then compressed.
size=$(du -sm "$stage" | cut -f1)
hdiutil create -quiet -volname "$name" -srcfolder "$stage" -fs HFS+ -format UDRW -size $((size + 20))m "$work/rw.dmg"
out=$(hdiutil attach -readwrite -noverify -noautoopen "$work/rw.dmg")
dev=$(echo "$out" | awk '/Apple_HFS/ {print $1; exit}')
vol=$(echo "$out" | sed -n 's|.*\(/Volumes/.*\)$|\1|p' | tail -n 1)
[ -n "$dev" ] && [ -d "$vol" ] || { echo "could not mount the image" >&2; exit 1; }
disk=$(basename "$vol")

# The volume shows its icon (the custom-icon flag on its root).
if [ -f "$vol/.VolumeIcon.icns" ] && command -v SetFile >/dev/null 2>&1; then
  SetFile -c icnC "$vol/.VolumeIcon.icns" 2>/dev/null || true
  SetFile -a C "$vol" 2>/dev/null || true
fi

# Finder lays the window out. It may never answer (a Mac that has not
# allowed this to script Finder): give it two minutes.
layout() {
  osascript <<EOF
tell application "Finder"
  tell disk "$disk"
    open
    set current view of container window to icon view
    set toolbar visible of container window to false
    set statusbar visible of container window to false
    set the bounds of container window to {200, 120, 840, 548}
    set opts to the icon view options of container window
    set arrangement of opts to not arranged
    set icon size of opts to 112
    set text size of opts to 13
    set background picture of opts to file ".background:background.tiff"
    set position of item "$name.app" of container window to {170, 190}
    set position of item "Applications" of container window to {470, 190}
    update without registering applications
    delay 2
    close
  end tell
end tell
EOF
}
layout >"$work/layout.log" 2>&1 &
pid=$!
i=0
while kill -0 "$pid" 2>/dev/null && [ $i -lt 120 ]; do sleep 1; i=$((i + 1)); done
if kill -0 "$pid" 2>/dev/null; then
  kill "$pid" 2>/dev/null || true
  echo "Finder did not lay the window out in two minutes: the DMG gets a plain window" >&2
elif ! wait "$pid"; then
  echo "Finder could not lay the window out ($(tail -n 1 "$work/layout.log")): the DMG gets a plain window" >&2
else
  # Wait for Finder to write the layout down.
  i=0
  while [ ! -f "$vol/.DS_Store" ] && [ $i -lt 10 ]; do sleep 1; i=$((i + 1)); done
  echo "laid out the DMG window"
fi
chmod -Rf go-w "$vol" 2>/dev/null || true
sync
hdiutil detach -quiet "$dev" || { sleep 3; hdiutil detach -quiet "$dev" -force; }
dev=""
rm -f "$dmg"
hdiutil convert -quiet "$work/rw.dmg" -format UDZO -imagekey zlib-level=9 -o "$dmg"
echo "made $dmg"
