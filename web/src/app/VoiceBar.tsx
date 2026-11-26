import { useEffect, useState, useSyncExternalStore } from "react";
import { AudioLines, Mic, MicOff, PhoneOff } from "lucide-react";
import { appApi, type VoiceLine, type VoiceState } from "@/appapi";
import { ErrorText } from "@/app/dialogs";
import { L, lang } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import type { Agent } from "@/api";
import { Avatar } from "@/app/persona";

/*
 * Talking with an agent out loud, in your chat with it (voice.go), as voice
 * is in ChatGPT and Claude: with nothing typed, the box's round button
 * starts it; the call is a bar docked above the box. What you ask for goes
 * to the agent as if typed (a new task, or an addition to the one at work)
 * and shows in the chat marked as said by voice; the agent's answers are
 * in the chat as always. The bar keeps only the talk itself: what you said
 * and what the voice said back, the last few lines. The call goes on when
 * you go elsewhere in the App (VoiceChip).
 */

/** Opens Settings › Voice (AppMode listens). */
export function openVoiceSettings() {
  window.dispatchEvent(new CustomEvent("myco-open-settings", { detail: { section: "voice" } }));
}

let known: { available: boolean; ready: boolean } | null = null;

/** Whether this App can call (the Mac app) and voice is set up: the way in
 *  shows only then (Settings › 语音 is where it is set up). */
export function useVoiceReady(): { available: boolean; ready: boolean } | null {
  const [v, setV] = useState(known);
  useEffect(() => {
    let off = false;
    const load = () => appApi.voice().then((r) => {
      known = { available: r.available, ready: r.ready ?? (r.available && r.has_llm_key && r.has_tts_key && !!r.config.llm_base_url && !!r.config.llm_model) };
      if (!off) setV(known);
    }, () => undefined);
    void load();
    // Set up (or taken away) in Settings › 语音: the button follows.
    window.addEventListener("myco-voice-changed", load);
    return () => { off = true; window.removeEventListener("myco-voice-changed", load); };
  }, []);
  return v;
}

/** A call asked for from elsewhere (the project box): the chat with that
 *  agent starts it when it opens. */
let pendingCall: { hubId: string; agent: string; at: number } | null = null;
export function callWhenOpened(hubId: string, agent: string) { pendingCall = { hubId, agent, at: Date.now() }; }
export function takePendingCall(hubId: string, agent: string): boolean {
  const p = pendingCall;
  if (!p || p.hubId !== hubId || p.agent !== agent || Date.now() - p.at > 10_000) return false;
  pendingCall = null;
  return true;
}

/*
 * The call itself lives here, not in a chat: it goes on while you look at
 * another chat or page (VoiceChip, in the top bar, is it there), and ends
 * only when hung up. There is one at a time (the Mac app has one
 * microphone and one voice session): calling someone else hangs up the
 * call before. Starts and stops go to the backend one after another.
 */
export type VoiceCall = { key: number; hubId: string; agent: string; peer?: string; agentName: string; startedAt: number };
type Live = { lines: VoiceLine[]; state: VoiceState; muted: boolean; on: boolean; error: unknown; needsSetup: boolean };
const quiet: Live = { lines: [], state: "idle", muted: false, on: false, error: null, needsSetup: false };
let call: VoiceCall | null = null;
let live: Live = quiet;
let barsShown = 0; // chats showing the call's bar now
let snap: { call: VoiceCall | null; live: Live; barsShown: number } = { call, live, barsShown };
const subs = new Set<() => void>();
const emit = () => { snap = { call, live, barsShown }; subs.forEach((f) => f()); };
let ops: Promise<unknown> = Promise.resolve();
const inTurn = <T,>(f: () => Promise<T>): Promise<T> => {
  const p = ops.then(f, f);
  ops = p.catch(() => undefined);
  return p;
};

/** The call now (if any), how it is going, and whether a chat shows it. */
export function useVoiceCall() {
  return useSyncExternalStore((f) => { subs.add(f); return () => { subs.delete(f); }; }, () => snap);
}

/** Calls this agent; a call with anyone else is hung up first. */
export function startCall(c: { hubId: string; agent: string; peer?: string; agentName: string }) {
  if (call && call.hubId === c.hubId && call.agent === c.agent && call.peer === c.peer) return;
  if (call) void inTurn(() => appApi.voiceStop()).catch(() => undefined);
  call = { ...c, key: (call?.key || 0) + 1, startedAt: Date.now() };
  live = quiet;
  emit();
  void follow(call);
}

/** Hangs up. */
export function endCall() {
  if (!call) return;
  call = null;
  live = quiet;
  emit();
  void inTurn(() => appApi.voiceStop()).catch(() => undefined);
}

export function setCallMuted(muted: boolean) {
  const c = call;
  appApi.voiceMute(muted).then((v) => { if (call === c) { live = { ...live, muted: v.session.muted }; emit(); } },
    (e) => { if (call === c) { live = { ...live, error: e }; emit(); } });
}

async function follow(c: VoiceCall) {
  const mine = () => call === c;
  const set = (patch: Partial<Live> | ((l: Live) => Partial<Live>)) => {
    if (!mine()) return;
    live = { ...live, ...(typeof patch === "function" ? patch(live) : patch) };
    emit();
  };
  const merge = (evs: VoiceLine[]) => set((l) => {
    const byId = new Map(l.lines.map((x) => [x.id, x]));
    for (const e of evs) {
      if (new Date(e.at).getTime() < c.startedAt - 2000) continue; // earlier calls
      byId.set(e.id, e);
    }
    let out = [...byId.values()].sort((a, b) => a.id - b.id);
    // A partial hearing gives way to what followed it.
    out = out.filter((x, i) => x.kind !== "hearing" || i === out.length - 1);
    return { lines: out.slice(-40) };
  });
  let after = 0;
  try {
    const first = await appApi.voiceEvents(-1);
    after = first.seq;
    await inTurn(async () => { if (mine()) await appApi.voiceStart(c.hubId, c.agent, c.peer, lang() === "en" ? "en-US" : "zh-CN"); });
    if (!mine()) return;
    set({ on: true, state: "listening" });
  } catch (e) {
    set({ error: e, needsSetup: (e as { code?: string }).code === "not_configured" });
    return;
  }
  while (mine()) {
    try {
      const r = await appApi.voiceEvents(after);
      if (!mine()) return;
      after = r.seq;
      set({ state: r.state, on: r.on, ...(typeof r.muted === "boolean" ? { muted: r.muted } : {}) });
      merge(r.events);
      if (!r.on) await new Promise((res) => setTimeout(res, 1000));
    } catch (e) {
      if (!mine()) return;
      set({ error: e });
      await new Promise((res) => setTimeout(res, 2000));
    }
  }
}

function stateText(l: Live): string {
  if (!l.on) return L("正在接通…", "Connecting…");
  return { idle: L("正在接通…", "Connecting…"), listening: l.muted ? L("已静音", "Muted") : L("在听", "Listening"), thinking: L("在想", "Thinking"), speaking: L("在说", "Speaking") }[l.state];
}

/** The call, docked in the chat with whom it is with (the chat says when
 *  it is: hubId and agent match). The voice is that agent: its lines carry
 *  its face, not a "Voice" label. */
export function VoiceBar({ agent }: { agent?: Agent }) {
  const { call: c, live: l } = useVoiceCall();
  // Lines whose corrected names were undone, shown as heard.
  const [heardAsIs, setHeardAsIs] = useState<Set<number>>(() => new Set());
  useEffect(() => {
    barsShown++;
    emit();
    return () => { barsShown--; emit(); };
  }, []);
  if (!c) return null;
  // The talk, last few lines: what you said, what it said, what it did.
  const shown = l.lines.filter((x) => x.kind !== "notice" || x === l.lines[l.lines.length - 1]).slice(-3);
  return (
    <div className="vc-bar" role="region" aria-label={L(`和 ${c.agentName} 语音聊天`, `Voice chat with ${c.agentName}`)}
      onKeyDown={(e) => { if (e.key === "Escape") endCall(); }}>
      <div className="vc-bar-head">
        <span className={cn("vc-dot", `is-${l.on ? l.state : "idle"}`, l.muted && "is-muted")} aria-hidden />
        <span className="vc-bar-state" aria-live="polite">{stateText(l)}</span>
        <span className="vc-bar-who">{L(`和 ${c.agentName} 语音聊天`, `Voice chat with ${c.agentName}`)}</span>
        <CallButtons l={l} />
      </div>
      <div className="vc-bar-lines">
        {shown.length === 0 && !l.error && (
          <p className="vc-hint">{L(`说吧，比如「把登录那个 bug 修了」「做到哪了」。它说话时你随时可以打断。切到别的页面也不会挂断。`,
            "Go ahead, e.g. “fix the login bug” or “how far along is it?”. Cut in any time. Going to another page does not hang up.")}</p>
        )}
        {shown.map((x) => x.kind === "action" ? (
          <div key={x.id} className="vc-action">{x.text}</div>
        ) : x.kind === "notice" ? (
          <div key={x.id} className="vc-notice" title={x.text}>{shortNotice(x.text)}</div>
        ) : (
          <div key={x.id} className={cn("vc-line", x.kind === "assistant" ? "is-them" : "is-me", x.kind === "hearing" && "is-hearing", x.interrupted && "is-cut")}>
            {x.kind === "assistant" && agent ? (
              <span className="vc-line-who is-face" title={c.agentName}>
                <Avatar agent={agent} size={16} badge={false} />
                <span className="app-sr-only">{c.agentName}</span>
              </span>
            ) : (
              <span className="vc-line-who">{x.kind === "assistant" ? c.agentName : L("你", "You")}</span>
            )}
            <span>{x.corrected && heardAsIs.has(x.id) ? x.corrected : x.text || "…"}
            {x.corrected && (
              <button type="button" className={cn("vc-fixed", heardAsIs.has(x.id) && "is-off")}
                title={heardAsIs.has(x.id) ? L("显示纠正后的", "Show the corrected line") : L(`听到的是：${x.corrected}`, `Heard as: ${x.corrected}`)}
                onClick={() => setHeardAsIs((s) => { const n = new Set(s); if (n.has(x.id)) n.delete(x.id); else n.add(x.id); return n; })}>
                {heardAsIs.has(x.id) ? L("原话", "As heard") : L("已纠正", "Corrected")}
              </button>
            )}</span>
          </div>
        ))}
        <ErrorText error={l.error} />
        {l.needsSetup && (
          <button type="button" className="app-btn" onClick={openVoiceSettings}>{L("去填写语音设置", "Open voice settings")}</button>
        )}
      </div>
    </div>
  );
}

function CallButtons({ l }: { l: Live }) {
  return (
    <>
      <button type="button" className={cn("vc-bar-btn", l.muted && "is-on")} onClick={(e) => { e.stopPropagation(); setCallMuted(!l.muted); }} disabled={!l.on}
        aria-label={l.muted ? L("取消静音", "Unmute") : L("静音", "Mute")} title={l.muted ? L("取消静音", "Unmute") : L("静音", "Mute")}>
        {l.muted ? <MicOff /> : <Mic />}
      </button>
      <button type="button" className="vc-bar-btn vc-hangup" onClick={(e) => { e.stopPropagation(); endCall(); }} aria-label={L("挂断", "Hang up")} title={L("挂断", "Hang up")}>
        <PhoneOff />
      </button>
    </>
  );
}

/** The call, in the top bar, while no chat shows it: how it goes, mute and
 *  hang up, and back to its chat (AppMode listens for myco-voice-open). */
export function VoiceChip() {
  const { call: c, live: l, barsShown: shown } = useVoiceCall();
  if (!c || shown > 0) return null;
  const back = L(`回到和 ${c.agentName} 的对话`, `Back to the chat with ${c.agentName}`);
  return (
    <div className="vc-chip" role="region" aria-label={L(`和 ${c.agentName} 语音聊天`, `Voice chat with ${c.agentName}`)}>
      <button type="button" className="vc-chip-main" title={back} aria-label={back}
        onClick={() => window.dispatchEvent(new CustomEvent("myco-voice-open", { detail: { hubId: c.hubId, agent: c.agent } }))}>
        <span className={cn("vc-dot", `is-${l.on ? l.state : "idle"}`, l.muted && "is-muted")} aria-hidden />
        <span className="vc-chip-who">{c.agentName}</span>
        <span className="vc-chip-state" aria-live="polite">{l.error ? L("出错了", "Trouble") : stateText(l)}</span>
      </button>
      <CallButtons l={l} />
    </div>
  );
}

/** A notice in words: a service's raw answer ("model HTTP 400: {json}")
 *  shortened to its message; all of it on hover. */
export function shortNotice(text: string): string {
  const m = /^([\s\S]*?)\{[\s\S]*\}\s*$/.exec(text);
  if (!m) return text;
  let said = "";
  try {
    const j = JSON.parse(text.slice(m[1].length)) as { message?: string; error?: { message?: string } | string };
    said = (typeof j.error === "object" ? j.error?.message : typeof j.error === "string" ? j.error : "") || j.message || "";
  } catch { /* not JSON after all */ }
  const head = m[1].replace(/[:：]\s*$/, "");
  return said ? `${head}：${said.length > 140 ? said.slice(0, 140) + "…" : said}` : head;
}

/** What the voice passed on ends with "——\n（经语音转述。原话：…）"
 *  (voice_hub.go): the chat shows the text with a small mark instead,
 *  the words as heard on hover. */
export function viaVoice(body: string): { text: string; heard: string } | null {
  const m = /\n*——\n（经语音转述(?:。原话：([\s\S]*))?）\s*$/.exec(body);
  if (!m) return null;
  return { text: body.slice(0, m.index).trimEnd(), heard: (m[1] || "").trim() };
}

export function VoiceMark({ heard }: { heard: string }) {
  return (
    <span className="conv-tag neutral conv-voice-tag" title={heard ? L(`经语音转述。原话：${heard}`, `Passed on by voice. As said: ${heard}`) : L("经语音转述", "Passed on by voice")}>
      <AudioLines size={11} />{L("经语音", "By voice")}
    </span>
  );
}
