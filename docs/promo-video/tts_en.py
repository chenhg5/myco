"""English narration with Kokoro (offline): script.json -> $OUT/NN.wav, durations.json."""
import json, os, sys
import soundfile as sf
from kokoro_onnx import Kokoro
here = os.path.dirname(os.path.abspath(__file__))
out = sys.argv[1]
m = os.environ.get("KOKORO_DIR", here)
k = Kokoro(f"{m}/kokoro-v1.0.onnx", f"{m}/voices-v1.0.bin")
voice, speed = os.environ.get("KOKORO_VOICE", "af_heart"), float(os.environ.get("KOKORO_SPEED", "1.08"))
os.makedirs(out, exist_ok=True)
durs = []
for i, ln in enumerate(json.load(open(f"{here}/script.json"))["lines"]):
    f = f"{out}/{i:02d}.wav"
    if not os.path.exists(f):
        s, r = k.create(ln.get("say_en", ln["en"]), voice=voice, speed=speed, lang="en-us")
        sf.write(f, s, r)
    info = sf.info(f); durs.append(round(info.frames / info.samplerate, 3))
    print(i, durs[-1], ln["en"][:60], flush=True)
json.dump(durs, open(f"{out}/durations.json", "w"))
