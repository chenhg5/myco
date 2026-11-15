import { useEffect, useRef, useState } from "react";
import { AudioLines, Mic, MicOff, PhoneOff } from "lucide-react";
import { appApi, type VoiceLine, type VoiceState } from "@/appapi";
import { ErrorText } from "@/app/dialogs";
import { L, lang } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * Talking with an agent out loud, in your chat with it (voice.go), as voice
 * is in ChatGPT and Claude: with nothing typed, the box's round button
 * starts it; the call is a bar docked above the box. What you ask for goes
 * to the agent as if typed (a new task, or an addition to the one at work)
 * and shows in the chat marked as said by voice; the agent's answers are
 * in the chat as always. The bar keeps only the talk itself: what you said
 * and what the voice said back, the last few lines.
 */

/** Opens Settings › Voice (AppMode listens). */
export function openVoiceSettings() {
  window.dispatchEvent(new CustomEvent("myco-open-settings", { detail: { section: "voice" } }));
}

let known: { available: boolean; ready: boolean } | null = null;

/** Whether this App can call (the Mac app) and has its keys. */
export function useVoiceReady(): { available: boolean; ready: boolean } | null {
  const [v, setV] = useState(known);
  useEffect(() => {
    let off = false;
    appApi.voice().then((r) => {
      known = { available: r.available, ready: r.has_llm_key && r.has_tts_key };
      if (!off) setV(known);
    }, () => undefined);
    return () => { off = true; };
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

/** A call with one agent, docked in its chat; unmounting hangs up. */
export function VoiceBar({ hubId, agent, peer, agentName, onClose }: {
  hubId: string;
  agent: string;
  peer?: string;
  agentName: string;
  onClose: () => void;
}) {
  const [lines, setLines] = useState<VoiceLine[]>([]);
  const [state, setState] = useState<VoiceState>("idle");
  const [muted, setMuted] = useState(false);
  const [on, setOn] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [needsSetup, setNeedsSetup] = useState(false);
  const startedAt = useRef(Date.now());

  useEffect(() => {
    let off = false;
    let after = 0;
    const merge = (evs: VoiceLine[]) => setLines((prev) => {
      const byId = new Map(prev.map((l) => [l.id, l]));
      for (const e of evs) {
        if (new Date(e.at).getTime() < startedAt.current - 2000) continue; // earlier calls
        byId.set(e.id, e);
      }
      let out = [...byId.values()].sort((a, b) => a.id - b.id);
      // A partial hearing gives way to what followed it.
      out = out.filter((l, i) => l.kind !== "hearing" || i === out.length - 1);
      return out.slice(-40);
    });
    const follow = async () => {
      while (!off) {
        try {
          const r = await appApi.voiceEvents(after);
          if (off) return;
          after = r.seq;
          setState(r.state);
          setOn(r.on);
          if (typeof r.muted === "boolean") setMuted(r.muted);
          merge(r.events);
          if (!r.on) await new Promise((res) => setTimeout(res, 1000));
        } catch (e) {
          if (off) return;
          setError(e);
          await new Promise((res) => setTimeout(res, 2000));
        }
      }
    };
    (async () => {
      try {
        const first = await appApi.voiceEvents(-1);
        after = first.seq;
        await appApi.voiceStart(hubId, agent, peer, lang() === "en" ? "en-US" : "zh-CN");
        if (off) return;
        setOn(true);
        setState("listening");
        void follow();
      } catch (e) {
        if (off) return;
        if ((e as { code?: string }).code === "not_configured") setNeedsSetup(true);
        setError(e);
      }
    })();
    return () => {
      off = true;
      void appApi.voiceStop().catch(() => undefined);
    };
  }, [hubId, agent, peer]);

  const hangUp = () => {
    void appApi.voiceStop().catch(() => undefined);
    onClose();
  };
  const toggleMute = async () => {
    try {
      const v = await appApi.voiceMute(!muted);
      setMuted(v.session.muted);
    } catch (e) { setError(e); }
  };

  const stateText: Record<VoiceState, string> = {
    idle: L("正在接通…", "Connecting…"),
    listening: muted ? L("已静音", "Muted") : L("在听", "Listening"),
    thinking: L("在想", "Thinking"),
    speaking: L("在说", "Speaking"),
  };
  // The talk, last few lines: what you said, what it said, what it did.
  const shown = lines.filter((l) => l.kind !== "notice" || l === lines[lines.length - 1]).slice(-3);

  return (
    <div className="vc-bar" role="region" aria-label={L(`和 ${agentName} 语音聊天`, `Voice chat with ${agentName}`)}
      onKeyDown={(e) => { if (e.key === "Escape") hangUp(); }}>
      <div className="vc-bar-head">
        <span className={cn("vc-dot", `is-${on ? state : "idle"}`, muted && "is-muted")} aria-hidden />
        <span className="vc-bar-state" aria-live="polite">{stateText[on ? state : "idle"]}</span>
        <span className="vc-bar-who">{L(`和 ${agentName} 语音聊天`, `Voice chat with ${agentName}`)}</span>
        <button type="button" className={cn("vc-bar-btn", muted && "is-on")} onClick={() => void toggleMute()} disabled={!on}
          aria-label={muted ? L("取消静音", "Unmute") : L("静音", "Mute")} title={muted ? L("取消静音", "Unmute") : L("静音", "Mute")}>
          {muted ? <MicOff /> : <Mic />}
        </button>
        <button type="button" className="vc-bar-btn vc-hangup" onClick={hangUp} aria-label={L("挂断", "Hang up")} title={L("挂断", "Hang up")}>
          <PhoneOff />
        </button>
      </div>
      <div className="vc-bar-lines">
        {shown.length === 0 && !error && (
          <p className="vc-hint">{L(`说吧，比如「把登录那个 bug 修了」「做到哪了」。它说话时你随时可以打断。`,
            "Go ahead, e.g. “fix the login bug” or “how far along is it?”. Cut in any time.")}</p>
        )}
        {shown.map((l) => l.kind === "action" ? (
          <div key={l.id} className="vc-action">{l.text}</div>
        ) : l.kind === "notice" ? (
          <div key={l.id} className="vc-notice">{l.text}</div>
        ) : (
          <div key={l.id} className={cn("vc-line", l.kind === "assistant" ? "is-them" : "is-me", l.kind === "hearing" && "is-hearing", l.interrupted && "is-cut")}>
            <span className="vc-line-who">{l.kind === "assistant" ? L("语音", "Voice") : L("你", "You")}</span>
            {l.text || "…"}
          </div>
        ))}
        <ErrorText error={error} />
        {needsSetup && (
          <button type="button" className="app-btn" onClick={openVoiceSettings}>{L("去填写语音设置", "Open voice settings")}</button>
        )}
      </div>
    </div>
  );
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
