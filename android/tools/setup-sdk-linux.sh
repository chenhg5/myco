#!/usr/bin/env bash
# Install Google's verified Linux command-line tools and this client's SDK.
set -euo pipefail

sdk_dir="${1:?Usage: setup-sdk-linux.sh /absolute/path/to/android-sdk}"
mkdir -p "$sdk_dir/cmdline-tools"
sdk_dir="$(cd "$sdk_dir" && pwd)"
manager="$sdk_dir/cmdline-tools/latest/bin/sdkmanager"

if [ ! -x "$manager" ]; then
  archive="$(mktemp "$sdk_dir/.commandline.XXXXXX.zip")"
  staging="$(mktemp -d "$sdk_dir/.commandline.XXXXXX")"
  curl -fL --retry 3 'https://dl.google.com/android/repository/commandlinetools-linux-15859902_latest.zip' -o "$archive"
  printf '%s  %s\n' '4e4c464f145a7512b57d088ac6c278c03c9eea610886b35a5e0804e74eedf583' "$archive" | sha256sum -c -
  unzip -q "$archive" -d "$staging"
  mv "$staging/cmdline-tools" "$sdk_dir/cmdline-tools/latest"
  rmdir "$staging"
  unlink "$archive"
fi

# Process substitution keeps sdkmanager's real exit code when yes gets SIGPIPE.
"$manager" --sdk_root="$sdk_dir" --licenses < <(yes) > /dev/null
"$manager" --sdk_root="$sdk_dir" 'platform-tools' 'platforms;android-36' 'build-tools;35.0.0'

if [ -n "${GITHUB_ENV:-}" ]; then
  printf 'ANDROID_HOME=%s\nANDROID_SDK_ROOT=%s\n' "$sdk_dir" "$sdk_dir" >> "$GITHUB_ENV"
fi
if [ -n "${GITHUB_PATH:-}" ]; then
  printf '%s\n%s\n' "$sdk_dir/cmdline-tools/latest/bin" "$sdk_dir/platform-tools" >> "$GITHUB_PATH"
fi
printf 'Android SDK ready at %s\n' "$sdk_dir"
