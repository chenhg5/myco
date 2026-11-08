// Chinese narration with Doubao TTS: script.json -> $1/NN.mp3, durations.json.
//   DOUBAO_TTS=/path/to/client.mjs node tts_zh.mjs <out dir>   (the client exports tts(text, file))
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
const out = process.argv[2];
const { tts } = await import(process.env.DOUBAO_TTS);
const { lines } = JSON.parse(readFileSync(join(import.meta.dirname, 'script.json'), 'utf8'));
mkdirSync(out, { recursive: true });
const durs = [];
for (const [i, l] of lines.entries()) {
  const f = join(out, `${String(i).padStart(2, '0')}.mp3`);
  if (!existsSync(f)) await tts(l.say_zh ?? l.zh, f);
  durs.push(+Number(execFileSync('ffprobe', ['-v', 'error', '-show_entries', 'format=duration', '-of', 'csv=p=0', f]).toString()).toFixed(3));
  console.log(i, durs[i], l.zh);
}
writeFileSync(join(out, 'durations.json'), JSON.stringify(durs));
