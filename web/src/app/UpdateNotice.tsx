import { useEffect, useState } from "react";
import { ArrowUpCircle, X } from "lucide-react";
import { appApi, type AppUpdate } from "@/appapi";
import { ErrorText } from "@/app/dialogs";
import { L } from "@/lib/i18n";

const DISMISSED_KEY = "myco-update-dismissed";
const CHECK_EVERY = 6 * 60 * 60 * 1000;

function dismissed(): string {
  try { return localStorage.getItem(DISMISSED_KEY) || ""; } catch { return ""; }
}

/**
 * A newer myco release, checked at start and every few hours. Dismissing
 * hides that version (a later one shows again).
 */
export function UpdateNotice({ onOpenSettings }: { onOpenSettings: () => void }) {
  const [u, setU] = useState<AppUpdate | null>(null);
  const [hidden, setHidden] = useState(dismissed);
  const [step, setStep] = useState<"idle" | "installing" | "relaunching">("idle");
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let off = false;
    const check = () => appApi.update().then((r) => { if (!off) setU(r); }).catch(() => undefined);
    // Not at the very first paint: the window comes up first.
    const first = window.setTimeout(check, 4000);
    const every = window.setInterval(check, CHECK_EVERY);
    return () => { off = true; window.clearTimeout(first); window.clearInterval(every); };
  }, []);

  if (!u || !u.available || !u.latest || u.error || (hidden === u.latest && step === "idle")) return null;

  const dismiss = () => {
    try { localStorage.setItem(DISMISSED_KEY, u.latest!); } catch { /* not kept */ }
    setHidden(u.latest!);
  };
  const install = async () => {
    setStep("installing");
    setError(null);
    try {
      await appApi.installUpdate();
      setStep("relaunching");
    } catch (e) {
      setError(e);
      setStep("idle");
    }
  };

  return (
    <div className="app-update-notice" role="status">
      <ArrowUpCircle className="app-update-icon" />
      <div className="app-update-text">
        <b>{step === "relaunching" ? L(`已安装 ${u.latest}`, `Installed ${u.latest}`) : L(`新版本 ${u.latest}`, `myco ${u.latest} is out`)}</b>
        <span>
          {step === "installing" ? L("正在下载并安装…", "Downloading and installing…")
            : step === "relaunching" ? L("正在重新打开…", "Reopening…")
            : L(`当前 ${u.current}`, `You have ${u.current}`)}
        </span>
        {error !== null && <ErrorText error={error} />}
        {step === "idle" && (
          <span className="app-update-actions">
            {u.can_install
              ? <button type="button" className="app-btn app-btn-primary" onClick={() => void install()}>{L("更新", "Update")}</button>
              : <button type="button" className="app-btn" onClick={onOpenSettings}>{L("怎么更新", "How to update")}</button>}
            <a className="app-update-link" href="https://github.com/chenhg5/myco/releases/latest" target="_blank" rel="noreferrer">{L("看看改了什么", "What's new")}</a>
          </span>
        )}
      </div>
      {step === "idle" && (
        <button type="button" className="app-update-close" aria-label={L("这个版本不再提醒", "Don't remind me of this version")} title={L("这个版本不再提醒", "Don't remind me of this version")} onClick={dismiss}><X /></button>
      )}
    </div>
  );
}
