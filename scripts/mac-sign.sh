#!/bin/sh
# Builds the Mac app as one universal bundle (Apple silicon + Intel), signs
# it with a Developer ID, has Apple notarize it, and packs it into a DMG.
# Runs on a Mac whose keychain holds the Developer ID Application identity
# and a notarytool profile:
#   xcrun notarytool store-credentials myco --apple-id <id> --team-id <team>
#
#   MYCO_SIGN_IDENTITY="Developer ID Application: Name (TEAMID)" scripts/mac-sign.sh v0.17.2
#
# Output in dist/: myco.app (stapled), myco-app-darwin-universal.zip, myco.dmg.
# With MYCO_UPLOAD=1 it adds myco.dmg and the zip to the GitHub release
# <version> (with gh) and their lines to the release's SHA256SUMS.
set -eu
VERSION=${1:?usage: scripts/mac-sign.sh <version>}
: "${MYCO_SIGN_IDENTITY:?set MYCO_SIGN_IDENTITY to the Developer ID Application identity}"
PROFILE=${MYCO_NOTARY_PROFILE:-myco}
cd "$(dirname "$0")/.."
rm -rf dist && mkdir -p dist

# notarize <file>: submit it, then ask for its status until Apple decides.
# Each request is retried, so a network drop (a sleeping laptop, a proxy)
# does not lose a submission Apple is still working on.
notarize() {
  id=""
  for try in 1 2 3 4 5; do
    id=$(xcrun notarytool submit "$1" --keychain-profile "$PROFILE" --output-format json 2>/dev/null |
      sed -n 's/.*"id" *: *"\([^"]*\)".*/\1/p') && [ -n "$id" ] && break
    echo "submitting $1 failed (try $try); retrying" >&2; sleep 15
  done
  [ -n "$id" ] || { echo "could not submit $1 for notarization" >&2; exit 1; }
  echo "submitted $1: $id"
  for i in $(seq 1 240); do
    st=$(xcrun notarytool info "$id" --keychain-profile "$PROFILE" --output-format json 2>/dev/null |
      sed -n 's/.*"status" *: *"\([^"]*\)".*/\1/p')
    case "$st" in
      Accepted) echo "notarized $1"; return 0 ;;
      Invalid|Rejected)
        xcrun notarytool log "$id" --keychain-profile "$PROFILE" >&2 || true
        echo "notarization of $1: $st" >&2; exit 1 ;;
    esac
    sleep 15
  done
  echo "notarization of $1 still not done after an hour: $id" >&2; exit 1
}

# One bundle per architecture, then lipo the two executables together.
for arch in arm64 amd64; do
  GOARCH=$arch MYCO_SIGN_IDENTITY= make app VERSION="$VERSION" >/dev/null
  rm -rf "dist/$arch.app" && ditto bin/myco.app "dist/$arch.app"
done
ditto dist/arm64.app dist/myco.app
for f in Contents/MacOS/myco-app Contents/Resources/myco; do
  lipo -create "dist/arm64.app/$f" "dist/amd64.app/$f" -output "dist/myco.app/$f"
done
rm -rf dist/arm64.app dist/amd64.app
lipo -archs dist/myco.app/Contents/MacOS/myco-app

# Sign inside out: the bundled CLI, then the app (hardened runtime, timestamp).
codesign --force --options runtime --timestamp --sign "$MYCO_SIGN_IDENTITY" dist/myco.app/Contents/Resources/myco
# The app's entitlements: the microphone, for speaking instead of typing.
codesign --force --options runtime --timestamp --entitlements cmd/myco-app/build/darwin/entitlements.plist --sign "$MYCO_SIGN_IDENTITY" dist/myco.app
codesign --verify --strict --deep dist/myco.app

echo "==> notarizing the app"
ditto -c -k --keepParent dist/myco.app dist/notarize.zip
notarize dist/notarize.zip
xcrun stapler staple dist/myco.app
rm dist/notarize.zip
ditto -c -k --keepParent dist/myco.app dist/myco-app-darwin-universal.zip

echo "==> making the DMG"
stage=$(mktemp -d)
ditto dist/myco.app "$stage/myco.app"
ln -s /Applications "$stage/Applications"
hdiutil create -quiet -volname myco -srcfolder "$stage" -ov -format UDZO dist/myco.dmg
rm -rf "$stage"
codesign --force --timestamp --sign "$MYCO_SIGN_IDENTITY" dist/myco.dmg
notarize dist/myco.dmg
xcrun stapler staple dist/myco.dmg

spctl --assess --type execute -vv dist/myco.app
spctl --assess --type open --context context:primary-signature -vv dist/myco.dmg
echo "SIGNED dist/myco.dmg dist/myco-app-darwin-universal.zip"

if [ "${MYCO_UPLOAD:-}" = 1 ]; then
  echo "==> uploading to the $VERSION release"
  gh release download "$VERSION" --pattern SHA256SUMS --dir dist --clobber
  (cd dist && grep -v -e ' myco.dmg$' -e ' myco-app-darwin-universal.zip$' SHA256SUMS > SHA256SUMS.new || true
   shasum -a 256 myco.dmg myco-app-darwin-universal.zip >> SHA256SUMS.new && mv SHA256SUMS.new SHA256SUMS)
  gh release upload "$VERSION" dist/myco.dmg dist/myco-app-darwin-universal.zip dist/SHA256SUMS --clobber
  echo "UPLOADED myco.dmg myco-app-darwin-universal.zip SHA256SUMS"
fi
