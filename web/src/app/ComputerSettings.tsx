import { useEffect, useMemo, useRef, useState } from "react";
import { Terminal, Trash2 } from "lucide-react";
import type { Project } from "@/api";
import { appApi } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { agentKind, nodeName } from "@/app/nodes";
import { Avatar, whereOf } from "@/app/persona";
import { projectsOf } from "@/app/Members";
import { isNobody, nobody } from "@/app/AddToProject";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { L, E } from "@/lib/i18n";
import { senderEdit, removedSenders } from "@/lib/allow-from";
import { cn } from "@/lib/utils";

type Projects = { hubId: string; p: Project }[];

/**
 * One computer (a folder on a computer, a shell node), everything about it
 * in one place: which agents may run commands there, which projects have
 * it, its terminal, and removing it. Opened from the sidebar (right click,
 * or the ⋯ on hover), the Computers page, and the terminal's header.
 */
export function ComputerSettings({ row, focus, rows, projects, onClose, onChanged, onOpenTerminal }: {
  /** the computer; none: closed */
  row: AgentRow | null;
  /** what it was opened for (the sidebar's menu): its name, or removing it */
  focus?: "name" | "remove";
  rows: AgentRow[];
  projects: Projects;
  onClose: () => void;
  onChanged: () => void;
  onOpenTerminal: (r: AgentRow) => void;
}) {
  const a = row?.st.agent;
  const hubId = row?.hub.id || "";
  // The workspace's agents (not people, not other computers).
  const agents = useMemo(() => rows.filter((r) => r.hub.id === hubId && agentKind(r.st.agent) === "agent")
    .sort((x, y) => nodeName(x.st.agent, x.hub).localeCompare(nodeName(y.st.agent, y.hub))), [rows, hubId]);
  const anyone = !!a?.allow_from.includes("*");
  const [allow, setAllow] = useState<string[]>([]);
  const [allowBefore, setAllowBefore] = useState<string[]>([]);
  const [confirmAllow, setConfirmAllow] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [saved, setSaved] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [adding, setAdding] = useState("");
  const [title, setTitle] = useState("");
  const [named, setNamed] = useState(false);
  const nameRef = useRef<HTMLInputElement>(null);
  const key = a ? `${hubId}/${a.name}` : "";
  useEffect(() => {
    if (!a) return;
    setAllowBefore(a.allow_from);
    setConfirmAllow(false);
    setAllow(a.allow_from.filter((n) => n !== "*" && !isNobody(n)));
    setError(null);
    setSaved(false);
    setRemoving(focus === "remove");
    setAdding("");
    setTitle(a.display_name || "");
    setNamed(false);
    if (focus === "name") setTimeout(() => { nameRef.current?.focus(); nameRef.current?.select(); }, 50);
    // Only when another computer is opened: a refresh must not undo ticks.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  if (!row || !a) return null;

  const inProjects = projectsOf(row, projects);
  const others = projects.filter((x) => x.hubId === hubId && !x.p.archived && !inProjects.some((p) => p.name === x.p.name));
  const before = a.allow_from.filter((n) => n !== "*" && !isNobody(n));
  const changed = anyone || allow.length !== before.length || allow.some((n) => !before.includes(n));
  const toggle = (n: string) => { setSaved(false); setAllow((s) => (s.includes(n) ? s.filter((x) => x !== n) : [...s, n])); };

  const run = async (f: () => Promise<unknown>) => {
    setBusy(true);
    setError(null);
    try {
      await f();
      onChanged();
      return true;
    } catch (e) {
      setError(e);
      return false;
    } finally {
      setBusy(false);
    }
  };
  // Nobody ticked: a placeholder no agent is called, so none may.
  const removals = removedSenders(allowBefore, allow);
  const commitAllow = () => run(async () => {
    const result = await appApi.changeAllowFrom(hubId, a.name, senderEdit(allowBefore, allow, nobody()));
    setAllowBefore(result.allow_from);
    setAllow(result.allow_from.filter((n) => n !== "*" && !isNobody(n)));
  }).then((ok) => { if (ok) { setConfirmAllow(false); setSaved(true); } });
  const save = () => removals.length ? setConfirmAllow(true) : void commitAllow();
  // Into a project: it is in it, and the project's agents may use it too.
  const addTo = (name: string) => run(async () => {
    const p = projects.find((x) => x.hubId === hubId && x.p.name === name)?.p;
    await appApi.putProjectMember(hubId, name, a.name);
    const theirs = (p?.members || []).map((m) => m.name).filter((n) => agents.some((r) => r.st.agent.name === n));
    if (!anyone && theirs.some((n) => !allow.includes(n))) {
      const next = [...new Set([...allow, ...theirs])];
      const result = await appApi.addAllowedSenders(hubId, a.name, theirs);
      setAllowBefore(result.allow_from);
      setAllow(next);
    }
    setAdding("");
  });
  const leave = (name: string) => run(() => appApi.removeProjectMember(hubId, name, a.name));
  // Its name as people see it (the address stays).
  const rename = () => run(() => appApi.updateProfile(hubId, a.name, { display_name: title.trim() })).then((ok) => ok && setNamed(true));
  const remove = () => run(() => appApi.removeAgent(hubId, a.name)).then((ok) => { if (ok) { setRemoving(false); onClose(); } });

  return (
    <>
      <Modal open={!removing && !confirmAllow} onOpenChange={(o) => !o && !busy && onClose()} title={nodeName(a, row.hub)}
        description={[whereOf(row), row.st.state === "offline" ? L("不在线", "Away") : L("在线", "Online")].filter(Boolean).join(" · ")}
        className="pcs">
        <div data-testid="computer-settings">
          <form className="app-field" onSubmit={(e) => { e.preventDefault(); void rename(); }}>
            <span className="app-field-label">{L("名字", "Name")}</span>
            <div className="pcs-name">
              <input ref={nameRef} className="app-input" value={title} onChange={(e) => { setTitle(e.target.value); setNamed(false); }}
                placeholder={a.name} disabled={busy} aria-label={L("名字", "Name")} maxLength={60} />
              <button type="submit" className="app-btn app-btn-small" disabled={busy || title.trim() === (a.display_name || "")}>{L("改名", "Rename")}</button>
              {named && title.trim() === (a.display_name || "") && <span className="tr-tried">{L("已改", "Renamed")}</span>}
            </div>
          </form>
          <div className="app-field">
            <span className="app-field-label">{L("谁能在这里跑命令", "Who can run commands here")}</span>
            {anyone && <div className="app-small-note">{L("现在任何 agent 都能用它。勾选几个，就只给它们用。", "Any agent can use it now. Tick some to keep it to them.")}</div>}
            {agents.length === 0 ? (
              <div className="app-small-note">{L("这个工作区还没有 agent。", "No agents in this workspace yet.")}</div>
            ) : (
              <div className="pcs-agents">
                {agents.map((r) => {
                  const n = r.st.agent.name;
                  const on = allow.includes(n);
                  return (
                    <label key={n} className={cn("pcs-agent", on && "is-on")}>
                      <input type="checkbox" checked={on} onChange={() => toggle(n)} disabled={busy} />
                      <Avatar agent={r.st.agent} size={22} badge />
                      <span className="pcs-agent-name">{nodeName(r.st.agent, r.hub)}</span>
                    </label>
                  );
                })}
              </div>
            )}
            <div className="pcs-save">
              <span className="app-small-note">{allow.length === 0
                ? L("一个都不勾：只有你自己能在终端里用。", "None ticked: only you can use it, in its terminal.")
                : L(E`勾上的 ${allow.length} 个 agent 能用你的身份在这个文件夹里跑命令。`, E`The ${allow.length} ticked can run commands in this folder as you.`)}</span>
              <button type="button" className="app-btn app-btn-small app-btn-primary" onClick={() => void save()} disabled={busy || !changed}>{L("保存", "Save")}</button>
              {saved && !changed && <span className="tr-tried">{L("已保存", "Saved")}</span>}
            </div>
          </div>

          <div className="app-field">
            <span className="app-field-label">{L("在哪些项目里", "In which projects")}</span>
            <div className="pcs-projects">
              {inProjects.map((p) => (
                <span key={p.name} className="pcs-project">
                  {p.title || p.name}
                  <button type="button" className="pcs-project-x" onClick={() => void leave(p.name)} disabled={busy}
                    aria-label={L(E`从「${p.title || p.name}」拿出来`, E`Take it out of ${p.title || p.name}`)} title={L("从这个项目拿出来", "Take it out of this project")}>×</button>
                </span>
              ))}
              {inProjects.length === 0 && <span className="app-small-note">{L("还不在任何项目里。", "Not in a project yet.")}</span>}
              {others.length > 0 && (
                <select className="app-select-field pcs-add" value={adding} disabled={busy}
                  onChange={(e) => { setAdding(e.target.value); if (e.target.value) void addTo(e.target.value); }}
                  aria-label={L("加进项目", "Put it in a project")}>
                  <option value="">{L("加进项目…", "Put it in a project…")}</option>
                  {others.map((x) => <option key={x.p.name} value={x.p.name}>{x.p.title || x.p.name}</option>)}
                </select>
              )}
            </div>
          </div>

          <ErrorText error={error} />
          <div className="app-modal-actions">
            <button type="button" className="app-btn app-btn-quiet pcs-remove" onClick={() => setRemoving(true)} disabled={busy}><Trash2 />{L("移除这个终端", "Remove this terminal")}</button>
            <button type="button" className="app-btn" onClick={() => { onOpenTerminal(row); onClose(); }}><Terminal />{L("打开终端", "Open the terminal")}</button>
            <button type="button" className="app-btn app-btn-primary" onClick={onClose} disabled={busy}>{L("好", "OK")}</button>
          </div>
        </div>
      </Modal>
      <ConfirmDialog open={confirmAllow} onOpenChange={setConfirmAllow}
        title={L("移除这些派任务权限？", "Remove these sender permissions?")}
        description={removals.map((n) => n === "*" ? L("任何 agent", "Any agent") : n).join(", ")}
        confirmLabel={L("保存", "Save")} onConfirm={() => void commitAllow()} busy={busy}
        error={error ? <ErrorText error={error} /> : undefined} />
      <ConfirmDialog open={removing} onOpenChange={(o) => { if (!o) { setRemoving(false); if (focus === "remove") onClose(); } }}
        title={L("移除这个终端？", "Remove this terminal?")}
        subject={<><Avatar agent={a} size={24} badge={false} /><span>{nodeName(a, row.hub)}</span></>}
        description={L("agent 就不能再在这里跑命令了。文件夹里的东西不会动，聊过的内容都保留。", "Agents can no longer run commands here. Nothing in the folder is touched, and the conversations are kept.")}
        confirmLabel={L("移除", "Remove")} onConfirm={() => void remove()} busy={busy} error={error ? <ErrorText error={error} /> : undefined} />
    </>
  );
}
