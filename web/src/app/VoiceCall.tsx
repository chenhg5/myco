import { useEffect, useRef, useState } from "react";
import { Mic, MicOff, PhoneOff, Settings } from "lucide-react";
import { appApi, type VoiceLine, type VoiceState } from "@/appapi";
import { ErrorText } from "@/app/dialogs";
import { L, lang } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/** A voice call with the voice assistant (voice.go): it listens, hands work
 *  to the workspace's agents as you, and talks back; it stops when you
 *  speak. The circle breathes with the state; what was said is the
 *  transcript below; mute and hang up at the bottom. */
export function VoiceCall({ hubId, hubName, onClose, onSettings }: {
  hubId: string;
  hubName: string;
  onClose: () => void;
  /** opens Settings › Voice (the call cannot start without keys) */
  onSettings: () => void;
}) {
  const [lines, setLines] = useState<VoiceLine[]>([]);
  const [state, setState] = useState<VoiceState>("idle");
  const [muted, setMuted] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [needsSetup, setNeedsSetup] = useState(false);
  const [on, setOn] = useState(false);
  const startedAt = useRef(Date.now());
  const body = useRef<HTMLDivElement>(null);

  // Start the call, then follow its lines until it ends.
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
      return out.slice(-80);
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
          if (!r.on) {
            await new Promise((res) => setTimeout(res, 1000));
          }
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
        await appApi.voiceStart(hubId, lang() === "en" ? "en-US" : "zh-CN");
        if (off) return;
        setOn(true);
        setState("listening");
        void follow();
      } catch (e) {
        if (off) return;
        const code = (e as { code?: string }).code;
        if (code === "not_configured") setNeedsSetup(true);
        setError(e);
      }
    })();
    return () => {
      off = true;
      void appApi.voiceStop().catch(() => undefined);
    };
  }, [hubId]);

  // Follow the newest line.
  useEffect(() => {
    const el = body.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [lines]);

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
    idle: on ? L("正在接通…", "Connecting…") : L("正在接通…", "Connecting…"),
    listening: muted ? L("已静音", "Muted") : L("在听", "Listening"),
    thinking: L("在想", "Thinking"),
    speaking: L("在说", "Speaking"),
  };

  return (
    <div className="vc-overlay app-portal" role="dialog" aria-modal="true" aria-label={L("语音", "Voice")}
      onKeyDown={(e) => { if (e.key === "Escape") hangUp(); }}>
      <div className="vc-panel">
        <div className="vc-head">
          <span className="vc-where">{L(`和「${hubName}」的 agent 们`, `With the agents of ${hubName}`)}</span>
          <button type="button" className="app-btn app-btn-plain" onClick={onSettings} title={L("语音设置", "Voice settings")} aria-label={L("语音设置", "Voice settings")}>
            <Settings />
          </button>
        </div>
        <div className={cn("vc-orb", `is-${state}`, muted && "is-muted")} aria-hidden>
          <span className="vc-orb-ring" />
          <span className="vc-orb-core" />
        </div>
        <div className="vc-state" aria-live="polite">{stateText[state]}</div>

        <div className="vc-lines" ref={body}>
          {lines.length === 0 && !error && (
            <p className="vc-hint">{L("说吧，比如「让后端把登录的 bug 修了」「前端那边怎么样了」。它说话时你随时可以打断。",
              "Go ahead, e.g. “have the backend fix the login bug” or “how is the frontend doing?”. Cut in any time.")}</p>
          )}
          {lines.map((l) => l.kind === "action" ? (
            <div key={l.id} className="vc-action">{l.text}</div>
          ) : l.kind === "notice" ? (
            <div key={l.id} className="vc-notice">{l.text}</div>
          ) : (
            <div key={l.id} className={cn("vc-line", l.kind === "assistant" ? "is-them" : "is-me", l.kind === "hearing" && "is-hearing", l.interrupted && "is-cut")}>
              {l.text || (l.kind === "assistant" ? "…" : "")}
            </div>
          ))}
          <ErrorText error={error} />
          {needsSetup && (
            <button type="button" className="app-btn" onClick={onSettings}>{L("去填写语音设置", "Open voice settings")}</button>
          )}
        </div>

        <div className="vc-controls">
          <button type="button" className={cn("vc-btn", muted && "is-on")} onClick={() => void toggleMute()} disabled={!on}
            aria-label={muted ? L("取消静音", "Unmute") : L("静音", "Mute")} title={muted ? L("取消静音", "Unmute") : L("静音", "Mute")}>
            {muted ? <MicOff /> : <Mic />}
          </button>
          <button type="button" className="vc-btn vc-hangup" onClick={hangUp} aria-label={L("挂断", "Hang up")} title={L("挂断", "Hang up")}>
            <PhoneOff />
          </button>
        </div>
      </div>
    </div>
  );
}
