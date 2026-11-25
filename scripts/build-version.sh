#!/bin/sh
# VERSION is the release base even when its tag lives on a release branch.
# Keep git-describe-compatible suffixes for development and dirty builds.
set -eu
base=$(tr -d '[:space:]' < VERSION 2>/dev/null || true)
if ! printf '%s\n' "$base" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  printf 'dev\n'
  exit 0
fi
if ! sha=$(git rev-parse --short HEAD 2>/dev/null); then
  printf '%s\n' "$base"
  exit 0
fi
if [ "$(git describe --tags --exact-match --match "$base" HEAD 2>/dev/null || true)" = "$base" ]; then
  version=$base
else
  count=$(git rev-list --count "$base..HEAD" 2>/dev/null || printf '1')
  [ "$count" -gt 0 ] || count=1
  version="$base-$count-g$sha"
fi
if ! git diff-index --quiet HEAD --; then version="$version-dirty"; fi
printf '%s\n' "$version"
