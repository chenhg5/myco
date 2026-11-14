import { useEffect, useState, useSyncExternalStore } from "react";
import { appApi } from "@/appapi";
import { MessageContent } from "@/app/Markdown";
import { L, lang } from "@/lib/i18n";

/*
 * Reading what an agent wrote in the language of the App: each answer and
 * progress update can be translated (the 翻译 button by its time), or all
 * of them that are in another language, when 自动翻译 is on (Settings ›
 * 翻译). The translation shows in place of the words, with a line under it
 * to see the original again. The service is the console's (translate.go):
 * MyMemory for free, or DeepL, Google or an OpenAI-compatible API with a
 * key kept in the keychain.
 */

type To = "zh" | "en";
export const appTo = (): To => (lang() === "zh" ? "zh" : "en");

// ---- settings and per-message choices (kept on this computer) ----

const AUTO_KEY = "myco-translate-auto";
let auto = (() => { try { return localStorage.getItem(AUTO_KEY) === "1"; } catch { return false; } })();
const listeners = new Set<() => void>();
const emit = () => listeners.forEach((l) => l());
const subscribe = (cb: () => void) => { listeners.add(cb); return () => { listeners.delete(cb); }; };

export function setAutoTranslate(on: boolean) {
  auto = on;
  try { localStorage.setItem(AUTO_KEY, on ? "1" : "0"); } catch { /* not kept */ }
  emit();
}
export const useAutoTranslate = () => useSyncExternalStore(subscribe, () => auto, () => auto);

/** Messages you asked to translate, or to see in the original. */
const chosen = new Map<string, boolean>();
let version = 0;
export function chooseTranslation(id: string, on: boolean) {
  chosen.set(id, on);
  version++;
  emit();
}
const useChoice = (id: string) => {
  useSyncExternalStore(subscribe, () => version, () => version);
  return chosen.get(id);
};

// ---- is it in another language? ----

/** Whether text is mostly in a language other than the App's: Chinese
 *  characters against Latin words, leaving out code, paths and links. */
export function inOtherLanguage(text: string, to: To = appTo()): boolean {
  const prose = text
    .replace(/```[\s\S]*?(```|$)/g, " ")
    .replace(/`[^`\n]*`/g, " ")
    .replace(/https?:\/\/\S+/g, " ")
    .replace(/[\w.-]*[/\\][\w./\\-]+/g, " ");
  const han = (prose.match(/[㐀-鿿豈-﫿]/g) || []).length;
  const words = (prose.match(/[A-Za-z]{2,}/g) || []).length;
  if (to === "zh") return words >= 4 && han < words * 0.5;
  return han >= 6 && han > words * 2;
}

// ---- translations (each text once) ----

const done = new Map<string, Promise<string>>();
function translated(text: string, to: To): Promise<string> {
  const key = `${to}\u0000${text}`;
  let p = done.get(key);
  if (!p) {
    p = appApi.translate(text, to).then((r) => r.text);
    p.catch(() => done.delete(key)); // a failure can be tried again
    done.set(key, p);
  }
  return p;
}

function failure(e: unknown): string {
  const code = (e as { code?: string })?.code;
  switch (code) {
    case "quota": return L("今天免费翻译的额度用完了，可以在 设置 › 翻译 换一个服务", "Today's free translations are used up; another service can be chosen in Settings › Translation");
    case "unauthorized": return L("翻译服务不认这个密钥（设置 › 翻译）", "The translation service did not accept the key (Settings › Translation)");
    case "not_configured": return L("这个翻译服务还没填密钥（设置 › 翻译）", "This translation service has no key yet (Settings › Translation)");
    case "unreachable": return L("连不上翻译服务", "Could not reach the translation service");
    case "not_found": return L("这个版本的 App 还不能翻译", "This version of the app cannot translate");
    default: return L("没翻译成", "Could not translate this");
  }
}

/** What an agent wrote, translated when asked or when automatic and in
 *  another language; a line under it switches to the original and back. */
export function TranslatableBody({ id, body }: { id: string; body: string }) {
  const autoOn = useAutoTranslate();
  const choice = useChoice(id);
  const to = appTo();
  const want = choice ?? (autoOn && inOtherLanguage(body, to));
  const [st, setSt] = useState<{ text?: string; error?: string }>({});
  useEffect(() => {
    if (!want) return;
    let live = true;
    setSt({});
    translated(body, to).then((text) => live && setSt({ text }), (e) => live && setSt({ error: failure(e) }));
    return () => { live = false; };
  }, [want, body, to]);
  if (!want) return <MessageContent body={body} />;
  return (
    <>
      <MessageContent body={st.text ?? body} />
      <div className="conv-translated">
        {st.error ? <span className="is-error">{st.error}</span>
          : st.text === undefined ? <span>{L("正在翻译…", "Translating…")}</span>
          : <span>{L("已翻译", "Translated")}</span>}
        <button type="button" onClick={() => chooseTranslation(id, false)}>{L("显示原文", "Show original")}</button>
      </div>
    </>
  );
}

/** The 翻译 / 原文 switch by a message's time. */
export function TranslateToggle({ id, body }: { id: string; body: string }) {
  const autoOn = useAutoTranslate();
  const choice = useChoice(id);
  const on = choice ?? (autoOn && inOtherLanguage(body));
  return (
    <button type="button" className="conv-translate-btn" onClick={() => chooseTranslation(id, !on)}
      title={on ? L("显示原文", "Show the original") : L("翻译成 App 的语言", "Translate into the app's language")}>
      {on ? L("原文", "Original") : L("翻译", "Translate")}
    </button>
  );
}
