# Promo film source

A 60-second product film in English and Chinese, made only from the app's own interface: the agents scattered in terminals land in one app, then a new conversation, several agents at once and handing work to each other, another computer's terminal and the phone, automations (and, in Chinese, the apps), and the end card with the slogan. Motion on a 120 BPM grid, narration, a synthesized beat and sound effects. Every frame of `film.html` is a pure function of time; `render.mjs` steps through it with Playwright and pipes the frames to ffmpeg, then mixes narration, music and effects.

| File | What it is |
|---|---|
| `script.json` | The sections (on the beat grid), the length, and the narration lines in both languages with their start times |
| `capture.cjs` | Captures the app states the film shows from the showcase mock (`web/mock/showcase.mjs`), dark, 1280×800 @2x (the phone at 390×844 @3x), and where the film aims in them (`boxes.json`) |
| `film.html` | The film: the logo, the terminals (drawn in HTML), the app window and its camera over the captures, the phone, the end card; and the sound-effect cues (`window.CUES`) |
| `tts_en.py` | English narration with [Kokoro](https://github.com/thewh1teagle/kokoro-onnx) (offline) |
| `tts_zh.mjs` | Chinese narration with Doubao TTS (`DOUBAO_TTS` points to a client module exporting `tts(text, file)`) |
| `music.py` | The music (sparse intro, build, drop at 0:18, lift at 0:36, final hit at 0:52) and the sound effects, synthesized with numpy: no samples, no licences |
| `render.mjs` | `stills`, `film` and `mix` |

Everything generated goes to `$WORK` (default `./work`, not committed). Node 22, Python 3 with numpy, ffmpeg, Playwright with Chromium:

```bash
npm i                                      # fonts (package.json)
export WORK=$PWD/work
# app captures, both languages (the showcase mock and the app's dev server running)
(cd ../../web && MOCK_SHELL=mac MOCK_SHOWCASE=zh node mock/app-server.mjs & npx vite --port 5174)
node capture.cjs zh $WORK/assets/zh        # restart the mock with MOCK_SHOWCASE=en, then: node capture.cjs en $WORK/assets/en
# narration, music, effects
pip install kokoro-onnx soundfile numpy    # and the two Kokoro model files in $KOKORO_DIR
python3 tts_en.py $WORK/vo-en
NODE_USE_ENV_PROXY=1 TTS_KEY_FILE=<key file> TTS_SPEAKER=zh_male_wennuanahu_uranus_bigtts TTS_RATE=16 DOUBAO_TTS=<client.mjs> node tts_zh.mjs $WORK/vo-zh
python3 music.py $WORK/audio
# picture and sound
LANG_FILM=zh node render.mjs stills 3 9 13 19.5 22.6 25 30 33 38 42 46 50 54   # spot-check frames
LANG_FILM=zh node render.mjs film && LANG_FILM=zh node render.mjs mix     # $WORK/out-zh/myco-promo-zh.mp4
LANG_FILM=en node render.mjs film && LANG_FILM=en node render.mjs mix     # $WORK/out-en/myco-promo-en.mp4
```

When the interface changes, capture again and render again: the film reads the captures and `boxes.json`, so only a scene whose layout moved needs its camera numbers in `film.html` adjusted.

For the README the film is re-encoded for size (CRF 26) and uploaded as a GitHub attachment (not committed: the repository stays small). Open the README in GitHub's web editor, drop the new file where the old attachment link is, and commit:

```bash
ffmpeg -i $WORK/out-en/myco-promo-en.mp4 -c:v libx264 -preset slow -crf 26 -pix_fmt yuv420p -c:a aac -b:a 128k -movflags +faststart myco-promo-en-small.mp4
ffmpeg -i $WORK/out-zh/myco-promo-zh.mp4 -c:v libx264 -preset slow -crf 26 -pix_fmt yuv420p -c:a aac -b:a 128k -movflags +faststart myco-promo-zh-small.mp4
```
