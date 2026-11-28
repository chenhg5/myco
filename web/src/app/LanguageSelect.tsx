import { useState } from "react";
import { L, LANGUAGES, languageProblem, type LangChoice } from "@/lib/i18n";
import { useContextMenu } from "@/app/ContextMenu";
import { cn } from "@/lib/utils";

/** Native language names remain readable after an accidental language change. */
export function LanguageSelect({ value, onChange, className }: {
  value: LangChoice;
  onChange: (choice: LangChoice) => void | Promise<void>;
  className?: string;
}) {
  const menu = useContextMenu("language-menu");
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(languageProblem);
  const [retryChoice, setRetryChoice] = useState(value);
  const choose = async (choice: LangChoice) => {
    setRetryChoice(choice); setBusy(true); setFailed(false);
    try { await onChange(choice); } catch { setFailed(true); } finally { setBusy(false); }
  };
  return <div className={cn("language-control", className)}>
    {/* A native WebKit popup can still report its selection after the language
        remount removes the control, crashing the WebContent process. */}
    <button type="button" className="app-select-field app-language-select" data-testid="language-select" data-value={value}
      aria-label={L("语言", "Language")} aria-haspopup="menu" aria-expanded={!!menu.element} disabled={busy}
      onClick={(e) => menu.openUnder(e.currentTarget, [
        { label: L("跟随系统", "System default"), checked: value === "system", onSelect: () => void choose("system") },
        ...LANGUAGES.map((language) => ({ label: language.name, checked: value === language.id, onSelect: () => void choose(language.id) })),
      ])}>
      {value === "system" ? L("跟随系统", "System default") : LANGUAGES.find((language) => language.id === value)?.name}
    </button>
    {menu.element}
    {busy && <span className="app-kv-note" role="status">{L("正在切换语言", "Changing language")}</span>}
    {failed && <span className="app-kv-note err" role="alert" data-testid="language-error">{L("语言包暂时无法加载，请重试。", "The language pack could not be loaded. Please try again.")} <button type="button" className="app-kv-link" data-testid="language-retry" onClick={() => void choose(retryChoice)} disabled={busy}>{L("重试", "Retry")}</button></span>}
  </div>;
}
