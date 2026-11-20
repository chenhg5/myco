import { useEffect, useState } from "react";
import { ChevronDown, RefreshCw, RotateCw, Stethoscope, ArrowUpCircle } from "lucide-react";
import { ApiError, appApi, type Diagnosis, type RemoteRoute } from "@/appapi";
import type { Machine } from "@/api";
import { ErrorText } from "@/app/dialogs";
import { CopyButton } from "@/app/copy";
import { remoteErrorText } from "@/app/ConnectOver";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/**
 * A computer's check-up, from this App over SSH or into WSL: is myco there,
 * is its runner running (and kept running after logout), can it reach the
 * workspace directly and through the proxy its service uses, and its last
 * log lines; then restart the runner or upgrade myco
 * (docs/design/computers.md).
 */
export function ComputerCheck({ hubId, machine }: { hubId: string; machine: Machine }) {
  const [route, setRoute] = useState<RemoteRoute | null | undefined>(undefined);
  const [unsupported, setUnsupported] = useState(false);
  const [diag, setDiag] = useState<Diagnosis | null>(null);
  const [busy, setBusy] = useState<"" | "check" | "restart" | "upgrade">("");
  const [error, setError] = useState<unknown>(null);
  const [fixed, setFixed] = useState<{ action: string; ok: boolean; log: string[] } | null>(null);
  const [changing, setChanging] = useState(false);

  useEffect(() => {
    let off = false;
    setRoute(undefined);
    setDiag(null);
    setFixed(null);
    setError(null);
    appApi.machineReach(hubId, machine.id)
      .then((r) => { if (!off) setRoute(r.route); })
      .catch(() => { if (!off) { setRoute(null); setUnsupported(true); } });
    return () => { off = true; };
  }, [hubId, machine.id]);

  const check = async () => {
    setBusy("check");
    setError(null);
    setFixed(null);
    try {
      setDiag(await appApi.diagnoseMachine(hubId, machine.id));
    } catch (e) {
      setError(e);
    } finally {
      setBusy("");
    }
  };
  const fix = async (action: "restart" | "upgrade") => {
    setBusy(action);
    setError(null);
    try {
      const r = await appApi.fixMachine(hubId, machine.id, action);
      setFixed({ action, ...r });
      if (r.ok) setDiag(await appApi.diagnoseMachine(hubId, machine.id).catch(() => null));
    } catch (e) {
      setError(e);
    } finally {
      setBusy("");
    }
  };

  if (unsupported || route === undefined) return null;
  if (!route || changing) {
    return (
      <div className="pc-details cc" data-testid="computer-check">
        <PickRoute hubId={hubId} machine={machine} onCancel={route ? () => setChanging(false) : undefined}
          onPicked={(rt) => { setRoute(rt); setChanging(false); setDiag(null); }} />
      </div>
    );
  }
  const err = error instanceof ApiError && error.code && remoteCodes.has(error.code)
    ? { code: error.code, message: error.message, command: error.suggestion } : null;
  return (
    <div className="pc-details cc" data-testid="computer-check">
      <div className="cc-head">
        <span className="cc-route">
          {route.via === "wsl" ? L("经 WSL", "Through WSL") : L("经 SSH", "Over SSH")} <span className="mono">{route.target}{route.port ? `:${route.port}` : ""}</span>
          <button type="button" className="app-group-action" onClick={() => setChanging(true)}>{L("换一个", "Change")}</button>
        </span>
        <span className="cc-actions">
          <button type="button" className="app-btn app-btn-small" onClick={() => void check()} disabled={!!busy}>
            {busy === "check" ? <span className="app-spinner" /> : <Stethoscope />}{diag ? L("再检查一次", "Check again") : L("检查", "Check")}
          </button>
          <button type="button" className="app-btn app-btn-small" onClick={() => void fix("restart")} disabled={!!busy}>
            {busy === "restart" ? <span className="app-spinner" /> : <RotateCw />}{L("重启后台", "Restart runner")}
          </button>
          <button type="button" className="app-btn app-btn-small" onClick={() => void fix("upgrade")} disabled={!!busy}>
            {busy === "upgrade" ? <span className="app-spinner" /> : <ArrowUpCircle />}{L("升级 myco", "Upgrade myco")}
          </button>
        </span>
      </div>
      {!diag && !busy && !error && !fixed && (
        <div className="app-small-note">{L("它掉线了、不干活时点「检查」：看 myco 后台在不在跑、连不连得上工作区、日志最后说了什么。", "When it is away or idle, Check looks at whether its runner runs, whether it reaches the workspace, and what its log says last.")}</div>
      )}
      {busy === "upgrade" && <div className="app-small-note">{L("正在升级：下载、换上新版本、重启后台，要一两分钟…", "Upgrading: downloading, swapping in the new version, restarting the runner; a minute or two…")}</div>}
      {err ? (
        <div className="cx-error">
          <div>{remoteErrorText(err, route)}</div>
          {err.command && <div className="app-command"><code>{err.command}</code><CopyButton value={err.command} /></div>}
        </div>
      ) : <ErrorText error={error} />}
      {fixed && (
        <div className={cn("cc-fixed", !fixed.ok && "is-bad")}>
          {fixed.ok
            ? (fixed.action === "upgrade" ? L("升级好了，后台已重启。", "Upgraded; the runner restarted.") : L("后台重启好了。", "The runner restarted."))
            : (fixed.action === "upgrade" ? L("升级没成功：看下面它打印的。", "The upgrade failed: see what it printed below.") : L("重启没成功：看下面它打印的。", "The restart failed: see what it printed below."))}
          {fixed.log.length > 0 && <LogBlock title={L("它打印的", "What it printed")} lines={fixed.log} open={!fixed.ok} />}
        </div>
      )}
      {diag && <Findings diag={diag} />}
    </div>
  );
}

const remoteCodes = new Set(["auth", "host_unknown", "host_changed", "refused", "unknown_host", "unreachable", "timeout", "no_shell", "wsl_missing", "ssh_missing"]);

type Tone = "ok" | "warn" | "bad";

/** The check-up in words: one line per thing looked at, the worst first. */
function Findings({ diag }: { diag: Diagnosis }) {
  const f = diag.found;
  const rows: { tone: Tone; title: string; detail?: string; command?: string }[] = [];
  if (!f.myco || f.myco === "none") {
    rows.push({ tone: "bad", title: L("没装 myco", "myco is not installed"), detail: L("从「连一台电脑」重新连它一次就会装上。", "Connect it again from Connect a computer to install it.") });
  } else {
    rows.push({ tone: "ok", title: `myco ${f.myco.replace(/^myco\s+/, "")}`, detail: f.myco_path });
  }
  const svc = f.service || "";
  if (svc === "active" || svc === "running") {
    rows.push({ tone: "ok", title: L("myco 后台在运行", "Its runner is running"), detail: f.since ? L(`从 ${f.since} 起`, `since ${f.since}`) : undefined });
  } else if (svc === "no_systemd") {
    rows.push({ tone: f.running === "yes" ? "warn" : "bad", title: f.running === "yes" ? L("myco 后台在跑，但不是开机自启的服务", "Its runner runs, but not as a service") : L("myco 后台没在跑", "Its runner is not running"),
      detail: L("这台电脑没有 systemd 用户服务（比如容器、没开 systemd 的 WSL），重启或注销后要手动运行 myco runner start --all。", "This computer has no systemd user services (a container, or WSL without systemd): after a restart or logout, run myco runner start --all by hand."),
      command: "myco runner start --all" });
  } else {
    rows.push({ tone: "bad", title: L("myco 后台没在运行", "Its runner is not running"), detail: svc ? L(`服务的状态是 ${svc}。点「重启后台」，还不行就看下面的日志。`, `Its service is ${svc}. Restart it; if that fails, see the log below.`) : undefined });
  }
  if (f.linger === "no") {
    rows.push({ tone: "warn", title: L("注销后后台会停", "The runner stops on logout"),
      detail: L("关掉最后一个 SSH 或 WSL 窗口一会儿后，这个用户的服务会被停掉。在那台电脑上运行下面这条（v0.20.18 起安装后台时会自动开）：", "A while after the last SSH or WSL window closes, this user's services stop. Run this there (from v0.20.18, installing the runner does it):"),
      command: "sudo loginctl enable-linger $(id -un)" });
  }
  const proxy = f.proxy || f.proxy_all;
  for (const h of diag.hubs) {
    const mine = !!h.mine;
    const name = mine ? L("这个工作区", "This workspace") : h.url;
    const directOK = answered(h.direct);
    if (proxy && h.proxy !== undefined) {
      const proxyOK = answered(h.proxy);
      if (!proxyOK && directOK) {
        rows.push({ tone: "bad", title: L(`经代理连不上${mine ? "工作区" : name}，直连是通的`, `${name}: unreachable through the proxy, reachable directly`),
          detail: L(`myco 后台被设成走代理 ${proxy}，这个代理现在连不上。修好代理，或者让 myco 连工作区时不走代理：把工作区的地址加进服务的 NO_PROXY，再重启后台。`, `Its runner goes through the proxy ${proxy}, which does not get through now. Fix the proxy, or let myco reach the workspace directly: add the workspace's host to the service's NO_PROXY and restart the runner.`) });
        continue;
      }
      if (!proxyOK && !directOK) {
        rows.push({ tone: "bad", title: L(`连不上${mine ? "工作区" : name}`, `${name}: unreachable`), detail: L(`经代理 ${proxy} 和直连都不通：看看那台电脑的网络。`, `Neither through the proxy ${proxy} nor directly: check its network.`) });
        continue;
      }
      rows.push({ tone: "ok", title: L(`连得上${mine ? "工作区" : name}`, `Reaches ${name}`), detail: L(`经代理 ${proxy}`, `through the proxy ${proxy}`) });
      continue;
    }
    rows.push(directOK
      ? { tone: "ok", title: L(`连得上${mine ? "工作区" : name}`, `Reaches ${name}`) }
      : { tone: "bad", title: L(`连不上${mine ? "工作区" : name}`, `${name}: unreachable`), detail: L("那台电脑访问不到工作区的地址：多半是网络或防火墙，或者工作区只在局域网里。", "It cannot reach the workspace's address: its network or a firewall, or the workspace is only on a local network.") });
  }
  if (diag.hubs.length === 0 && f.myco && f.myco !== "none") {
    rows.push({ tone: "bad", title: L("它还没连上任何工作区", "It is in no workspace"), detail: L("从「连一台电脑」重新连它一次。", "Connect it again from Connect a computer.") });
  }
  const order: Record<Tone, number> = { bad: 0, warn: 1, ok: 2 };
  rows.sort((a, b) => order[a.tone] - order[b.tone]);
  return (
    <div className="cc-findings" data-testid="check-findings">
      {rows.map((r, i) => (
        <div key={i} className={cn("cc-row", `is-${r.tone}`)}>
          <span className="cc-dot" />
          <div>
            <div className="cc-title">{r.title}</div>
            {r.detail && <div className="cc-detail">{r.detail}</div>}
            {r.command && <div className="app-command"><code>{r.command}</code><CopyButton value={r.command} /></div>}
          </div>
        </div>
      ))}
      {(diag.log?.length || 0) > 0 && <LogBlock title={L("后台日志最后 20 行", "The runner log's last 20 lines")} lines={diag.log!} />}
    </div>
  );
}

function LogBlock({ title, lines, open }: { title: string; lines: string[]; open?: boolean }) {
  return (
    <details className="cx-log" open={open || undefined}>
      <summary>{title}<ChevronDown /></summary>
      <pre className="mono">{lines.join("\n")}</pre>
    </details>
  );
}

/** An HTTP answer from the hub (any status: it answered). */
function answered(code?: string): boolean {
  return !!code && code !== "000" && /^\d{3}$/.test(code);
}

/** How to reach a computer this App did not connect: an SSH host or a WSL
 *  distribution of this computer. */
function PickRoute({ hubId, machine, onPicked, onCancel }: { hubId: string; machine: Machine; onPicked: (rt: RemoteRoute) => void; onCancel?: () => void }) {
  const [options, setOptions] = useState<{ label: string; sub: string; route: RemoteRoute }[] | null>(null);
  const [pick, setPick] = useState("");
  const [error, setError] = useState<unknown>(null);
  const load = () => {
    setOptions(null);
    Promise.all([appApi.sshHosts().catch(() => ({ hosts: [] })), appApi.wslDistros().catch(() => ({ supported: false, distros: [] }))]).then(([s, w]) => {
      const out = [
        ...w.distros.map((d) => ({ label: d.name, sub: "WSL", route: { via: "wsl" as const, target: d.name } })),
        ...s.hosts.map((h) => ({ label: h.name, sub: `${h.user ? h.user + "@" : ""}${h.host}${h.port && h.port !== 22 ? `:${h.port}` : ""}`, route: { via: "ssh" as const, target: h.name } })),
      ];
      setOptions(out);
      const host = (machine.hostname || "").toLowerCase();
      const guess = out.find((o) => o.label.toLowerCase() === machine.name.toLowerCase() || (host && (o.label.toLowerCase() === host || o.sub.toLowerCase().includes(host))));
      setPick(guess ? keyOf(guess.route) : "");
    });
  };
  useEffect(load, [machine.id]); // eslint-disable-line react-hooks/exhaustive-deps
  const save = () => {
    const o = options?.find((x) => keyOf(x.route) === pick);
    if (!o) return;
    setError(null);
    appApi.setMachineReach(hubId, machine.id, o.route).then((r) => onPicked(r.route)).catch(setError);
  };
  return (
    <div className="cc-pick">
      <div className="app-small-note">{L("从这台电脑用 SSH（或这台 Windows 里的 WSL）连得上它时，可以在这里检查它为什么掉线、重启它的 myco 后台、升级 myco。选它是哪一个：", "If this computer can reach it over SSH (or it is WSL on this Windows), you can check why it is away, restart its runner and upgrade myco from here. Which one is it:")}</div>
      {options === null ? <div className="app-small-note"><span className="app-spinner" />{L("正在读 SSH 配置…", "Reading your SSH config…")}</div> : options.length === 0 ? (
        <div className="app-small-note">{L("SSH 配置（~/.ssh/config）里没有主机。", "Your SSH config (~/.ssh/config) has no hosts.")}</div>
      ) : (
        <div className="cc-pick-row">
          <select className="app-select-field" value={pick} onChange={(e) => setPick(e.target.value)} aria-label={L("它是哪一个", "Which one")}>
            <option value="">{L("选一个", "Pick one")}</option>
            {options.map((o) => <option key={keyOf(o.route)} value={keyOf(o.route)}>{o.label} · {o.sub}</option>)}
          </select>
          <button type="button" className="app-btn" onClick={save} disabled={!pick}>{L("用这个", "Use this")}</button>
          {onCancel && <button type="button" className="app-btn" onClick={onCancel}>{L("取消", "Cancel")}</button>}
          <button type="button" className="app-group-action" onClick={load} title={L("重新读", "Read again")}><RefreshCw /></button>
        </div>
      )}
      <ErrorText error={error} />
    </div>
  );
}

function keyOf(rt: RemoteRoute): string {
  return `${rt.via}:${rt.target}`;
}
