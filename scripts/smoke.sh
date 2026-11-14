#!/usr/bin/env bash
# Smoke test: build the real myco binary and drive it end to end as separate
# processes, the way a user does. Nothing touches your ~/.myco: HOME is a
# temporary directory, and the hub listens on a random localhost port.
#
#   scripts/smoke.sh                  # builds ./bin/myco first
#   MYCO_BIN=/path/to/myco scripts/smoke.sh   # test an existing binary (e.g. a release)
#
# Exits non-zero on the first failing step and prints the logs it collected.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
T="$(mktemp -d)"
PIDS=()
cleanup() {
  for p in "${PIDS[@]:-}"; do [[ -n "$p" ]] && kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
  rm -rf "$T"
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  for f in "$T"/*.log; do [[ -f "$f" ]] && { echo "--- $(basename "$f")" >&2; tail -n 30 "$f" >&2; }; done
  exit 1
}
step() { printf '%-44s' "$1"; }
ok() { echo "ok${1:+  ($1)}"; }
# wait_for <seconds> <command...>: retry until the command succeeds
wait_for() {
  local n=$(( $1 * 10 )); shift
  for ((i = 0; i < n; i++)); do "$@" >/dev/null 2>&1 && return 0; sleep 0.1; done
  return 1
}

if [[ -z "${MYCO_BIN:-}" ]]; then
  step "build"
  (cd "$ROOT" && CGO_ENABLED=0 go build -o "$T/myco" ./cmd/myco) || fail "go build"
  MYCO_BIN="$T/myco"
  ok
fi
myco() { "$MYCO_BIN" "$@"; }

# Never let a real network leak in: drop every MYCO_* variable from the caller
# (e.g. MYCO_HUB / MYCO_ENROLL_TOKEN set for a real hub) before anything runs.
while IFS='=' read -r k _; do [[ "$k" == MYCO_* && "$k" != MYCO_BIN ]] && unset "$k"; done < <(env)
export HOME="$T/home" MYCO_FORMAT=json
mkdir -p "$HOME"
ADMIN="smoke-admin-$(date +%s)-$RANDOM-$RANDOM"
VERSION="$(myco version --format text 2>/dev/null || myco version)"

step "hub serve"
MYCO_ADMIN_TOKEN="$ADMIN" myco hub serve --addr 127.0.0.1:0 --db "$T/hub.db" --lease 30s >"$T/hub.log" 2>&1 &
PIDS+=($!)
wait_for 10 grep -q 'myco hub listening' "$T/hub.log" || fail "hub did not start"
ADDR="$(grep -o 'addr=[0-9.:]*' "$T/hub.log" | head -1 | cut -d= -f2)"
export MYCO_HUB="http://$ADDR"
ok "$MYCO_HUB, $VERSION"

step "health"
curl -fsS "$MYCO_HUB/v1/health" | grep -q '"ok":true' || fail "health"
ok

step "register agents (admin)"
P="$HOME/.myco/agents"
mkdir -p "$P"
MYCO_ADMIN_TOKEN="$ADMIN" myco agent register --name alice --config "$HOME/.myco/config.yaml" >/dev/null || fail "register alice"
MYCO_ADMIN_TOKEN="$ADMIN" myco agent register --name worker --harness shell --label team=smoke --config "$P/worker.yaml" >/dev/null || fail "register worker"
MYCO_ADMIN_TOKEN="$ADMIN" myco agent register --name ci --harness exec --allow-from alice --workdir "$T" --config "$P/ci.yaml" >/dev/null || fail "register ci"
# worker answers with the upper-cased message body; ci runs only `echo ...`
cat >>"$P/worker.yaml" <<EOF
runner:
  harness: shell
  workdir: $T
  poll_wait: 2s
  command: "sed -n '/^--- message body ---\$/,\$p' | tail -n +2 | tr a-z A-Z"
EOF
cat >>"$P/ci.yaml" <<EOF
runner:
  harness: exec
  workdir: $T
  poll_wait: 2s
  exec:
    allow: ["echo *"]
EOF
ok "alice, worker (shell), ci (exec)"

step "agent list"
myco agent list | grep -q '"worker"' || fail "agent list"
ok

step "runner start --all (background)"
myco runner start --all --max-concurrent 2 >"$T/runner.log" 2>&1 &
PIDS+=($!)
wait_for 10 bash -c "[[ \$(grep -c 'runner ready' '$T/runner.log') -ge 2 ]]" || fail "runner did not pick up both profiles"
ok

step "task → reply (message send --wait)"
OUT="$(myco message send --to worker --body "smoke test" --wait 20s 2>>"$T/cli.log")" || fail "send --wait: $OUT"
grep -q 'SMOKE TEST' <<<"$OUT" || fail "unexpected reply: $OUT"
ok

step "exec node runs an allowed command"
OUT="$(myco exec --to ci --cmd "echo smoke-exec-ok" --timeout 20s 2>>"$T/cli.log")" || fail "exec: $OUT"
grep -q 'smoke-exec-ok' <<<"$OUT" || fail "exec output: $OUT"
ok

step "exec node refuses others"
if OUT="$(myco exec --to ci --cmd "id" --timeout 20s 2>>"$T/cli.log")"; then fail "command outside the allow-list ran: $OUT"; fi
grep -q '"denied": true' <<<"$OUT" || fail "expected a denied result: $OUT"
ok

step "broadcast by label"
OUT="$(myco broadcast send --selector team=smoke --body "ping" --wait 20s 2>>"$T/cli.log")" || fail "broadcast: $OUT"
grep -q 'PING' <<<"$OUT" || fail "broadcast replies: $OUT"
ok

step "join link serves an install script"
LINK="$(MYCO_ADMIN_TOKEN="$ADMIN" myco link create --uses 1 --expires 1h | grep -o 'http[^" ]*/j/[^" ]*' | head -1)"
[[ -n "$LINK" ]] || fail "link create printed no link"
SCRIPT="$(curl -fsS "$LINK")" || fail "GET join link"
grep -q "$ADDR" <<<"$SCRIPT" || fail "join script does not point at the hub"
grep -q "$ADMIN" <<<"$SCRIPT" && fail "join script leaks the admin token"
ok

step "workspace: create, use, remove"
OUT="$(MYCO_ADMIN_TOKEN="$ADMIN" myco workspace create --name smoke-b --format json)" || fail "workspace create"
grep -q "\"url\": *\"$MYCO_HUB/w/smoke-b\"" <<<"$OUT" || fail "workspace URL: $OUT"
grep -q "/w/smoke-b/c/" <<<"$OUT" || fail "owner app link should name the workspace: $OUT"
curl -fsS "$MYCO_HUB/w/smoke-b/v1/health" | grep -q '"workspace":"smoke-b"' || fail "workspace health"
MYCO_ADMIN_TOKEN="$ADMIN" MYCO_HUB="$MYCO_HUB/w/smoke-b" myco agent register --name b-bot --config "$T/b-bot.yaml" >/dev/null || fail "register in workspace"
MYCO_ADMIN_TOKEN="$ADMIN" myco agent list --format json | grep -q '"b-bot"' && fail "workspace agent leaked into the default workspace"
MYCO_ADMIN_TOKEN="$ADMIN" myco workspace list --format json | grep -q '"smoke-b"' || fail "workspace list"
MYCO_ADMIN_TOKEN="$ADMIN" myco workspace remove --name smoke-b >/dev/null || fail "workspace remove"
curl -fsS "$MYCO_HUB/w/smoke-b/v1/health" >/dev/null 2>&1 && fail "removed workspace still answers"
ok

step "dashboard (myco ui)"
UI_PORT=$(( 20000 + RANDOM % 20000 ))
myco ui --port "$UI_PORT" >"$T/ui.log" 2>&1 &
PIDS+=($!)
wait_for 10 grep -q 'key=' "$T/ui.log" || fail "ui did not start"
URL="$(grep -o 'http://127.0.0.1:[0-9]*/?key=[0-9a-f]*' "$T/ui.log" | head -1)"
# the one-time key sets a cookie and redirects; everything after needs that cookie
JAR="$T/cookies"
curl -fsS -L -c "$JAR" -b "$JAR" "$URL" | grep -q 'id="root"' || fail "dashboard page"
CT="$(curl -fsS -o /dev/null -w '%{content_type}' -b "$JAR" "http://127.0.0.1:$UI_PORT/favicon.svg")" || fail "favicon"
[[ "$CT" == image/svg+xml* ]] || fail "favicon served as $CT"
CODE="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$UI_PORT/")"
[[ "$CODE" == 401 ]] || fail "dashboard without the key returned $CODE, want 401"
CODE="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$UI_PORT/?key=wrong")"
[[ "$CODE" == 403 ]] || fail "dashboard with a wrong key returned $CODE, want 403"
ok

step "schema"
myco schema --all | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d' || fail "schema --all is not JSON"
ok

echo "smoke: all steps passed"
