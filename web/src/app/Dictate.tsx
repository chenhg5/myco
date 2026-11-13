import { useEffect, useRef, useState } from "react";
import { Mic } from "lucide-react";
import { appApi } from "@/appapi";
import { L, lang } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * Speaking instead of typing, with what the device already has. In the
 * desktop app the button starts the system's own dictation (macOS
 * Dictation, Windows voice typing) into the message box: the system
 * listens, shows that it does, and types; it asks to turn dictation on the
 * first time. In a browser that has speech recognition the page listens
 * itself, writing what it hears into the box as you speak; click again to
 * stop. Where there is neither, there is no button.
 */

/** Whether the desktop app can start the system's dictation (from
 *  /app/info; the app says so once it has asked). */
let systemDictation = false;
export function setSystemDictation(on: boolean) { systemDictation = on; }

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

/** Puts the caret at the end of the box, so what is said goes after what
 *  was typed. */
function focusEnd(el: HTMLTextAreaElement | null) {
  if (!el) return;
  el.focus();
  el.selectionStart = el.selectionEnd = el.value.length;
}

export function DictateButton({ input, value, onText, disabled }: {
  input: React.RefObject<HTMLTextAreaElement | null>;
  /** what is in the box now */
  value: string;
  /** the box's new text while the page listens itself */
  onText: (text: string) => void;
  disabled?: boolean;
}) {
  const [listening, setListening] = useState(false);
  const [note, setNote] = useState("");
  const rec = useRef<Recognition | null>(null);
  const Recog = browserRecognition();
  useEffect(() => () => rec.current?.abort(), []);
  if (!systemDictation && !Recog) return null;

  const start = async () => {
    setNote("");
    focusEnd(input.current);
    if (systemDictation) {
      try {
        await appApi.dictate();
      } catch (e) {
        setNote((e as Error)?.message || L("没能打开听写", "Could not start dictation"));
      }
      return;
    }
    if (!Recog) return;
    if (listening) { rec.current?.stop(); return; }
    const r = new Recog();
    r.lang = lang() === "zh" ? "zh-CN" : "en-US";
    r.continuous = true;
    r.interimResults = true;
    const base = value && !/\s$/.test(value) ? `${value}${lang() === "zh" ? "" : " "}` : value;
    let said = "";
    r.onresult = (e) => {
      let interim = "";
      for (let i = e.resultIndex; i < e.results.length; i++) {
        const res = e.results[i];
        if (res.isFinal) said += res[0].transcript;
        else interim += res[0].transcript;
      }
      onText(base + said + interim);
    };
    r.onerror = (e) => {
      if (e.error === "not-allowed" || e.error === "service-not-allowed") setNote(L("没有麦克风权限", "No access to the microphone"));
      else if (e.error !== "aborted" && e.error !== "no-speech") setNote(L("没听清，再试一次", "Didn't catch that; try again"));
    };
    r.onend = () => { setListening(false); rec.current = null; onText(base + said); };
    rec.current = r;
    setListening(true);
    try { r.start(); } catch { setListening(false); rec.current = null; }
  };

  const title = note
    || (listening ? L("正在听…再点一下停止", "Listening… click to stop")
      : systemDictation ? L("语音输入（用系统的听写）", "Dictate (the system's dictation)")
      : L("语音输入", "Dictate"));
  return (
    <button type="button" className={cn("att-btn dictate-btn", listening && "is-listening", note && "is-note")} disabled={disabled}
      onMouseDown={(e) => e.preventDefault() /* keep the caret in the box */}
      onClick={(e) => { e.stopPropagation(); void start(); }}
      title={title} aria-label={title} aria-pressed={listening}>
      <Mic />
    </button>
  );
}
