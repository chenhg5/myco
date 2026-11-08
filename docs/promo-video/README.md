# Promo film source

A ~66 s product film in English and Chinese: motion graphics on a 120 BPM grid, narration, a synthesized beat and sound effects. Every frame of `film.html` is a pure function of time; `render.mjs` steps through it with Playwright and pipes the frames to ffmpeg, then mixes narration, music and effects.

| File | What it is |
|---|---|
| `script.json` | The sections (on the beat grid) and the narration lines in both languages, with their start times |
| `capture.cjs` | Captures the app states the film shows, from the showcase mock (`web/mock/showcase.mjs`) |
| `tts_en.py` | English narration with [Kokoro](https://github.com/thewh1teagle/kokoro-onnx) (offline) |
| `tts_zh.mjs` | Chinese narration with Doubao TTS (`DOUBAO_TTS` points to a client module exporting `tts(text, file)`) |
| `music.py` | The music (sparse intro, build, drop at 0:14, lift at 0:50, final hit at 0:58) and the sound effects, synthesized with numpy: no samples, no licences |
| `film.html` | The film: five agents alone (Claude Code, Codex, Cursor, a Grok bot, a cloud agent), the copy-paste trap, the mycelium drop into the logo, the app (see, add, give a task), agents handing work to each other, a montage and the end card. It also lists the sound-effect cues |
| `render.mjs` | `stills`, `film` and `mix` |

Everything generated goes to `$WORK` (default `./work`, not committed). Node 22, Python 3 with numpy, ffmpeg, Playwright with Chromium:

```bash
npm i --no-save @fontsource/inter@5.1.0 @fontsource/noto-sans-sc@5.1.0 @fontsource/jetbrains-mono@5.1.0
export WORK=$PWD/work
# app captures, both languages (with the showcase mock and the app's dev server running)
(cd ../../web && MOCK_SHELL=mac MOCK_SHOWCASE=en node mock/app-server.mjs & npx vite --port 5174)
node capture.cjs en $WORK/assets/en        # restart the mock with MOCK_SHOWCASE=zh, then: node capture.cjs zh $WORK/assets/zh
# narration, music, effects
pip install kokoro-onnx soundfile numpy    # and the two Kokoro model files in $KOKORO_DIR
python3 tts_en.py $WORK/vo-en
NODE_USE_ENV_PROXY=1 TTS_KEY_FILE=<key file> TTS_SPEAKER=zh_male_wennuanahu_uranus_bigtts TTS_RATE=16 DOUBAO_TTS=<client.mjs> node tts_zh.mjs $WORK/vo-zh
python3 music.py $WORK/audio
# picture and sound
LANG_FILM=en node render.mjs stills 6 16.8 29 38 46 61   # spot-check frames
LANG_FILM=en node render.mjs film && LANG_FILM=en node render.mjs mix     # $WORK/out-en/myco-promo-en.mp4
LANG_FILM=zh node render.mjs film && LANG_FILM=zh node render.mjs mix     # $WORK/out-zh/myco-promo-zh.mp4
```

Product names in the film (Claude Code, Codex, Cursor, Grok, claude.ai) are shown as text in drawn windows; no third-party logos are used.
