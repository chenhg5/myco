import { useEffect, useMemo, useRef, useState } from "react";
import { recall, remember } from "@/lib/memo";
import { Check, ChevronDown, Download, ExternalLink, File as FileIcon, FileImage, FileText, Lock, MoreHorizontal, Pencil, Plus, Upload, Users } from "lucide-react";
import { IconChat, IconExchange, IconNewChat, IconNodes } from "@/components/icons";
import { useContextMenu } from "@/app/ContextMenu";
import { appApi, isOldHub, type RecentItem } from "@/appapi";
import type { ConversationRow } from "@/app/ConversationsPage";
import { summarize } from "@/app/summary";
import type { Project, ProjectDoc, ProjectMember } from "@/api";
import type { AgentRow } from "@/app/types";
import { agentKind, canManage, isMe, nodeIcon, nodeName } from "@/app/nodes";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { Markdown } from "@/app/Markdown";
import { NodePicker } from "@/app/NodePicker";
import { RowsSkeleton, TextSkeleton } from "@/app/Skeleton";
import { stateMeta } from "@/components/status";
import { cn, timeAgo } from "@/lib/utils";
import { L } from "@/lib/i18n";

export interface ProjectRef { hubId: string; name: string }

/**
 * Runs fn when a dialog opens, never while it stays open: the page re-renders
 * every second and polls the project, and a reset then would throw away what
 * the person is typing or picking.
 */
function useOnOpen(open: boolean, fn: () => void) {
  const was = useRef(false);
  const latest = useRef(fn);
  latest.current = fn;
  useEffect(() => {
    if (open && !was.current) latest.current();
    was.current = open;
  }, [open]);
}

type Dialog =
  | null
  | { kind: "about" }
  | { kind: "brief" }
  | { kind: "links" }
  | { kind: "add" }
  | { kind: "member"; member: ProjectMember }
  | { kind: "doc"; doc: ProjectDoc }
  | { kind: "settings" }
  | { kind: "delete" };

/** One project: its goal, members, brief, shared documents and links. */
export function ProjectPage({ target, reloadKey, hubName, rows, now, onOpenNode, onNewNode, onChanged, onDeleted, onOpenConversation, onAllConversations, onNewConversation }: {
  target: ProjectRef;
  /** Changes when something outside the page changed the project. */
  reloadKey?: number;
  hubName: string;
  rows: AgentRow[];
  now: number;
  onOpenNode: (row: AgentRow) => void;
  /** Opens 添加节点; a node added there joins this project. */
  onNewNode: () => void;
  onChanged: () => void;
  onDeleted: () => void;
  onOpenConversation: (row: ConversationRow) => void;
  /** The conversations page, filtered to this project. */
  onAllConversations: () => void;
  /** A new conversation in this project (with a member, when given). */
  onNewConversation: (agent?: string) => void;
}) {
  const memoKey = `project\u0000${target.hubId}\u0000${target.name}`;
  const [p, setPState] = useState<Project | null>(() => recall<Project>(memoKey) ?? null);
  const setP = (v: Project | null) => { if (v) remember(memoKey, v); setPState(v); };
  const [error, setError] = useState<unknown>(null);
  const [dialog, setDialog] = useState<Dialog>(null);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<unknown>(null);
  const [tick, setTick] = useState(0);
  const fileInput = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState<string | null>(null);
  const [uploadError, setUploadError] = useState<unknown>(null);
  const [dragging, setDragging] = useState(false);
  const upload = async (files: File[]) => {
    if (files.length === 0) return;
    setUploadError(null);
    try {
      for (const f of files) {
        setUploading(f.name);
        await appApi.uploadProjectFile(target.hubId, target.name, f);
      }
      setTick((t) => t + 1);
    } catch (e) {
      setUploadError(e);
    } finally {
      setUploading(null);
    }
  };
  // The project's conversations (first page): its progress.
  const [recent, setRecentState] = useState<RecentItem[] | null>(() => recall<RecentItem[]>(memoKey + "\u0000recent") ?? null);
  const setRecent = (v: RecentItem[] | null) => { if (v) remember(memoKey + "\u0000recent", v); setRecentState(v); };

  useEffect(() => {
    let off = false;
    // Another project: its last data at once (or nothing the first time);
    // the answers below replace it.
    setPState(recall<Project>(memoKey) ?? null);
    setError(null);
    setRecentState(recall<RecentItem[]>(memoKey + "\u0000recent") ?? null);
    const load = (silent: boolean) => {
      appApi.recent(target.hubId, { project: target.name })
        .then((r) => { if (!off) setRecent(r.fallback ? [] : r.items); })
        .catch(() => { if (!off && !silent) setRecent([]); });
      return appApi.project(target.hubId, target.name)
        .then((r) => { if (!off) { setP(r); setError(null); } })
        .catch((e) => { if (!off && !silent) setError(e); });
    };
    void load(false);
    const t = window.setInterval(() => { if (document.visibilityState === "visible") void load(true); }, 8000);
    return () => { off = true; window.clearInterval(t); };
  }, [target.hubId, target.name, tick, reloadKey]);

  const reload = () => setTick((t) => t + 1);
  const run = async (fn: () => Promise<unknown>, close = true) => {
    setBusy(true);
    setActionError(null);
    try {
      const r = await fn();
      if (r && typeof r === "object" && "members" in (r as Project)) setP(r as Project);
      else reload();
      if (close) setDialog(null);
      onChanged();
    } catch (e) {
      setActionError(e);
    } finally {
      setBusy(false);
    }
  };
  const open = (d: Dialog) => { setActionError(null); setDialog(d); };
  const addMenu = useContextMenu();

  const hubRows = useMemo(() => rows.filter((r) => r.hub.id === target.hubId), [rows, target.hubId]);
  const rowOf = (name: string) => hubRows.find((r) => r.st.agent.name === name);

  if (error) {
    return (
      <div className="app-page">
        {isOldHub(error)
          ? <div className="app-empty-note">{L("这个工作区的版本较旧，还没有项目。在「设置 →工作区」里升级后就能用。", "This hub is older and has no projects yet. Upgrade it in Settings → Hubs.")}</div>
          : <ErrorText error={error} />}
      </div>
    );
  }
  if (!p) {
    return (
      <div className="app-page">
        <div className="app-page-head"><p className="app-summary"><TextSkeleton width={160} /></p></div>
        <RowsSkeleton rows={4} />
      </div>
    );
  }

  const coordinators = p.members.filter((m) => m.role === "coordinator").length;
  const threads = (recent || []).filter((it) => it.kind === "thread" || it.task);
  const count = (st: RecentItem["state"]) => threads.filter((it) => it.state === st).length;
  // What each member has on hand: open tasks addressed to it.
  const onHand = (name: string) => {
    const mine = threads.filter((it) => it.task?.to === name);
    return { working: mine.filter((it) => it.state === "working").length, queued: mine.filter((it) => it.state === "queued").length };
  };
  const nameOf = (n?: string) => {
    if (!n) return "";
    if (n.startsWith("@console:")) return isMe(n, hub) ? L("你", "You") : n.slice(9);
    const row = rowOf(n);
    if (row) return nodeName(row.st.agent, row.hub);
    const a = p.members.find((m) => m.name === n)?.agent;
    return a ? nodeName(a, hub) : n;
  };
  const stateLabel: Record<string, string> = { queued: L("排队中", "Queued"), working: L("处理中", "Working"), failed: L("失败", "Failed"), done: L("已完成", "Done") };
  const hub = hubRows[0]?.hub;
  // Nodes this person may add: theirs (or all, when they manage the hub).
  const addable = hubRows
    .map((r) => r.st.agent)
    .filter((a) => agentKind(a) !== "human" && a.binding.harness !== "machine" && !p.members.some((m) => m.name === a.name))
    .filter((a) => canManage(hub, a.owner_id));

  return (
    <div className="app-page app-project">
      <div className="app-project-head">
        <div className="app-project-titles">
          <h1>
            {p.title || p.name}
            {p.archived && <span className="app-pill">{L("已归档", "Archived")}</span>}
          </h1>
          <p className={cn("app-project-goal", !p.goal && "is-empty")}>{p.goal || L("还没有写目标。", "No goal yet.")}</p>
          <p className="app-project-meta">
            <span>{p.name}</span>
            <span>{hubName}</span>
            {p.owner && <span>{L(`${p.owner} 创建`, `by ${p.owner}`)}</span>}
            <span className="app-project-vis">
              {p.visibility === "members" ? <><Lock />{L("参与者可见", "Participants")}</> : <><Users />{L("团队可见", "Team")}</>}
            </span>
          </p>
        </div>
        <div className="app-project-actions">
          {p.can_edit && <button type="button" className="app-btn" onClick={() => open({ kind: "about" })}><Pencil />{L("编辑", "Edit")}</button>}
          {p.can_manage && <button type="button" className="app-btn app-btn-plain" aria-label={L("项目设置", "Project settings")} title={L("项目设置", "Project settings")} onClick={() => open({ kind: "settings" })}><MoreHorizontal /></button>}
        </div>
      </div>

      <div className="app-project-progress">
        {recent === null ? <TextSkeleton width={220} /> : threads.length === 0 ? (
          <span className="muted">{L("还没有这个项目的对话。给成员发个任务，或者让 agent 用 myco message send --project 发。", "No conversations in this project yet. Give a member a task, or have an agent send with myco message send --project.")}</span>
        ) : (
          <>
            {(["working", "queued", "failed", "done"] as const).map((st) => (
              <span key={st} className={cn("app-project-stat", `is-${st}`)}><i />{stateLabel[st]} <b>{count(st)}</b></span>
            ))}
          </>
        )}
        <span className="app-project-progress-actions">
          <button type="button" className="app-btn app-btn-quiet" onClick={() => onNewConversation()} disabled={p.archived}><IconNewChat />{L("发任务", "New task")}</button>
        </span>
      </div>

      <section className="app-group">
        <div className="app-group-title">
          <span className="name">{L(`成员 ${p.members.length}`, `Members ${p.members.length}`)}{coordinators > 0 && <span className="app-group-note">{L(` · ${coordinators} 个协调者`, ` · ${coordinators} coordinating`)}</span>}</span>
          <span className="app-group-actions">
            <button
              type="button"
              className="app-group-action"
              aria-haspopup="menu"
              disabled={p.archived}
              onMouseDown={(e) => e.stopPropagation()}
              onClick={(e) => addMenu.openUnder(e.currentTarget, [
                { label: L("加入已有的 agent", "Add existing nodes"), icon: IconNodes, onSelect: () => open({ kind: "add" }) },
                { label: L("新建一个 agent", "Start a new node"), icon: Plus, onSelect: onNewNode },
              ])}
            >
              <Plus />{L("添加 agent", "Add nodes")}<ChevronDown className="app-group-action-caret" />
            </button>
            {addMenu.element}
          </span>
        </div>
        <div className="app-list">
          {p.members.length === 0 && <div className="app-empty-note">{L("还没有 agent。添加已有的 agent，或新建一个。", "No nodes yet. Add existing ones, or create one.")}</div>}
          {p.members.map((m) => {
            const row = rowOf(m.name);
            const a = m.agent;
            const Icon = a ? nodeIcon(a) : Users;
            const st = row?.st.state;
            const skills = a?.skills?.length ? a.skills.map((s) => s.name) : a?.capabilities || [];
            const mayAct = p.can_edit || p.can_manage || (a && canManage(hub, a.owner_id));
            return (
              <div className="app-project-member" key={m.name}>
                <button type="button" className="app-project-member-main" onClick={() => row && onOpenNode(row)} disabled={!row}>
                  <span className="app-row-icon"><Icon /></span>
                  <span className="app-row-main">
                    <span className="app-row-titleline">
                      <span className="app-row-title">{a ? nodeName(a, hub) : m.name}</span>
                      {m.role === "coordinator" && <span className="app-label group">{L("协调者", "Coordinator")}</span>}
                      {st && <span className={cn("dot", st)} title={stateMeta[st].label} />}
                    </span>
                    <span className="app-row-sub">
                      {m.note || (a?.description ? a.description : a ? "" : L("别人的 agent （仅自己可见）", "Someone's private node"))}
                    </span>
                    {skills.length > 0 && (
                      <span className="app-project-skills">
                        {skills.slice(0, 4).map((s) => <span key={s} className="app-label">{s}</span>)}
                        {skills.length > 4 && <span className="app-label">+{skills.length - 4}</span>}
                      </span>
                    )}
                  </span>
                </button>
                {(() => {
                  const h = onHand(m.name);
                  return (h.working > 0 || h.queued > 0) ? (
                    <span className="app-project-onhand">
                      {[h.working ? L(`处理中 ${h.working}`, `${h.working} working`) : "", h.queued ? L(`排队 ${h.queued}`, `${h.queued} queued`) : ""].filter(Boolean).join(" · ")}
                    </span>
                  ) : null;
                })()}
                {mayAct && (
                  <button type="button" className="app-btn app-btn-plain app-project-member-more" aria-label={L("成员设置", "Member settings")}
                    onClick={() => open({ kind: "member", member: m })}><MoreHorizontal /></button>
                )}
              </div>
            );
          })}
        </div>
      </section>

      {threads.length > 0 && (
        <section className="app-group">
          <div className="app-group-title">
            <span className="name">{L("最近对话", "Recent conversations")}</span>
            <button type="button" className="app-group-action" onClick={onAllConversations}>{L("全部对话", "All")}<span className="app-chev">›</span></button>
          </div>
          <div className="app-list">
            {threads.slice(0, 6).map((it) => {
              const ps = it.participants.length ? it.participants : [it.first.from, it.first.to];
              const Icon = ps.some((x) => x.startsWith("@console:")) ? IconChat : IconExchange;
              return (
                <button key={it.thread_id} type="button" className="conv-row" onClick={() => onOpenConversation({ hubId: target.hubId, hubName, item: it })}
                  title={it.state ? stateLabel[it.state] : undefined}>
                  <span className="conv-row-icon">
                    <Icon aria-hidden />
                    {it.state && it.state !== "done" && <span className={cn("conv-row-state", `is-${it.state}`)} />}
                  </span>
                  <span className="conv-row-main">
                    <span className="conv-row-top">
                      <span className="conv-row-title">{`${nameOf(ps[0])} → ${nameOf(ps[1])}${ps.length > 2 ? ` +${ps.length - 2}` : ""}`}</span>
                      <span className="conv-row-time">{timeAgo(it.updated_at, now)}</span>
                    </span>
                    <span className="conv-row-summary">{summarize(it.last)}</span>
                  </span>
                </button>
              );
            })}
          </div>
        </section>
      )}

      <section className="app-group">
        <div className="app-group-title">
          <span className="name">{L("项目说明", "Brief")}</span>
          {p.can_edit && <button type="button" className="app-group-action" onClick={() => open({ kind: "brief" })}><Pencil />{L("编辑", "Edit")}</button>}
        </div>
        <div className="app-list app-project-brief">
          {p.brief ? <Markdown text={p.brief} /> : (
            <div className="app-empty-note">{L("写下所有成员都该知道的事：仓库、分支和发版规则、怎么测试、注意事项。 agent 用 myco project show / pull 读取。", "Write what every member should know: repository, branch and release rules, how to test, what to watch out for. Agents read it with myco project show / pull.")}</div>
          )}
        </div>
      </section>

      <section className={cn("app-group app-project-files", dragging && "is-dragging")}
        onDragOver={(e) => { if (!p.archived && e.dataTransfer.types.includes("Files")) { e.preventDefault(); setDragging(true); } }}
        onDragLeave={(e) => { if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setDragging(false); }}
        onDrop={(e) => { e.preventDefault(); setDragging(false); if (!p.archived) void upload(Array.from(e.dataTransfer.files)); }}>
        <div className="app-group-title">
          <span className="name">{L(`文件 ${p.docs.length || ""}`.trim(), `Files ${p.docs.length || ""}`.trim())}</span>
          <button type="button" className="app-group-action" onClick={() => fileInput.current?.click()} disabled={p.archived || uploading !== null}><Upload />{L("上传", "Upload")}</button>
          <input ref={fileInput} type="file" multiple hidden onChange={(e) => { const fs = Array.from(e.target.files || []); e.target.value = ""; void upload(fs); }} />
        </div>
        <div className="app-list">
          {uploading !== null && (
            <div className="app-row app-row-static app-project-uploading">
              <span className="app-spinner" />
              <span className="app-row-main"><span className="app-row-sub">{L(`正在上传 ${uploading}…`, `Uploading ${uploading}…`)}</span></span>
            </div>
          )}
          {uploadError !== null && <div className="app-project-upload-error"><ErrorText error={uploadError} /></div>}
          {p.docs.length === 0 && uploading === null && (
            <div className="app-empty-note">{L("把文件拖到这里，或点「上传」。设计稿、约定、截图都可以（20 MB 以内）；成员 agent 用 myco project file put 上传，用 myco project pull 拉到本地。", "Drop files here, or click Upload: designs, conventions, screenshots (up to 20 MB). Member agents upload with myco project file put and get them with myco project pull.")}</div>
          )}
          {p.docs.map((d) => {
            const Icon = fileIcon(d);
            return (
              <button type="button" key={d.name} className="app-row app-project-doc" onClick={() => open({ kind: "doc", doc: d })}>
                <span className="app-row-icon"><Icon strokeWidth={1.75} /></span>
                <span className="app-row-main">
                  <span className="app-row-title">{d.name}</span>
                  <span className="app-row-sub">{[d.updated_by, timeAgo(d.updated_at, now), fileSize(d.size)].filter(Boolean).join(" · ")}</span>
                </span>
              </button>
            );
          })}
        </div>
      </section>

      <section className="app-group">
        <div className="app-group-title">
          <span className="name">{L("资料链接", "Links")}</span>
          {p.can_edit && <button type="button" className="app-group-action" onClick={() => open({ kind: "links" })}><Pencil />{L("编辑", "Edit")}</button>}
        </div>
        <div className="app-list">
          {p.links.length === 0 && <div className="app-empty-note">{L("仓库、设计稿、看板等放这里。", "Repository, designs, boards and the like.")}</div>}
          {p.links.map((l) => (
            <a key={l.url} className="app-row app-project-link" href={l.url} target="_blank" rel="noreferrer">
              <span className="app-row-icon"><ExternalLink strokeWidth={1.75} /></span>
              <span className="app-row-main">
                <span className="app-row-title">{l.title || l.url}</span>
                {l.title && <span className="app-row-sub mono">{l.url}</span>}
              </span>
            </a>
          ))}
        </div>
      </section>

      <AboutDialog open={dialog?.kind === "about"} project={p} busy={busy} error={actionError} onClose={() => setDialog(null)}
        onSave={(title, goal) => void run(() => appApi.updateProject(target.hubId, p.name, { title, goal }))} />
      <BriefDialog open={dialog?.kind === "brief"} project={p} busy={busy} error={actionError} onClose={() => setDialog(null)}
        onSave={(brief) => void run(() => appApi.updateProject(target.hubId, p.name, { brief }))} />
      <LinksDialog open={dialog?.kind === "links"} project={p} busy={busy} error={actionError} onClose={() => setDialog(null)}
        onSave={(links) => void run(() => appApi.updateProject(target.hubId, p.name, { links }))} />
      <AddDialog open={dialog?.kind === "add"} nodes={addable} busy={busy} error={actionError} onClose={() => setDialog(null)}
        onAdd={(names) => void run(async () => {
          let last: Project | undefined;
          for (const n of names) last = await appApi.putProjectMember(target.hubId, p.name, n);
          return last;
        })} />
      {dialog?.kind === "member" && (
        <MemberDialog member={dialog.member} project={p} mayRemove={p.can_edit || !!(dialog.member.agent && canManage(hub, dialog.member.agent.owner_id))}
          busy={busy} error={actionError} onClose={() => setDialog(null)}
          onSave={(input) => void run(() => appApi.putProjectMember(target.hubId, p.name, dialog.member.name, input))}
          onRemove={() => void run(() => appApi.removeProjectMember(target.hubId, p.name, dialog.member.name))} />
      )}
      {dialog?.kind === "doc" && (
        <FileDialog hubId={target.hubId} project={p} doc={dialog.doc} busy={busy} error={actionError}
          onClose={() => setDialog(null)}
          onDelete={(name) => void run(() => appApi.deleteProjectDoc(target.hubId, p.name, name))} />
      )}
      <SettingsDialog open={dialog?.kind === "settings"} project={p} busy={busy} error={actionError} onClose={() => setDialog(null)}
        onVisibility={(visibility) => void run(() => appApi.updateProject(target.hubId, p.name, { visibility }), false)}
        onArchive={(archived) => void run(() => appApi.updateProject(target.hubId, p.name, { archived }), false)}
        onDelete={() => open({ kind: "delete" })} />
      <ConfirmDialog
        open={dialog?.kind === "delete"}
        onOpenChange={(o) => !o && setDialog(null)}
        title={L(`删除项目 ${p.title || p.name}`, `Delete ${p.title || p.name}`)}
        description={L("项目的成员关系、说明、文档和链接都会删除； agent 本身和它们的对话不受影响。", "Its members, brief, documents and links are deleted; the nodes and their conversations stay.")}
        confirmLabel={L("删除", "Delete")}
        busy={busy}
        error={actionError ? <ErrorText error={actionError} /> : undefined}
        onConfirm={() => void (async () => {
          setBusy(true);
          try {
            await appApi.deleteProject(target.hubId, p.name);
            setDialog(null);
            onDeleted();
          } catch (e) {
            setActionError(e);
          } finally {
            setBusy(false);
          }
        })()}
      />
    </div>
  );
}

function Actions({ busy, onCancel, onSave, saveLabel, disabled }: { busy: boolean; onCancel: () => void; onSave: () => void; saveLabel?: string; disabled?: boolean }) {
  return (
    <div className="app-modal-actions">
      <button type="button" className="app-btn" onClick={onCancel} disabled={busy}>{L("取消", "Cancel")}</button>
      <button type="button" className="app-btn app-btn-primary" onClick={onSave} disabled={busy || disabled}>{busy ? L("正在保存…", "Saving…") : saveLabel || L("保存", "Save")}</button>
    </div>
  );
}

function AboutDialog({ open, project, busy, error, onClose, onSave }: { open: boolean; project: Project; busy: boolean; error: unknown; onClose: () => void; onSave: (title: string, goal: string) => void }) {
  const [title, setTitle] = useState("");
  const [goal, setGoal] = useState("");
  useOnOpen(open, () => { setTitle(project.title); setGoal(project.goal || ""); });
  return (
    <Modal open={open} onOpenChange={(o) => !o && !busy && onClose()} title={L("编辑项目", "Edit project")}>
      <label className="app-field">
        <span className="app-field-label">{L("名称", "Title")}</span>
        <input className="app-input" value={title} onChange={(e) => setTitle(e.target.value)} disabled={busy} />
      </label>
      <label className="app-field">
        <span className="app-field-label">{L("目标", "Goal")}</span>
        <textarea className="app-input" rows={3} value={goal} onChange={(e) => setGoal(e.target.value)} disabled={busy}
          placeholder={L("这个项目要做成什么，一两句话", "What this project should achieve, in a sentence or two")} />
      </label>
      <ErrorText error={error} />
      <Actions busy={busy} onCancel={onClose} onSave={() => onSave(title.trim(), goal.trim())} disabled={!title.trim()} />
    </Modal>
  );
}

function BriefDialog({ open, project, busy, error, onClose, onSave }: { open: boolean; project: Project; busy: boolean; error: unknown; onClose: () => void; onSave: (brief: string) => void }) {
  const [brief, setBrief] = useState("");
  useOnOpen(open, () => setBrief(project.brief || ""));
  return (
    <Modal open={open} onOpenChange={(o) => !o && !busy && onClose()} title={L("项目说明", "Brief")} wide
      description={L("所有成员都该知道的事（Markdown，8 KB 以内）。 agent 用 myco project show / pull 读取。", "What every member should know (Markdown, up to 8 KB). Agents read it with myco project show / pull.")}>
      <textarea className="app-input app-project-editor" rows={16} value={brief} onChange={(e) => setBrief(e.target.value)} disabled={busy}
        placeholder={L("## 仓库\n…\n\n## 规则\n…\n\n## 怎么测试\n…", "## Repository\n…\n\n## Rules\n…\n\n## Testing\n…")} />
      <ErrorText error={error} />
      <Actions busy={busy} onCancel={onClose} onSave={() => onSave(brief)} />
    </Modal>
  );
}

function LinksDialog({ open, project, busy, error, onClose, onSave }: { open: boolean; project: Project; busy: boolean; error: unknown; onClose: () => void; onSave: (links: { title?: string; url: string }[]) => void }) {
  const [text, setText] = useState("");
  useOnOpen(open, () => setText(project.links.map((l) => (l.title ? `${l.title} ${l.url}` : l.url)).join("\n")));
  const parse = () => text.split("\n").map((l) => l.trim()).filter(Boolean).map((l) => {
    const i = l.search(/\S+:\/\/\S+$/);
    return i > 0 ? { title: l.slice(0, i).trim(), url: l.slice(i) } : { url: l };
  });
  return (
    <Modal open={open} onOpenChange={(o) => !o && !busy && onClose()} title={L("资料链接", "Links")}
      description={L("每行一个：名称和地址，例如「仓库 https://github.com/owner/repo」。", "One per line: a title and the address, e.g. “Repo https://github.com/owner/repo”.")}>
      <textarea className="app-input mono" rows={8} value={text} onChange={(e) => setText(e.target.value)} disabled={busy} />
      <ErrorText error={error} />
      <Actions busy={busy} onCancel={onClose} onSave={() => onSave(parse())} />
    </Modal>
  );
}

function AddDialog({ open, nodes, busy, error, onClose, onAdd }: { open: boolean; nodes: import("@/api").Agent[]; busy: boolean; error: unknown; onClose: () => void; onAdd: (names: string[]) => void }) {
  const [picked, setPicked] = useState<string[]>([]);
  useOnOpen(open, () => setPicked([]));
  return (
    <Modal open={open} onOpenChange={(o) => !o && !busy && onClose()} title={L("添加 agent", "Add nodes")}
      description={L("选你的 agent 加入这个项目（管理员可以选任何 agent）。别人的 agent 由它的主人加入。", "Pick your nodes to add (admins can pick any). Other people add their own.")}>
      {nodes.length === 0
        ? <div className="app-empty-note">{L("没有可以添加的 agent 了。", "No more nodes you can add.")}</div>
        : <NodePicker nodes={nodes} value={picked} onChange={setPicked} disabled={busy} listedOnly emptyPlaceholder={L("搜索 agent", "Search nodes")} />}
      <ErrorText error={error} />
      <Actions busy={busy} onCancel={onClose} onSave={() => onAdd(picked)} saveLabel={L(`添加 ${picked.length || ""}`.trim(), `Add ${picked.length || ""}`.trim())} disabled={picked.length === 0} />
    </Modal>
  );
}

function MemberDialog({ member, project, mayRemove, busy, error, onClose, onSave, onRemove }: {
  member: ProjectMember; project: Project; mayRemove: boolean; busy: boolean; error: unknown;
  onClose: () => void; onSave: (input: { role?: "member" | "coordinator"; note?: string }) => void; onRemove: () => void;
}) {
  const [note, setNote] = useState(member.note || "");
  const [role, setRole] = useState(member.role);
  const name = member.agent ? nodeName(member.agent) : member.name;
  return (
    <Modal open onOpenChange={(o) => !o && !busy && onClose()} title={name}>
      <label className="app-field">
        <span className="app-field-label">{L("在这个项目里负责什么", "What it does here")}</span>
        <input className="app-input" value={note} onChange={(e) => setNote(e.target.value)} disabled={busy || !project.can_edit}
          placeholder={L("例如：后端、界面、测试", "e.g. backend, UI, tests")} />
      </label>
      {project.can_manage && (
        <div className="app-provider-list" role="radiogroup">
          <label className={cn("app-provider-opt", role === "member" && "picked")}>
            <input type="radio" name="role" checked={role === "member"} onChange={() => setRole("member")} disabled={busy} />
            <span><b>{L("成员", "Member")}</b><small>{L("读项目说明和文档，接活", "Reads the brief and documents, takes work")}</small></span>
          </label>
          <label className={cn("app-provider-opt", role === "coordinator" && "picked")}>
            <input type="radio" name="role" checked={role === "coordinator"} onChange={() => setRole("coordinator")} disabled={busy} />
            <span><b>{L("协调者", "Coordinator")}</b><small>{L("还能改项目说明、文档、链接，改其他成员的介绍和技能，加入同一主人的 agent", "Also edits the brief, documents and links, other members' descriptions and skills, and adds nodes of its owner")}</small></span>
          </label>
        </div>
      )}
      <ErrorText error={error} />
      <div className="app-modal-actions">
        {mayRemove && <button type="button" className="app-btn app-btn-danger" onClick={onRemove} disabled={busy} style={{ marginRight: "auto" }}>{L("移出项目", "Remove")}</button>}
        <button type="button" className="app-btn" onClick={onClose} disabled={busy}>{L("取消", "Cancel")}</button>
        <button type="button" className="app-btn app-btn-primary" disabled={busy || (note === (member.note || "") && role === member.role)}
          onClick={() => onSave({ ...(note !== (member.note || "") ? { note } : {}), ...(role !== member.role ? { role } : {}) })}>{L("保存", "Save")}</button>
      </div>
    </Modal>
  );
}

function fileIcon(d: ProjectDoc) {
  const t = d.content_type || "";
  if (t.startsWith("image/")) return FileImage;
  if (!d.binary) return FileText;
  return FileIcon;
}

function fileSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

/** A project file: text is shown (Markdown), images previewed, the rest downloaded. */
function FileDialog({ hubId, project, doc, busy, error, onClose, onDelete }: {
  hubId: string; project: Project; doc: ProjectDoc; busy: boolean; error: unknown;
  onClose: () => void; onDelete: (name: string) => void;
}) {
  const [full, setFull] = useState<ProjectDoc | null>(null);
  const [loadError, setLoadError] = useState<unknown>(null);
  const [saved, setSaved] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<unknown>(null);
  const image = (doc.content_type || "").startsWith("image/");
  useEffect(() => {
    if (doc.binary) return;
    let off = false;
    appApi.projectDoc(hubId, project.name, doc.name)
      .then((d) => { if (!off) setFull(d); })
      .catch((e) => { if (!off) setLoadError(e); });
    return () => { off = true; };
  }, [hubId, project.name, doc.name, doc.binary]);
  const save = async () => {
    setSaving(true);
    setSaveError(null);
    try {
      const r = await appApi.saveProjectFile(hubId, project.name, doc.name);
      setSaved(r.path);
    } catch (e) {
      setSaveError(e);
    } finally {
      setSaving(false);
    }
  };
  const markdown = !doc.binary && (!doc.content_type || /markdown|text\/plain/.test(doc.content_type) || /\.(md|markdown|txt)$/i.test(doc.name));
  return (
    <Modal open onOpenChange={(o) => !o && !busy && onClose()} title={doc.name} wide
      description={[doc.updated_by && L(`${doc.updated_by} 上传`, `by ${doc.updated_by}`), fileSize(doc.size)].filter(Boolean).join(" · ")}>
      {!doc.binary && !full && !loadError && <RowsSkeleton rows={3} className="is-flat" />}
      <ErrorText error={loadError} />
      {full && (markdown
        ? <div className="app-project-doc-body"><Markdown text={full.body || ""} /></div>
        : <pre className="app-project-doc-body app-project-doc-code">{full.body}</pre>)}
      {image && <div className="app-project-preview"><img src={appApi.projectFileURL(hubId, project.name, doc.name)} alt={doc.name} /></div>}
      {doc.binary && !image && <div className="app-empty-note">{L("这个文件不能在这里预览，下载后用对应的应用打开。", "This file can't be previewed here; download it and open it in its app.")}</div>}
      {saved && <div className="app-success"><Check /><span>{L(`已保存到 ${saved}`, `Saved to ${saved}`)}</span></div>}
      <ErrorText error={saveError ?? error} />
      <div className="app-modal-actions">
        <button type="button" className="app-btn app-btn-danger" onClick={() => onDelete(doc.name)} disabled={busy} style={{ marginRight: "auto" }}>{L("删除", "Delete")}</button>
        <button type="button" className="app-btn" onClick={onClose} disabled={busy}>{L("关闭", "Close")}</button>
        <button type="button" className="app-btn app-btn-primary" onClick={() => void save()} disabled={saving}>
          <Download />{saving ? L("正在下载…", "Downloading…") : L("下载", "Download")}
        </button>
      </div>
    </Modal>
  );
}

function SettingsDialog({ open, project, busy, error, onClose, onVisibility, onArchive, onDelete }: {
  open: boolean; project: Project; busy: boolean; error: unknown; onClose: () => void;
  onVisibility: (v: "members" | "team") => void; onArchive: (archived: boolean) => void; onDelete: () => void;
}) {
  return (
    <Modal open={open} onOpenChange={(o) => !o && !busy && onClose()} title={L("项目设置", "Project settings")}>
      <div className="app-provider-list" role="radiogroup">
        <label className={cn("app-provider-opt", project.visibility === "members" && "picked")}>
          <input type="radio" name="pvis" checked={project.visibility === "members"} onChange={() => onVisibility("members")} disabled={busy} />
          <span><b>{L("参与者可见", "Participants")}</b><small>{L("项目的创建者、成员 agent 的主人和成员 agent 能看到；管理员也能看到", "Its creator, the owners of its member nodes and the nodes themselves; admins too")}</small></span>
        </label>
        <label className={cn("app-provider-opt", project.visibility === "team" && "picked")}>
          <input type="radio" name="pvis" checked={project.visibility === "team"} onChange={() => onVisibility("team")} disabled={busy} />
          <span><b>{L("团队可见", "Team")}</b><small>{L("工作区上所有人都能看到，也能把自己的 agent 加进来", "Everyone on the hub sees it and can add their own nodes")}</small></span>
        </label>
      </div>
      <ErrorText error={error} />
      <div className="app-modal-actions">
        <button type="button" className="app-btn app-btn-danger" onClick={onDelete} disabled={busy} style={{ marginRight: "auto" }}>{L("删除项目", "Delete")}</button>
        <button type="button" className="app-btn" onClick={() => onArchive(!project.archived)} disabled={busy}>{project.archived ? L("恢复", "Unarchive") : L("归档", "Archive")}</button>
        <button type="button" className="app-btn app-btn-primary" onClick={onClose} disabled={busy}>{L("完成", "Done")}</button>
      </div>
    </Modal>
  );
}

/** A new project: name (address), title, goal. */
export function NewProjectDialog({ open, hubs, defaultHubId, onClose, onCreated }: {
  open: boolean;
  hubs: { id: string; name: string }[];
  defaultHubId?: string;
  onClose: () => void;
  onCreated: (ref: ProjectRef) => void;
}) {
  const [hubId, setHubId] = useState("");
  const [name, setName] = useState("");
  const [title, setTitle] = useState("");
  const [goal, setGoal] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  useOnOpen(open, () => {
    setHubId(defaultHubId || hubs[0]?.id || "");
    setName(""); setTitle(""); setGoal(""); setError(null);
  });
  // With a hub picked in the switcher the project goes there; otherwise pick one.
  const fixedHub = defaultHubId ? hubs.find((h) => h.id === defaultHubId) : hubs.length === 1 ? hubs[0] : undefined;
  const slug = name.trim().toLowerCase();
  const create = async () => {
    setBusy(true);
    setError(null);
    try {
      const p = await appApi.createProject(hubId, { name: slug, title: title.trim() || undefined, goal: goal.trim() || undefined });
      onCreated({ hubId, name: p.name });
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open={open} onOpenChange={(o) => !o && !busy && onClose()} title={L("新建项目", "New project")}
      description={L("把要一起干的 agent 放进一个项目，给它们共享的目标、说明和文档。", "Put the nodes that work together in a project, with a shared goal, brief and documents.")}>
      {fixedHub ? (
        <div className="app-field">
          <span className="app-field-label">Hub</span>
          <span className="app-field-static">{fixedHub.name}</span>
        </div>
      ) : (
        <label className="app-field">
          <span className="app-field-label">Hub</span>
          <select className="app-select-field" value={hubId} onChange={(e) => setHubId(e.target.value)} disabled={busy}>
            {hubs.map((h) => <option key={h.id} value={h.id}>{h.name}</option>)}
          </select>
        </label>
      )}
      <label className="app-field">
        <span className="app-field-label">{L("名称", "Title")}</span>
        <input className="app-input" value={title} onChange={(e) => {
          setTitle(e.target.value);
          if (!name || name === slugOf(title)) setName(slugOf(e.target.value));
        }} disabled={busy} placeholder={L("例如 myco 开发", "e.g. myco development")} autoFocus />
      </label>
      <label className="app-field">
        <span className="app-field-label">{L("短名（命令里用）", "Short name (for commands)")}</span>
        <input className="app-input mono" value={name} onChange={(e) => setName(e.target.value)} disabled={busy} placeholder="myco" />
      </label>
      <label className="app-field">
        <span className="app-field-label">{L("目标", "Goal")}</span>
        <textarea className="app-input" rows={3} value={goal} onChange={(e) => setGoal(e.target.value)} disabled={busy}
          placeholder={L("这个项目要做成什么，一两句话", "What this project should achieve, in a sentence or two")} />
      </label>
      <ErrorText error={error} />
      <Actions busy={busy} onCancel={onClose} onSave={() => void create()} saveLabel={L("创建", "Create")} disabled={!slug || !hubId} />
    </Modal>
  );
}

function slugOf(s: string): string {
  return s.toLowerCase().replace(/[^a-z0-9_-]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 40);
}
