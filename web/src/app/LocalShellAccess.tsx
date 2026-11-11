import { useEffect, useState } from "react";
import { AlertTriangle } from "lucide-react";
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
    ? L("选中的节点能在那台机器上以它的用户身份运行任何命令。", "The selected nodes can run any command on that machine as its user.")
    : L("完全权限可以让它读写那台机器上的任何文件、运行任何命令。", "Full access lets it read and write any file and run any command on that machine.");

  if (!here) {
    return (
      <div className="app-warn">
        <AlertTriangle />
        <span>{risk}{" "}{L("需要那台机器在本地允许：在它的 ~/.myco/machine.yaml 里加上 spawn: {max_access: full}，或者在那台电脑的 myco App 里打开。", "That machine must allow it locally: add spawn: {max_access: full} to its ~/.myco/machine.yaml, or switch it on in the myco app on that computer.")}</span>
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
        {L("这台电脑已允许 Shell 节点和完全权限的会话。", "This computer allows shell nodes and full-access sessions.")}{" "}
        <button type="button" className="app-summary-action" onClick={() => void apply(false)} disabled={busy}>
          {L("不再允许", "Stop allowing")}
        </button>
        <ErrorText error={error} />
      </div>
    );
  }

  return (
    <div className="app-warn" style={{ alignItems: "flex-start" }}>
      <AlertTriangle />
      <div style={{ flex: 1 }}>
        {!confirming ? (
          <>
            <span>{risk}{" "}{L("这台电脑还没有允许。", "This computer does not allow it yet.")}</span>
            <div style={{ marginTop: 8 }}>
              <button type="button" className="app-btn app-btn-small" onClick={() => setConfirming(true)}>
                {L("在这台电脑上允许", "Allow on this computer")}
              </button>
            </div>
          </>
        ) : (
          <>
            <div style={{ fontWeight: 600, marginBottom: 4 }}>{L("允许别的节点在这台电脑上执行命令？", "Let other nodes run commands on this computer?")}</div>
            <div>
              {L("允许后，这台电脑可以添加 Shell 节点和完全权限的会话。你选中的节点就能以你的用户身份运行任何命令、读写任何文件，和你自己坐在电脑前一样；它不是沙箱。只选你信任的节点。每条命令都会记录，随时可以在这里关掉（已添加的节点要在节点页删除）。",
                "Once allowed, this computer can have shell nodes and full-access sessions. The nodes you choose can then run any command and read or write any file as your user, as if you were at this computer; it is not a sandbox. Only choose nodes you trust. Every command is recorded, and you can switch this off here at any time (remove nodes already added on the Nodes page).")}
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
