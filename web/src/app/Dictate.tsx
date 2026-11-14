import { useEffect, useRef, useState } from "react";
import { Mic } from "lucide-react";
import { appApi } from "@/appapi";
import { L, lang } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * Speaking instead of typing, in the language chosen for the app (not the
 * system's), with what the device has:
 *   - the Mac app listens itself (Apple's speech recognition, on this Mac
 *     when it can) and the words grow into the box as you speak;
 *   - the Windows app opens the system's voice typing, switched to that
 *     language, which types into the box itself;
 *   - a browser with speech recognition listens itself, as the Mac app.
 * The microphone sits beside Send, as in Codex and Claude. While it
 * listens it is red and breathes; click again to stop. A pause does not
 * stop it (the recognizer starts its next sentence); a long silence does. Typing, or sending,
 * stops it too (nothing late is written into a box you emptied). Where
 * there is none of these, there is no button.
 */

/** Whether the desktop app can do it (from /app/info). */
let appDictation = false;
export function setSystemDictation(on: boolean) { appDictation = on; }

/** The app's language as a speech tag. */
const speechLang = () => (lang() === "zh" ? "zh-CN" : "en-US");

type Recognition = {
  lang: string; continuous: boolean; interimResults: boolean;
  start: () => void; stop: () => void; abort: () => void;
  onresult: ((e: { resultIndex: number; results: ArrayLike<{ isFinal: boolean; 0: { transcript: string } }> }) => void) | null;
  onend: (() => void) | null;
  onerror: ((e: { error: string }) => void) | null;
};

function browserRecognition(): (new () => Recognition) | null {
  const w = window as unknown as { SpeechRecognition?: new () => Recognition; webkitSpeechRecognition?: new () => Recognition };
  return w.SpeechRecognition || w.webkitSpeechRecognition || null;
}

/** What a reason the app gives means, in words. */
function reasonOf(code: string): string {
  switch (code) {
    case "speech-denied": return L("没有语音识别权限：在 系统设置 › 隐私与安全性 › 语音识别 里打开 myco", "No speech recognition access: allow myco in System Settings › Privacy & Security › Speech Recognition");
    case "mic-denied":
    case "not-allowed":
    case "service-not-allowed": return L("没有麦克风权限：在 系统设置 › 隐私与安全性 › 麦克风 里打开 myco", "No microphone access: allow myco in System Settings › Privacy & Security › Microphone");
    case "unavailable": return L("这台电脑现在没法识别这个语言", "This computer cannot recognise this language right now");
    case "microphone": return L("麦克风打不开", "Could not open the microphone");
    default: return code || L("没听清，再试一次", "Didn't catch that; try again");
  }
}

function focusEnd(el: HTMLTextAreaElement | null) {
  if (!el) return;
  el.focus();
  el.selectionStart = el.selectionEnd = el.value.length;
}

export function DictateButton({ input, value, onText, disabled }: {
  input: React.RefObject<HTMLTextAreaElement | null>;
  /** what is in the box now */
  value: string;
  /** the box's new text as words are heard */
  onText: (text: string) => void;
  disabled?: boolean;
}) {
  const [listening, setListening] = useState(false);
  const [note, setNote] = useState("");
  // The current listening: how to stop it, and the text it last wrote (any
  // other change means you took over).
  // (the last few texts it wrote: two can be written before the box shows
  // the first, and that is not you typing)
  const live = useRef<{ stop: () => void; wrote: string[] } | null>(null);
  const Recog = browserRecognition();

  useEffect(() => () => live.current?.stop(), []);
  // You typed or sent: stop listening, and write nothing more.
  useEffect(() => {
    if (live.current && !live.current.wrote.includes(value)) {
      live.current.stop();
      live.current = null;
      setListening(false);
    }
  }, [value]);
  if (!appDictation && !Recog) return null;

  const begin = () => {
    const base = value && !/\s$/.test(value) ? `${value}${lang() === "zh" ? "" : " "}` : value;
    const write = (heard: string) => {
      if (!live.current) return;
      live.current.wrote = [...live.current.wrote.slice(-7), base + heard];
      onText(base + heard);
    };
    return { base, write };
  };

  const start = async () => {
    setNote("");
    focusEnd(input.current);
    if (listening) { live.current?.stop(); return; }
    if (appDictation) {
      try {
        const r = await appApi.startDictation(speechLang());
        if (r.mode !== "listen" || !r.id) return; // the system types it in itself
        const { write } = begin();
        let ended = false;
        live.current = { wrote: [value], stop: () => { if (!ended) void appApi.stopDictation(r.id!).catch(() => undefined); } };
        setListening(true);
        let seq = 0;
        for (;;) {
          const st = await appApi.dictationState(r.id, seq);
          seq = st.seq;
          write(st.text);
          if (st.done) {
            ended = true;
            if (st.error) setNote(reasonOf(st.error));
            break;
          }
        }
      } catch (e) {
        setNote((e as Error)?.message || L("没能开始听", "Could not start listening"));
      } finally {
        live.current = null;
        setListening(false);
      }
      return;
    }
    if (!Recog) return;
    const r = new Recog();
    r.lang = speechLang();
    r.continuous = true;
    r.interimResults = true;
    const { write } = begin();
    let said = "";
    r.onresult = (e) => {
      let interim = "";
      for (let i = e.resultIndex; i < e.results.length; i++) {
        const res = e.results[i];
        if (res.isFinal) said += res[0].transcript;
        else interim += res[0].transcript;
      }
      write(said + interim);
    };
    // The browser ends a session after a pause; while you have not stopped,
    // the next one starts (what was said stays), a few silent ones at most.
    let stopped = false;
    let quiet = 0;
    let heardSince = false;
    const onresult = r.onresult;
    r.onresult = (e) => { heardSince = true; onresult?.(e); };
    r.onerror = (e) => {
      if (e.error === "no-speech" || e.error === "aborted") return;
      stopped = true;
      setNote(reasonOf(e.error));
    };
    r.onend = () => {
      quiet = heardSince ? 0 : quiet + 1;
      heardSince = false;
      if (!stopped && live.current && quiet < 4) {
        try { r.start(); return; } catch { /* ends below */ }
      }
      write(said);
      live.current = null;
      setListening(false);
    };
    live.current = { wrote: [value], stop: () => { stopped = true; r.stop(); } };
    setListening(true);
    try { r.start(); } catch { live.current = null; setListening(false); }
  };

  const title = note
    || (listening ? L("正在听…再点一下停止", "Listening… click to stop")
      : L(`语音输入（${lang() === "zh" ? "中文" : "English"}，跟 App 的语言）`, `Dictate (${lang() === "zh" ? "Chinese" : "English"}, the app's language)`));
  return (
    <button type="button" className={cn("att-btn dictate-btn", listening && "is-listening", note && "is-note")} disabled={disabled}
      onMouseDown={(e) => e.preventDefault() /* keep the caret in the box */}
      onClick={(e) => { e.stopPropagation(); void start(); }}
      title={title} aria-label={title} aria-pressed={listening}>
      <Mic />
    </button>
  );
}
