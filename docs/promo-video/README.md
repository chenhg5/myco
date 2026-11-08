# Promo film source

A ~66 s product film in English and Chinese: motion graphics on a 120 BPM grid, narration, a synthesized beat and sound effects. Every frame of `film.html` is a pure function of time; `render.mjs` steps through it with Playwright and pipes the frames to ffmpeg, then mixes narration, music and effects.

| File | What it is |
|---|---|
| `script.json` | The sections (on the beat grid) and the narration lines in both languages, with their start times |
| `capture.cjs` | Captures the app states the film shows, from the showcase mock (`web/mock/showcase.mjs`) |
| `fetch-logos.sh` | The agents' marks (Claude, OpenAI for Codex, Cursor, Grok) from their sources, as PNGs; trademarks of their owners, used to name the products myco works with |
| `windows.html`, `tex.cjs` | The five agent windows of the opening, styled after each product's interface, rendered to textures |
| `film3d.html` | The 3D world (Three.js): the title, the agents, the trap, the network of logo tiles (seeing each other, messages, files) and the end |
| `tts_en.py` | English narration with [Kokoro](https://github.com/thewh1teagle/kokoro-onnx) (offline) |
| `tts_zh.mjs` | Chinese narration with Doubao TTS (`DOUBAO_TTS` points to a client module exporting `tts(text, file)`) |
| `music.py` | The music (sparse intro, build, drop at 0:14, lift at 0:50, final hit at 0:58) and the sound effects, synthesized with numpy: no samples, no licences |
| `film.html` | The app section (0:48-0:58): the node list, a node's exchanges with other agents, the read-only thread between two agents, adding an agent |
| `render.mjs` | `stills`, `film` and `mix` |

Everything generated goes to `$WORK` (default `./work`, not committed). Node 22, Python 3 with numpy, ffmpeg, Playwright with Chromium:

```bash
npm i                                      # three, fonts, simple-icons (package.json)
sh fetch-logos.sh && node tex.cjs          # marks and window textures into $WORK/tex (plus $WORK/tex/logo.png: the myco mark as a PNG)
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

The agents' marks are their owners' trademarks, shown to name the products myco works with: Claude and Cursor from simple-icons 16.36.0, OpenAI from simple-icons 13.21.0, Grok from grok.com.
