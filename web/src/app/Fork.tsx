import { useEffect, useState } from "react";
import type { Project } from "@/api";
import { appApi } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { ErrorText, Modal } from "@/app/dialogs";
import { addToProject } from "@/app/AddToProject";
import { canManage, isInbox, machineOfRow, nodeName } from "@/app/nodes";
import { L } from "@/lib/i18n";

/*
 * 复制一份: a new agent whose session is a copy of this one's (Claude Code
 * --fork-session, a Codex fork), on the same computer and folder. It
 * remembers what was said so far; the original goes on as it was. Any
 * menu opens it with:
 *   openFork({ hubId, name, project? })
 * (AppMode shows the dialog and opens the copy's chat when it is made).
 */

export type ForkTarget = { hubId: string; name: string; project?: Project };

export function openFork(t: ForkTarget) {
  window.dispatchEvent(new CustomEvent("myco-fork", { detail: t }));
}

/** Whether the agent's session can be copied from here, and if not shown
 *  at all (hidden) or shown greyed out (why). */
export function forkEntry(row: AgentRow | undefined): { show: boolean; why?: string } {
  const a = row?.st.agent;
  if (!row || !a || isInbox(a)) return { show: false };
  const cli = a.binding.harness;
  if ((cli !== "claude" && cli !== "codex") || !a.binding.session_id || !a.binding.workdir) return { show: false };
  const machine = machineOfRow(row);
  if (!machine) return { show: false };
  if (!canManage(row.hub, machine.owner_id)) return { show: true, why: L("只有那台电脑的主人能在上面开会话", "Only that computer's owner can start sessions on it") };
  if (!machine.online) return { show: true, why: L(`「${machine.name}」不在线`, `“${machine.name}” is away`) };
  if (!machine.can_spawn) return { show: true, why: L(`「${machine.name}」现在开不了新会话`, `“${machine.name}” cannot start sessions now`) };
  return { show: true };
}

/** The menu item for a node's right-click menu (none where it does not apply). */
export function forkItem(row: AgentRow | undefined, project?: Project): { label: string; onSelect: () => void; disabled?: boolean }[] {
  const e = forkEntry(row);
  if (!row || !e.show) return [];
  return [{ label: e.why ? `${L("复制一份", "Fork a copy")}（${e.why}）` : L("复制一份", "Fork a copy"), disabled: !!e.why,
    onSelect: () => openFork({ hubId: row.hub.id, name: row.st.agent.name, project }) }];
}

/** Makes the copy: a new agent on the same computer and folder with a copy
 *  of the session; its name is returned. */
export async function forkAgent(row: AgentRow, displayName?: string): Promise<string> {
  const a = row.st.agent;
  const machine = machineOfRow(row);
  if (!machine) throw new Error(L("不知道它在哪台电脑上", "It is not known which computer it is on"));
  const names = new Set((row.hub.overview?.agents || []).map((x) => x.agent.name));
  let n = 2;
  while (names.has(`${a.name}-${n}`)) n++;
  const r = await appApi.spawn(row.hub.id, machine.id, {
    cli: a.binding.harness as "claude" | "codex", dir: a.binding.workdir || "", fork_from: a.binding.session_id,
    name: `${a.name}-${n}`, description: a.description || undefined,
    display_name: displayName || `${nodeName(a, row.hub)} ${L("副本", "copy")}`,
    ...(a.runtime?.access ? { access: a.runtime.access } : {}),
  });
  return r.agent || `${a.name}-${n}`;
}

export function ForkDialog({ target, rows, onClose, onDone }: {
  target: ForkTarget | null;
  rows: AgentRow[];
  onClose: () => void;
  /** the copy is made (and in the project when asked): open its chat */
  onDone: (hubId: string, agent: string, project?: Project) => void;
}) {
  const row = target ? rows.find((r) => r.hub.id === target.hubId && r.st.agent.name === target.name) : undefined;
  const [name, setName] = useState("");
  const [inProject, setInProject] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    if (!target) return;
    setName(row ? `${nodeName(row.st.agent, row.hub)} ${L("副本", "copy")}` : "");
    setInProject(true);
    setBusy(false);
    setError(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target?.hubId, target?.name]);
  if (!target || !row) return null;
  const machine = machineOfRow(row);
  const why = forkEntry(row).why;
  const project = target.project;
  const go = async (e?: React.FormEvent) => {
    e?.preventDefault();
    if (busy || why) return;
    setBusy(true);
    setError(null);
    try {
      const copy = await forkAgent(row, name.trim() || undefined);
      const into = project && inProject ? project : undefined;
      if (into) await addToProject(row.hub.id, into, rows, copy).catch(() => undefined);
      onDone(row.hub.id, copy, into);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open onOpenChange={(o) => { if (!o && !busy) onClose(); }} title={L(`复制一份「${nodeName(row.st.agent, row.hub)}」`, `Fork “${nodeName(row.st.agent, row.hub)}”`)}
      description={L(
        `复制它现在的会话，做成一个新的 agent：之前聊过的它都记得，在同一台电脑（${machine?.name || ""}）、同一个文件夹里干活。原来的照常用，两边互不影响。`,
        `Its session is copied into a new agent: it remembers everything said so far and works on the same computer (${machine?.name || ""}) in the same folder. The original carries on; the two do not affect each other.`)}>
      <form onSubmit={(e) => void go(e)} data-testid="fork-dialog">
        <label className="app-field">
          <span className="app-field-label">{L("名字", "Name")}</span>
          <input className="app-input" value={name} onChange={(e) => setName(e.target.value)} maxLength={40} autoFocus />
        </label>
        <div className="app-small-note mono" title={row.st.agent.binding.workdir}>{row.st.agent.binding.workdir}</div>
        {project && (
          <label className="app-check-line">
            <input type="checkbox" checked={inProject} onChange={(e) => setInProject(e.target.checked)} />
            <span>{L(`也放进「${project.title || project.name}」`, `Also put it in “${project.title || project.name}”`)}</span>
          </label>
        )}
        {why && <div className="app-small-note">{why}</div>}
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={onClose} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="submit" className="app-btn app-btn-primary" disabled={busy || !!why}>
            {busy ? <><span className="app-spinner" />{L("正在复制…", "Forking…")}</> : L("复制", "Fork")}
          </button>
        </div>
      </form>
    </Modal>
  );
}
