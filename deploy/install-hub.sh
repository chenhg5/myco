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
PORT="7777"

usage() {
  cat <<EOF
Usage: sudo bash install-hub.sh [--domain <name>|auto] [--version vX.Y.Z] [--port 7777]
  --domain   Public name for HTTPS via Caddy (port 80/443 must be open).
             'auto' uses <public-ip>.sslip.io. Omit to listen on 127.0.0.1 only.
  --version  Release tag to install (default: latest).
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain) DOMAIN="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --port) PORT="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage; exit 2 ;;
  esac
done

[[ $EUID -eq 0 ]] || { echo "run as root (sudo)" >&2; exit 1; }
command -v systemctl >/dev/null || { echo "systemd is required" >&2; exit 1; }

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

echo "==> configuring /etc/myco"
install -d -m 0700 /etc/myco
if [[ ! -s /etc/myco/hub.env ]]; then
  umask 077
  echo "MYCO_ADMIN_TOKEN=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')" > /etc/myco/hub.env
  NEW_TOKEN=1
fi

if [[ "$DOMAIN" == "auto" ]]; then
  IP="$(curl -fsS https://api.ipify.org)"
  DOMAIN="${IP//./-}.sslip.io"
fi
PUBLIC_URL="http://127.0.0.1:$PORT"
[[ -n "$DOMAIN" ]] && PUBLIC_URL="https://$DOMAIN"
# Lets `sudo /usr/local/bin/myco link create` (and other admin commands) on
# this machine find the hub and the admin token without any flags.
if grep -q '^MYCO_HUB=' /etc/myco/hub.env; then
  sed -i "s#^MYCO_HUB=.*#MYCO_HUB=http://127.0.0.1:$PORT#" /etc/myco/hub.env
else
  echo "MYCO_HUB=http://127.0.0.1:$PORT" >> /etc/myco/hub.env
fi

cat > /etc/systemd/system/myco-hub.service <<EOF
[Unit]
Description=myco hub
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=/etc/myco/hub.env
ExecStart=/usr/local/bin/myco hub serve --addr 127.0.0.1:$PORT --db /var/lib/myco/hub.db --public-url $PUBLIC_URL
StateDirectory=myco
DynamicUser=yes
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now myco-hub
systemctl restart myco-hub

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
  GLOBAL=""
  if port_busy 80; then
    echo "==> port 80 is used by another program; Caddy will use :443 only (TLS-ALPN challenge)"
    ss -Hltnp "sport = :80" 2>/dev/null | sed 's/^/    /' || true
    GLOBAL=$'{\n\thttp_port 18080\n}\n\n'
  fi
  if port_busy 443; then
    echo "!! port 443 is already used by another program:" >&2
    ss -Hltnp "sport = :443" >&2 || true
    echo "!! free it, or put the hub behind that program (reverse proxy to 127.0.0.1:$PORT)." >&2
    exit 1
  fi
  printf '%s' "$GLOBAL" > /etc/caddy/Caddyfile
  cat >> /etc/caddy/Caddyfile <<EOF
$DOMAIN {
	reverse_proxy 127.0.0.1:$PORT {
		transport http {
			read_timeout 120s
		}
	}
}
EOF
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
echo "Admin token:  stored in /etc/myco/hub.env (root only; never paste it into chats)"

# First install: create a join link right away so adding machines is one paste.
JOIN_URL=""
APP_LINK=""
if [[ -n "${NEW_TOKEN:-}" ]]; then
  LINK_JSON="$(/usr/local/bin/myco link create --note "created by install-hub.sh" --format json 2>/dev/null || true)"
  JOIN_URL="$(printf '%s' "$LINK_JSON" | sed -n 's/.*"join_url": *"\([^"]*\)".*/\1/p' | head -1)"
  APP_JSON="$(/usr/local/bin/myco console link --name app --format json 2>/dev/null || true)"
  APP_LINK="$(printf '%s' "$APP_JSON" | sed -n 's/.*"link": *"\([^"]*\)".*/\1/p' | head -1)"
fi
echo
if [[ -n "$APP_LINK" ]]; then
  echo "Add this hub to the myco app (连接已有的 Hub), paste this link (admin access: keep it private):"
  echo "  $APP_LINK"
  echo
else
  echo "To add this hub to the myco app, make an app link: sudo /usr/local/bin/myco console link"
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
echo "  sudo /usr/local/bin/myco link create"
