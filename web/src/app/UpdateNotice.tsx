import { useEffect, useState } from "react";
import { ArrowUpCircle, X } from "lucide-react";
import { ApiError, appApi, type AppUpdate } from "@/appapi";
import { ErrorText, type ErrorSummary } from "@/app/dialogs";
import { L } from "@/lib/i18n";

const DISMISSED_KEY = "myco-update-dismissed";
const CHECK_EVERY = 6 * 60 * 60 * 1000;

function dismissed(): string {
  try { return localStorage.getItem(DISMISSED_KEY) || ""; } catch { return ""; }
}

/** What a failed app update (POST /app/update) means, in the person's words. */
export function updateFailure(e: unknown): ErrorSummary | null {
  const code = e instanceof ApiError ? e.code : undefined;
  switch (code) {
    case "download_failed":
      return {
        message: L("没能从 GitHub 或 Gitee 下载新版本。", "Couldn't download the new version from GitHub or Gitee."),
        suggestion: L("这个网络访问 GitHub 和 Gitee 都可能很慢或被挡住了：打开你的代理软件，或在设置 › 代理里设一个，再重试。", "GitHub and Gitee may both be slow or blocked on this network: turn on your proxy app, or set one under Settings › Proxy, then try again."),
      };
    case "install_failed":
      return {
        message: L("新版本下载好了，但没能替换当前的 App。", "The new version downloaded, but couldn't replace the current app."),
        suggestion: L("可能有别的程序（杀毒软件、OneDrive 这类同步软件）正占用 myco 的文件。稍等片刻再试，或到 GitHub 手动下载新版本。", "Another program (antivirus, or a sync app such as OneDrive) may be holding myco's file. Try again in a moment, or download the new version from GitHub."),
      };
    case "start_failed":
      return {
        message: L("新版本没能启动，已换回当前版本，可以照常使用。", "The new version didn't start, so the current version was kept and keeps working."),
        suggestion: L("如果杀毒软件拦住了新的 myco，在杀毒软件里放行后再试，或到 GitHub 手动下载新版本。", "If an antivirus blocked the new myco, allow it there and try again, or download the new version from GitHub."),
      };
  }
  return null;
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
        {error !== null && <ErrorText error={error} summary={updateFailure(error)} />}
        {step === "idle" && (
          <span className="app-update-actions">
            {u.can_install
              ? <button type="button" className="app-btn app-btn-primary" onClick={() => void install()}>{L("更新", "Update")}</button>
              : <button type="button" className="app-btn" onClick={onOpenSettings}>{L("怎么更新", "How to update")}</button>}
            <a className="app-update-link" href="https://github.com/myco-ai/myco/releases/latest" target="_blank" rel="noreferrer">{L("看看改了什么", "What's new")}</a>
          </span>
        )}
      </div>
      {step === "idle" && (
        <button type="button" className="app-update-close" aria-label={L("这个版本不再提醒", "Don't remind me of this version")} title={L("这个版本不再提醒", "Don't remind me of this version")} onClick={dismiss}><X /></button>
      )}
    </div>
  );
}
