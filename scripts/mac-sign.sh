#!/bin/sh
# Builds the Mac app as one universal bundle (Apple silicon + Intel), signs
# it with a Developer ID, has Apple notarize it, and packs it into a DMG.
# Runs on a Mac whose keychain holds the Developer ID Application identity
# and a notarytool profile:
#   xcrun notarytool store-credentials myco --apple-id <id> --team-id <team>
#
#   MYCO_SIGN_IDENTITY="Developer ID Application: Name (TEAMID)" scripts/mac-sign.sh v0.17.2
#
# The release workflow runs it too, on GitHub's macOS runner, with the
# identity and the notarytool profile in a keychain of its own
# (MYCO_NOTARY_KEYCHAIN); see docs/release.md.
#
# Output in dist/: myco.app (stapled), myco-app-darwin-universal.zip, myco.dmg.
# With MYCO_UPLOAD=1 it waits for the GitHub release <version> (made by
# release.yml from the same commit), then adds myco.dmg and the zip to it
# (with gh), with their lines in the release's SHA256SUMS.
set -eu
VERSION=${1:?usage: scripts/mac-sign.sh <version>}
: "${MYCO_SIGN_IDENTITY:?set MYCO_SIGN_IDENTITY to the Developer ID Application identity}"
PROFILE=${MYCO_NOTARY_PROFILE:-myco}
# notarytool looks for the profile in this keychain (default: the login one).
KC=${MYCO_NOTARY_KEYCHAIN:+--keychain $MYCO_NOTARY_KEYCHAIN}
cd "$(dirname "$0")/.."
rm -rf dist && mkdir -p dist

# notarize <file>: submit it, then ask for its status until Apple decides.
# Each request is retried, so a network drop (a sleeping laptop, a proxy)
# does not lose a submission Apple is still working on.
notarize() {
  id=""
  for try in 1 2 3 4 5; do
    id=$(xcrun notarytool submit "$1" --keychain-profile "$PROFILE" $KC --output-format json 2>dist/notary.err |
      sed -n 's/.*"id" *: *"\([^"]*\)".*/\1/p') && [ -n "$id" ] && break
    # Say why (a missing keychain profile, the network, Apple): the reason
    # decides whether waiting helps or someone has to act on this Mac.
    echo "submitting $1 failed (try $try): $(tail -n 2 dist/notary.err 2>/dev/null | tr '\n' ' '); retrying" >&2; sleep 15
  done
  [ -n "$id" ] || { echo "could not submit $1 for notarization" >&2; exit 1; }
  echo "submitted $1: $id"
  for i in $(seq 1 240); do
    st=$(xcrun notarytool info "$id" --keychain-profile "$PROFILE" $KC --output-format json 2>/dev/null |
      sed -n 's/.*"status" *: *"\([^"]*\)".*/\1/p')
    case "$st" in
      Accepted) echo "notarized $1"; return 0 ;;
      Invalid|Rejected)
        xcrun notarytool log "$id" --keychain-profile "$PROFILE" $KC >&2 || true
        echo "notarization of $1: $st" >&2; exit 1 ;;
    esac
    sleep 15
  done
  echo "notarization of $1 still not done after an hour: $id" >&2; exit 1
}

# One bundle per architecture, then lipo the two executables together.
for arch in arm64 amd64; do
  GOARCH=$arch MYCO_SIGN_IDENTITY='' make app VERSION="$VERSION" >/dev/null
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
scripts/mac-dmg.sh dist/myco.app dist/myco.dmg
codesign --force --timestamp --sign "$MYCO_SIGN_IDENTITY" dist/myco.dmg
notarize dist/myco.dmg
xcrun stapler staple dist/myco.dmg

# Gatekeeper's verdict, without the origin line (the signing identity's
# name), which would otherwise land in the release workflow's public log.
assess() {
  out=$(spctl --assess -vv "$@" 2>&1) || { echo "$out" | grep -v '^origin=' >&2; exit 1; }
  echo "$out" | grep -v '^origin='
}
assess --type execute dist/myco.app
assess --type open --context context:primary-signature dist/myco.dmg
echo "SIGNED dist/myco.dmg dist/myco-app-darwin-universal.zip"

if [ "${MYCO_UPLOAD:-}" = 1 ]; then
  # Signing starts when the release commit is pushed, alongside GitHub's
  # release workflow: wait (up to 30 minutes) for the release it creates.
  echo "==> waiting for the $VERSION release on GitHub"
  for i in $(seq 1 120); do
    gh release view "$VERSION" --json assets --jq '.assets[].name' 2>/dev/null | grep -qx SHA256SUMS && break
    [ "$i" = 120 ] && { echo "the $VERSION release did not appear within 30 minutes" >&2; exit 1; }
    sleep 15
  done
  if gh release view "$VERSION" --json assets --jq '.assets[].name' | grep -qx myco.dmg; then
    # The release workflow signed it already (signing secrets are set).
    echo "UPLOADED: the $VERSION release already has a signed myco.dmg (from the release workflow); left as it is"
    exit 0
  fi
  echo "==> uploading to the $VERSION release"
  gh release download "$VERSION" --pattern SHA256SUMS --dir dist --clobber
  (cd dist && grep -v -e ' myco.dmg$' -e ' myco-app-darwin-universal.zip$' SHA256SUMS > SHA256SUMS.new || true
   shasum -a 256 myco.dmg myco-app-darwin-universal.zip >> SHA256SUMS.new && mv SHA256SUMS.new SHA256SUMS)
  gh release upload "$VERSION" dist/myco.dmg dist/myco-app-darwin-universal.zip dist/SHA256SUMS --clobber
  echo "UPLOADED myco.dmg myco-app-darwin-universal.zip SHA256SUMS"
fi
