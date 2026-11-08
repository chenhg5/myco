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
codesign --force --options runtime --timestamp --sign "$MYCO_SIGN_IDENTITY" dist/myco.app
codesign --verify --strict --deep dist/myco.app

echo "==> notarizing the app"
ditto -c -k --keepParent dist/myco.app dist/notarize.zip
xcrun notarytool submit dist/notarize.zip --keychain-profile "$PROFILE" --wait
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
xcrun notarytool submit dist/myco.dmg --keychain-profile "$PROFILE" --wait
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
