#!/usr/bin/env bash
# How well speech recognition hears Chinese with English words in it, before
# and after myco's help (words to listen for, then correcting known words).
# macOS only: it uses Apple's recognizer, as the App does.
#
#   scripts/speech-eval/run.sh              # synthesize the samples, recognize, score
#   MODES="base-ondevice hints-ondevice" scripts/speech-eval/run.sh
#
# Samples are samples.tsv, said by macOS voices (`say`); a real recording at
# recordings/<id>.wav (16 kHz mono is fine) is used instead when there is
# one. Results go to .cache/speech-eval/ and the table to stdout. The first
# run asks for the speech recognition permission (for the terminal).
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
if [ "$(uname)" != Darwin ]; then
  echo "macOS only: it runs Apple's speech recognition" >&2
  exit 2
fi
here=scripts/speech-eval
out="$(pwd)/.cache/speech-eval"
mkdir -p "$out/audio" "$out/bin"
MODES=${MODES:-"base-ondevice hints-ondevice base-server hints-server lm-ondevice"}
ZH_VOICE=${ZH_VOICE:-Tingting}
EN_VOICE=${EN_VOICE:-Samantha}

# The recognizer, with an Info.plist inside (the permission prompt needs it).
plist="$out/bin/Info.plist"
cat > "$plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>ai.myco.speech-eval</string>
<key>NSSpeechRecognitionUsageDescription</key><string>Measures how well speech recognition hears the samples.</string>
</dict></plist>
PLIST
if [ ! -x "$out/bin/recognize" ] || [ "$here/recognize.swift" -nt "$out/bin/recognize" ]; then
  swiftc -O -o "$out/bin/recognize" "$here/recognize.swift" \
    -Xlinker -sectcreate -Xlinker __TEXT -Xlinker __info_plist -Xlinker "$plist"
fi

# The words to listen for: the common ones and the workspace stand-ins.
SPEECH_EVAL_HINTS="$out/hints.txt" go test -count=1 -run TestSpeechEvalHints ./internal/console >/dev/null

# frames: the audio file has sound in it (say can write an empty file).
frames() { afinfo "$1" 2>/dev/null | awk '/audio packets|valid frames/ { if ($NF + 0 > 0) ok = 1 } END { exit !ok }'; }

# The audio: a recording when there is one, else a macOS voice saying it.
zh=() en=()
while IFS=$'\t' read -r id lang said _; do
  case "$id" in ''|'#'*) continue ;; esac
  wav="$out/audio/$id.wav"
  if [ -f "$here/recordings/$id.wav" ]; then
    cp "$here/recordings/$id.wav" "$wav"
  elif [ ! -s "$wav" ] || ! frames "$wav"; then
    voice=$ZH_VOICE; [ "${lang%%-*}" = en ] && voice=$EN_VOICE
    say -v "$voice" -o "$wav" --file-format=WAVE --data-format=LEI16@16000 "$said"
  fi
  if ! frames "$wav"; then
    rm -f "$wav"
    echo "$id: no audio came out of say -v $voice: check the voice (say -v ?) and try in Terminal on the Mac" >&2
    exit 1
  fi
  if [ "${lang%%-*}" = en ]; then en+=("$wav"); else zh+=("$wav"); fi
done < "$here/samples.tsv"

results="$out/results.jsonl"
: > "$results"
for mode in $MODES; do
  [ ${#zh[@]} -gt 0 ] && "$out/bin/recognize" --locale zh-CN --mode "$mode" --hints "$out/hints.txt" "${zh[@]}" >> "$results"
  [ ${#en[@]} -gt 0 ] && "$out/bin/recognize" --locale en-US --mode "$mode" --hints "$out/hints.txt" "${en[@]}" >> "$results"
done

# The score, each mode as heard and after correcting known words.
SPEECH_EVAL_RESULTS="$results" go test -count=1 -v -run TestSpeechEvalScore ./internal/console | sed -n '/^ *speech_eval_test.go/{s/^ *speech_eval_test.go:[0-9]*: //;p;}'
