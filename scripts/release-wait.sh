#!/bin/sh
# Wait for main's CI before pushing a release:
#
#   scripts/release-wait.sh            # origin/main, at most 15 minutes
#   scripts/release-wait.sh <sha> 900  # one commit, at most 900 seconds
#
# Follows main while other people push: when the branch moves it watches the
# newest commit's ci run, because GitHub cancels the old run. It succeeds once
# the code jobs (go, windows, web, app-macos) are green and returns at once
# after that; it does not wait for the ui jobs, whose failures are usually the
# runner's Playwright install rather than the code (the release workflow builds
# and signs the app itself). A run cancelled by a newer push is not a failure.
#
# Exit 0: green, 1: a code job failed, 2: timed out (or no run appeared).
set -eu
SHA=${1:-}
TIMEOUT=${2:-900}
REPO=${MYCO_REPO:-myco-ai/myco}
REQUIRED="go windows web app-macos"

say() { echo "release-wait: $*"; }
if [ -n "$SHA" ] && [ "$SHA" != main ]; then
  SHA=$(git rev-parse "$SHA" 2>/dev/null || printf '%s' "$SHA")
fi
start=$(date +%s)
deadline=$((start + TIMEOUT))
last=""

while :; do
  if [ -z "$SHA" ] || [ "$SHA" = main ]; then
    git fetch -q origin main
    SHA=$(git rev-parse origin/main)
  fi
  short=$(printf '%.8s' "$SHA")
  run=$(gh run list --repo "$REPO" --branch main --limit 20 \
    --json name,headSha,status,conclusion,databaseId,url 2>/dev/null |
    SHA=$SHA python3 -c '
import json, os, sys
sha = os.environ["SHA"]
best = None
for r in json.load(sys.stdin):
    if r["name"] != "ci" or r["headSha"] != sha:
        continue
    if best is None or (best["conclusion"] != "success" and r["conclusion"] == "success"):
        best = r
print("" if best is None else "%s\t%s\t%s\t%s" % (
    best["databaseId"], best["status"], best["conclusion"] or "-", best["url"]))
') || { say "cannot ask GitHub; retrying"; sleep 10; continue; }

  if [ -z "$run" ]; then
    [ "$(date +%s)" -ge "$deadline" ] && { say "no ci run for $short within ${TIMEOUT}s"; exit 2; }
    [ "$last" != waiting ] && { say "waiting for a ci run on $short"; last=waiting; }
    sleep 10
    continue
  fi

  id=$(printf '%s' "$run" | cut -f1)
  status=$(printf '%s' "$run" | cut -f2)
  conclusion=$(printf '%s' "$run" | cut -f3)
  url=$(printf '%s' "$run" | cut -f4)

  jobs=$(gh run view "$id" --repo "$REPO" --json jobs 2>/dev/null |
    python3 -c '
import json, sys
for j in json.load(sys.stdin)["jobs"]:
    print("%s\t%s" % (j["name"], j["conclusion"] or "-"))
') || { sleep 10; continue; }

  bad="" ok=0
  all_ok=1
  for name in $REQUIRED; do
    c=$(printf '%s\n' "$jobs" | awk -F'\t' -v n="$name" '$1 == n { print $2 }')
    case "$c" in
      success|skipped) ok=$((ok + 1)) ;;
      failure|timed_out|startup_failure) bad="$bad $name" ;;
      *) all_ok=0 ;;
    esac
  done
  ui=$(printf '%s\n' "$jobs" | awk -F'\t' '$1 == "ui" { print $2 }')
  ui=${ui:--}

  if [ -n "$bad" ]; then
    say "a code job failed on $short:$bad"
    say "see $url"
    exit 1
  fi
  if [ "$all_ok" = 1 ] && [ "$ok" = 4 ]; then
    elapsed=$(( $(date +%s) - start ))
    say "$short is green (go, windows, web, app-macos; ui: $ui) in ${elapsed}s"
    exit 0
  fi
  if [ "$status" = completed ]; then
    if [ "$conclusion" = cancelled ]; then
      old=$SHA
      git fetch -q origin main
      new=$(git rev-parse origin/main)
      if [ "$new" != "$old" ]; then
        say "main moved to $(printf '%.8s' "$new"); following it"
        SHA=$new
        last=""
        continue
      fi
    fi
    say "ci on $short ended $conclusion before the code jobs passed"
    say "see $url"
    exit 1
  fi
  [ "$last" != "$short/$status/$ui" ] && {
    say "waiting on $short ($status; required done: $ok/4; ui: $ui)"
    last="$short/$status/$ui"
  }
  [ "$(date +%s)" -ge "$deadline" ] && { say "still running after ${TIMEOUT}s: $url"; exit 2; }
  sleep 15
done
