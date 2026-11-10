#!/usr/bin/env bash
# Install or upgrade the myco CLI on macOS / Linux / Windows (Git Bash, MSYS2,
# Cygwin), then run a health check. In PowerShell use install-client.ps1.
#
#   curl -fsSL https://raw.githubusercontent.com/chenhg5/myco/main/deploy/install-client.sh | bash
#
# Options (environment variables):
#   MYCO_VERSION=v0.6.1     install a specific release (default: latest)
#   MYCO_INSTALL_DIR=...    where to put the binary (default: the directory of
#                           an existing `myco`, else ~/.local/bin)
#   MYCO_ADD_PATH=1         add that directory to PATH in your shell profile
#   MYCO_NO_DOCTOR=1        skip the health check (used by join links)
# No secrets are read or printed.
set -euo pipefail

REPO="chenhg5/myco"
VERSION="${MYCO_VERSION:-latest}"

EXE=""
case "$(uname -s)" in
  Darwin) OS=darwin ;;
  Linux)  OS=linux ;;
  MINGW*|MSYS*|CYGWIN*) OS=windows; EXE=".exe" ;;
  *) echo "unsupported OS: $(uname -s) (on Windows use Git Bash, or PowerShell with install-client.ps1)" >&2; exit 1 ;;
esac
MACHINE="$(uname -m)"
if [[ "$OS" == windows && -n "${PROCESSOR_ARCHITEW6432:-${PROCESSOR_ARCHITECTURE:-}}" ]]; then
  MACHINE="${PROCESSOR_ARCHITEW6432:-$PROCESSOR_ARCHITECTURE}"
fi
case "$MACHINE" in
  x86_64|amd64|AMD64) ARCH=amd64 ;;
  arm64|aarch64|ARM64) ARCH=arm64 ;;
  *) echo "unsupported architecture: $MACHINE" >&2; exit 1 ;;
esac

if [[ -n "${MYCO_INSTALL_DIR:-}" ]]; then
  DIR="$MYCO_INSTALL_DIR"
elif existing="$(command -v myco 2>/dev/null)"; then
  DIR="$(dirname "$existing")"
else
  DIR="$HOME/.local/bin"
fi
mkdir -p "$DIR"

if [[ "$VERSION" == "latest" ]]; then
  BASE="https://github.com/$REPO/releases/latest/download"
else
  BASE="https://github.com/$REPO/releases/download/$VERSION"
fi
ASSET="myco-$OS-$ARCH$EXE"

echo "==> current: $(command -v myco >/dev/null 2>&1 && myco version || echo 'not installed')"
echo "==> downloading $BASE/$ASSET"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
# GitHub's download redirect answers 502/504 now and then: retry those.
curl -fsSL --retry 5 --retry-delay 2 "$BASE/$ASSET" -o "$tmp/myco$EXE"
curl -fsSL --retry 5 --retry-delay 2 "$BASE/SHA256SUMS" -o "$tmp/SHA256SUMS"

want="$(awk -v f="$ASSET" '$2==f {print $1}' "$tmp/SHA256SUMS")"
if command -v shasum >/dev/null; then got="$(shasum -a 256 "$tmp/myco$EXE" | awk '{print $1}')"; else got="$(sha256sum "$tmp/myco$EXE" | awk '{print $1}')"; fi
if [[ -z "$want" || "$want" != "$got" ]]; then
  echo "checksum mismatch for $ASSET (want $want, got $got); aborting" >&2
  exit 1
fi
echo "==> checksum ok"

chmod +x "$tmp/myco$EXE"
if [[ -w "$DIR" || "$OS" == windows ]]; then
  # Windows: a running myco.exe cannot be overwritten, but it can be renamed.
  [[ "$OS" == windows && -f "$DIR/myco.exe" ]] && mv -f "$DIR/myco.exe" "$DIR/myco.exe.old" 2>/dev/null || true
  mv -f "$tmp/myco$EXE" "$DIR/myco$EXE"
else
  echo "==> $DIR is not writable, using sudo"
  sudo mv "$tmp/myco" "$DIR/myco"
fi
# Downloads via curl are not quarantined, but clear the flag just in case.
[[ "$OS" == darwin ]] && xattr -d com.apple.quarantine "$DIR/myco" 2>/dev/null || true

echo "==> installed: $DIR/myco$EXE $("$DIR/myco$EXE" version)"
case ":$PATH:" in
  *":$DIR:"*) ;;
  *)
    if [[ "${MYCO_ADD_PATH:-}" == 1 ]]; then
      # Make `myco` available to new shells (and to agents started from them).
      line="export PATH=\"$DIR:\$PATH\"  # added by myco"
      for rc in "$HOME/.profile" "$HOME/.bashrc" "$HOME/.zshrc"; do
        if [[ "$rc" == "$HOME/.profile" || -f "$rc" ]] && ! grep -qs "# added by myco" "$rc"; then
          echo "$line" >> "$rc"
        fi
      done
      echo "==> added $DIR to PATH in your shell profile (open a new terminal to use it)"
      if [[ "$OS" == windows ]] && command -v cygpath >/dev/null && command -v powershell.exe >/dev/null; then
        # Also for PowerShell, cmd and agent CLIs started from Windows itself.
        WINDIR_PATH="$(cygpath -w "$DIR")"
        powershell.exe -NoProfile -Command "\$d='$WINDIR_PATH'; \$p=[Environment]::GetEnvironmentVariable('Path','User'); if (-not ((\$p -split ';') -contains \$d)) { [Environment]::SetEnvironmentVariable('Path', ((@(\$p, \$d) | Where-Object { \$_ }) -join ';'), 'User') }" >/dev/null 2>&1 \
          && echo "==> added $WINDIR_PATH to your Windows user PATH"
      fi
    else
      echo "NOTE: $DIR is not on PATH. Add it, e.g.: echo 'export PATH=\"$DIR:\$PATH\"' >> ~/.zshrc"
    fi
    ;;
esac

if [[ "${MYCO_NO_DOCTOR:-}" == 1 ]]; then
  exit 0
fi

echo
echo "==> myco doctor --all"
if "$DIR/myco$EXE" doctor --all --format text; then
  echo
  echo "All checks passed."
else
  echo
  echo "Some checks failed: follow the suggestions above (myco doctor --all --format json for details)."
fi
echo
echo "If a runner is already running, restart it to use the new version:"
echo "  myco runner start --all"
