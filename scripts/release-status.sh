#!/bin/sh
# The GitHub Actions runs of one commit, for releasing:
#   scripts/release-status.sh <sha>            ci and release
#   scripts/release-status.sh <sha> ci         ci only (before a release: main must be green)
#   scripts/release-status.sh <sha> ci --wait  ask every minute until they end
# GitHub is asked through scripts/gh-get.sh (signed in when it can be).
# A short SHA (or a branch or tag) is resolved to the full commit first: the
# repository this script lives in, then GitHub for a commit it does not have.
# GitHub's head_sha filter is an exact match, so a short SHA would otherwise
# find no run and --wait would wait forever.
# Prints one line per workflow (name=status/conclusion). Exit 0: all of them
# finished and succeeded; 1: one failed; 2: still running or not started;
# 3: the SHA could not be resolved.
set -eu
SHA=${1:?usage: scripts/release-status.sh <sha> [ci|release|all] [--wait]}
WANT=${2:-all}
WAIT=${3:-}
REPO=${MYCO_REPO:-myco-ai/myco}
here=$(cd "$(dirname "$0")" && pwd)

is_full_sha() {
  printf '%s' "$1" | grep -Eq '^[0-9a-f]{40}$'
}
resolve() {
  if full=$(git -C "$here/.." rev-parse --verify "$1^{commit}" 2>/dev/null) && is_full_sha "$full"; then
    printf '%s\n' "$full"
    return 0
  fi
  if json=$("$here/gh-get.sh" "repos/$REPO/commits/$1"); then
    full=$(printf '%s\n' "$json" |
      python3 -c 'import json, sys; print(json.load(sys.stdin)["sha"])' 2>/dev/null) || full=""
    if is_full_sha "$full"; then printf '%s\n' "$full"; return 0; fi
  fi
  return 1
}
if ! SHA=$(resolve "$SHA"); then
  echo "release-status: cannot resolve $1 to a commit; pass the full SHA" >&2
  exit 3
fi

while :; do
  out=$("$here/gh-get.sh" "repos/$REPO/actions/runs?head_sha=$SHA" | python3 -c '
import json, sys
want = sys.argv[1]
names = ["ci", "release"] if want == "all" else [want]
runs = {}
for r in json.load(sys.stdin)["workflow_runs"]:
    # Newest first: keep the latest run, unless an earlier run of the same
    # workflow on this commit passed (a duplicate run, or one whose macOS
    # job was cancelled waiting for a runner, does not undo a green one).
    seen = runs.get(r["name"])
    if seen is None or (seen["conclusion"] != "success" and r["conclusion"] == "success"):
        runs[r["name"]] = r
code = 0
for n in names:
    r = runs.get(n)
    if not r:
        print(f"{n}=not-started"); code = max(code, 2); continue
    print("%s=%s/%s %s" % (n, r["status"], r["conclusion"], r["html_url"]))
    if r["status"] != "completed": code = max(code, 2)
    elif r["conclusion"] != "success": code = 1 if code != 2 else 2
sys.exit(code)
' "$WANT") && { echo "$out"; exit 0; } || code=$?
  if [ "$code" != 2 ] || [ "$WAIT" != --wait ]; then echo "$out"; exit "$code"; fi
  sleep 60
done
