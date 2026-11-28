import { useEffect, useState } from "react";
import { appApi, isOldHub, type BackupsView } from "@/appapi";
import type { Whoami } from "@/api";
import { CopyButton } from "@/app/copy";
import { ErrorText, Modal } from "@/app/dialogs";
import { L, E } from "@/lib/i18n";
import { bytes, cn, timeAgo } from "@/lib/utils";

/** A workspace's backups, for its owner: the workspace makes one a day and
 *  keeps the last few on its server; this computer can keep copies too (a
 *  backup on the server's disk does not survive that disk). */
export function BackupRow({ hubId, me }: { hubId: string; me?: Whoami }) {
  const owner = !!me && (me.admin || me.member?.role === "owner");
  const [view, setView] = useState<BackupsView | null>(null);
  const [hidden, setHidden] = useState(false);
  const [open, setOpen] = useState(false);
  const load = async () => {
    try {
      setView(await appApi.backups(hubId));
    } catch (e) {
      if (isOldHub(e)) setHidden(true);
    }
  };
  useEffect(() => {
    if (owner) void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hubId, owner]);
  if (!owner || hidden || !view) return null;
  const last = view.backups[0];
  return (
    <>
      <div className="app-kv" data-testid="backup-row">
        <span className="app-kv-label">{L("备份", "Backups")}</span>
        <span className="app-kv-value">
          <span className="app-status-text">
            {last ? L(E`最近一次 ${timeAgo(last.created_at)}`, E`Last ${timeAgo(last.created_at)}`) : L("还没有", "None yet")}
            {view.auto && L(" · 这台电脑也留了一份", " · copies kept here")}
          </span>
          <button type="button" className="app-link-btn" onClick={() => setOpen(true)}>{L("管理", "Manage")}</button>
        </span>
      </div>
      {open && <BackupDialog hubId={hubId} view={view} onClose={() => setOpen(false)} onChanged={load} />}
    </>
  );
}

function BackupDialog({ hubId, view, onClose, onChanged }: { hubId: string; view: BackupsView; onClose: () => void; onChanged: () => Promise<void> }) {
  const [busy, setBusy] = useState<"" | "now" | "auto" | "show">("");
  const [error, setError] = useState<unknown>(null);
  const [saved, setSaved] = useState("");
  const run = async (what: "now" | "auto" | "show", fn: () => Promise<unknown>) => {
    setBusy(what);
    setError(null);
    try {
      await fn();
      await onChanged();
    } catch (e) {
      setError(e);
    } finally {
      setBusy("");
    }
  };
  const backUpNow = () => run("now", async () => {
    const b = await appApi.createBackup(hubId);
    const r = await appApi.saveBackup(hubId, b.name);
    setSaved(r.path);
  });
  const last = view.backups[0];
  const local = view.local[0];
  return (
    <Modal open onOpenChange={(o) => !o && !busy && onClose()} title={L("工作区备份", "Workspace backups")}
      description={L(E`工作区每天自动备份一份，在服务器上保留最近 ${view.keep} 份。备份里有所有对话和文件，只有你（主人）能看到和下载。`,
        E`The workspace backs itself up once a day and keeps the last ${view.keep} on its server. A backup holds every conversation and file; only you, its owner, can see and download them.`)}>
      <div className="app-setting-rows" data-testid="backup-dialog">
        <div className="app-kv">
          <span className="app-kv-label">{L("服务器上", "On the server")}</span>
          <span className="app-kv-value">
            {last
              ? L(E`${view.backups.length} 份，最近一次 ${timeAgo(last.created_at)}（${bytes(last.size)}）`, E`${view.backups.length}, the last ${timeAgo(last.created_at)} (${bytes(last.size)})`)
              : L("还没有，第一份在工作区启动后几分钟内做好", "None yet; the first is made a few minutes after the workspace starts")}
          </span>
        </div>
        <div className="app-kv">
          <span className="app-kv-label">{L("这台电脑上", "On this computer")}</span>
          <span className="app-kv-value">
            {local
              ? L(E`${view.local.length} 份，最近一次 ${timeAgo(local.created_at)}`, E`${view.local.length}, the last ${timeAgo(local.created_at)}`)
              : L("还没有", "None yet")}
            <span className="app-kv-sub mono clip" title={view.dir}>{view.dir}</span>
          </span>
        </div>
      </div>
      <div className="app-setting-line" data-setting="backup-auto">
        <div className="app-setting-line-text">
          <b>{L("在这台电脑上也留一份", "Keep copies on this computer")}</b>
          <span>{L("App 开着时，每天把最新的备份存到这台电脑，留最近 3 份。服务器的磁盘坏了，也能从这里恢复。",
            "While the App is open, it saves the newest backup here every day and keeps the last 3: if the server's disk fails, you restore from these.")}</span>
        </div>
        <button type="button" role="switch" aria-checked={view.auto} disabled={busy !== ""}
          aria-label={view.auto ? L("不再在这台电脑上留备份", "Stop keeping copies here") : L("在这台电脑上留备份", "Keep copies here")}
          className={cn("app-switch", view.auto && "is-on")} onClick={() => void run("auto", () => appApi.setBackupAuto(hubId, !view.auto))}>
          <span className="app-switch-knob" />
        </button>
      </div>
      <details className="app-backup-restore">
        <summary>{L("怎么恢复", "How to restore")}</summary>
        <p className="app-small-note">{L("把备份文件放到服务器上，停掉工作区后运行下面的命令（用 install-hub.sh 装的服务器会自己停、自己启动）。原来的数据会挪到旁边保留，不会删掉。",
          "Put the backup file on the server and run this with the workspace stopped (a server installed with install-hub.sh stops and starts it for you). What was there is moved aside, not deleted.")}</p>
        <div className="app-command"><code>{restoreCommand(view)}</code><CopyButton value={restoreCommand(view)} /></div>
      </details>
      {saved && <p className="app-small-note" data-testid="backup-saved">{L(E`已保存到 ${saved}`, E`Saved to ${saved}`)}</p>}
      <ErrorText error={error} />
      <div className="app-modal-actions">
        {view.local.length > 0 && (
          <button type="button" className="app-btn" disabled={busy !== ""} onClick={() => void run("show", () => appApi.revealBackups(hubId))} style={{ marginRight: "auto" }}>
            {L("在文件夹中显示", "Show in folder")}
          </button>
        )}
        <button type="button" className="app-btn" onClick={onClose} disabled={busy !== ""}>{L("关闭", "Close")}</button>
        <button type="button" className="app-btn app-btn-primary" disabled={busy !== ""} onClick={() => void backUpNow()}>
          {busy === "now" ? <><span className="app-spinner" />{L("正在备份…", "Backing up…")}</> : L("立即备份并存到这台电脑", "Back up now and save here")}
        </button>
      </div>
    </Modal>
  );
}

function restoreCommand(v: BackupsView): string {
  const m = /--db (\S+)/.exec(v.restore || "");
  return `sudo myco hub restore --from <${L("备份文件", "backup file")}> --db ${m ? m[1] : "/var/lib/myco/hub.db"}`;
}
