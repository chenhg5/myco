#!/bin/bash
# Builds a preview of the Mac app on the maintainer's Mac and opens it beside
# the installed one: main (or the ref given) into ~/Applications/myco-preview.app.
# /Applications/myco.app, the released app, is never touched.
#
#   scripts/mac-preview.sh [ref]
#   MYCO_PREVIEW_LOG=~/code/myco-preview-app.log scripts/mac-preview.sh   (keep its log)
#
# The Mac keeps a copy as ~/code/.myco-preview.sh, run through its exec node
# (.claude/skills/myco-preview). The last line says PREVIEW-OK or fails.
set -e
ref=${1:-main}
dir=${MYCO_PREVIEW_DIR:-$HOME/code/myco-app-build}
export GOTOOLCHAIN=auto PATH=$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/local/go/bin:$PATH
# GitHub through the Mac's own local proxy when one is listening (the
# Mac's settings are used as they are, never changed).
proxy=${MYCO_PREVIEW_PROXY-http://127.0.0.1:10808}
if [ -n "$proxy" ] && nc -z "$(echo "$proxy" | sed -E 's#^[a-z]+://([^:/]+).*#\1#')" "$(echo "$proxy" | sed -E 's#.*:([0-9]+)/?$#\1#')" 2>/dev/null; then
  export HTTPS_PROXY=$proxy HTTP_PROXY=$proxy
fi

[ -d "$dir/.git" ] || git clone -q --depth 1 https://github.com/chenhg5/myco.git "$dir"
cd "$dir"
git fetch -q --depth 1 origin "$ref" && git checkout -q FETCH_HEAD
git log -1 --oneline
make app VERSION="$(cat VERSION)-preview"

app=$HOME/Applications/myco-preview.app
# Stop the old preview by its path: both apps have the same bundle id, so
# "quit app" by name would also quit the released one.
pkill -f "$app/Contents/MacOS" 2>/dev/null || true
for _ in 1 2 3 4 5 6 7 8 9 10; do pgrep -f "$app/Contents/MacOS" >/dev/null || break; sleep 1; done
rm -rf "$app" && mkdir -p "$HOME/Applications" && cp -R bin/myco.app "$app"
touch "$app"
# MYCO_PREVIEW_LOG=file: start it with its log (the app's own messages) kept
# there, to see what it did; otherwise as from the Finder.
if [ -n "$MYCO_PREVIEW_LOG" ]; then
  (nohup "$app/Contents/MacOS/myco-app" > "$MYCO_PREVIEW_LOG" 2>&1 &)
else
  open -n "$app"
fi
echo PREVIEW-OK
