import { useState } from "react";
import { L, LANGUAGES, languageProblem, type LangChoice } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/** Native language names remain readable after an accidental language change. */
export function LanguageSelect({ value, onChange, className }: {
  value: LangChoice;
  onChange: (choice: LangChoice) => void | Promise<void>;
  className?: string;
}) {
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(languageProblem);
  const choose = async (choice: LangChoice) => {
    setBusy(true); setFailed(false);
    try { await onChange(choice); } catch { setFailed(true); } finally { setBusy(false); }
  };
  return <div className={cn("language-control", className)}>
    <select className="app-select-field app-language-select" data-testid="language-select" aria-label={L("语言", "Language")}
      value={value} disabled={busy} onChange={(e) => void choose(e.target.value as LangChoice)}>
      <option value="system">{L("跟随系统", "System default")}</option>
      {LANGUAGES.map((language) => <option key={language.id} value={language.id}>{language.name}</option>)}
    </select>
    {busy && <span className="app-kv-note" role="status">{L("正在切换语言", "Changing language")}</span>}
    {failed && <span className="app-kv-note err" role="alert">{L("语言包暂时无法加载，请重试。", "The language pack could not be loaded. Please try again.")} <button type="button" className="app-kv-link" onClick={() => void choose(value)} disabled={busy}>{L("重试", "Retry")}</button></span>}
  </div>;
}
