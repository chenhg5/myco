# Demo video source

`docs/assets/myco-demo.mp4` (and the README GIF) are rendered from these files. Every frame is drawn by
`film.html` as a pure function of time; `render.mjs` steps through it with Playwright and pipes the frames
to ffmpeg. The team sequence uses messages from a real myco run (three Claude Code agents and an exec
node), shortened.

| File | What it is |
|---|---|
| `lines.json` | Narration and bilingual subtitles, one entry per line, grouped by shot |
| `tts.py` | English narration with [Kokoro](https://github.com/thewh1teagle/kokoro-onnx) (offline); writes `vo/NN.wav` |
| `film.html` | All seven shots, subtitles and transitions |
| `render.mjs` | Builds the timeline from the narration lengths, renders frames, mixes voice and music |

To rebuild (Node 22, Python 3, ffmpeg, Playwright with Chromium):

```bash
npm i @fontsource/inter@5.1.0 @fontsource/jetbrains-mono@5.1.0 @fontsource/noto-sans-sc@5.1.0
pip install kokoro-onnx soundfile        # and download kokoro-v1.0.onnx + voices-v1.0.bin (kokoro-onnx releases)
KOKORO_DIR=<folder with the two model files> python tts.py
cp ../assets/dashboard-dark.png .
node render.mjs stills 10 40 70          # spot-check frames
node render.mjs film                     # out/video.mp4
node render.mjs mix                      # out/myco-demo.mp4 (needs out/bgm.wav)
```

The background music was generated with a small synth script; any quiet track saved as `out/bgm.wav` works.
