#!/usr/bin/env bash
# Push your commits to main the way every agent on the team does it, in one
# command: rebuild the embedded web UI when web/ changed, rebase onto
# origin/main (a conflict only in internal/web/dist is resolved by rebuilding
# it), then push. Conflicts anywhere else stop the script for you to resolve.
#
#   scripts/ship.sh            rebase and push
#   scripts/ship.sh --check    also run make vet and make test first
#   scripts/ship.sh --dry-run  show what it would do, change nothing
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

check=0 dry=0
for a in "$@"; do
  case "$a" in
    --check) check=1 ;;
    --dry-run) dry=1 ;;
    -h|--help) sed -n '2,10p' "$0"; exit 0 ;;
    *) echo "unknown flag $a" >&2; exit 2 ;;
  esac
done

run() { if [ "$dry" = 1 ]; then echo "+ $*"; else "$@"; fi; }
restore_buildinfo() { git checkout -q HEAD -- web/tsconfig.tsbuildinfo 2>/dev/null || true; }

if [ -n "$(git status --porcelain --untracked-files=no | grep -v ' web/tsconfig.tsbuildinfo$' || true)" ]; then
  echo "uncommitted changes: commit or stash them first" >&2
  git status --short --untracked-files=no >&2
  exit 1
fi
restore_buildinfo

git fetch -q origin main
ahead=$(git rev-list --count origin/main..HEAD)
if [ "$ahead" = 0 ]; then
  echo "nothing to push: HEAD is on origin/main"
  exit 0
fi
echo "$ahead commit(s) to push"

# Rebuild dist when your commits touch web/ but left dist stale.
rebuild_dist() {
  run make web >/dev/null
  restore_buildinfo
  if [ "$dry" = 0 ] && [ -n "$(git status --porcelain internal/web/dist)" ]; then
    git add -A internal/web/dist
    return 0
  fi
  return 1
}
if git diff --name-only origin/main...HEAD | grep -q '^web/'; then
  if rebuild_dist; then
    git commit -q -m "Rebuild web dist"
    echo "rebuilt internal/web/dist"
  fi
fi

if [ "$check" = 1 ]; then
  run make vet
  run make test
fi

if [ "$dry" = 1 ]; then
  echo "+ git pull --rebase origin main (rebuild dist on dist-only conflicts)"
  echo "+ git push origin HEAD:main"
  exit 0
fi

for attempt in 1 2 3 4 5; do
  if ! git pull -q --rebase origin main; then
    if ! [ -d "$(git rev-parse --git-path rebase-merge)" ] && ! [ -d "$(git rev-parse --git-path rebase-apply)" ]; then
      echo "git pull --rebase failed (see above)" >&2
      exit 1
    fi
    while [ -d "$(git rev-parse --git-path rebase-merge)" ] || [ -d "$(git rev-parse --git-path rebase-apply)" ]; do
      conflicts=$(git diff --name-only --diff-filter=U)
      others=$(printf '%s\n' "$conflicts" | grep -v '^internal/web/dist/' | grep -v '^web/tsconfig.tsbuildinfo$' | grep -v '^$' || true)
      if [ -n "$others" ]; then
        echo "conflicts outside internal/web/dist; resolve them, then run git rebase --continue and this script again:" >&2
        printf '  %s\n' $others >&2
        exit 1
      fi
      git checkout -q --theirs -- internal/web/dist 2>/dev/null || true
      make web >/dev/null
      restore_buildinfo
      git add -A internal/web/dist
      GIT_EDITOR=true git rebase --continue >/dev/null || true
    done
    echo "resolved internal/web/dist by rebuilding it"
  fi
  if git push -q origin HEAD:main; then
    echo "pushed $(git rev-parse --short HEAD) to main"
    exit 0
  fi
  echo "push rejected (attempt $attempt): main moved, rebasing again"
  sleep $((attempt * 2))
done
echo "could not push after 5 attempts" >&2
exit 1
