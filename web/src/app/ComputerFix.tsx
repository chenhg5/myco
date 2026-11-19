import { useEffect, useState } from "react";
import { ArrowUpCircle, Info, RotateCw } from "lucide-react";
import { appApi, type RemoteRoute } from "@/appapi";
import type { Machine } from "@/api";
import { ErrorText } from "@/app/dialogs";
import { CopyButton } from "@/app/copy";
import { machineTellsInfo } from "@/app/hubVersion";
import { L } from "@/lib/i18n";

/**
 * Why a computer cannot list its sessions or start one, and the fix done
 * from here: its myco is too old (upgrade it), or the part of myco that
 * starts sessions there is not connected (restart it). On this computer the
 * App does it; on another one it does it over SSH or WSL when it knows how
 * to reach it; otherwise it gives the command to run there.
 */
export function ComputerFix({ hubId, machine, here }: { hubId: string; machine: Machine; here: boolean }) {
  const old = !machineTellsInfo(machine.myco_version);
  const [route, setRoute] = useState<RemoteRoute | null | undefined>(here ? null : undefined);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [done, setDone] = useState("");

  useEffect(() => {
    if (here) return;
    let off = false;
    appApi.machineReach(hubId, machine.id).then((r) => { if (!off) setRoute(r.route); }).catch(() => { if (!off) setRoute(null); });
    return () => { off = true; };
  }, [hubId, machine.id, here]);

  const fix = async () => {
    setBusy(true);
    setError(null);
    setDone("");
    try {
      if (here) {
        const r = await appApi.upgradeMyco();
        setDone(r.from && r.from !== r.version
          ? L(`已升级到 ${r.version} 并重启，半分钟内就能用。`, `Upgraded to ${r.version} and restarted; ready within half a minute.`)
          : L("已重启，半分钟内就能用。", "Restarted; ready within half a minute."));
      } else {
        const r = await appApi.fixMachine(hubId, machine.id, old ? "upgrade" : "restart");
        if (!r.ok) throw new Error(r.log.slice(-3).join("\n") || L("没修好", "It did not work"));
        setDone(L("好了，半分钟内就能用。", "Done; ready within half a minute."));
      }
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const text = old
    ? here
      ? L("这台电脑上的 myco 版本太旧，列不出会话、也开不了新的。点一下升级（几分钟内完成，不用打开终端）：", "This computer's myco is too old to list sessions or start one. Upgrade it here (a few minutes, no terminal):")
      : L(`「${machine.name}」上的 myco 版本太旧，列不出会话、也开不了新的。`, `The myco on “${machine.name}” is too old to list sessions or start one.`)
    : here
      ? L("这台电脑上负责开会话的 myco 后台没连上工作区。点一下修复，会把 myco 升到最新并重启它：", "The part of myco that starts sessions on this computer is not connected. Fix it here: myco is brought up to date and restarted:")
      : L(`「${machine.name}」上负责开会话的 myco 后台没连上工作区。`, `The part of myco that starts sessions on “${machine.name}” is not connected.`);
  const canFix = here || !!route;
  const command = old ? "myco upgrade" : "myco runner service restart";
  return (
    <div className="app-notice ca" data-testid={old ? "info-too-old" : "info-no-spawn"}>
      <Info />
      <div>
        <div className="ca-text">{text}</div>
        {canFix ? (
          <div className="ca-ways">
            <button type="button" className="app-btn app-btn-small app-btn-primary" disabled={busy || !!done} onClick={() => void fix()}>
              {busy ? <span className="app-spinner" /> : old ? <ArrowUpCircle /> : <RotateCw />}
              {busy
                ? old ? L("正在升级…", "Upgrading…") : L("正在修复…", "Fixing…")
                : old ? L("升级 myco", "Upgrade myco") : L("修复", "Fix it")}
            </button>
            {!here && route && <span className="ca-host">{route.via === "wsl" ? L("经 WSL", "through WSL") : L("经 SSH", "over SSH")} {route.target}</span>}
          </div>
        ) : route === null ? (
          <>
            <div className="ca-text">{L("在那台电脑上运行这行，再回来：", "Run this on that computer, then come back:")}</div>
            <div className="app-command"><code>{command}</code><CopyButton value={command} /></div>
            <div className="ca-text app-small-note">{L("在「电脑」页给它设好 SSH 或 WSL，以后就能在这里一键修。", "Set up SSH or WSL for it on the Computers page to fix it from here next time.")}</div>
          </>
        ) : null}
        {done && <div className="ca-text">{done}</div>}
        <ErrorText error={error} />
      </div>
    </div>
  );
}
