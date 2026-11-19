#!/usr/bin/env bash
# Try myco on this Mac as a brand-new user, then put everything back.
#
#   bash scripts/mac-fresh-start.sh clean [--keep-app]   # back up, then clear myco from this Mac
#   bash scripts/mac-fresh-start.sh restore              # put the backup back
#   bash scripts/mac-fresh-start.sh status               # what is there, and the backup
#   add --dry-run to clean or restore to see what it would do
#
# clean quits the App, stops myco's background services (the runner and any
# local hub on this Mac) and MOVES myco's things into a backup folder:
#   ~/.myco                                   agents, machine membership, local hubs' data, logs
#   ~/Library/Application Support/myco        the App's workspaces list and settings
#   ~/Library/LaunchAgents/ai.myco.*.plist    the background services
#   ~/Library/WebKit|Caches/dev.myco.console  the App's page data (onboarding, layout)
#   ~/.local/bin/myco                         the command line
#   /Applications/myco.app                    the App (kept with --keep-app)
# Nothing is deleted. The App's sign-ins stay in the keychain under ids the
# fresh App does not use, so they come back with the backup.
#
# While cleaned, this Mac's agents (and mac-exec) are offline. Workspaces on
# servers keep everything: messages, projects, members, automations.
#
# restore quits the App, stops whatever the try-out started, moves what it
# left into the backup folder (leftovers-<time>, to look at or delete), and
# moves the backup back, then starts the services again.
set -euo pipefail

ROOT="$HOME/myco-fresh-backup"
CURRENT="$ROOT/current"
DRY=0
KEEP_APP=0
CMD="${1:-}"
shift || true
for a in "$@"; do
  case "$a" in
    --dry-run) DRY=1 ;;
    --keep-app) KEEP_APP=1 ;;
    *) echo "unknown option: $a" >&2; exit 2 ;;
  esac
done

[[ "$(uname -s)" == Darwin ]] || { echo "this script is for macOS" >&2; exit 1; }

say() { printf '%s\n' "$*"; }
run() {
  if [[ $DRY == 1 ]]; then say "  would: $*"; else "$@"; fi
}

# What clean moves, relative to /: home paths start with $HOME.
items() {
  local list=(
    "$HOME/.myco"
    "$HOME/Library/Application Support/myco"
    "$HOME/Library/WebKit/dev.myco.console"
    "$HOME/Library/Caches/dev.myco.console"
    "$HOME/Library/HTTPStorages/dev.myco.console"
    "$HOME/Library/Saved Application State/dev.myco.console.savedState"
    "$HOME/.local/bin/myco"
  )
  local p
  for p in "$HOME"/Library/LaunchAgents/ai.myco.*.plist; do
    [[ -e "$p" ]] && list+=("$p")
  done
  [[ $KEEP_APP == 1 ]] || list+=("/Applications/myco.app")
  printf '%s\n' "${list[@]}"
}

quit_app() {
  if pgrep -xq myco 2>/dev/null || pgrep -f "/Applications/myco.app/" >/dev/null 2>&1; then
    say "Quitting the myco App"
    run osascript -e 'quit app "myco"' >/dev/null 2>&1 || true
    [[ $DRY == 1 ]] || sleep 2
    run pkill -f "/Applications/myco.app/Contents/MacOS/" 2>/dev/null || true
  fi
}

stop_services() {
  local p label
  for p in "$HOME"/Library/LaunchAgents/ai.myco.*.plist; do
    [[ -e "$p" ]] || continue
    label="$(basename "$p" .plist)"
    say "Stopping $label"
    run launchctl bootout "gui/$(id -u)/$label" 2>/dev/null || true
  done
  # A runner or hub started by hand.
  if pgrep -f "myco (runner start|hub serve)" >/dev/null 2>&1; then
    say "Stopping myco processes started by hand"
    run pkill -f "myco (runner start|hub serve)" || true
  fi
}

start_services() {
  local p label
  for p in "$HOME"/Library/LaunchAgents/ai.myco.*.plist; do
    [[ -e "$p" ]] || continue
    label="$(basename "$p" .plist)"
    say "Starting $label"
    run launchctl bootstrap "gui/$(id -u)" "$p" 2>/dev/null || run launchctl kickstart -k "gui/$(id -u)/$label" || true
  done
}

# move SRC into DIR, keeping its path below / (so restore knows where it goes).
move_into() {
  local src="$1" dir="$2"
  local dest="$dir${src}"
  run mkdir -p "$(dirname "$dest")"
  if [[ "$src" == /Applications/* && ! -w /Applications ]]; then
    run sudo mv "$src" "$dest"
  else
    run mv "$src" "$dest"
  fi
}

case "$CMD" in
  clean)
    if [[ -e "$CURRENT" ]]; then
      say "A backup is already in place: $(readlink "$CURRENT")"
      say "Run restore first (or move $ROOT away if you are sure)."
      exit 1
    fi
    stamp="$(date +%Y%m%d-%H%M%S)"
    dir="$ROOT/$stamp"
    quit_app
    stop_services
    say "Backing up into $dir"
    n=0
    while IFS= read -r p; do
      [[ -e "$p" ]] || continue
      say "  $p"
      move_into "$p" "$dir"
      n=$((n + 1))
    done < <(items)
    run ln -sfn "$dir" "$CURRENT"
    if [[ $DRY == 0 ]]; then
      printf '%s\n' "$stamp" > "$dir/.myco-fresh-backup"
    fi
    say
    if [[ $DRY == 1 ]]; then
      say "Dry run: would move $n things. Nothing was changed."
      exit 0
    fi
    say "Done: $n things moved. This Mac now has no myco."
    if [[ $KEEP_APP == 1 ]]; then
      say "Open the myco App to start as a new user."
    else
      say "Download the App (https://github.com/chenhg5/myco/releases/latest) to start as a new user."
    fi
    say "Afterwards: bash scripts/mac-fresh-start.sh restore"
    ;;

  restore)
    [[ -e "$CURRENT" ]] || { say "No backup to restore ($CURRENT is missing)."; exit 1; }
    dir="$(readlink "$CURRENT")"
    [[ -f "$dir/.myco-fresh-backup" || $DRY == 1 ]] || { say "$dir does not look like a backup made by this script"; exit 1; }
    quit_app
    stop_services
    left="$dir/leftovers-$(date +%Y%m%d-%H%M%S)"
    say "Moving what the try-out left into $left"
    while IFS= read -r p; do
      [[ -e "$p" ]] || continue
      say "  $p"
      move_into "$p" "$left"
    done < <(KEEP_APP=0 items)
    say "Putting the backup back"
    back=(
      "$HOME/.myco"
      "$HOME/Library/Application Support/myco"
      "$HOME/Library/WebKit/dev.myco.console"
      "$HOME/Library/Caches/dev.myco.console"
      "$HOME/Library/HTTPStorages/dev.myco.console"
      "$HOME/Library/Saved Application State/dev.myco.console.savedState"
      "$HOME/.local/bin/myco"
      "/Applications/myco.app"
    )
    for f in "$dir$HOME"/Library/LaunchAgents/ai.myco.*.plist; do
      [[ -e "$f" ]] && back+=("${f#"$dir"}")
    done
    for p in "${back[@]}"; do
      [[ -e "$dir$p" ]] || continue
      say "  $p"
      run mkdir -p "$(dirname "$p")"
      if [[ "$p" == /Applications/* && ! -w /Applications ]]; then
        run sudo mv "$dir$p" "$p"
      else
        run mv "$dir$p" "$p"
      fi
    done
    start_services
    run rm -f "$CURRENT"
    say
    say "Restored. The try-out's files are in $left (delete them when you no longer need them)."
    say "Open the myco App: your workspaces, agents and settings are back."
    ;;

  status)
    say "myco on this Mac:"
    while IFS= read -r p; do
      [[ -e "$p" ]] && say "  present  $p"
    done < <(KEEP_APP=0 items)
    for p in "$HOME"/Library/LaunchAgents/ai.myco.*.plist; do
      [[ -e "$p" ]] || continue
      label="$(basename "$p" .plist)"
      if launchctl print "gui/$(id -u)/$label" >/dev/null 2>&1; then say "  running  $label"; else say "  stopped  $label"; fi
    done
    if [[ -e "$CURRENT" ]]; then
      say "Backup in place: $(readlink "$CURRENT") (restore with: bash scripts/mac-fresh-start.sh restore)"
    else
      say "No backup in place."
    fi
    ;;

  *)
    sed -n '2,8p' "$0"
    exit 2
    ;;
esac
