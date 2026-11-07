// Build the timeline from the narration clips, then render film.html frame by frame.
//   node render.mjs stills 3.2 10 25 ...   -> stills/t<sec>.png (quick checks)
//   node render.mjs film                   -> out/video.mp4 (silent), out/tl.json
//   node render.mjs mix                    -> out/myco-demo.mp4 (narration + music)
import { createRequire } from 'node:module';
const { chromium } = createRequire(import.meta.url)('playwright');
import { spawn, spawnSync, execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';

const DIR = import.meta.dirname;
const FPS = 30;
const LANG = process.env.LANG_FILM === 'zh' ? 'zh' : 'en';
const VO = LANG === 'zh' ? 'vo-zh' : 'vo';
const OUT = LANG === 'zh' ? 'out-zh' : 'out';
const clip = (i) => join(DIR, `${VO}/${String(i).padStart(2, '0')}.${LANG === 'zh' ? 'mp3' : 'wav'}`);
const lines = JSON.parse(readFileSync(join(DIR, LANG === 'zh' ? 'lines-zh.json' : 'lines.json'), 'utf8'));
const durs = JSON.parse(readFileSync(join(DIR, `${VO}/durations.json`), 'utf8'));
// per shot: silence before the first line, between lines, and after the last line (seconds)
const CFG = {
  scatter: { lead: 1.4, gap: 0.7, tail: 1.6, gaps: { 1: 1.0 } },
  connect: { lead: 0.9, gap: 0.6, tail: 2.6 },
  setup: { lead: 0.7, gap: 1.6, tail: 2.2 },
  team: { lead: 1.0, gap: 1.0, tail: 2.6, gaps: { 9: 0.6, 10: 1.6 } },
  dash: { lead: 0.8, gap: 0.6, tail: 1.8 },
  features: { lead: 0.7, gap: 0.9, tail: 1.6 },
  end: { lead: 1.0, gap: 0.6, tail: 3.2 },
};
function timeline() {
  const shots = {}; const out = []; let t = 0; let prev = null;
  lines.forEach((ln, i) => {
    const c = CFG[ln.shot];
    if (ln.shot !== prev) {
      if (prev) { t += CFG[prev].tail; shots[prev].end = t; }
      shots[ln.shot] = { start: t }; t += c.lead; prev = ln.shot;
    } else t += c.gaps?.[i] ?? c.gap;
    out.push({ ...ln, start: +t.toFixed(3), end: +(t + durs[i]).toFixed(3), dur: durs[i] });
    t += durs[i];
  });
  t += CFG[prev].tail; shots[prev].end = t;
  return { shots, lines: out, total: +t.toFixed(3) };
}
const TL = timeline();
mkdirSync(join(DIR, OUT), { recursive: true });
writeFileSync(join(DIR, OUT + '/tl.json'), JSON.stringify(TL, null, 1));

async function page() {
  const b = await chromium.launch();
  const p = await b.newPage({ viewport: { width: 1920, height: 1080 }, deviceScaleFactor: 1 });
  await p.addInitScript(([tl, lang]) => { window.TL = tl; window.LANG = lang; }, [TL, LANG]);
  p.on('pageerror', (e) => console.error('page error:', e.message));
  await p.goto('file://' + join(DIR, 'film.html'));
  await p.evaluate(() => window.ready);
  return { b, p };
}

const mode = process.argv[2];
if (mode === 'stills') {
  const { b, p } = await page(); mkdirSync(join(DIR, 'stills'), { recursive: true });
  for (const s of process.argv.slice(3)) { await p.evaluate((t) => window.seek(t), +s); await p.screenshot({ path: join(DIR, `stills/${LANG}-t${s}.png`) }); }
  await b.close(); console.log('total', TL.total, 's'); for (const [k, v] of Object.entries(TL.shots)) console.log(k, v.start.toFixed(1), v.end.toFixed(1));
} else if (mode === 'film') {
  const { b, p } = await page();
  const n = Math.ceil(TL.total * FPS);
  const ff = spawn('ffmpeg', ['-v', 'error', '-y', '-f', 'image2pipe', '-framerate', String(FPS), '-c:v', 'mjpeg', '-i', '-', '-c:v', 'libx264', '-preset', 'slow', '-crf', '18', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', join(DIR, OUT + '/video.mp4')], { stdio: ['pipe', 'inherit', 'inherit'] });
  const t0 = Date.now();
  for (let f = 0; f < n; f++) {
    await p.evaluate((t) => window.seek(t), f / FPS);
    const buf = await p.screenshot({ type: 'jpeg', quality: 94 });
    if (!ff.stdin.write(buf)) await new Promise((r) => ff.stdin.once('drain', r));
    if (f % 300 === 0) console.error(`frame ${f}/${n} (${((Date.now() - t0) / 1000).toFixed(0)}s)`);
  }
  ff.stdin.end(); await new Promise((r) => ff.on('close', r)); await b.close();
  console.log('video', n, 'frames', TL.total, 's');
} else if (mode === 'mix') {
  // narration: each clip starts at its line's start; light music under it, ducked by the voice
  const inputs = []; const fl = [];
  TL.lines.forEach((l, i) => { inputs.push('-i', clip(i)); const ms = Math.round(l.start * 1000); fl.push(`[${i}:a]aresample=48000,adelay=${ms}|${ms}[a${i}]`); });
  const k = TL.lines.length; const T = TL.total.toFixed(2);
  const vo = `${fl.join(';')};${TL.lines.map((_, i) => `[a${i}]`).join('')}amix=inputs=${k}:normalize=0,apad,atrim=0:${T}[vo]`;
  const music = `[${k}:a]aresample=48000,volume=-30dB,afade=t=in:d=2,afade=t=out:st=${(TL.total - 3.5).toFixed(2)}:d=3.5,atrim=0:${T}[m]`;
  const graph = `${vo};${music};[vo]asplit=2[v1][v2];[m][v2]sidechaincompress=threshold=0.03:ratio=4:attack=30:release=600[duck];[v1][duck]amix=inputs=2:normalize=0:duration=first[aout]`;
  execFileSync('ffmpeg', ['-v', 'error', '-y', ...inputs, '-stream_loop', '-1', '-i', join(DIR, OUT + '/bgm.wav'), '-filter_complex', graph, '-map', '[aout]', '-ac', '2', '-c:a', 'aac', '-b:a', '192k', join(DIR, OUT + '/audio.m4a')], { stdio: 'inherit' });
  // one fixed gain to about -16 LUFS plus a limiter (keeps the voice/music balance; loudnorm's dynamic mode lifts the music in the gaps)
  const r = spawnSync('ffmpeg', ['-hide_banner', '-i', join(DIR, OUT + '/audio.m4a'), '-af', 'ebur128', '-f', 'null', '-'], { encoding: 'utf8' });
  const sum = /Integrated loudness:\s+I:\s+(-?[\d.]+)/.exec(r.stderr)[1];
  const gain = (-16 - Number(sum)).toFixed(1); console.log('measured', sum, 'LUFS, gain', gain, 'dB');
  execFileSync('ffmpeg', ['-v', 'error', '-y', '-i', join(DIR, OUT + '/audio.m4a'), '-af', `volume=${gain}dB,alimiter=limit=0.89:level=false`, '-c:a', 'aac', '-b:a', '192k', join(DIR, OUT + '/audio2.m4a')], { stdio: 'inherit' });
  execFileSync('ffmpeg', ['-v', 'error', '-y', '-i', join(DIR, OUT + '/video.mp4'), '-i', join(DIR, OUT + '/audio2.m4a'), '-map', '0:v', '-map', '1:a', '-c:v', 'copy', '-c:a', 'copy', '-shortest', '-movflags', '+faststart', join(DIR, OUT + (LANG === 'zh' ? '/myco-demo-zh.mp4' : '/myco-demo.mp4'))], { stdio: 'inherit' });
  console.log('→', OUT + (LANG === 'zh' ? '/myco-demo-zh.mp4' : '/myco-demo.mp4'));
}
