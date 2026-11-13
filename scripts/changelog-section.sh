#!/bin/sh
# Prints the section of CHANGELOG.md for version $1 (e.g. v0.19.3): the
# lines after its "## v0.19.3" heading, up to the next "## " heading.
set -eu
v="$1"
file="${2:-CHANGELOG.md}"
awk -v v="$v" '
  /^## / { if (on) exit; on = ($2 == v); next }
  on { print }
' "$file" | sed -e '/./,$!d' | sed -e ':a' -e '/^\n*$/{$d;N;ba' -e '}'
