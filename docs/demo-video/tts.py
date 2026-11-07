"""Narrate lines.json with Kokoro (offline). Writes vo/NN.wav and vo/durations.json."""
import json
import os

import soundfile as sf
from kokoro_onnx import Kokoro

HERE = os.path.dirname(os.path.abspath(__file__))
M = os.environ.get("KOKORO_DIR", HERE)
k = Kokoro(f"{M}/kokoro-v1.0.onnx", f"{M}/voices-v1.0.bin")
voice = os.environ.get("KOKORO_VOICE", "af_heart")
speed = float(os.environ.get("KOKORO_SPEED", "1.0"))
os.makedirs(f"{HERE}/vo", exist_ok=True)
lines = json.load(open(f"{HERE}/lines.json"))
durs = []
for i, ln in enumerate(lines):
    out = f"{HERE}/vo/{i:02d}.wav"
    if not os.path.exists(out):
        samples, rate = k.create(ln.get("say", ln["en"]), voice=voice, speed=speed, lang="en-us")
        sf.write(out, samples, rate)
    info = sf.info(out)
    durs.append(round(info.frames / info.samplerate, 3))
    print(i, durs[-1], ln["en"][:50], flush=True)
json.dump(durs, open(f"{HERE}/vo/durations.json", "w"))
