#!/bin/sh
# GET a GitHub API path for the release scripts, signed in when possible:
# GitHub allows 60 unauthenticated requests an hour per address, which a
# poll every minute (and other tools on the same address) soon uses up.
#   scripts/gh-get.sh repos/chenhg5/myco/actions/runs?head_sha=...
# Uses GH_TOKEN or GITHUB_TOKEN if set, else a signed-in gh, else none.
# A refused request says why (rate limit and when it resets) on stderr.
set -eu
path=${1:?usage: scripts/gh-get.sh <api path>}
url="https://api.github.com/$path"
tok=${GH_TOKEN:-${GITHUB_TOKEN:-}}
if [ -z "$tok" ] && command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
  exec gh api "$path"
fi
hdr=$(mktemp)
trap 'rm -f "$hdr"' EXIT
body=$(if [ -n "$tok" ]; then curl -sS -D "$hdr" -H "Authorization: Bearer $tok" "$url"; else curl -sS -D "$hdr" "$url"; fi)
status=$(sed -n '1s/^HTTP[^ ]* \([0-9]*\).*/\1/p' "$hdr")
if [ "$status" = 200 ]; then
  printf '%s\n' "$body"
  exit 0
fi
remaining=$(sed -n 's/^[Xx]-[Rr]ate[Ll]imit-[Rr]emaining: *\([0-9]*\).*/\1/p' "$hdr")
reset=$(sed -n 's/^[Xx]-[Rr]ate[Ll]imit-[Rr]eset: *\([0-9]*\).*/\1/p' "$hdr")
if [ "$remaining" = 0 ] && [ -n "$reset" ]; then
  echo "GitHub API rate limit reached for this address; it resets in $(( reset - $(date +%s) ))s. Set GH_TOKEN (or sign in to gh) to get 5000 an hour." >&2
  exit 3
fi
echo "GitHub API $path: HTTP $status" >&2
exit 1
