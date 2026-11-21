import { t as uiText } from "../_lib/i18n";
// Speech to text for captions and for a reference video's lines: on this
// computer, with the App's transcriber (myco.os speech.*, as 会议记录
// uses). The sound is written to a temporary 16 kHz WAV first.
import { join, os, osState, toBase64, toOSError } from "../_lib/os";
import type { OSSegment, OSSpeechStatus } from "../_lib/myco";

export type SpeechState = { can: boolean; ready: boolean; why: string; sizeMb: number };

/** Whether this computer can turn speech into text now, or after a download. */
export async function speechState(): Promise<SpeechState> {
  if (osState !== "ready") return { can: false, ready: false, why: osState === "outside" ? uiText("在 myco App 里打开才能识别语音") : uiText("更新 myco App 后才能识别语音"), sizeMb: 0 };
  let st: OSSpeechStatus;
  try { st = await os().speech.status(); } catch (e) { return { can: false, ready: false, why: toOSError(e).message, sizeMb: 0 }; }
  const size = (st.engine ? 0 : st.engine_size_mb) + st.models.filter((m) => !m.installed).reduce((a, m) => a + m.size_mb, 0);
  return { can: true, ready: st.ready, why: "", sizeMb: Math.round(size) };
}

/** Downloads the transcriber (the first time); progress 0…1. */
export async function installSpeech(onProgress: (p: number) => void) {
  const job = os().speech.install({});
  job.on("progress", (p: { received?: number; total?: number }) => { if (p.total) onProgress((p.received || 0) / p.total); });
  try { await job.done; } catch (e) { throw toOSError(e); }
}

/** Sentences of a WAV (bytes), times from its start. */
export async function transcribe(wav: Uint8Array, onProgress: (p: number) => void, signal?: AbortSignal): Promise<OSSegment[]> {
  const o = os();
  const temp = (await o.fs.paths()).temp;
  const path = join(temp, `myco-motion-${Date.now().toString(36)}.wav`);
  try {
    // (in pieces: one call carries at most a few MB)
    const step = 768 * 1024;
    for (let a = 0; a < wav.length; a += step) {
      if (signal?.aborted) throw new Error("cancelled");
      await o.fs.write({ path, content: toBase64(wav.subarray(a, a + step)), encoding: "base64", append: a > 0 });
    }
    const job = o.speech.transcribe({ path });
    signal?.addEventListener("abort", () => void job.cancel(), { once: true });
    job.on("progress", (p: { done?: number; total?: number }) => { if (p.total) onProgress((p.done || 0) / p.total); });
    try { return (await job.done).segments; } catch (e) { throw toOSError(e); }
  } finally {
    void o.fs.delete({ path }).catch(() => undefined);
  }
}
