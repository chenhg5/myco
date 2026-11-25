#!/usr/bin/env bash
# Run the voice chat against a real model: fixed conversations through a real
# call, checking that the agent speaks of its work in the first person
# (internal/console/voice_eval_test.go). Any OpenAI-compatible API works.
#
#   VOICE_EVAL_BASE_URL=https://api.deepseek.com/v1 VOICE_EVAL_MODEL=deepseek-chat \
#   VOICE_EVAL_KEY=... scripts/voice-eval.sh
#
# VOICE_EVAL_RUNS=3 repeats each scenario; VOICE_EVAL_LANG=zh|en runs one
# language. The key is read from the environment only; never commit it.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
for v in VOICE_EVAL_BASE_URL VOICE_EVAL_MODEL VOICE_EVAL_KEY; do
  if [ -z "${!v:-}" ]; then
    echo "set $v (see the top of this script)" >&2
    exit 2
  fi
done
exec go test -count=1 -v -timeout 30m ./internal/console -run 'TestVoiceEvalRealModel|TestVoiceThirdPersonCheck' "$@"
