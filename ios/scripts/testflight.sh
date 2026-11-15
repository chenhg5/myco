#!/bin/bash
# Builds the app for the App Store and sends it to TestFlight.
#
#   TEAM=<Apple team id> ios/scripts/testflight.sh            # archive, export, upload
#   TEAM=<Apple team id> UPLOAD=0 ios/scripts/testflight.sh   # archive only: does a release build work?
#
# The upload signs in as the account Xcode is signed in to on this Mac, or
# with an App Store Connect API key: ASC_KEY_PATH (the .p8 file), ASC_KEY_ID
# and ASC_ISSUER_ID. The app must exist in App Store Connect under this
# project's bundle id first.
#
# VERSION is the version testers see (the project's when not given); BUILD
# must grow with every upload, so it is the time (UTC) unless given.
set -euo pipefail
cd "$(dirname "$0")/.."

: "${TEAM:?set TEAM to the Apple developer team id}"
build="${BUILD:-$(date -u +%Y%m%d%H%M)}"
out="${OUT:-build/testflight}"
archive="$out/Myco.xcarchive"

settings=(DEVELOPMENT_TEAM="$TEAM" CODE_SIGN_STYLE=Automatic CURRENT_PROJECT_VERSION="$build")
if [ -n "${VERSION:-}" ]; then settings+=(MARKETING_VERSION="$VERSION"); fi

# With a key, xcodebuild signs in to App Store Connect by itself.
# (\${auth[@]+...}: the Mac's bash 3.2 calls an empty array unset.)
auth=()
if [ -n "${ASC_KEY_PATH:-}" ]; then
  auth=(-authenticationKeyPath "$ASC_KEY_PATH" -authenticationKeyID "${ASC_KEY_ID:?}" -authenticationKeyIssuerID "${ASC_ISSUER_ID:?}")
fi

rm -rf "$archive"
xcodebuild archive -project Myco.xcodeproj -scheme Myco -configuration Release \
  -destination 'generic/platform=iOS' -archivePath "$archive" \
  -allowProvisioningUpdates ${auth[@]+"${auth[@]}"} "${settings[@]}"
echo "archived: $archive (build $build)"

if [ "${UPLOAD:-1}" = "0" ]; then exit 0; fi

options="$out/ExportOptions.plist"
cat > "$options" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>method</key>
	<string>app-store-connect</string>
	<key>destination</key>
	<string>upload</string>
	<key>teamID</key>
	<string>$TEAM</string>
	<key>signingStyle</key>
	<string>automatic</string>
	<key>manageAppVersionAndBuildNumber</key>
	<false/>
	<key>uploadSymbols</key>
	<true/>
</dict>
</plist>
PLIST
xcodebuild -exportArchive -archivePath "$archive" -exportOptionsPlist "$options" -exportPath "$out/export" \
  -allowProvisioningUpdates ${auth[@]+"${auth[@]}"}
echo "uploaded build $build: it shows in TestFlight once App Store Connect has processed it (some minutes)"
