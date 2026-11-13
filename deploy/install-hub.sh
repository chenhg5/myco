#!/usr/bin/env bash
# Install a myco hub on a Linux VPS (systemd + optional Caddy for HTTPS).
#
#   curl -fsSLO https://raw.githubusercontent.com/chenhg5/myco/main/deploy/install-hub.sh
#   less install-hub.sh                       # read it first
#   sudo bash install-hub.sh --domain hub.example.com
#
# Without a domain, --domain auto uses <ip>.sslip.io (a public wildcard DNS
# that resolves to your IP) so Caddy can still get a real TLS certificate.
# Re-running is safe: the existing admin token and database are kept.
set -euo pipefail

REPO="chenhg5/myco"
VERSION="latest"
DOMAIN=""
PORT=""
NAME=""
NAME_SET=0

usage() {
  cat <<EOF
Usage: sudo bash install-hub.sh [--name <name>] [--domain <domain>|auto] [--version vX.Y.Z] [--port 7777]
  --name     Named hub (1-32 lowercase letters, digits or hyphens); omit for the original hub.
  --domain   Public name for HTTPS via Caddy (port 80/443 must be open).
             'auto' uses <public-ip>.sslip.io. Omit to listen on 127.0.0.1 only.
  --version  Release tag to install (default: latest).
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain) DOMAIN="$2"; shift 2 ;;
    --name) NAME="$2"; NAME_SET=1; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --port) PORT="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 2 ;;
  esac
done

[[ $EUID -eq 0 ]] || { echo "run as root (sudo)" >&2; exit 1; }
command -v systemctl >/dev/null || { echo "systemd is required" >&2; exit 1; }
if [[ "$NAME_SET" -eq 1 && ( -z "$NAME" || ${#NAME} -gt 32 || ! "$NAME" =~ ^[a-z0-9-]+$ ) ]]; then
  echo "--name must be 1-32 lowercase letters, digits or hyphens" >&2; exit 2
fi

ENV_FILE=/etc/myco/hub.env
DATA_DIR=/var/lib/myco
UNIT=myco-hub.service
STATE_DIR=myco
SITE=default
HUB_NAME_ARGS=()
if [[ -n "$NAME" ]]; then
  ENV_FILE="/etc/myco/hubs/$NAME/hub.env"
  # A sibling of /var/lib/myco, not inside it: systemd hands a unit's whole
  # StateDirectory (recursively) to that unit's dynamic user on every start,
  # so a named hub nested in the default one would lose its files to it.
  DATA_DIR="/var/lib/myco-$NAME"
  UNIT="myco-hub-$NAME.service"
  STATE_DIR="myco-$NAME"
  SITE="named-$NAME"
  HUB_NAME_ARGS=(--hub-name "$NAME")
fi

OLD_PORT="$(sed -n 's#^MYCO_HUB=http://127\.0\.0\.1:\([0-9]*\).*#\1#p' "$ENV_FILE" 2>/dev/null | head -1 || true)"
if [[ -z "$PORT" ]]; then
  if [[ -n "$OLD_PORT" ]]; then PORT="$OLD_PORT"
  elif [[ -z "$NAME" ]]; then PORT=7777
  else
    PORT=7778
    # Also reserve ports of stopped hubs, whose sockets are not listening.
    while [[ -n "$(grep -El "^MYCO_HUB=http://127\\.0\\.0\\.1:$PORT$" /etc/myco/hub.env /etc/myco/hubs/*/hub.env 2>/dev/null || true)" ]] ||
      ss -Hltn "sport = :$PORT" 2>/dev/null | grep -q .; do
      PORT=$((PORT + 1))
    done
  fi
fi
[[ "$PORT" =~ ^[0-9]+$ ]] && (( PORT >= 1 && PORT <= 65535 )) || { echo "--port must be 1-65535" >&2; exit 2; }

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
install -m 0755 "$tmp" /usr/local/bin/myco
rm -f "$tmp"
/usr/local/bin/myco version

echo "==> configuring $ENV_FILE"
install -d -m 0700 "$(dirname "$ENV_FILE")"
if [[ ! -s "$ENV_FILE" ]]; then
  umask 077
  echo "MYCO_ADMIN_TOKEN=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')" > "$ENV_FILE"
  NEW_TOKEN=1
fi

if [[ "$DOMAIN" == "auto" ]]; then
  IP="$(curl -fsS https://api.ipify.org)"
  DOMAIN="${IP//./-}.sslip.io"
  [[ -n "$NAME" ]] && DOMAIN="$NAME.$DOMAIN"
fi
PUBLIC_URL="http://127.0.0.1:$PORT"
[[ -n "$DOMAIN" ]] && PUBLIC_URL="https://$DOMAIN"
# Lets `sudo /usr/local/bin/myco link create` (and other admin commands) on
# this machine find the hub and the admin token without any flags.
if grep -q '^MYCO_HUB=' "$ENV_FILE"; then
  sed -i "s#^MYCO_HUB=.*#MYCO_HUB=http://127.0.0.1:$PORT#" "$ENV_FILE"
else
  echo "MYCO_HUB=http://127.0.0.1:$PORT" >> "$ENV_FILE"
fi

cat > "/etc/systemd/system/$UNIT" <<EOF
[Unit]
Description=myco hub${NAME:+ $NAME}
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=$ENV_FILE
ExecStart=/usr/local/bin/myco hub serve --addr 127.0.0.1:$PORT --db $DATA_DIR/hub.db --public-url $PUBLIC_URL
StateDirectory=$STATE_DIR
DynamicUser=yes
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now "$UNIT"
systemctl restart "$UNIT"

# Move the site written by older install-hub.sh versions into a site file,
# leaving unrelated Caddyfile blocks untouched. The old block is identified
# by its reverse proxy to the default hub and the script's read_timeout.
migrate_legacy_caddy() {
  local file=/etc/caddy/Caddyfile old_port tmp
  [[ -f "$file" ]] || return 0
  old_port="$(sed -n 's#^MYCO_HUB=http://127\.0\.0\.1:\([0-9]*\).*#\1#p' /etc/myco/hub.env 2>/dev/null | head -1 || true)"
  [[ -z "$NAME" && -n "$OLD_PORT" ]] && old_port="$OLD_PORT"
  [[ -n "$old_port" ]] || old_port=7777
  tmp="$(mktemp)"
  awk -v port="$old_port" -v site=/etc/caddy/myco.d/default.caddy '
    function delta(s, o, c, x) { x=s; o=gsub(/\{/, "", x); x=s; c=gsub(/\}/, "", x); return o-c }
    {
      if (!block && $0 ~ /^[^#[:space:]].*\{[[:space:]]*$/) { block=$0 ORS; depth=delta($0); next }
      if (block) {
        block=block $0 ORS; depth+=delta($0)
        if (depth==0) {
          if (index(block, "reverse_proxy 127.0.0.1:" port) && index(block, "read_timeout 120s")) printf "%s", block > site
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
  [[ -f /etc/caddy/myco.d/default.caddy ]] && chmod 0644 /etc/caddy/myco.d/default.caddy
  rm -f "$tmp"
}

# The Caddyfile the caddy package ships serves a welcome page on :80. Older
# versions of this script replaced it; keep doing that when it is still the
# untouched sample (it would also hold :80 when that port is taken).
clear_caddy_sample() {
  local file=/etc/caddy/Caddyfile
  [[ -f "$file" ]] || return 0
  local body
  body="$(sed -e 's/#.*//' -e 's/[[:space:]]\+/ /g' -e 's/^ //' -e 's/ $//' "$file" | grep -v '^$' | tr '\n' '|')"
  if [[ "$body" == ":80 {|root * /usr/share/caddy|file_server|}|" ]]; then
    : > "$file"
  fi
}

ensure_caddy_import() {
  local file=/etc/caddy/Caddyfile tmp
  [[ -f "$file" ]] || install -m 0644 /dev/null "$file"
  if ! grep -Fqx 'import /etc/caddy/myco.d/*.caddy' "$file"; then
    printf '\nimport /etc/caddy/myco.d/*.caddy\n' >> "$file"
  fi
}

ensure_caddy_http_port() {
  local file=/etc/caddy/Caddyfile tmp
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

if [[ -n "$DOMAIN" ]]; then
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
  port_busy() { ss -Hltn "sport = :$1" 2>/dev/null | grep -q . && ! ss -Hltnp "sport = :$1" 2>/dev/null | grep -q caddy; }
  if port_busy 80; then
    echo "==> port 80 is used by another program; Caddy will use :443 only (TLS-ALPN challenge)"
    ss -Hltnp "sport = :80" 2>/dev/null | sed 's/^/    /' || true
  fi
  if port_busy 443; then
    echo "!! port 443 is already used by another program:" >&2
    ss -Hltnp "sport = :443" >&2 || true
    echo "!! free it, or put the hub behind that program (reverse proxy to 127.0.0.1:$PORT)." >&2
    exit 1
  fi
  install -d -m 0755 /etc/caddy/myco.d
  CADDY_BACKUP="$(mktemp -d)"
  [[ -f /etc/caddy/Caddyfile ]] && cp -p /etc/caddy/Caddyfile "$CADDY_BACKUP/Caddyfile"
  [[ -f /etc/caddy/myco.d/default.caddy ]] && cp -p /etc/caddy/myco.d/default.caddy "$CADDY_BACKUP/default.caddy"
  [[ -f "/etc/caddy/myco.d/$SITE.caddy" ]] && cp -p "/etc/caddy/myco.d/$SITE.caddy" "$CADDY_BACKUP/site.caddy"
  clear_caddy_sample
  migrate_legacy_caddy
  ensure_caddy_import
  if port_busy 80; then ensure_caddy_http_port; fi
  cat > "/etc/caddy/myco.d/$SITE.caddy" <<EOF
$DOMAIN {
	reverse_proxy 127.0.0.1:$PORT {
		transport http {
			read_timeout 120s
		}
	}
}
EOF
  chmod 0644 "/etc/caddy/myco.d/$SITE.caddy"
  if ! caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile; then
    echo "!! Caddy configuration failed validation; restoring its previous files" >&2
    if [[ -f "$CADDY_BACKUP/Caddyfile" ]]; then cp -p "$CADDY_BACKUP/Caddyfile" /etc/caddy/Caddyfile; else rm -f /etc/caddy/Caddyfile; fi
    if [[ -f "$CADDY_BACKUP/default.caddy" ]]; then cp -p "$CADDY_BACKUP/default.caddy" /etc/caddy/myco.d/default.caddy; else rm -f /etc/caddy/myco.d/default.caddy; fi
    if [[ "$SITE" != default ]]; then
      if [[ -f "$CADDY_BACKUP/site.caddy" ]]; then cp -p "$CADDY_BACKUP/site.caddy" "/etc/caddy/myco.d/$SITE.caddy"; else rm -f "/etc/caddy/myco.d/$SITE.caddy"; fi
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
elif [[ -n "$NAME" && -f "/etc/caddy/myco.d/$SITE.caddy" ]]; then
  # Reconfiguring a named Hub for local access removes its old public route.
  rm -f "/etc/caddy/myco.d/$SITE.caddy"
  systemctl reload caddy || true
fi

sleep 1
curl -fsS "http://127.0.0.1:$PORT/v1/health" >/dev/null && echo "==> hub is healthy"
if [[ -n "$DOMAIN" && -z "${CADDY_FAILED:-}" ]]; then
  echo "==> waiting for the TLS certificate (up to 60s)"
  for _ in $(seq 1 12); do
    if curl -fsS --max-time 5 --resolve "$DOMAIN:443:127.0.0.1" "https://$DOMAIN/v1/health" >/dev/null 2>&1; then
      echo "==> HTTPS is up: https://$DOMAIN"; HTTPS_OK=1; break
    fi
    sleep 5
  done
  [[ -z "${HTTPS_OK:-}" ]] && echo "WARNING: HTTPS not answering yet; check: journalctl -u caddy -n 50 --no-pager"
fi

echo
[[ -n "${CADDY_FAILED:-}" ]] && echo "WARNING: HTTPS is not up yet (see Caddy errors above)."
echo "Hub URL:      $PUBLIC_URL"
echo "Admin token:  stored in $ENV_FILE (root only; never paste it into chats)"

# First install: create a join link right away so adding machines is one paste.
JOIN_URL=""
APP_LINK=""
if [[ -n "${NEW_TOKEN:-}" ]]; then
  LINK_JSON="$(/usr/local/bin/myco link create "${HUB_NAME_ARGS[@]}" --note "created by install-hub.sh" --format json 2>/dev/null || true)"
  JOIN_URL="$(printf '%s' "$LINK_JSON" | sed -n 's/.*"join_url": *"\([^"]*\)".*/\1/p' | head -1)"
  APP_JSON="$(/usr/local/bin/myco console link "${HUB_NAME_ARGS[@]}" --name app --format json 2>/dev/null || true)"
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
