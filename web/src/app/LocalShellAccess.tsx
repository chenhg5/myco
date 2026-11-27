import { useEffect, useState } from "react";
import { ShieldCheck } from "lucide-react";
import type { Machine } from "@/api";
import { appApi, type LocalState } from "@/appapi";
import { ErrorText } from "@/app/dialogs";
import { CopyButton } from "@/app/copy";
import { ALLOW_SHELL_COMMAND, DENY_SHELL_COMMAND, thisComputerId } from "@/app/nodes";
import type { AgentRow } from "@/app/types";
import { L } from "@/lib/i18n";

/**
 * Shell nodes and full-access sessions need the machine to allow them in its
 * own machine.yaml (spawn.max_access: full). When the machine is this
 * computer, the app can switch that on here, after the person confirms;
 * for any other machine it says where to do it.
 */
/** Restarting the background runner on this computer (an older runner
 *  reads spawn.max_access only when it starts). */
export const RESTART_RUNNER_COMMAND = "myco runner service restart";

export function LocalShellAccess({ machine, hub, what, onlyLocal, onChange, shell, agent }: {
  machine?: Machine; what: "shell" | "full"; onlyLocal?: boolean; onChange?: () => Promise<void>;
  /** the workspace the machine is in: tells exactly whether it is this computer */
  hub?: AgentRow["hub"];
  /** whether that computer allows it, as it reported (machineShell) */
  shell?: "yes" | "no" | "unknown";
  /** for one agent's Full access (run settings): its name, and whether its
   *  runner picks a new limit up by itself ("live_max_access"). Once
   *  allowed, this waits for the runner to say so (the caller stops showing
   *  this when it does), or says to restart myco here. */
  agent?: { name: string; live: boolean };
}) {
  const [local, setLocal] = useState<LocalState | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  // Allowed for an agent's Full access: since when, and a tick to re-read.
  const [since, setSince] = useState(() => Date.now());
  const [tick, setTick] = useState(0);
  const [restarting, setRestarting] = useState<"" | "busy" | "done">("");

  useEffect(() => {
    let off = false;
    appApi.local().then((l) => { if (!off) setLocal(l); }).catch(() => {});
    return () => { off = true; };
  }, []);
  // Allowed, and it still runs under less: ask again every 2s (for a minute)
  // until its runner reports the new limit, which takes this away.
  const waiting = what === "full" && !!agent && !!local?.shell_allowed;
  useEffect(() => {
    if (!waiting || Date.now() - since > 60_000) return;
    const t = window.setTimeout(() => { void onChange?.(); setTick((n) => n + 1); }, 2_000);
    return () => window.clearTimeout(t);
  }, [waiting, tick, since]); // eslint-disable-line react-hooks/exhaustive-deps

  const here = !!machine && !!local && (hub ? thisComputerId(hub, local) === machine.id
    : (local.machines || []).includes(machine.name) || (!!local.hostname && machine.hostname === local.hostname));
  const risk = what === "shell"
    ? L("agent 在这台电脑上跑命令，就和你自己坐在电脑前一样。", "Agents running commands here is just like you sitting at that computer.")
    : L("完全权限可以让它读写那台电脑上的任何文件、运行任何命令。", "Full access lets it read and write any file and run any command on that computer.");

  if (!here) {
    // (an agent's Full access on another computer: that computer decides)
    if (onlyLocal && what === "full" && agent) {
      return <span>{L("要更高，在那台电脑上把 ~/.myco/machine.yaml 里的 spawn.max_access 设成 full，或在它的 myco App 里允许。",
        "For more, set spawn.max_access: full in ~/.myco/machine.yaml on that computer, or allow it in the myco app there.")}</span>;
    }
    if (onlyLocal) return null;
    // Another computer decides for itself: the App can only say how.
    if (shell === "yes") {
      return (
        <div className="app-small-note" style={{ marginTop: 6 }} data-testid="shell-allowed">
          {L("那台电脑已允许 agent 在上面跑命令。要关掉，在那台电脑上运行 ", "That computer lets agents run commands on it. To stop it, run on that computer ")}
          <code className="mono">{DENY_SHELL_COMMAND}</code>
        </div>
      );
    }
    return (
      <div className="app-notice" style={{ alignItems: "flex-start" }} data-testid="shell-ask">
        <ShieldCheck />
        <div style={{ flex: 1, minWidth: 0 }}>
          <span>{risk}{" "}{shell === "no"
            ? L("那台电脑还没允许。在那台电脑上运行这条命令（或在它的 myco App 里点「允许」），允许后这里会自己更新：",
              "That computer hasn't allowed it. Run this on it (or choose Allow in the myco app there); this updates by itself once it has:")
            : L("要那台电脑自己允许：在那台电脑上运行这条命令，或在它的 myco App 里点「允许」。",
              "That computer has to allow it: run this on it, or choose Allow in the myco app there.")}</span>
          <div className="app-command" style={{ marginTop: 8 }}>
            <code>{ALLOW_SHELL_COMMAND}</code>
            <CopyButton value={ALLOW_SHELL_COMMAND} />
          </div>
        </div>
      </div>
    );
  }

  const apply = async (allow: boolean) => {
    setBusy(true);
    setError(null);
    try {
      const r = await appApi.shellAccess(allow);
      setLocal({ ...local!, shell_allowed: r.shell_allowed });
      setConfirming(false);
      setSince(Date.now());
      setTick(0);
      await onChange?.();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  // Allowed for this agent's Full access, and it does not run under it yet:
  // a runner that reloads gets a few seconds; an older one (or one still
  // without it after half a minute) needs myco here restarted, said plainly.
  if (local!.shell_allowed && what === "full" && agent) {
    const late = !agent.live || Date.now() - since > 30_000;
    const restart = async () => {
      setRestarting("busy");
      setError(null);
      try {
        const r = await appApi.restartRunner();
        if (r.runner !== "running") throw new Error(r.runner);
        setRestarting("done");
        setSince(Date.now());
        setTick(0);
      } catch (e) {
        setError(e);
        setRestarting("");
      }
    };
    if (!late) {
      return (
        <div className="app-small-note app-full-pending" style={{ marginTop: 6 }} data-testid="full-access-pending">
          <span className="app-spinner" aria-hidden />
          {L(`已允许。${agent.name} 正在换上新的权限，几秒后这里就能选「完全放开」。`,
            `Allowed. ${agent.name} is picking up the new limit; Full can be chosen here in a few seconds.`)}
        </div>
      );
    }
    return (
      <div className="app-notice" style={{ alignItems: "flex-start" }} data-testid="full-access-restart">
        <ShieldCheck />
        <div style={{ flex: 1, minWidth: 0 }}>
          <span>{restarting === "done"
            ? L(`已重启这台电脑上的 myco，${agent.name} 重新上线后就能选「完全放开」。`, `myco restarted on this computer; Full can be chosen once ${agent.name} is back online.`)
            : agent.live
              ? L(`已允许，但 ${agent.name} 还没换上新的权限。重启这台电脑上的 myco 后台就能生效：`,
                `Allowed, but ${agent.name} has not picked up the new limit yet. Restart myco on this computer to apply it:`)
              : L(`已允许，但 ${agent.name} 用的 myco 版本要重启后才生效。重启这台电脑上的 myco 后台：`,
                `Allowed, but ${agent.name}'s myco applies it only after a restart. Restart myco on this computer:`)}</span>
          {restarting !== "done" && (
            <>
              <div style={{ marginTop: 8 }}>
                <button type="button" className="app-btn app-btn-small" onClick={() => void restart()} disabled={restarting === "busy"} data-testid="full-access-restart-now">
                  {restarting === "busy" ? L("正在重启…", "Restarting…") : L("重启 myco 后台", "Restart myco")}
                </button>
              </div>
              <div className="app-command" style={{ marginTop: 8 }}>
                <code>{RESTART_RUNNER_COMMAND}</code>
                <CopyButton value={RESTART_RUNNER_COMMAND} />
              </div>
            </>
          )}
          <ErrorText error={error} />
        </div>
      </div>
    );
  }

  if (local!.shell_allowed) {
    return (
      <div className="app-small-note" style={{ marginTop: 6 }}>
        {L("这台电脑已允许 agent 在上面跑命令。", "This computer lets agents run commands on it.")}{" "}
        <button type="button" className="app-summary-action" onClick={() => void apply(false)} disabled={busy}>
          {L("不再允许", "Stop allowing")}
        </button>
        <ErrorText error={error} />
      </div>
    );
  }

  return (
    <div className="app-notice" style={{ alignItems: "flex-start" }}>
      <ShieldCheck />
      <div style={{ flex: 1 }}>
        {!confirming ? (
          <>
            <span>{risk}{" "}{L("这台电脑还没允许。", "This computer hasn't allowed it yet.")}</span>
            <div style={{ marginTop: 8 }}>
              <button type="button" className="app-btn app-btn-small" onClick={() => setConfirming(true)}>
                {L("在这台电脑上允许", "Allow on this computer")}
              </button>
            </div>
          </>
        ) : (
          <>
            <div style={{ fontWeight: 600, marginBottom: 4 }}>{L("让 agent 在这台电脑上跑命令？", "Let agents run commands on this computer?")}</div>
            <div>
              {L("允许后，你项目里的 agent 能用你的身份在这台电脑上运行命令、读写文件，就和你自己坐在电脑前一样。只给你信任的 agent 用。每条命令你都看得到，随时可以在这里关掉。",
                "Once allowed, the agents in your project can run commands and read or write files on this computer as you, just as if you were sitting at it. Only let agents you trust use it. You see every command, and you can switch this off here at any time.")}
            </div>
            <ErrorText error={error} />
            <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
              <button type="button" className="app-btn app-btn-small" onClick={() => setConfirming(false)} disabled={busy}>{L("取消", "Cancel")}</button>
              <button type="button" className="app-btn app-btn-small app-btn-primary" onClick={() => void apply(true)} disabled={busy}>
                {busy ? L("处理中…", "Working…") : L("允许", "Allow")}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
