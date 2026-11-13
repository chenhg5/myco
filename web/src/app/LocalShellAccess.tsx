import { useEffect, useState } from "react";
import { ShieldCheck } from "lucide-react";
import type { Machine } from "@/api";
import { appApi, type LocalState } from "@/appapi";
import { ErrorText } from "@/app/dialogs";
import { L } from "@/lib/i18n";

/**
 * Shell nodes and full-access sessions need the machine to allow them in its
 * own machine.yaml (spawn.max_access: full). When the machine is this
 * computer, the app can switch that on here, after the person confirms;
 * for any other machine it says where to do it.
 */
export function LocalShellAccess({ machine, what }: { machine?: Machine; what: "shell" | "full" }) {
  const [local, setLocal] = useState<LocalState | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let off = false;
    appApi.local().then((l) => { if (!off) setLocal(l); }).catch(() => {});
    return () => { off = true; };
  }, []);

  const here = !!machine && !!local &&
    ((local.machines || []).includes(machine.name) || (!!local.hostname && machine.hostname === local.hostname));
  const risk = what === "shell"
    ? L("agent 在这台电脑上跑命令，就和你自己坐在电脑前一样。", "Agents running commands here is just like you sitting at that computer.")
    : L("完全权限可以让它读写那台电脑上的任何文件、运行任何命令。", "Full access lets it read and write any file and run any command on that computer.");

  if (!here) {
    return (
      <div className="app-notice">
        <ShieldCheck />
        <span>{risk}{" "}{L("这需要那台电脑自己点头：在那台电脑上打开 myco App，到这里点「允许」。", "That computer has to agree first: open the myco app on it and choose Allow there.")}</span>
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
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

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
