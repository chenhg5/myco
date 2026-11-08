// Render the promo film.
//   LANG_FILM=en|zh WORK=/path/to/work node render.mjs stills 3 15 27 ...  -> $WORK/stills-<lang>/t<sec>.png
//   ... node render.mjs film                                               -> $WORK/out-<lang>/video.mp4 (silent)
//   ... node render.mjs mix                                                -> $WORK/out-<lang>/myco-promo-<lang>.mp4
// $WORK holds: assets/<lang>/*.png (capture.cjs), vo-<lang>/ (tts_en.py / tts_zh.mjs),
// audio/bgm.wav and audio/sfx/*.wav (music.py).
import { createRequire } from 'node:module';
const { chromium } = createRequire(import.meta.url)('playwright');
import { spawn, execFileSync } from 'node:child_process';
import { readFileSync, mkdirSync, existsSync } from 'node:fs';
import { join } from 'node:path';

const DIR = import.meta.dirname;
const WORK = process.env.WORK || join(DIR, 'work');
const LANG = process.env.LANG_FILM === 'zh' ? 'zh' : 'en';
const OTHER = LANG === 'zh' ? 'en' : 'zh';
const FPS = Number(process.env.FPS || 30);
const TOTAL = 66;
const OUT = join(WORK, `out-${LANG}`);
mkdirSync(OUT, { recursive: true });

const script = JSON.parse(readFileSync(join(DIR, 'script.json'), 'utf8'));
const VO = join(WORK, `vo-${LANG}`);
const durs = JSON.parse(readFileSync(join(VO, 'durations.json'), 'utf8'));
const ext = LANG === 'zh' ? 'mp3' : 'wav';
// Each line starts at `at`; a clip longer than its slot (until the next line) is sped up, at most 1.25x.
const LINES = script.lines.map((l, i) => {
  const next = script.lines[i + 1]?.at ?? TOTAL - 1;
  const slot = next - l.at - 0.25;
  const tempo = Math.min(1.25, Math.max(1, durs[i] / slot));
  if (tempo > 1.001) console.error(`line ${i} sped up ${tempo.toFixed(2)}x`);
  return { at: l.at, dur: +(durs[i] / tempo).toFixed(3), tempo, text: l[LANG], file: join(VO, `${String(i).padStart(2, '0')}.${ext}`) };
});

async function page() {
  const b = await chromium.launch();
  const p = await b.newPage({ viewport: { width: 1920, height: 1080 }, deviceScaleFactor: 1 });
  await p.addInitScript(([lang, assets, other, lines]) => { window.LANG = lang; window.ASSETS = assets; window.ASSETS_OTHER = other; window.LINES = lines; },
    [LANG, 'file://' + join(WORK, 'assets', LANG), 'file://' + join(WORK, 'assets', OTHER), LINES]);
  p.on('pageerror', (e) => console.error('page error:', e.message));
  p.on('console', (m) => { if (m.type() === 'error') console.error('console:', m.text()); });
  await p.goto('file://' + join(DIR, 'film.html'));
  await p.evaluate(() => window.ready);
  return { b, p };
}

const mode = process.argv[2];
if (mode === 'stills') {
  const { b, p } = await page();
  const dir = join(WORK, `stills-${LANG}`); mkdirSync(dir, { recursive: true });
  for (const s of process.argv.slice(3)) {
    await p.evaluate((t) => window.seek(t), +s); await p.waitForTimeout(50);
    await p.screenshot({ path: join(dir, `t${s}.png`) });
  }
  await b.close();
} else if (mode === 'film') {
  const { b, p } = await page();
  const n = Math.ceil(TOTAL * FPS);
  const ff = spawn('ffmpeg', ['-v', 'error', '-y', '-f', 'image2pipe', '-framerate', String(FPS), '-c:v', 'mjpeg', '-i', '-',
    '-c:v', 'libx264', '-preset', 'slow', '-crf', '18', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', join(OUT, 'video.mp4')], { stdio: ['pipe', 'inherit', 'inherit'] });
  const t0 = Date.now();
  for (let f = 0; f < n; f++) {
    await p.evaluate((t) => window.seek(t), f / FPS);
    const buf = await p.screenshot({ type: 'jpeg', quality: 93 });
    if (!ff.stdin.write(buf)) await new Promise((r) => ff.stdin.once('drain', r));
    if (f % 300 === 0) console.error(`frame ${f}/${n} (${((Date.now() - t0) / 1000).toFixed(0)}s)`);
  }
  ff.stdin.end(); await new Promise((r) => ff.on('close', r)); await b.close();
  console.log('video', n, 'frames');
} else if (mode === 'mix') {
  const { b, p } = await page();
  const cues = await p.evaluate(() => window.CUES);
  await b.close();
  const SFX = join(WORK, 'audio', 'sfx');
  const inputs = ['-i', join(OUT, 'video.mp4'), '-i', join(WORK, 'audio', 'bgm.wav')];
  const fl = [];
  let k = 2;
  const vo = [];
  for (const l of LINES) {
    inputs.push('-i', l.file);
    const ms = Math.round(l.at * 1000);
    fl.push(`[${k}:a]aresample=48000,aformat=channel_layouts=stereo${l.tempo > 1.001 ? `,atempo=${l.tempo.toFixed(3)}` : ''},adelay=${ms}|${ms}[v${k}]`);
    vo.push(`[v${k}]`); k++;
  }
  const fx = [];
  for (const [at, name, gain] of cues) {
    const f = join(SFX, `${name}.wav`); if (!existsSync(f)) { console.error('missing sfx', name); continue; }
    inputs.push('-i', f);
    const ms = Math.round(at * 1000);
    fl.push(`[${k}:a]aresample=48000,volume=${gain},adelay=${ms}|${ms}[f${k}]`);
    fx.push(`[f${k}]`); k++;
  }
  const T = TOTAL.toFixed(2);
  fl.push(`${vo.join('')}amix=inputs=${vo.length}:normalize=0,apad,atrim=0:${T},volume=1.6[vo]`);
  fl.push(`[vo]asplit=2[vo1][vokey]`);
  fl.push(`${fx.join('')}amix=inputs=${fx.length}:normalize=0,apad,atrim=0:${T},volume=0.9[fx]`);
  // music ducks under the voice
  fl.push(`[1:a]aresample=48000,atrim=0:${T},volume=0.8[bgm]`);
  fl.push(`[bgm][vokey]sidechaincompress=threshold=0.04:ratio=6:attack=15:release=350:makeup=1[duck]`);
  fl.push(`[duck][vo1][fx]amix=inputs=3:normalize=0,loudnorm=I=-15:TP=-1.2:LRA=9,afade=t=out:st=${TOTAL - 1.2}:d=1.2[a]`);
  const out = join(OUT, `myco-promo-${LANG}.mp4`);
  execFileSync('ffmpeg', ['-v', 'error', '-y', ...inputs, '-filter_complex', fl.join(';'), '-map', '0:v', '-map', '[a]',
    '-c:v', 'copy', '-c:a', 'aac', '-b:a', '192k', '-ar', '48000', '-shortest', '-movflags', '+faststart', out], { stdio: 'inherit' });
  console.log('wrote', out);
}
