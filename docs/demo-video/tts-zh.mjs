// Chinese narration with Doubao TTS. The client module is passed in DOUBAO_TTS.
//   DOUBAO_TTS=/path/to/tts.mjs node tts-zh.mjs   -> vo-zh/NN.mp3, vo-zh/durations.json
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
const DIR = import.meta.dirname;
// DOUBAO_TTS: path to a module exporting tts(text, outFile) that calls Doubao TTS and writes an mp3
if (!process.env.DOUBAO_TTS) { console.error('set DOUBAO_TTS to your Doubao TTS client module'); process.exit(2); }
const { tts } = await import(process.env.DOUBAO_TTS);
const lines = JSON.parse(readFileSync(join(DIR, 'lines-zh.json'), 'utf8'));
mkdirSync(join(DIR, 'vo-zh'), { recursive: true });
const durs = [];
for (const [i, l] of lines.entries()) {
  const f = join(DIR, `vo-zh/${String(i).padStart(2, '0')}.mp3`);
  if (!existsSync(f)) await tts(l.say ?? l.text, f);
  durs.push(+Number(execFileSync('ffprobe', ['-v', 'error', '-show_entries', 'format=duration', '-of', 'csv=p=0', f]).toString()).toFixed(3));
  console.log(i, durs[i], l.text);
}
writeFileSync(join(DIR, 'vo-zh/durations.json'), JSON.stringify(durs));
