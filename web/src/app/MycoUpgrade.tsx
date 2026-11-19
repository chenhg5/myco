import { useEffect, useState } from "react";
import { ArrowUpCircle } from "lucide-react";
import { ApiError, appApi } from "@/appapi";
import type { Machine } from "@/api";
import type { AgentRow } from "@/app/types";
import { ErrorText } from "@/app/dialogs";
import { CopyButton } from "@/app/copy";
import { hubAtLeast } from "@/app/hubVersion";
import { ver } from "@/app/nodes";
import { L } from "@/lib/i18n";

/*
 * Bringing myco up to date from the App: a computer (its machine node
 * upgrades it and restarts its runner; this computer, the App itself), a
 * workspace (it upgrades itself and restarts). Where none of that works
 * (an older myco, a computer away), the command to run there.
 */

let latestOnce: Promise<string> | null = null;
/** The newest release ("" while unknown), as the App's update check knows it. */
export function useLatestRelease(): string {
  const [latest, setLatest] = useState("");
  useEffect(() => {
    latestOnce ??= appApi.update().then((u) => (u.latest || "").replace(/^v/, ""), () => "");
    let off = false;
    void latestOnce.then((v) => { if (!off) setLatest(v); });
    return () => { off = true; };
  }, []);
  return latest;
}

/** Whether version (a myco build) is behind latest ("" unknown: no). */
export function behind(version: string | undefined, latest: string): boolean {
  return !!latest && !!version && !hubAtLeast(version, latest);
}

type How = "here" | "node" | "none";
export function upgradeHow(m: Machine, here: boolean): How {
  if (here) return "here";
  if (m.online && m.can_spawn) return "node";
  return "none";
}

/** Upgrades one computer; resolves with what to say. */
export async function upgradeComputer(hubId: string, m: Machine, here: boolean): Promise<string> {
  if (here) {
    const r = await appApi.upgradeMyco();
    return L(`已升级到 ${ver(r.version)}，后台已重启`, `Upgraded to ${ver(r.version)}; restarted`);
  }
  const r = await appApi.upgradeMachine(hubId, m.id);
  if (!r.to) return L("已经是最新的了", "Already up to date");
  return r.restarting
    ? L(`已升级到 ${ver(r.to)}，它的后台几秒后重启`, `Upgraded to ${ver(r.to)}; its runner restarts in a few seconds`)
    : L(`已升级到 ${ver(r.to)}；它的后台不是服务，要手动重启一次`, `Upgraded to ${ver(r.to)}; restart its runner by hand`);
}

/** A computer's version, and the way to bring it up to date. */
export function MachineUpgrade({ hubId, machine, here, latest, onDone }: {
  hubId: string; machine: Machine; here: boolean; latest: string; onDone?: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState("");
  const [error, setError] = useState<unknown>(null);
  if (!behind(machine.myco_version, latest) && !done) return null;
  const how = upgradeHow(machine, here);
  const tooOld = error instanceof ApiError && error.code === "too_old";
  const go = async () => {
    setBusy(true);
    setError(null);
    try {
      setDone(await upgradeComputer(hubId, machine, here));
      window.setTimeout(() => onDone?.(), 8000);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="app-notice mu" data-testid="machine-upgrade">
      <ArrowUpCircle />
      <div>
        <div className="mu-text">
          {done || L(`myco ${ver(machine.myco_version)} 不是最新（最新 ${ver(latest)}）`, `myco ${ver(machine.myco_version)} is not the latest (${ver(latest)})`)}
        </div>
        {!done && how !== "none" && !tooOld && (
          <>
            <div className="mu-note">{L("升级会重启它上面的 myco 后台，正在跑的任务会中断。", "Upgrading restarts myco there; tasks running on it are cut short.")}</div>
            <button type="button" className="app-btn app-btn-small app-btn-primary" disabled={busy} onClick={() => void go()}>
              {busy ? <><span className="app-spinner" />{L("正在升级…", "Upgrading…")}</> : L("升级", "Upgrade")}
            </button>
          </>
        )}
        {!done && (how === "none" || tooOld) && (
          <>
            <div className="mu-note">{!machine.online
              ? L("它现在不在线：上线后这里就能一键升级；也可以在那台电脑上运行：", "It is away: once it is back it upgrades from here; or run this on it:")
              : L("它上面的 myco 太旧，还不能从这里升级：在那台电脑上运行这一次（或在下面「检查和修复」设好 SSH 后在这里升级），以后就能一键升级了：", "Its myco is too old to be upgraded from here: run this on it once (or set up SSH under Check and fix below); after that it upgrades with a click:")}</div>
            <div className="app-command"><code>myco upgrade</code><CopyButton value="myco upgrade" /></div>
          </>
        )}
        {!tooOld && <ErrorText error={error} />}
      </div>
    </div>
  );
}

/** Every computer behind the latest release, upgraded in turn. */
export function UpgradeAll({ items, latest, onDone }: {
  items: { hubId: string; m: Machine; here: boolean }[]; latest: string; onDone?: () => void;
}) {
  const [busy, setBusy] = useState<string>("");
  const [results, setResults] = useState<Record<string, string>>({});
  // Those behind, and those just upgraded (their result stays in view).
  const old = items.filter((x) => behind(x.m.myco_version, latest) || results[x.m.id]);
  if (!old.length) return null;
  const able = old.filter((x) => !results[x.m.id] && upgradeHow(x.m, x.here) !== "none");
  const go = async () => {
    for (const x of able) {
      setBusy(x.m.id);
      try {
        const said = await upgradeComputer(x.hubId, x.m, x.here);
        setResults((r) => ({ ...r, [x.m.id]: said }));
      } catch (e) {
        setResults((r) => ({ ...r, [x.m.id]: e instanceof Error ? e.message : String(e) }));
      }
    }
    setBusy("");
    window.setTimeout(() => onDone?.(), 8000);
  };
  return (
    <div className="app-notice mu" data-testid="upgrade-all">
      <ArrowUpCircle />
      <div>
        <div className="mu-text">{L(`${old.length} 台电脑的 myco 不是最新（最新 ${ver(latest)}）`, `${old.length} computer${old.length > 1 ? "s" : ""} not on the latest myco (${ver(latest)})`)}</div>
        <ul className="mu-list">
          {old.map((x) => (
            <li key={`${x.hubId}/${x.m.id}`}>
              <b>{x.m.name}</b> {ver(x.m.myco_version)}
              {busy === x.m.id ? <span className="mu-state"><span className="app-spinner" />{L("升级中…", "Upgrading…")}</span>
                : results[x.m.id] ? <span className="mu-state">{results[x.m.id]}</span>
                : upgradeHow(x.m, x.here) === "none" ? <span className="mu-state">{x.m.online
                  ? L("太旧，还不能从这里升级：在它上面运行一次 myco upgrade", "Too old to upgrade from here: run myco upgrade on it once")
                  : L("不在线：上线后再升级，或在它上面运行 myco upgrade", "Away: upgrade it once it is back, or run myco upgrade on it")}</span> : null}
            </li>
          ))}
        </ul>
        {able.length > 0 && (
          <button type="button" className="app-btn app-btn-small app-btn-primary" disabled={!!busy} onClick={() => void go()}>
            {busy ? <><span className="app-spinner" />{L("正在升级…", "Upgrading…")}</> : L(`全部升级（${able.length} 台）`, `Upgrade all (${able.length})`)}
          </button>
        )}
      </div>
    </div>
  );
}

/** A workspace's version, and upgrading it: on this computer, the App does
 *  it; elsewhere, the workspace upgrades itself, and its new version is
 *  waited for. */
export function HubUpgrade({ hubId, version, local, manages, latest, onDone }: {
  hubId?: string; version?: string; local: boolean; manages: boolean; latest: string; onDone?: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState("");
  const [error, setError] = useState<unknown>(null);
  if (!manages || (!behind(version, latest) && !done)) return null;
  const tooOld = error instanceof ApiError && error.code === "too_old";
  const go = async () => {
    setBusy(true);
    setError(null);
    try {
      if (local) {
        const r = await appApi.upgradeMyco();
        setDone(L(`已升级到 ${ver(r.version)} 并重启`, `Upgraded to ${ver(r.version)} and restarted`));
      } else if (hubId) {
        const r = await appApi.upgradeHub(hubId);
        // It restarts on its own: wait for the new version to answer.
        const want = r.to.replace(/^v/, "");
        for (let i = 0; i < 60; i++) {
          await new Promise((res) => window.setTimeout(res, 3000));
          const hs = await appApi.hubs().catch(() => null);
          const v = hs?.find((h) => h.id === hubId)?.hub_version;
          if (v && hubAtLeast(v, want)) break;
        }
        setDone(L(`已升级到 ${ver(r.to)}`, `Upgraded to ${ver(r.to)}`));
      }
      onDone?.();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="app-notice mu" data-testid="hub-upgrade">
      <ArrowUpCircle />
      <div>
        <div className="mu-text">{done || L(`工作区的 myco ${ver(version)} 不是最新（最新 ${ver(latest)}）`, `The workspace runs myco ${ver(version)}; ${ver(latest)} is out`)}</div>
        {!done && !tooOld && (
          <>
            <div className="mu-note">{L("升级时工作区会重启，大约半分钟连不上，消息不会丢。", "It restarts while upgrading: about half a minute away; no messages are lost.")}</div>
            <button type="button" className="app-btn app-btn-small app-btn-primary" disabled={busy} onClick={() => void go()}>
              {busy ? <><span className="app-spinner" />{L("正在升级，等它重启…", "Upgrading; waiting for it to restart…")}</> : L("升级工作区", "Upgrade the workspace")}
            </button>
          </>
        )}
        {!done && tooOld && (
          <>
            <div className="mu-note">{L("这个工作区的 myco 还不能从 App 里升级：在服务器上运行这一次，以后就能在这里一键升级了：", "This workspace cannot upgrade from the App yet: run this on the server once; after that it upgrades from here:")}</div>
            <div className="app-command"><code>sudo myco upgrade</code><CopyButton value="sudo myco upgrade" /></div>
          </>
        )}
        {!tooOld && <ErrorText error={error} />}
      </div>
    </div>
  );
}

export type { AgentRow };
