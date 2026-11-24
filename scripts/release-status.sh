#!/bin/sh
# The GitHub Actions runs of one commit, for releasing:
#   scripts/release-status.sh <sha>            ci and release
#   scripts/release-status.sh <sha> ci         ci only (before a release: main must be green)
#   scripts/release-status.sh <sha> ci --wait  ask every minute until they end
# GitHub is asked through scripts/gh-get.sh (signed in when it can be).
# Prints one line per workflow (name=status/conclusion). Exit 0: all of them
# finished and succeeded; 1: one failed; 2: still running or not started.
set -eu
SHA=${1:?usage: scripts/release-status.sh <sha> [ci|release|all] [--wait]}
WANT=${2:-all}
WAIT=${3:-}
REPO=${MYCO_REPO:-myco-ai/myco}
while :; do
  out=$("$(dirname "$0")/gh-get.sh" "repos/$REPO/actions/runs?head_sha=$SHA" | python3 -c '
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
