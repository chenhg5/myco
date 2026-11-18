#!/usr/bin/env bash
# Packs myco's official apps (internal/templates/official, built from
# templates/) as the "apps" release carries them: dist/apps/apps.zip, one
# folder an app, and SHA256SUMS. Hubs fetch it once a day (templates.md).
set -euo pipefail
cd "$(dirname "$0")/.."
rm -rf dist/apps
mkdir -p dist/apps
(cd internal/templates/official && find . -type f ! -name '.*' | LC_ALL=C sort | zip -X -q -@ ../../../dist/apps/apps.zip)
(cd dist/apps && sha256sum apps.zip > SHA256SUMS && cat SHA256SUMS)
