#!/usr/bin/env bash
# Install a myco hub on a Linux server (systemd), and make it reachable.
#
#   curl -fsSLO https://raw.githubusercontent.com/chenhg5/myco/main/deploy/install-hub.sh
#   less install-hub.sh                       # read it first
#   sudo bash install-hub.sh                  # asks a few questions
#
# Four ways to reach the hub (run without arguments to be asked):
#   A  --domain <name>|auto            automatic HTTPS: Caddy on port 443
#   B  --behind nginx|panel|caddy|other --domain <name>
#                                      behind the web server that already has 443
#   C  --tunnel cloudflare [--hostname <name>] | --tunnel tailscale
#                                      no open ports: a Cloudflare or Tailscale tunnel
#   D  --http [--port 7777]            plain HTTP on a port (trusted networks only)
#      --local                         this server only (127.0.0.1)
# --domain auto uses <ip>.sslip.io (a public wildcard DNS that resolves to
# your IP), so a certificate works without buying a domain.
# Re-running is safe: the existing admin token and database are kept.
set -euo pipefail

REPO="chenhg5/myco"
VERSION="latest"
DOMAIN=""
PORT=""
NAME=""
NAME_SET=0
MODE=""   # caddy | behind | tunnel | http | local
BEHIND="" # nginx | panel | caddy | other
TUNNEL="" # cloudflare | tailscale
TUNNEL_HOST=""
TUNNEL_NAME=""
CHECK_ONLY=0
ASSUME_YES=0
NARGS=$#

# For tests: every path the script writes lives under MYCO_INSTALL_ROOT, and
# questions are read from MYCO_INSTALL_TTY. Both are unset on real servers.
ROOT="${MYCO_INSTALL_ROOT:-}"
TTY_PATH="${MYCO_INSTALL_TTY:-/dev/tty}"

usage() {
  cat <<EOF
Usage: sudo bash install-hub.sh [how] [--name <name>] [--port <port>] [--version vX.Y.Z]

Without arguments it asks how others should reach the hub. How:
  --domain <name>|auto   A: automatic HTTPS with Caddy on port 443 (80 optional).
                         'auto' uses <public-ip>.sslip.io.
  --behind <server>      B: behind the web server that already has port 443
                         (nginx | panel | caddy | other); needs --domain.
                         nginx: a site file of its own is written and reloaded;
                         panel (宝塔 / 1Panel), caddy, other: the settings to
                         fill in are printed, nothing is changed.
  --tunnel <kind>        C: no open ports; the hub is published through a tunnel.
                         cloudflare --hostname hub.example.com: your domain on
                         Cloudflare (run 'cloudflared tunnel login' first);
                         cloudflare without --hostname: a temporary
                         trycloudflare.com address that changes on every
                         restart (to try it out); tailscale: Tailscale Funnel at
                         https://<machine>.<tailnet>.ts.net.
  --http                 D: plain HTTP on --port (default 7777) for everyone who
                         can reach it. Tokens travel unencrypted: trusted
                         networks and tests only.
  --local                Only this server (127.0.0.1). Same as giving other
                         flags without a way.
Also:
  --name     Named hub (1-32 lowercase letters, digits or hyphens); omit for the original hub.
  --port     Port of the hub on this server (D: the public port).
  --version  Release tag to install (default: latest).
  --check    Only check an installed hub: is it reachable, and if not, where it stops.
  --yes      Never ask (without a way: --local).
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain) DOMAIN="${2:-}"; shift 2 ;;
    --behind) BEHIND="${2:-}"; shift 2 ;;
    --http) MODE=http; shift ;;
    --tunnel) TUNNEL="${2:-}"; shift 2 ;;
    --hostname) TUNNEL_HOST="${2:-}"; shift 2 ;;
    --tunnel-name) TUNNEL_NAME="${2:-}"; shift 2 ;;
    --local) MODE=local; shift ;;
    --name) NAME="${2:-}"; NAME_SET=1; shift 2 ;;
    --version) VERSION="${2:-}"; shift 2 ;;
    --port) PORT="${2:-}"; shift 2 ;;
    --check) CHECK_ONLY=1; shift ;;
    -y|--yes) ASSUME_YES=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 2 ;;
  esac
done

if [[ -n "$BEHIND" ]]; then
  case "$BEHIND" in
    nginx|panel|caddy|other) ;;
    *) echo "--behind must be nginx, panel, caddy or other" >&2; exit 2 ;;
  esac
  [[ "$MODE" == http || "$MODE" == local ]] && { echo "--behind cannot be combined with --$MODE" >&2; exit 2; }
  [[ -n "$DOMAIN" ]] || { echo "--behind needs --domain <name> (the name the web server answers to; 'auto' for <ip>.sslip.io)" >&2; exit 2; }
  MODE=behind
fi
if [[ -n "$TUNNEL" ]]; then
  case "$TUNNEL" in
    cloudflare|tailscale) ;;
    *) echo "--tunnel must be cloudflare or tailscale" >&2; exit 2 ;;
  esac
  [[ -n "$BEHIND" || "$MODE" == http || "$MODE" == local ]] && { echo "--tunnel cannot be combined with --behind, --http or --local" >&2; exit 2; }
  # --domain reads as the tunnel's hostname, for those who reach for it.
  [[ -z "$TUNNEL_HOST" && -n "$DOMAIN" && "$DOMAIN" != auto ]] && TUNNEL_HOST="$DOMAIN"
  [[ "$TUNNEL" == tailscale && -n "$TUNNEL_HOST" ]] && { echo "--hostname is for --tunnel cloudflare; Tailscale picks <machine>.<tailnet>.ts.net" >&2; exit 2; }
  DOMAIN=""
  MODE=tunnel
elif [[ -n "$TUNNEL_HOST" || -n "$TUNNEL_NAME" ]]; then
  echo "--hostname and --tunnel-name go with --tunnel cloudflare" >&2; exit 2
fi
if [[ -z "$MODE" ]]; then
  if [[ -n "$DOMAIN" ]]; then MODE=caddy
  elif [[ "$NARGS" -gt 0 ]]; then MODE=local # flags without a way: as before
  fi
fi
[[ "$MODE" == local && -n "$DOMAIN" ]] && { echo "--local cannot be combined with --domain" >&2; exit 2; }

[[ $EUID -eq 0 || -n "$ROOT" ]] || { echo "run as root (sudo)" >&2; exit 1; }
command -v systemctl >/dev/null || { echo "systemd is required" >&2; exit 1; }
if [[ "$NAME_SET" -eq 1 && ( -z "$NAME" || ${#NAME} -gt 32 || ! "$NAME" =~ ^[a-z0-9-]+$ ) ]]; then
  echo "--name must be 1-32 lowercase letters, digits or hyphens" >&2; exit 2
fi

ENV_REAL=/etc/myco/hub.env
DATA_DIR=/var/lib/myco
UNIT=myco-hub.service
STATE_DIR=myco
SITE=default
HUB_NAME_ARGS=()
if [[ -n "$NAME" ]]; then
  ENV_REAL="/etc/myco/hubs/$NAME/hub.env"
  # A sibling of /var/lib/myco, not inside it: systemd hands a unit's whole
  # StateDirectory (recursively) to that unit's dynamic user on every start,
  # so a named hub nested in the default one would lose its files to it.
  DATA_DIR="/var/lib/myco-$NAME"
  UNIT="myco-hub-$NAME.service"
  STATE_DIR="myco-$NAME"
  SITE="named-$NAME"
  HUB_NAME_ARGS=(--hub-name "$NAME")
fi
ENV_FILE="$ROOT$ENV_REAL"
MYCO="$ROOT/usr/local/bin/myco"
UNIT_FILE="$ROOT/etc/systemd/system/$UNIT"
CADDY_DIR="$ROOT/etc/caddy"
CADDY_SITE="$CADDY_DIR/myco.d/$SITE.caddy"
NGINX_DIR="$ROOT/etc/nginx"
# C: our cloudflared (named tunnel) service and its files, and the name of
# the tunnel in the Cloudflare account.
CF_UNIT="myco-cloudflared${NAME:+-$NAME}.service"
CF_UNIT_FILE="$ROOT/etc/systemd/system/$CF_UNIT"
CF_DIR_REAL="/etc/myco/cloudflared"
[[ -n "$NAME" ]] && CF_DIR_REAL="/etc/myco/hubs/$NAME/cloudflared"
CF_DIR="$ROOT$CF_DIR_REAL"
[[ -n "$TUNNEL_NAME" ]] || TUNNEL_NAME="myco${NAME:+-$NAME}"
HUB_UPLOAD_LIMIT=64m # the hub's attachments are at most 50 MB by default

# ---------------------------------------------------------------- helpers

public_ip() {
  local ip
  ip="$(curl -fsS --max-time 8 https://api.ipify.org 2>/dev/null || true)"
  [[ "$ip" =~ ^[0-9.]+$ || "$ip" =~ ^[0-9a-fA-F:]+$ ]] || ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  printf '%s' "$ip"
}

# Output is captured, not piped into grep -q: with pipefail, a writer cut
# short by grep's early exit would turn a match into a failure.
listening() { [[ -n "$(ss -Hltn "sport = :$1" 2>/dev/null)" ]]; }
ufw_active() { command -v ufw >/dev/null && [[ "$(ufw status 2>/dev/null)" == *"Status: active"* ]]; }

is_1panel() { [[ -d "$ROOT/opt/1panel" ]] || command -v 1pctl >/dev/null 2>&1; }

# Who holds a port: free, nginx, bt (宝塔 / aaPanel), 1panel, caddy, apache,
# or other:<program>.
holder_of() {
  local line proc
  line="$(ss -Hltnp "sport = :$1" 2>/dev/null | head -1 || true)"
  [[ -n "$line" ]] || { echo free; return; }
  proc="$(sed -n 's/.*users:(("\([^"]*\)".*/\1/p' <<<"$line")"
  case "$proc" in
    nginx|openresty|docker-proxy)
      if [[ -d "$ROOT/www/server/panel" ]]; then echo bt
      elif is_1panel; then echo 1panel
      elif [[ "$proc" == nginx ]]; then echo nginx
      else echo "other:$proc"; fi ;;
    caddy) echo caddy ;;
    apache2|httpd) echo apache ;;
    "") echo "other:unknown" ;;
    *) echo "other:$proc" ;;
  esac
}

holder_words() {
  case "$1" in
    free) echo "空着 (free)" ;;
    nginx) echo "nginx" ;;
    bt) echo "宝塔面板的 nginx (BT / aaPanel)" ;;
    1panel) echo "1Panel" ;;
    caddy) echo "Caddy" ;;
    apache) echo "Apache" ;;
    *) echo "${1#other:}" ;;
  esac
}

# A temporary address is printed by the hub's cloudflared once it is up.
quick_tunnel_url() {
  local since=() u
  [[ -n "${1:-}" ]] && since=(--since "@$1")
  u="$(journalctl -u "$UNIT" "${since[@]}" -n 400 --no-pager -o cat 2>/dev/null | grep -o 'https://[a-z0-9-]*\.trycloudflare\.com' | grep -v '^https://api\.' | tail -1 || true)"
  printf '%s' "$u"
}

# The way that fits what holds 443: A when it is free or Caddy's, B otherwise.
recommended_for() {
  case "$1" in
    free|caddy) echo caddy ;;
    nginx) echo behind:nginx ;;
    bt|1panel) echo behind:panel ;;
    *) echo behind:other ;;
  esac
}

# The command that does what was chosen, to run again without questions.
equivalent_command() {
  local c="sudo bash install-hub.sh"
  case "$MODE" in
    caddy) c+=" --domain $DOMAIN" ;;
    behind) c+=" --behind $BEHIND --domain $DOMAIN" ;;
    tunnel) c+=" --tunnel $TUNNEL"; [[ -n "$TUNNEL_HOST" ]] && c+=" --hostname $TUNNEL_HOST" ;;
    http) c+=" --http"; [[ -n "$DOMAIN" ]] && c+=" --domain $DOMAIN" ;;
    local) c+=" --local" ;;
  esac
  [[ -n "$PORT" ]] && c+=" --port $PORT"
  [[ -n "$NAME" ]] && c+=" --name $NAME"
  [[ "$VERSION" != latest ]] && c+=" --version $VERSION"
  printf '%s' "$c"
}

TTY_FD=""
open_tty() {
  [[ -n "$TTY_FD" ]] && return 0
  if [[ -t 0 && -z "${MYCO_INSTALL_TTY:-}" ]]; then TTY_FD=0; return 0; fi
  # curl ... | sudo bash: stdin is the script, but the terminal is still there.
  if [[ -r "$TTY_PATH" ]] && { exec 3<"$TTY_PATH"; } 2>/dev/null; then TTY_FD=3; return 0; fi
  return 1
}

ask() { # ask <prompt> <default> -> REPLY
  local p="$1" d="$2"
  printf '%s' "$p" >&2
  [[ -n "$d" ]] && printf ' [%s]' "$d" >&2
  printf ': ' >&2
  REPLY=""
  read -r -u "$TTY_FD" REPLY || true
  [[ -n "$REPLY" ]] || REPLY="$d"
}

# No arguments: ask how others should reach the hub.
choose_way() {
  if ! open_tty; then
    cat >&2 <<EOF
How should others reach this hub? Run again with one of:

  A  Automatic HTTPS (Caddy on port 443):
       sudo bash install-hub.sh --domain auto
  B  Behind the web server that already has port 443:
       sudo bash install-hub.sh --behind nginx --domain hub.example.com
       sudo bash install-hub.sh --behind panel --domain hub.example.com   # 宝塔 / 1Panel
  C  No open ports, through a tunnel:
       sudo bash install-hub.sh --tunnel cloudflare --hostname hub.example.com
       sudo bash install-hub.sh --tunnel tailscale
  D  Plain HTTP on a port, trusted networks only:
       sudo bash install-hub.sh --http --port 7777
     This server only:
       sudo bash install-hub.sh --local

(To be asked instead, download the script and run it in a terminal: sudo bash install-hub.sh)
EOF
    exit 2
  fi
  local h443 h80 rec def choice
  h443="$(holder_of 443)"
  h80="$(holder_of 80)"
  rec="$(recommended_for "$h443")"
  echo "==> port 443: $(holder_words "$h443")    port 80: $(holder_words "$h80")" >&2
  echo >&2
  echo "怎么让别的电脑连到这个 Hub？ How should other computers reach this hub?" >&2
  local a b
  a="  1) 自动 HTTPS：装 Caddy，用 443 端口 / Automatic HTTPS with Caddy on 443"
  b="  2) 放到已有的 $(holder_words "$h443") 后面，HTTPS 用它的 / Behind the web server already on 443"
  [[ "$h443" == free ]] && b="  2) 放到已有的网站服务器后面（nginx、宝塔、1Panel…）/ Behind a web server you run"
  [[ "$rec" == caddy ]] && { a+="  ← 推荐 recommended"; def=1; }
  [[ "$rec" == behind:* ]] && { b+="  ← 推荐 recommended"; def=2; }
  echo "$a" >&2
  echo "$b" >&2
  echo "  3) 不开任何端口：用 Cloudflare 或 Tailscale 隧道 / No open ports: a Cloudflare or Tailscale tunnel" >&2
  echo "  4) 直接 HTTP，自选端口：明文，只适合内网和测试 / Plain HTTP on a port: trusted networks and tests only" >&2
  echo "  5) 只给这台服务器自己用 / This server only" >&2
  while :; do
    ask "选一个 Choose" "$def"; choice="$REPLY"
    case "$choice" in 1|2|3|4|5) break ;; esac
    echo "   1, 2, 3, 4 or 5" >&2
  done
  case "$choice" in
    1) MODE=caddy ;;
    2) MODE=behind
       case "$rec" in behind:*) BEHIND="${rec#behind:}" ;; *) BEHIND="" ;; esac
       if [[ -z "$BEHIND" || "$h443" == free ]]; then
         ask "哪一种？ Which one: nginx / panel (宝塔、1Panel) / caddy / other" "nginx"; BEHIND="$REPLY"
         case "$BEHIND" in nginx|panel|caddy|other) ;; *) echo "nginx, panel, caddy or other" >&2; exit 2 ;; esac
       fi ;;
    3) MODE=tunnel
       ask "哪一种？ Which: cloudflare / tailscale" "cloudflare"; TUNNEL="$REPLY"
       case "$TUNNEL" in cloudflare|tailscale) ;; *) echo "cloudflare or tailscale" >&2; exit 2 ;; esac
       if [[ "$TUNNEL" == cloudflare ]]; then
         ask "你在 Cloudflare 上的域名，回车用临时地址（每次重启都会变）/ Hostname on your Cloudflare domain, Enter for a temporary address" ""
         TUNNEL_HOST="$REPLY"
       fi ;;
    4) MODE=http ;;
    5) MODE=local ;;
  esac
  case "$MODE" in
    caddy|behind)
      ask "域名，回车用 <IP>.sslip.io / Domain, Enter for <IP>.sslip.io" "auto"; DOMAIN="$REPLY" ;;
    http)
      ask "端口 Port" "${PORT:-7777}"; PORT="$REPLY"
      ask "别人用什么地址连，回车用公网 IP / Name or IP others use, Enter for the public IP" ""; DOMAIN="$REPLY" ;;
  esac
  echo >&2
  echo "==> 下次可以直接运行 / Same without questions:" >&2
  echo "    $(equivalent_command)" >&2
  ask "继续？ Continue? (Y/n)" "Y"
  [[ "$REPLY" =~ ^[Yy]|^$ ]] || exit 1
}

# -------------------------------------------------------------- check only

hub_public_url() { sed -n 's#.*--public-url \([^ ]*\).*#\1#p' "$UNIT_FILE" 2>/dev/null | head -1; }

if [[ "$CHECK_ONLY" -eq 1 ]]; then
  [[ -f "$UNIT_FILE" ]] || { echo "no hub installed here ($UNIT_FILE missing)" >&2; exit 1; }
  PORT="$(sed -n 's#^MYCO_HUB=http://127\.0\.0\.1:\([0-9]*\).*#\1#p' "$ENV_FILE" 2>/dev/null | head -1 || true)"
  PUBLIC_URL="$(hub_public_url)"
  if [[ "$(cat "$UNIT_FILE")" == *"--tunnel cloudflare"* ]]; then
    MODE=tunnel; TUNNEL=cloudflare
  elif [[ -f "$CF_UNIT_FILE" ]]; then
    MODE=tunnel; TUNNEL=cloudflare; TUNNEL_HOST="${PUBLIC_URL#https://}"
  elif [[ "$PUBLIC_URL" == https://*.ts.net ]]; then
    MODE=tunnel; TUNNEL=tailscale
  fi
  [[ "$MODE" == tunnel ]] || case "$PUBLIC_URL" in
    https://*) DOMAIN="${PUBLIC_URL#https://}"
      if [[ -f "$CADDY_SITE" ]]; then MODE=caddy; else MODE=behind; fi
      if [[ -e "$NGINX_DIR/conf.d/myco-$SITE.conf" || -e "$NGINX_DIR/sites-enabled/myco-$SITE.conf" ]]; then MODE=behind; BEHIND=nginx; fi ;;
    http://127.0.0.1:*) MODE=local ;;
    http://*)
      if [[ -e "$NGINX_DIR/conf.d/myco-$SITE.conf" || -e "$NGINX_DIR/sites-enabled/myco-$SITE.conf" ]]; then MODE=behind; BEHIND=nginx; else MODE=http; fi ;;
  esac
fi

# ------------------------------------------------------------- the install

if [[ "$CHECK_ONLY" -eq 0 && -z "$MODE" ]]; then
  if [[ "$ASSUME_YES" -eq 1 ]]; then MODE=local; else choose_way; fi
fi

OLD_PORT="$(sed -n 's#^MYCO_HUB=http://127\.0\.0\.1:\([0-9]*\).*#\1#p' "$ENV_FILE" 2>/dev/null | head -1 || true)"
if [[ -z "$PORT" ]]; then
  if [[ -n "$OLD_PORT" ]]; then PORT="$OLD_PORT"
  elif [[ -z "$NAME" ]]; then PORT=7777
  else
    PORT=7778
    # Also reserve ports of stopped hubs, whose sockets are not listening.
    while [[ -n "$(grep -El "^MYCO_HUB=http://127\\.0\\.0\\.1:$PORT$" "$ROOT/etc/myco/hub.env" "$ROOT"/etc/myco/hubs/*/hub.env 2>/dev/null || true)" ]] ||
      listening "$PORT"; do
      PORT=$((PORT + 1))
    done
  fi
fi
if ! [[ "$PORT" =~ ^[0-9]+$ ]] || (( PORT < 1 || PORT > 65535 )); then echo "--port must be 1-65535" >&2; exit 2; fi
# How the hub was reached before this run (to undo our own parts of it).
OLD_UNIT_TEXT="$(cat "$UNIT_FILE" 2>/dev/null || true)"

# ------------------------------------------------- C: what a tunnel needs

# cloudflared's account certificate (from `cloudflared tunnel login`), which
# may have been made as root or as the user who ran sudo.
find_cf_cert() {
  local home c
  home="$(getent passwd "${SUDO_USER:-root}" 2>/dev/null | cut -d: -f6 || true)"
  for c in "${TUNNEL_ORIGIN_CERT:-}" "$CF_DIR/cert.pem" "${HOME:-/root}/.cloudflared/cert.pem" "$ROOT/root/.cloudflared/cert.pem" \
    ${home:+"$ROOT$home/.cloudflared/cert.pem"} "$ROOT/etc/cloudflared/cert.pem" "$ROOT/usr/local/etc/cloudflared/cert.pem"; do
    [[ -n "$c" && -f "$c" ]] && { printf '%s' "$c"; return 0; }
  done
  return 1
}

# A tunnel's credentials file (<id>.json), wherever `tunnel create` left it.
find_cf_creds() {
  local id="$1" home d
  home="$(getent passwd "${SUDO_USER:-root}" 2>/dev/null | cut -d: -f6 || true)"
  for d in "$CF_DIR" "$(dirname "$CF_CERT")" "${HOME:-/root}/.cloudflared" "$ROOT/root/.cloudflared" ${home:+"$ROOT$home/.cloudflared"} "$ROOT/etc/cloudflared"; do
    [[ -f "$d/$id.json" ]] && { printf '%s' "$d/$id.json"; return 0; }
  done
  return 1
}

# The id of the named tunnel in the account ("" when there is none).
cf_tunnel_id() {
  local out
  out="$(cloudflared --origincert "$CF_CERT" tunnel list --output json --name "$TUNNEL_NAME" 2>/dev/null || true)"
  out="$(grep -o '"id": *"[^"]*"' <<<"$out" | head -1 || true)"
  out="${out%\"}"; printf '%s' "${out##*\"}"
}

# This device's name on the tailnet, from `tailscale status --json` (the
# first DNSName after "Self"; peers follow it).
ts_dns_name() {
  awk '/"Self"/ {self=1} self && /"DNSName"/ { sub(/.*"DNSName": *"/, ""); sub(/".*/, ""); sub(/\.$/, ""); print; exit }' <<<"$1"
}

# Checks what the tunnel needs before anything is downloaded or written, and
# says what to run when something is missing.
tunnel_preflight() {
  case "$TUNNEL" in
    cloudflare)
      if ! command -v cloudflared >/dev/null; then
        echo "!! cloudflared is not installed. Install it, then run this again:" >&2
        echo "     Debian / Ubuntu:  curl -fsSLo /tmp/cloudflared.deb https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-$(dpkg --print-architecture 2>/dev/null || echo amd64).deb && sudo dpkg -i /tmp/cloudflared.deb" >&2
        echo "     others:           https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/" >&2
        exit 1
      fi
      [[ -z "$TUNNEL_HOST" ]] && return 0 # temporary address: no account needed
      if ! CF_CERT="$(find_cf_cert)"; then
        echo "!! cloudflared is not logged in to your Cloudflare account. Run this, choose the zone of $TUNNEL_HOST, then run this again:" >&2
        echo "     cloudflared tunnel login" >&2
        exit 1
      fi ;;
    tailscale)
      if ! command -v tailscale >/dev/null; then
        echo "!! Tailscale is not installed. Install it and log in, then run this again:" >&2
        echo "     curl -fsSL https://tailscale.com/install.sh | sh && sudo tailscale up" >&2
        exit 1
      fi
      local st state
      st="$(tailscale status --json 2>/dev/null || true)"
      state="$(sed -n 's/.*"BackendState": *"\([^"]*\)".*/\1/p' <<<"$st" | head -1)"
      TS_DNS="$(ts_dns_name "$st")"
      if [[ "$state" != Running || -z "$TS_DNS" ]]; then
        echo "!! Tailscale is not connected${state:+ (state $state)}. Log in, then run this again:" >&2
        echo "     sudo tailscale up" >&2
        exit 1
      fi
      # Funnel is set up first (nothing listens yet; that is fine), so a
      # tailnet without Funnel stops here, before anything is installed.
      local out
      if ! out="$(tailscale funnel --bg "$PORT" 2>&1)"; then
        echo "!! Tailscale Funnel could not be turned on:" >&2
        printf '%s\n' "$out" | sed 's/^/     /' >&2
        echo "   Enable Funnel (and HTTPS certificates) for this device in the Tailscale admin console (the link above), then run this again." >&2
        exit 1
      fi ;;
  esac
}
[[ "$CHECK_ONLY" -eq 0 && "$MODE" == tunnel ]] && tunnel_preflight

if [[ "$CHECK_ONLY" -eq 0 ]]; then

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac

if [[ "$VERSION" == "latest" ]]; then
  URL="https://github.com/$REPO/releases/latest/download/myco-linux-$ARCH"
else
  URL="https://github.com/$REPO/releases/download/$VERSION/myco-linux-$ARCH"
fi

echo "==> downloading $URL"
tmp="$(mktemp)"
if ! curl -fsSL --retry 3 "$URL" -o "$tmp"; then
  # github.com's download links answer 504 now and then; the release API's
  # link for the same asset goes straight to the storage host.
  echo "==> retrying through the GitHub release API"
  if [[ "$VERSION" == "latest" ]]; then API="https://api.github.com/repos/$REPO/releases/latest"
  else API="https://api.github.com/repos/$REPO/releases/tags/$VERSION"; fi
  ASSET_URL="$(curl -fsSL "$API" | awk -v want="myco-linux-$ARCH" '
    /"url": *"[^"]*\/releases\/assets\/[0-9]+"/ { match($0, /https:[^"]+/); u = substr($0, RSTART, RLENGTH) }
    /"name":/ && index($0, "\"" want "\"") { print u; exit }')"
  [[ -n "$ASSET_URL" ]] || { echo "cannot find myco-linux-$ARCH in $API" >&2; exit 1; }
  curl -fsSL --retry 3 -H "Accept: application/octet-stream" "$ASSET_URL" -o "$tmp"
fi
install -d -m 0755 "$(dirname "$MYCO")"
install -m 0755 "$tmp" "$MYCO"
rm -f "$tmp"
"$MYCO" version

echo "==> configuring $ENV_REAL"
install -d -m 0700 "$(dirname "$ENV_FILE")"
if [[ ! -s "$ENV_FILE" ]]; then
  umask 077
  echo "MYCO_ADMIN_TOKEN=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')" > "$ENV_FILE"
  NEW_TOKEN=1
fi

PUBLIC_IP=""
if [[ "$DOMAIN" == "auto" ]]; then
  PUBLIC_IP="$(public_ip)"
  [[ -n "$PUBLIC_IP" ]] || { echo "cannot find this server's public IP; give --domain <name>" >&2; exit 1; }
  if [[ "$MODE" == http ]]; then
    DOMAIN=""
  else
    DOMAIN="${PUBLIC_IP//[.:]/-}.sslip.io"
    [[ -n "$NAME" ]] && DOMAIN="$NAME.$DOMAIN"
  fi
fi
LISTEN="127.0.0.1:$PORT"
case "$MODE" in
  caddy|behind) PUBLIC_URL="https://$DOMAIN" ;;
  tunnel)
    case "$TUNNEL" in
      tailscale) PUBLIC_URL="https://$TS_DNS" ;;
      *) PUBLIC_URL="${TUNNEL_HOST:+https://$TUNNEL_HOST}" ;; # temporary: learned once it runs
    esac ;;
  http)
    LISTEN="0.0.0.0:$PORT"
    HOST="$DOMAIN"
    if [[ -z "$HOST" ]]; then
      [[ -n "$PUBLIC_IP" ]] || PUBLIC_IP="$(public_ip)"
      HOST="$PUBLIC_IP"
    fi
    [[ -n "$HOST" ]] || { echo "cannot find this server's public IP; give --domain <name or IP>" >&2; exit 1; }
    [[ "$HOST" == *:* && "$HOST" != \[* ]] && HOST="[$HOST]"
    PUBLIC_URL="http://$HOST:$PORT" ;;
  *) PUBLIC_URL="http://127.0.0.1:$PORT" ;;
esac
# nginx without a certificate yet answers on plain HTTP (see below).
if [[ "$MODE" == behind && "$BEHIND" == nginx && ! -f "$ROOT/etc/letsencrypt/live/$DOMAIN/fullchain.pem" ]] && ! command -v certbot >/dev/null; then
  PUBLIC_URL="http://$DOMAIN"
fi

# Lets `sudo /usr/local/bin/myco link create` (and other admin commands) on
# this machine find the hub and the admin token without any flags.
if grep -q '^MYCO_HUB=' "$ENV_FILE"; then
  sed -i "s#^MYCO_HUB=.*#MYCO_HUB=http://127.0.0.1:$PORT#" "$ENV_FILE"
else
  echo "MYCO_HUB=http://127.0.0.1:$PORT" >> "$ENV_FILE"
fi

write_unit() {
  local serve="--addr $LISTEN --db $DATA_DIR/hub.db"
  [[ -n "$PUBLIC_URL" ]] && serve+=" --public-url $PUBLIC_URL"
  # A temporary Cloudflare address is the hub's own cloudflared: it follows
  # the address, which changes whenever the tunnel restarts.
  [[ "$MODE" == tunnel && "$TUNNEL" == cloudflare && -z "$TUNNEL_HOST" ]] && serve+=" --tunnel cloudflare"
  install -d -m 0755 "$(dirname "$UNIT_FILE")"
  cat > "$UNIT_FILE" <<EOF
[Unit]
Description=myco hub${NAME:+ $NAME}
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=$ENV_REAL
ExecStart=/usr/local/bin/myco hub serve $serve
StateDirectory=$STATE_DIR
DynamicUser=yes
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
EOF
}
write_unit

# Upgrading from the App: the hub (an unprivileged service user) leaves a
# request in its state directory; this root path unit runs `myco upgrade`,
# which restarts it. (`sudo myco upgrade` adds the same to older servers.)
write_upgrade_units() {
  cat > /etc/systemd/system/myco-upgrade.path <<'UNIT'
[Unit]
Description=myco: upgrade when a hub asks for it (from the App)

[Path]
PathExistsGlob=/var/lib/private/myco*/upgrade.request
PathExistsGlob=/var/lib/myco*/upgrade.request

[Install]
WantedBy=multi-user.target
UNIT
  cat > /etc/systemd/system/myco-upgrade.service <<'UNIT'
[Unit]
Description=myco: upgrade asked for by a hub

[Service]
Type=oneshot
ExecStart=/bin/sh -c 'rm -f /var/lib/private/myco*/upgrade.request /var/lib/myco*/upgrade.request; exec /usr/local/bin/myco upgrade'
UNIT
}
[[ -z "$ROOT" ]] && write_upgrade_units
T0="$(date +%s)"
systemctl daemon-reload
systemctl enable --now "$UNIT"
if [[ -z "$ROOT" ]]; then systemctl enable --now myco-upgrade.path >/dev/null 2>&1 || true; fi
systemctl restart "$UNIT"

# ------------------------------------------------------------------ Caddy

# Move the site written by older install-hub.sh versions into a site file,
# leaving unrelated Caddyfile blocks untouched. The old block is identified
# by its reverse proxy to the default hub and the script's read_timeout
# (120s before the hub's waits grew to 5 minutes, 360s since).
migrate_legacy_caddy() {
  local file="$CADDY_DIR/Caddyfile" old_port tmp
  [[ -f "$file" ]] || return 0
  old_port="$(sed -n 's#^MYCO_HUB=http://127\.0\.0\.1:\([0-9]*\).*#\1#p' "$ROOT/etc/myco/hub.env" 2>/dev/null | head -1 || true)"
  [[ -z "$NAME" && -n "$OLD_PORT" ]] && old_port="$OLD_PORT"
  [[ -n "$old_port" ]] || old_port=7777
  tmp="$(mktemp)"
  awk -v port="$old_port" -v site="$CADDY_DIR/myco.d/default.caddy" '
    function delta(s, o, c, x) { x=s; o=gsub(/\{/, "", x); x=s; c=gsub(/\}/, "", x); return o-c }
    {
      if (!block && $0 ~ /^[^#[:space:]].*\{[[:space:]]*$/) { block=$0 ORS; depth=delta($0); next }
      if (block) {
        block=block $0 ORS; depth+=delta($0)
        if (depth==0) {
          if (index(block, "reverse_proxy 127.0.0.1:" port) && (index(block, "read_timeout 120s") || index(block, "read_timeout 360s"))) printf "%s", block > site
          else printf "%s", block
          block=""
        }
        next
      }
      print
    }
    END { if (block) printf "%s", block }
  ' "$file" > "$tmp"
  cat "$tmp" > "$file"
  [[ -f "$CADDY_DIR/myco.d/default.caddy" ]] && chmod 0644 "$CADDY_DIR/myco.d/default.caddy"
  rm -f "$tmp"
}

# The Caddyfile the caddy package ships serves a welcome page on :80. Older
# versions of this script replaced it; keep doing that when it is still the
# untouched sample (it would also hold :80 when that port is taken).
clear_caddy_sample() {
  local file="$CADDY_DIR/Caddyfile"
  [[ -f "$file" ]] || return 0
  local body
  body="$(sed -e 's/#.*//' -e 's/[[:space:]]\+/ /g' -e 's/^ //' -e 's/ $//' "$file" | grep -v '^$' | tr '\n' '|')"
  if [[ "$body" == ":80 {|root * /usr/share/caddy|file_server|}|" ]]; then
    : > "$file"
  fi
}

ensure_caddy_import() {
  local file="$CADDY_DIR/Caddyfile"
  [[ -f "$file" ]] || install -m 0644 /dev/null "$file"
  if ! grep -Fqx 'import /etc/caddy/myco.d/*.caddy' "$file"; then
    printf '\nimport /etc/caddy/myco.d/*.caddy\n' >> "$file"
  fi
}

ensure_caddy_http_port() {
  local file="$CADDY_DIR/Caddyfile" tmp
  grep -Eq '^[[:space:]]*http_port[[:space:]]+' "$file" && return 0
  tmp="$(mktemp)"
  if grep -Eq '^\{[[:space:]]*$' "$file"; then
    awk '!added && /^\{[[:space:]]*$/ {print; print "\thttp_port 18080"; added=1; next} {print}' "$file" > "$tmp"
  else
    printf '{\n\thttp_port 18080\n}\n\n' > "$tmp"
    cat "$file" >> "$tmp"
  fi
  cat "$tmp" > "$file"
  rm -f "$tmp"
}

caddy_snippet() {
  cat <<EOF
$DOMAIN {
	reverse_proxy 127.0.0.1:$PORT {
		transport http {
			read_timeout 360s
		}
	}
}
EOF
}

setup_caddy() {
  if ! command -v caddy >/dev/null; then
    echo "==> installing Caddy"
    if command -v apt-get >/dev/null; then
      apt-get install -y debian-keyring debian-archive-keyring apt-transport-https curl gpg >/dev/null
      curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor --yes -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
      curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' > /etc/apt/sources.list.d/caddy-stable.list
      apt-get update >/dev/null && apt-get install -y caddy >/dev/null
    elif command -v dnf >/dev/null; then
      dnf install -y 'dnf-command(copr)' >/dev/null && dnf copr enable -y @caddy/caddy >/dev/null && dnf install -y caddy >/dev/null
    else
      echo "install Caddy manually (https://caddyserver.com/docs/install), then re-run" >&2; exit 1
    fi
  fi
  # Something else (nginx, a panel, ...) may already own :80. Caddy can still
  # get a certificate through the TLS-ALPN challenge on :443 alone, so move
  # its HTTP listener aside instead of touching the other service.
  port_busy() { listening "$1" && [[ "$(ss -Hltnp "sport = :$1" 2>/dev/null)" != *caddy* ]]; }
  if port_busy 80; then
    echo "==> port 80 is used by another program; Caddy will use :443 only (TLS-ALPN challenge)"
    ss -Hltnp "sport = :80" 2>/dev/null | sed 's/^/    /' || true
  fi
  if port_busy 443; then
    local h
    h="$(holder_of 443)"
    echo "!! port 443 is already used by another program:" >&2
    ss -Hltnp "sport = :443" >&2 || true
    echo "!! put the hub behind it instead (it keeps 443 and its certificates):" >&2
    case "$(recommended_for "$h")" in
      behind:nginx) echo "!!   sudo bash install-hub.sh --behind nginx --domain $DOMAIN${NAME:+ --name $NAME}" >&2 ;;
      behind:panel) echo "!!   sudo bash install-hub.sh --behind panel --domain $DOMAIN${NAME:+ --name $NAME}" >&2 ;;
      *) echo "!!   sudo bash install-hub.sh --behind other --domain $DOMAIN${NAME:+ --name $NAME}" >&2 ;;
    esac
    echo "!! or free port 443 and re-run this command." >&2
    exit 1
  fi
  install -d -m 0755 "$CADDY_DIR/myco.d"
  CADDY_BACKUP="$(mktemp -d)"
  [[ -f "$CADDY_DIR/Caddyfile" ]] && cp -p "$CADDY_DIR/Caddyfile" "$CADDY_BACKUP/Caddyfile"
  [[ -f "$CADDY_DIR/myco.d/default.caddy" ]] && cp -p "$CADDY_DIR/myco.d/default.caddy" "$CADDY_BACKUP/default.caddy"
  [[ -f "$CADDY_SITE" ]] && cp -p "$CADDY_SITE" "$CADDY_BACKUP/site.caddy"
  clear_caddy_sample
  migrate_legacy_caddy
  ensure_caddy_import
  if port_busy 80; then ensure_caddy_http_port; fi
  caddy_snippet > "$CADDY_SITE"
  chmod 0644 "$CADDY_SITE"
  if ! caddy validate --config "$CADDY_DIR/Caddyfile" --adapter caddyfile; then
    echo "!! Caddy configuration failed validation; restoring its previous files" >&2
    if [[ -f "$CADDY_BACKUP/Caddyfile" ]]; then cp -p "$CADDY_BACKUP/Caddyfile" "$CADDY_DIR/Caddyfile"; else rm -f "$CADDY_DIR/Caddyfile"; fi
    if [[ -f "$CADDY_BACKUP/default.caddy" ]]; then cp -p "$CADDY_BACKUP/default.caddy" "$CADDY_DIR/myco.d/default.caddy"; else rm -f "$CADDY_DIR/myco.d/default.caddy"; fi
    if [[ "$SITE" != default ]]; then
      if [[ -f "$CADDY_BACKUP/site.caddy" ]]; then cp -p "$CADDY_BACKUP/site.caddy" "$CADDY_SITE"; else rm -f "$CADDY_SITE"; fi
    fi
    rm -rf "$CADDY_BACKUP"
    exit 1
  fi
  rm -rf "$CADDY_BACKUP"
  if systemctl is-active --quiet firewalld; then
    echo "==> opening ports 80/443 in firewalld"
    firewall-cmd --permanent --add-service=http --add-service=https >/dev/null
    firewall-cmd --reload >/dev/null
  fi
  systemctl enable caddy >/dev/null 2>&1 || true
  systemctl restart caddy || true
  sleep 3
  if ! systemctl is-active --quiet caddy; then
    echo "!! Caddy failed to start. Last log lines:" >&2
    journalctl -u caddy -n 30 --no-pager >&2 || true
    echo "!! Common causes: port 80/443 already used (check: ss -ltnp | grep -E ':(80|443)\\s'), or blocked by the cloud firewall." >&2
    echo "!! The hub itself is running on 127.0.0.1:$PORT; fix Caddy and re-run this script." >&2
    CADDY_FAILED=1
  fi
}

# Our own Caddy site for this hub, from an earlier install in another way.
remove_caddy_site() {
  [[ -f "$CADDY_SITE" ]] || return 0
  echo "==> removing this hub's Caddy site (it is reached another way now)"
  rm -f "$CADDY_SITE"
  systemctl reload caddy 2>/dev/null || true
}

# ------------------------------------------------------------------ nginx

# Where this nginx reads extra sites from: Debian's sites-enabled, or conf.d.
NGINX_FILE=""
NGINX_LINK=""
nginx_paths() {
  if [[ -d "$NGINX_DIR/sites-enabled" ]] && grep -qs 'sites-enabled' "$NGINX_DIR/nginx.conf"; then
    NGINX_FILE="$NGINX_DIR/sites-available/myco-$SITE.conf"
    NGINX_LINK="$NGINX_DIR/sites-enabled/myco-$SITE.conf"
  else
    NGINX_FILE="$NGINX_DIR/conf.d/myco-$SITE.conf"
    NGINX_LINK=""
  fi
}

# What a location for the hub needs: long polls (up to a few minutes),
# uploads of attachments, and the original host and scheme.
nginx_location() {
  cat <<EOF
	client_max_body_size $HUB_UPLOAD_LIMIT;
	location / {
		proxy_pass http://127.0.0.1:$PORT;
		proxy_http_version 1.1;
		proxy_set_header Host \$host;
		proxy_set_header X-Real-IP \$remote_addr;
		proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
		proxy_set_header X-Forwarded-Proto \$scheme;
		proxy_set_header Upgrade \$http_upgrade;
		proxy_set_header Connection \$http_connection;
		proxy_buffering off;
		proxy_request_buffering off;
		proxy_read_timeout 360s;
		proxy_send_timeout 360s;
	}
EOF
}

nginx_site() { # nginx_site http|https
  echo "# myco hub${NAME:+ $NAME}: written by install-hub.sh; re-running it rewrites this file."
  if [[ "$1" == https ]]; then
    cat <<EOF
server {
	listen 80;
	listen [::]:80;
	server_name $DOMAIN;
	return 301 https://\$host\$request_uri;
}

server {
	listen 443 ssl;
	listen [::]:443 ssl;
	server_name $DOMAIN;
	ssl_certificate /etc/letsencrypt/live/$DOMAIN/fullchain.pem;
	ssl_certificate_key /etc/letsencrypt/live/$DOMAIN/privkey.pem;
EOF
  else
    cat <<EOF
server {
	listen 80;
	listen [::]:80;
	server_name $DOMAIN;
EOF
  fi
  nginx_location
  echo "}"
}

# Write our site file, test the whole configuration, reload; on a failed test
# put back what was there and stop.
nginx_apply() { # nginx_apply http|https
  local backup="" out
  install -d -m 0755 "$(dirname "$NGINX_FILE")"
  if [[ -f "$NGINX_FILE" ]]; then backup="$(mktemp)"; cp -p "$NGINX_FILE" "$backup"; fi
  nginx_site "$1" > "$NGINX_FILE"
  chmod 0644 "$NGINX_FILE"
  if [[ -n "$NGINX_LINK" ]]; then install -d -m 0755 "$(dirname "$NGINX_LINK")"; ln -sfn "$NGINX_FILE" "$NGINX_LINK"; fi
  if ! out="$(nginx -t 2>&1)"; then
    echo "!! nginx rejected the configuration; putting back what was there:" >&2
    printf '%s\n' "$out" | sed 's/^/   /' >&2
    if [[ -n "$backup" ]]; then cp -p "$backup" "$NGINX_FILE"; rm -f "$backup"
    else rm -f "$NGINX_FILE"; [[ -n "$NGINX_LINK" ]] && rm -f "$NGINX_LINK"; fi
    echo "!! nothing else was changed; the hub itself runs on 127.0.0.1:$PORT." >&2
    exit 1
  fi
  rm -f "$backup"
  systemctl reload nginx
}

setup_nginx() {
  command -v nginx >/dev/null || { echo "nginx is not installed here; use --behind other (or install nginx) and re-run" >&2; exit 1; }
  nginx_paths
  local cert="$ROOT/etc/letsencrypt/live/$DOMAIN"
  echo "==> nginx: writing ${NGINX_FILE#"$ROOT"} (your other sites are not touched)"
  if [[ -f "$cert/fullchain.pem" && -f "$cert/privkey.pem" ]]; then
    echo "==> using the certificate in /etc/letsencrypt/live/$DOMAIN"
    nginx_apply https
    return
  fi
  nginx_apply http
  if command -v certbot >/dev/null; then
    echo "==> asking Let's Encrypt for a certificate for $DOMAIN (certbot, through nginx on port 80)"
    if certbot certonly --nginx -d "$DOMAIN" --non-interactive --agree-tos --register-unsafely-without-email --keep-until-expiring >/dev/null 2>&1 &&
      [[ -f "$cert/fullchain.pem" ]]; then
      nginx_apply https
      return
    fi
    echo "!! certbot could not get a certificate (port 80 must reach this server for $DOMAIN)." >&2
    echo "!! see: certbot certonly --nginx -d $DOMAIN ; then re-run this script. Meanwhile it answers on http://$DOMAIN" >&2
    PUBLIC_URL="http://$DOMAIN"
    write_unit; systemctl daemon-reload; systemctl restart "$UNIT"
  else
    echo "!! no certificate for $DOMAIN yet, so for now the hub answers on http://$DOMAIN (unencrypted)."
    echo "!! for HTTPS: apt-get install -y certbot python3-certbot-nginx (or dnf install certbot python3-certbot-nginx),"
    echo "!! then re-run this same command: it gets the certificate and switches to https://$DOMAIN."
  fi
}

remove_nginx_site() {
  local f
  for f in "$NGINX_DIR/conf.d/myco-$SITE.conf" "$NGINX_DIR/sites-enabled/myco-$SITE.conf" "$NGINX_DIR/sites-available/myco-$SITE.conf"; do
    [[ -e "$f" || -L "$f" ]] || continue
    echo "==> removing this hub's nginx site ${f#"$ROOT"} (it is reached another way now)"
    rm -f "$f"
    NGINX_CHANGED=1
  done
  if [[ -n "${NGINX_CHANGED:-}" ]] && command -v nginx >/dev/null && nginx -t >/dev/null 2>&1; then
    systemctl reload nginx 2>/dev/null || true
  fi
}

# ------------------------------------------- B: settings for others to enter

print_proxy_settings() {
  echo
  echo "==> 在$1里给 $DOMAIN 加一个反向代理 / Add a reverse proxy for $DOMAIN in $2:"
  case "$BEHIND" in
    panel)
      echo "    宝塔 / aaPanel：网站 → 添加站点（域名 $DOMAIN）→ 设置 → 反向代理 → 添加反向代理"
      echo "    1Panel：网站 → 创建网站 → 反向代理（主域名 $DOMAIN，代理地址 127.0.0.1:$PORT）"
      ;;
  esac
  cat <<EOF
    目标 URL / Target URL:        http://127.0.0.1:$PORT
    发送域名 / Host header:       \$host（原来的域名 / the original host）
    超时 / Timeouts:              360s（proxy_read_timeout、proxy_send_timeout）
    缓存 / Buffering:             关闭 / off（proxy_buffering off）
    上传大小 / Upload limit:      $HUB_UPLOAD_LIMIT（client_max_body_size）
    HTTPS：给 $DOMAIN 开启 HTTPS（面板里一键申请 Let's Encrypt 证书）/ turn on HTTPS for $DOMAIN
EOF
  case "$BEHIND" in
    caddy)
      echo
      echo "    Caddyfile 里加上这段，然后 caddy reload / Add to your Caddyfile, then caddy reload:"
      caddy_snippet | sed 's/^/      /'
      ;;
    *)
      echo
      echo "    nginx 格式（面板的「配置文件」里可以直接粘进 server { } 里）/ As nginx config, inside the site's server { }:"
      nginx_location | sed 's/^/      /'
      if [[ "$BEHIND" == other ]]; then
        echo
        echo "    Apache（需要 mod_proxy、mod_proxy_http、mod_headers）/ Apache:"
        echo "      ProxyPreserveHost On"
        echo "      ProxyPass / http://127.0.0.1:$PORT/ timeout=360"
        echo "      ProxyPassReverse / http://127.0.0.1:$PORT/"
        echo "      RequestHeader set X-Forwarded-Proto \"https\""
        echo "      LimitRequestBody 67108864"
      fi
      ;;
  esac
}

# ------------------------------------------------------------ C: tunnels

# A named Cloudflare tunnel runs as a service of its own (cloudflared, as
# Cloudflare's own service does), with its credentials kept in /etc/myco:
# the hub runs as a dynamic user without a home for ~/.cloudflared.
setup_cloudflare_named() {
  local id creds bin out
  install -d -m 0700 "$CF_DIR"
  id="$(cf_tunnel_id)"
  if [[ -z "$id" ]]; then
    echo "==> creating the Cloudflare tunnel $TUNNEL_NAME"
    cloudflared --origincert "$CF_CERT" tunnel create --credentials-file "$CF_DIR/new.json" "$TUNNEL_NAME" >/dev/null
    id="$(cf_tunnel_id)"
    [[ -n "$id" ]] || { echo "!! the tunnel $TUNNEL_NAME was created but cannot be found (cloudflared tunnel list)" >&2; exit 1; }
    mv -f "$CF_DIR/new.json" "$CF_DIR/$id.json"
  elif [[ ! -f "$CF_DIR/$id.json" ]]; then
    if ! creds="$(find_cf_creds "$id")"; then
      echo "!! the tunnel $TUNNEL_NAME exists in your Cloudflare account, but its credentials ($id.json) are not on this server." >&2
      echo "   Use another name (--tunnel-name myco-2), or copy that file from the machine that created it into $CF_DIR_REAL." >&2
      exit 1
    fi
    install -m 0600 "$creds" "$CF_DIR/$id.json"
  fi
  chmod 0600 "$CF_DIR/$id.json"
  [[ "$CF_CERT" == "$CF_DIR/cert.pem" ]] || install -m 0600 "$CF_CERT" "$CF_DIR/cert.pem"
  echo "==> pointing $TUNNEL_HOST at the tunnel (a DNS record in your Cloudflare zone)"
  if ! out="$(cloudflared --origincert "$CF_CERT" tunnel route dns --overwrite-dns "$TUNNEL_NAME" "$TUNNEL_HOST" 2>&1)"; then
    echo "!! cloudflared could not route $TUNNEL_HOST to the tunnel:" >&2
    printf '%s\n' "$out" | sed 's/^/     /' >&2
    echo "   Is $TUNNEL_HOST in a zone of the account you logged in to (cloudflared tunnel login)?" >&2
    exit 1
  fi
  cat > "$CF_DIR/config.yml" <<EOF
# Written by install-hub.sh; re-running it rewrites this file.
tunnel: $id
credentials-file: $CF_DIR_REAL/$id.json
ingress:
  - hostname: $TUNNEL_HOST
    service: http://127.0.0.1:$PORT
  - service: http_status:404
EOF
  chmod 0600 "$CF_DIR/config.yml"
  bin="$(command -v cloudflared)"
  cat > "$CF_UNIT_FILE" <<EOF
[Unit]
Description=Cloudflare tunnel for the myco hub${NAME:+ $NAME}
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=$bin tunnel --no-autoupdate --config $CF_DIR_REAL/config.yml run $id
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now "$CF_UNIT"
  systemctl restart "$CF_UNIT"
  echo "==> Cloudflare tunnel $TUNNEL_NAME: https://$TUNNEL_HOST -> 127.0.0.1:$PORT (service $CF_UNIT)"
}

# Our named-tunnel service, from an earlier install through Cloudflare. The
# tunnel itself stays in the account (and its credentials here).
remove_cf_tunnel_service() {
  [[ -f "$CF_UNIT_FILE" ]] || return 0
  echo "==> stopping this hub's Cloudflare tunnel service (it is reached another way now)"
  systemctl disable --now "$CF_UNIT" 2>/dev/null || true
  rm -f "$CF_UNIT_FILE" "$CF_DIR/config.yml"
  systemctl daemon-reload
  echo "   the tunnel $TUNNEL_NAME stays in your Cloudflare account; to delete it: cloudflared tunnel delete $TUNNEL_NAME"
}

# Our Funnel, from an earlier install through Tailscale.
remove_ts_funnel() {
  [[ "$OLD_UNIT_TEXT" =~ --public-url\ https://[^[:space:]]*\.ts\.net ]] || return 0
  command -v tailscale >/dev/null || return 0
  echo "==> turning off this hub's Tailscale Funnel (it is reached another way now)"
  tailscale funnel --bg "${OLD_PORT:-$PORT}" off >/dev/null 2>&1 || tailscale funnel "${OLD_PORT:-$PORT}" off >/dev/null 2>&1 ||
    echo "   could not turn it off; see: tailscale funnel status"
}

# ------------------------------------------------------- D: an open port

open_firewall_port() {
  if ufw_active; then
    echo "==> opening port $PORT/tcp in ufw"
    ufw allow "$PORT/tcp" >/dev/null
  fi
  if systemctl is-active --quiet firewalld; then
    echo "==> opening port $PORT/tcp in firewalld"
    firewall-cmd --permanent --add-port="$PORT/tcp" >/dev/null
    firewall-cmd --reload >/dev/null
  fi
}

case "$MODE" in
  tunnel) ;;
  *) remove_cf_tunnel_service
     remove_ts_funnel ;;
esac
case "$MODE" in
  caddy)
    remove_nginx_site
    setup_caddy ;;
  tunnel)
    remove_caddy_site
    remove_nginx_site
    if [[ "$TUNNEL" == cloudflare && -n "$TUNNEL_HOST" ]]; then setup_cloudflare_named; else remove_cf_tunnel_service; fi
    [[ "$TUNNEL" == tailscale ]] || remove_ts_funnel
    if [[ "$TUNNEL" == cloudflare && -z "$TUNNEL_HOST" ]]; then
      echo "==> waiting for the temporary Cloudflare address (up to 60s)"
      for _ in $(seq 1 12); do
        PUBLIC_URL="$(quick_tunnel_url "$T0")"
        [[ -n "$PUBLIC_URL" ]] && break
        sleep 5
      done
    fi ;;
  behind)
    remove_caddy_site
    if [[ "$BEHIND" == nginx ]]; then setup_nginx
    else remove_nginx_site
      case "$BEHIND" in
        panel) print_proxy_settings "面板（宝塔、1Panel）" "your panel" ;;
        caddy) print_proxy_settings " Caddy " "Caddy" ;;
        *) print_proxy_settings "你的网站服务器" "your web server" ;;
      esac
    fi ;;
  http)
    remove_caddy_site
    remove_nginx_site
    open_firewall_port ;;
  local)
    remove_caddy_site
    remove_nginx_site ;;
esac

fi # CHECK_ONLY

# ------------------------------------------------------------ the self-test

# Checks the hub from this server, layer by layer, and says where it stops.
# A computer elsewhere is the only true outside check: this one can only
# reach its own public address, which some clouds route around their firewall.
SELFTEST_OK=0
check_fail() { echo "!! $*"; }

self_test() {
  local scheme host port url ips
  echo
  echo "==> 自检 / checking"
  sleep 1
  if ! curl -fsS --max-time 5 "http://127.0.0.1:$PORT/v1/health" >/dev/null 2>&1; then
    check_fail "the hub does not answer on this server (127.0.0.1:$PORT). Layer: the hub service."
    check_fail "  look at: systemctl status $UNIT ; journalctl -u $UNIT -n 50 --no-pager"
    return 1
  fi
  echo "==> hub is healthy"
  [[ "$MODE" == local ]] && { SELFTEST_OK=1; return 0; }
  [[ "$MODE" == tunnel ]] && { tunnel_test; return; }

  url="$PUBLIC_URL"
  scheme="${url%%://*}"
  host="${url#*://}"; host="${host%%/*}"
  port="${host##*:}"
  if [[ "$host" == *:* && "$port" =~ ^[0-9]+$ && "$host" != *"]" ]]; then host="${host%:*}"; else port=""; fi
  if [[ -z "$port" ]]; then
    if [[ "$scheme" == https ]]; then port=443; else port=80; fi
  fi
  host="${host#[}"; host="${host%]}"

  # DNS: does the name lead here?
  if [[ ! "$host" =~ ^[0-9.]+$ && "$host" != *:* ]]; then
    [[ -n "$PUBLIC_IP" ]] || PUBLIC_IP="$(public_ip)"
    ips="$(getent ahosts "$host" 2>/dev/null | awk '{print $1}' | sort -u | tr '\n' ' ')"
    if [[ -z "$ips" ]]; then
      check_fail "$host does not resolve. Layer: DNS. Add an A record for $host pointing to this server (${PUBLIC_IP:-its public IP})."
      return 1
    fi
    if [[ -n "$PUBLIC_IP" && " $ips" != *" $PUBLIC_IP "* ]]; then
      check_fail "$host resolves to $ips but this server's public IP is $PUBLIC_IP. Layer: DNS (or a CDN in front)."
    fi
  fi

  # The web server in front (A, B) or the hub's own port (D), on this server.
  if [[ "$MODE" == http ]]; then
    if ! [[ "$(ss -Hltn "sport = :$PORT" 2>/dev/null)" =~ (0\.0\.0\.0|\*|\[::\]):${PORT}[[:space:]] ]]; then
      check_fail "the hub listens only on this server, not on port $PORT for others. Layer: the hub service (re-run this script)."
      return 1
    fi
  else
    if ! curl -fsS --max-time 8 --resolve "$host:$port:127.0.0.1" "$url/v1/health" >/dev/null 2>&1; then
      if [[ "$scheme" == https ]] && curl -fsS --max-time 8 --resolve "$host:80:127.0.0.1" "http://$host/v1/health" >/dev/null 2>&1; then
        check_fail "$url: HTTP works on this server but HTTPS does not. Layer: the certificate."
      else
        check_fail "$url does not reach the hub on this server. Layer: the web server in front of it."
      fi
      case "$MODE/$BEHIND" in
        caddy/*) check_fail "  look at: journalctl -u caddy -n 50 --no-pager" ;;
        behind/nginx) check_fail "  look at: nginx -t ; tail -n 50 /var/log/nginx/error.log" ;;
        *) check_fail "  check the reverse proxy settings above (target http://127.0.0.1:$PORT) and the site's certificate" ;;
      esac
      return 1
    fi
  fi

  # Through the public address, from here.
  if curl -fsS --max-time 8 "$url/v1/health" >/dev/null 2>&1; then
    echo "==> reachable at $url (checked from this server through its public address)"
    echo "    to be sure, open $url/v1/health on another computer: it should say {\"ok\":true,...}"
    SELFTEST_OK=1
    return 0
  fi
  if ufw_active && ! grep -Eq "^$port(/tcp)?[[:space:]].*ALLOW" <<<"$(ufw status 2>/dev/null)"; then
    check_fail "$url is not reachable: ufw on this server does not allow port $port. Layer: this server's firewall."
    check_fail "  fix: ufw allow $port/tcp"
  elif systemctl is-active --quiet firewalld && ! grep -qw "$port/tcp" <<<"$(firewall-cmd --list-ports 2>/dev/null)" &&
    ! grep -qw "$(if [[ $port == 443 ]]; then echo https; else echo http; fi)" <<<"$(firewall-cmd --list-services 2>/dev/null)"; then
    check_fail "$url is not reachable: firewalld on this server does not allow port $port. Layer: this server's firewall."
    check_fail "  fix: firewall-cmd --permanent --add-port=$port/tcp && firewall-cmd --reload"
  else
    check_fail "$url works on this server but not through its public address. Layer: the cloud provider's firewall"
    check_fail "  (security group): allow inbound TCP $port there. Some providers also block 80/443 for unregistered domains."
  fi
  return 1
}

# Through the tunnel: no port of this server is involved, so the layers are
# the tunnel itself, its DNS name, and (Tailscale) Funnel.
tunnel_test() {
  local url="$PUBLIC_URL" st
  if [[ -z "$url" ]]; then
    check_fail "the temporary Cloudflare tunnel has not reported its address. Layer: the tunnel."
    check_fail "  look at: journalctl -u $UNIT -n 50 --no-pager"
    return 1
  fi
  if curl -fsS --max-time 10 "$url/v1/health" >/dev/null 2>&1; then
    echo "==> reachable at $url (through the tunnel, checked from this server)"
    echo "    to be sure, open $url/v1/health on another computer: it should say {\"ok\":true,...}"
    SELFTEST_OK=1
    return 0
  fi
  case "$TUNNEL" in
    cloudflare)
      if [[ -z "$TUNNEL_HOST" ]]; then
        check_fail "$url does not reach the hub. Layer: the tunnel (a temporary address can take a minute to work)."
        check_fail "  look at: journalctl -u $UNIT -n 50 --no-pager"
      elif ! systemctl is-active --quiet "$CF_UNIT"; then
        check_fail "the Cloudflare tunnel is not running. Layer: the tunnel."
        check_fail "  look at: systemctl status $CF_UNIT ; journalctl -u $CF_UNIT -n 50 --no-pager"
      elif [[ -z "$(getent ahosts "$TUNNEL_HOST" 2>/dev/null || true)" ]]; then
        check_fail "$TUNNEL_HOST does not resolve yet. Layer: DNS (the record in your Cloudflare zone; it can take a few minutes)."
        check_fail "  check the zone's DNS page for a CNAME $TUNNEL_HOST -> <tunnel id>.cfargotunnel.com"
      else
        check_fail "the tunnel runs but $url does not reach the hub. Layer: the tunnel."
        check_fail "  look at: journalctl -u $CF_UNIT -n 50 --no-pager"
      fi ;;
    tailscale)
      st="$(tailscale funnel status 2>&1 || true)"
      if [[ "$st" != *"127.0.0.1:$PORT"* && "$st" != *"localhost:$PORT"* ]]; then
        check_fail "Tailscale Funnel is not serving this hub (port $PORT). Layer: Funnel."
        check_fail "  fix: tailscale funnel --bg $PORT ; and check Funnel is enabled for this device in the admin console"
      else
        check_fail "$url does not answer yet. Layer: Tailscale Funnel (the first certificate and DNS name can take a minute)."
        check_fail "  look at: tailscale funnel status"
      fi ;;
  esac
  return 1
}

if [[ "$CHECK_ONLY" -eq 1 ]]; then
  PUBLIC_IP=""
  [[ "$MODE" == tunnel && "$TUNNEL" == cloudflare && -z "$TUNNEL_HOST" ]] && PUBLIC_URL="$(quick_tunnel_url)"
  self_test && exit 0 || exit 1
fi

if [[ "$MODE" == caddy && -z "${CADDY_FAILED:-}" ]]; then
  echo "==> waiting for the TLS certificate (up to 60s)"
  for _ in $(seq 1 12); do
    if curl -fsS --max-time 5 --resolve "$DOMAIN:443:127.0.0.1" "https://$DOMAIN/v1/health" >/dev/null 2>&1; then
      echo "==> HTTPS is up: https://$DOMAIN"; HTTPS_OK=1; break
    fi
    sleep 5
  done
  [[ -z "${HTTPS_OK:-}" ]] && echo "WARNING: HTTPS not answering yet; check: journalctl -u caddy -n 50 --no-pager"
fi
if [[ "$MODE" == behind && "$BEHIND" != nginx && -n "$TTY_FD" ]]; then
  echo
  ask "填好后按回车自检 / Press Enter once it is set up, to check" ""
fi
self_test || true
if [[ "$SELFTEST_OK" -ne 1 ]]; then
  echo "   fix it, then check again: sudo bash install-hub.sh --check${NAME:+ --name $NAME}"
fi

echo
[[ -n "${CADDY_FAILED:-}" ]] && echo "WARNING: HTTPS is not up yet (see Caddy errors above)."
if [[ "$MODE" == http ]]; then
  echo "!! ================================================================"
  echo "!! 明文 HTTP：令牌和消息不加密地在网络上传输，只在信得过的网络里用。"
  echo "!! Plain HTTP: tokens and messages cross the network unencrypted."
  echo "!! Use it only on networks you trust, or for a test."
  echo "!! Also allow inbound TCP $PORT in your cloud provider's security group."
  echo "!! ================================================================"
fi
if [[ "$MODE" == tunnel && "$TUNNEL" == cloudflare && -z "$TUNNEL_HOST" ]]; then
  echo "!! ================================================================"
  echo "!! 临时地址：隧道每次重启（包括服务器重启）都会换一个新地址，"
  echo "!! 之前的链接和已经加入的电脑就连不上了。只适合试一试。"
  echo "!! Temporary address: it CHANGES whenever the tunnel restarts (also"
  echo "!! after a reboot); links and joined computers then stop working."
  echo "!! For trying it out only. For a fixed address use your own domain:"
  echo "!!   sudo bash install-hub.sh --tunnel cloudflare --hostname hub.example.com"
  echo "!! ================================================================"
fi
echo "Hub URL:      ${PUBLIC_URL:-(not known yet; see journalctl -u $UNIT)}"
echo "Admin token:  stored in $ENV_REAL (root only; never paste it into chats)"

# First install: create a join link right away so adding machines is one paste.
JOIN_URL=""
APP_LINK=""
if [[ -n "${NEW_TOKEN:-}" ]]; then
  LINK_JSON="$("$MYCO" link create "${HUB_NAME_ARGS[@]}" --note "created by install-hub.sh" --format json 2>/dev/null || true)"
  JOIN_URL="$(printf '%s' "$LINK_JSON" | sed -n 's/.*"join_url": *"\([^"]*\)".*/\1/p' | head -1)"
  APP_JSON="$("$MYCO" console link "${HUB_NAME_ARGS[@]}" --name app --format json 2>/dev/null || true)"
  APP_LINK="$(printf '%s' "$APP_JSON" | sed -n 's/.*"link": *"\([^"]*\)".*/\1/p' | head -1)"
fi
echo
if [[ -n "$APP_LINK" ]]; then
  echo "Add this hub to the myco app (连接已有的 Hub), paste this link (works once, within 24 hours; keep it private):"
  echo "  $APP_LINK"
  echo
else
  echo "To add this hub to the myco app, make an app link: sudo /usr/local/bin/myco console link ${HUB_NAME_ARGS[*]}"
  echo
fi
if [[ -n "$JOIN_URL" ]]; then
  echo "Add a computer or server (macOS / Linux / WSL), run this on it:"
  echo "  curl -fsSL $JOIN_URL | bash"
  echo
  echo "Connect an AI session (Claude Code, Codex, claude.ai), tell it:"
  echo "  接入 myco：$JOIN_URL"
  echo
  echo "The link works for 7 days / 10 machines. Make another one any time:"
else
  echo "To add computers and AI sessions, create a join link:"
fi
echo "  sudo /usr/local/bin/myco link create ${HUB_NAME_ARGS[*]}"
echo
echo "Next, your own agent (Claude Code, Codex…) can add computers and agents for you. Tell it:"
echo "接下来可以让你的 agent 帮你加电脑和 agent，把这句话发给它："
echo "  帮我把电脑和 agent 加进这个 myco 工作区：读 https://raw.githubusercontent.com/chenhg5/myco/main/docs/agent-setup.md ，照着做，要我决定的先问我。"
