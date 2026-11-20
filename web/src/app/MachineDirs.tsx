import { useEffect, useState } from "react";
import { FolderPlus, X } from "lucide-react";
import type { Machine } from "@/api";
import { appApi, type LocalState, type MachineInfo, type RemoteRoute } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { thisComputerId } from "@/app/nodes";
import { forgetMachineInfo } from "@/app/machineInfoCache";
import { ErrorText } from "@/app/dialogs";
import { CopyButton } from "@/app/copy";
import { L } from "@/lib/i18n";

/**
 * The folders agents may start in on a computer (spawn.dirs in its
 * machine.yaml). They are that computer's own rule, so the workspace never
 * changes them; this App does when it reaches the computer itself: this
 * computer, or one it connected over SSH or into WSL. For any other it
 * gives the command to run there. A change applies to the next agent
 * started there; nothing restarts.
 */
export function MachineDirs({ hubId, hub, machine, info, onChanged, prefill }: {
  hubId: string; hub: AgentRow["hub"]; machine: Machine; info: MachineInfo;
  onChanged: (dirs: string[]) => void;
  /** a folder to offer (the one an agent was refused) */
  prefill?: string;
}) {
  const [local, setLocal] = useState<LocalState | null>(null);
  const [route, setRoute] = useState<RemoteRoute | null | undefined>(undefined);
  const [adding, setAdding] = useState(prefill || "");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [said, setSaid] = useState("");
  useEffect(() => {
    let off = false;
    appApi.local().then((l) => { if (!off) setLocal(l); }).catch(() => {});
    appApi.machineReach(hubId, machine.id).then((r) => { if (!off) setRoute(r.route); }, () => { if (!off) setRoute(null); });
    return () => { off = true; };
  }, [hubId, machine.id]);
  useEffect(() => { if (prefill) setAdding(prefill); }, [prefill]);
  const here = !!local && thisComputerId(hub, local) === machine.id;
  const can = here || !!route;
  const dirs = info.dirs;
  const homeAll = dirs.length === 1 && dirs[0] === "~";
  const where = here ? L("这台电脑", "this computer") : route?.via === "wsl" ? `WSL ${route.target}` : route ? `SSH ${route.target}` : "";
  const whereZh = here ? "这台电脑" : ` ${where} `;

  const change = async (c: { add?: string[]; remove?: string[] }) => {
    setBusy(c.add ? "add" : (c.remove || [])[0] || "x"); setError(null); setSaid("");
    try {
      const r = await appApi.setMachineDirs(hubId, machine.id, { ...c, here });
      forgetMachineInfo(hubId, machine.id);
      onChanged(r.dirs);
      if (r.already_allowed?.length) setSaid(L(`${r.already_allowed.join("、")} 本来就在允许的文件夹里。`, `${r.already_allowed.join(", ")} was allowed already.`));
      if (c.add) setAdding("");
    } catch (e) { setError(e); } finally { setBusy(""); }
  };
  const command = (d: string) => `myco machine dirs --add '${d.replace(/'/g, "'\\''")}'`;
  const value = adding.trim();

  return (
    <div className="mdirs" data-testid="machine-dirs">
      <div className="mdirs-list">
        {dirs.map((d) => (
          <span key={d} className="pcd-chip mono mdirs-chip">
            {d === "~" ? L("~（整个用户目录）", "~ (all of home)") : d}
            {can && dirs.length > 1 && (
              <button type="button" className="mdirs-x" disabled={!!busy} aria-label={L(`不再允许 ${d}`, `Stop allowing ${d}`)} title={L("不再允许", "Stop allowing")}
                onClick={() => void change({ remove: [d] })}><X /></button>
            )}
          </span>
        ))}
      </div>
      {can ? (
        <form className="mdirs-add" onSubmit={(e) => { e.preventDefault(); if (value) void change({ add: [value] }); }}>
          <input className="app-input mono" value={adding} onChange={(e) => setAdding(e.target.value)} disabled={!!busy}
            placeholder={L("再允许一个文件夹，比如 ~/code/tapnow", "Allow another folder, e.g. ~/code/tapnow")} aria-label={L("要允许的文件夹", "Folder to allow")} spellCheck={false} />
          <button type="submit" className="app-btn app-btn-small" disabled={!value || !!busy}><FolderPlus />{busy === "add" ? L("正在改…", "Saving…") : L("允许", "Allow")}</button>
        </form>
      ) : route === null && local ? (
        <div className="mdirs-far">
          <span>{machine.online
            ? L("它在线，但这项只能在它自己那里改；这个 App 不在它上面，也没用 SSH / WSL 连它。在它上面运行：", "It is online, but this is changed on that computer itself; this App is not on it and has no SSH / WSL link to it. Run there:")
            : L("要在那台电脑上改，运行：", "Change it on that computer; run there:")}</span>
          <code className="mono">{command(value || "~/code/项目")}</code>
          <CopyButton value={command(value || "~/code/项目")} />
        </div>
      ) : null}
      <ErrorText error={error} />
      <p className="app-small-note mdirs-note">
        {said ? `${said} ` : ""}
        {can
          ? L(`在${whereZh}上的 ~/.myco/machine.yaml 里改（spawn.dirs），下一个新建的 agent 就按新的来，不用重启。${homeAll ? "现在整个用户目录都允许。" : ""}`,
            `Changed in ~/.myco/machine.yaml on ${where} (spawn.dirs); the next agent started there follows it, no restart.${homeAll ? " All of home is allowed now." : ""}`)
          : L("「在线」是说它连着工作区；允许哪些文件夹是那台电脑自己的规定，工作区替它改不了，这样别人拿到工作区的权限也放不开你的电脑。在它上面运行上面的命令，或用它上面的 myco App 改（WSL 的话，用同一台 Windows 上的 App）；这个 App 用 SSH 或 WSL 连上它之后，这里也能改。",
            "Online means it is connected to the workspace; which folders it allows is that computer's own rule, which the workspace cannot change for it, so access to the workspace cannot open up your computer. Run the command there or use the myco app on it (for WSL, the app on the same Windows PC); once this App reaches it over SSH or WSL, it can change it here too.")}
      </p>
    </div>
  );
}

/** Beside a folder a computer does not allow (新建 agent): allow it there
 *  at once when this App reaches that computer, else the command to run. */
export function AllowFolder({ hubId, hub, machine, dir, onAllowed }: {
  hubId: string; hub: AgentRow["hub"]; machine: Machine; dir: string; onAllowed: (dirs: string[]) => void;
}) {
  const [local, setLocal] = useState<LocalState | null>(null);
  const [route, setRoute] = useState<RemoteRoute | null | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    let off = false;
    appApi.local().then((l) => { if (!off) setLocal(l); }).catch(() => {});
    appApi.machineReach(hubId, machine.id).then((r) => { if (!off) setRoute(r.route); }, () => { if (!off) setRoute(null); });
    return () => { off = true; };
  }, [hubId, machine.id]);
  const here = !!local && thisComputerId(hub, local) === machine.id;
  const command = `myco machine dirs --add '${dir.replace(/'/g, "'\\''")}'`;
  if (here || route) {
    return (
      <span className="mdirs-allow">
        <button type="button" className="app-btn app-btn-small" disabled={busy} data-testid="allow-folder" onClick={async () => {
          setBusy(true); setError(null);
          try {
            const r = await appApi.setMachineDirs(hubId, machine.id, { add: [dir], here });
            forgetMachineInfo(hubId, machine.id);
            onAllowed(r.dirs);
          } catch (e) { setError(e); } finally { setBusy(false); }
        }}><FolderPlus />{busy ? L("正在改…", "Saving…") : L(`允许在 ${dir} 开工`, `Allow ${dir}`)}</button>
        <ErrorText error={error} />
      </span>
    );
  }
  if (route === undefined || !local) return null;
  return (
    <span className="mdirs-far">
      <span>{L("在那台电脑上运行：", "Run on that computer:")}</span>
      <code className="mono">{command}</code>
      <CopyButton value={command} />
    </span>
  );
}
