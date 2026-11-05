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
curl -fsSL "$URL" -o "$tmp"
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
  cat > /etc/caddy/Caddyfile <<EOF
$DOMAIN {
	reverse_proxy 127.0.0.1:$PORT {
		transport http {
			read_timeout 120s
		}
	}
}
EOF
  systemctl enable --now caddy
  systemctl reload caddy
fi

sleep 1
curl -fsS "http://127.0.0.1:$PORT/v1/health" >/dev/null && echo "==> hub is healthy"

echo
echo "Hub URL:      $PUBLIC_URL"
if [[ -n "${NEW_TOKEN:-}" ]]; then
  echo "Admin token:  $(cut -d= -f2 /etc/myco/hub.env)"
  echo "              (stored in /etc/myco/hub.env; keep it secret)"
else
  echo "Admin token:  unchanged (see /etc/myco/hub.env)"
fi
echo
echo "Next, on any machine:"
echo "  export MYCO_ADMIN_TOKEN=<admin token>"
echo "  myco agent register --hub $PUBLIC_URL --name <agent> --harness claude"
