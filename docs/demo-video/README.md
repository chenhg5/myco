# Demo video source

This is the earlier demo film, replaced in the READMEs by the one-minute film in [`docs/promo-video`](../promo-video/). Its rendered videos are no longer kept in the repo (see git history before this change); render them from these files if needed. Every frame is drawn by
`film.html` as a pure function of time; `render.mjs` steps through it with Playwright and pipes the frames
to ffmpeg. The team sequence uses messages from a real myco run (three Claude Code agents and an exec
node), shortened.

| File | What it is |
|---|---|
| `lines.json` | Narration and bilingual subtitles, one entry per line, grouped by shot |
| `tts.py` | English narration with [Kokoro](https://github.com/thewh1teagle/kokoro-onnx) (offline); writes `vo/NN.wav` |
| `film.html` | All seven shots, subtitles and transitions |
| `render.mjs` | Builds the timeline from the narration lengths, renders frames, mixes voice and music. `LANG_FILM=zh` renders the Chinese cut |
| `lines-zh.json` | Chinese narration and subtitles (same shots and line order as `lines.json`) |
| `tts-zh.mjs` | Chinese narration with Doubao TTS; writes `vo-zh/NN.mp3`. Bring your own client: `DOUBAO_TTS` points to a module exporting `tts(text, outFile)` |

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

For the Chinese cut, point `DOUBAO_TTS` at your Doubao TTS client module and run:

```bash
DOUBAO_TTS=/path/to/tts.mjs node tts-zh.mjs   # voice used: zh_male_wennuanahu_uranus_bigtts (温暖阿虎, seed-tts-2.0), speech_rate 12
LANG_FILM=zh node render.mjs film && LANG_FILM=zh node render.mjs mix   # out-zh/myco-demo-zh.mp4 (needs out-zh/bgm.wav)
```

The background music was generated with a small synth script; any quiet track saved as `out/bgm.wav` works.
