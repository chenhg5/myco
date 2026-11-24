#!/bin/sh
# Sets the release workflow's Mac signing secrets (MAC_CERT_P12_BASE64 and
# MAC_CERT_PASSWORD) from a .p12 the maintainer exported from Keychain
# Access, after checking on this Mac that the password opens it and that it
# holds a Developer ID Application identity. Run it on the Mac, signed in to
# gh with admin rights on the repository:
#
#   scripts/mac-signing-secrets.sh [~/Desktop/myco-sign.p12]
#
# The password is asked once (not shown) and goes to the check and to gh
# through stdin; nothing is printed or written elsewhere.
set -eu
P12=${1:-$HOME/Desktop/myco-sign.p12}
REPO=${MYCO_REPO:-myco-ai/myco}
[ -f "$P12" ] || { echo "no $P12: export the Developer ID Application certificate there first (Keychain Access > My Certificates > Export)" >&2; exit 1; }
command -v gh >/dev/null || { echo "gh is not installed" >&2; exit 1; }

printf 'Password you set when exporting %s: ' "$P12"
stty -echo; IFS= read -r PW; stty echo; echo

# Import into a throwaway keychain: the same check the release workflow does.
dir=$(mktemp -d)
kc="$dir/check.keychain-db"
security create-keychain -p "$(openssl rand -hex 16)" "$kc"
trap 'security delete-keychain "$kc" 2>/dev/null; rm -rf "$dir"' EXIT
if ! security import "$P12" -k "$kc" -P "$PW" >/dev/null 2>&1; then
  echo "that password does not open $P12; nothing was set" >&2; exit 1
fi
security find-identity -p codesigning "$kc" | grep -q 'Developer ID Application' ||
  { echo "$P12 holds no Developer ID Application identity; nothing was set" >&2; exit 1; }

base64 -i "$P12" | tr -d '\n' | gh secret set MAC_CERT_P12_BASE64 -R "$REPO"
printf '%s' "$PW" | gh secret set MAC_CERT_PASSWORD -R "$REPO"
echo "set MAC_CERT_P12_BASE64 and MAC_CERT_PASSWORD on $REPO; you can delete $P12 now"
