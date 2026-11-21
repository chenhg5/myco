import { t as uiText } from "../_lib/i18n";
// 任务追踪: issues, projects and comments for a team, kept like Linear:
// lists grouped by status, a board, an issue's page beside the list, and
// the keyboard for everything (C new, / search, J K move, S P A L change,
// ⌘K commands). People and agents share the records: an agent creates
// issues, moves them on and comments with the CLI, and the page follows.
import "../_lib/sheet.css";
import "./issues.css";
import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { marked } from "marked";
import {
  Bot, Calendar, Check, ChevronDown, ChevronRight, Columns3, Copy, FolderKanban, Inbox, LayoutList, ListTodo, Plus, Search,
  SlidersHorizontal, Tag, Trash2, CircleDot, Users, X, Command, Clock,
} from "lucide-react";
import { mount, titleSaver, uid, useKit, Toast } from "../_lib/appkit";
import {
  STATUSES, PRIORITIES, PROJECT_STATUSES, allLabels, byOrder, byPriority, comments, createIssue, isClosed, issues, keyOf, kit, nowIso, num, people,
  priorityName, projects, statusName, statusOf, team, teamKey, timeAgo, type Issue, type IssueRec, type StatusId,
} from "./model";
import { Avatar, LabelDot, PriorityIcon, StatusIcon } from "./icons";

const teamTitle = titleSaver(kit, team);

// ---- markdown: what people and agents wrote, kept to text formatting
// (no raw HTML, scripts, styles or pictures; links only to the web) ----
const KEEP = new Set(["P", "BR", "STRONG", "B", "EM", "I", "DEL", "S", "CODE", "PRE", "BLOCKQUOTE", "UL", "OL", "LI", "H1", "H2", "H3", "H4", "H5", "H6", "HR", "A", "TABLE", "THEAD", "TBODY", "TR", "TH", "TD", "INPUT"]);
function clean(html: string) {
  const t = document.createElement("template");
  t.innerHTML = html;
  const walk = (n: Element) => {
    for (const el of Array.from(n.children)) {
      if (!KEEP.has(el.tagName)) { el.replaceWith(document.createTextNode(el.textContent || "")); continue; }
      for (const a of Array.from(el.attributes)) {
        const keep = (el.tagName === "A" && a.name === "href" && /^(https?:|mailto:)/i.test(a.value))
          || (el.tagName === "INPUT" && (a.name === "type" || a.name === "checked" || a.name === "disabled"));
        if (!keep) el.removeAttribute(a.name);
      }
      if (el.tagName === "INPUT" && el.getAttribute("type") !== "checkbox") { el.remove(); continue; }
      if (el.tagName === "A") { el.setAttribute("target", "_blank"); el.setAttribute("rel", "noreferrer"); }
      walk(el);
    }
  };
  walk(t.content as unknown as Element);
  return t.innerHTML;
}
const md = (s: string) => ({ __html: clean(marked.parse(s || "")) });

// ---- views ----
type View = { kind: "mine" | "all" | "active" | "backlog" | "projects" } | { kind: "project"; id: string };
type Filters = { status: string[]; priority: number[]; assignee: string[]; labels: string[] };
const NO_FILTERS: Filters = { status: [], priority: [], assignee: [], labels: [] };
const VIEW_NAME: Record<string, string> = { mine: "我的问题", all: "全部问题", active: "进行中", backlog: "待规划", projects: "项目" };

const store = {
  get: (k: string) => { try { return localStorage.getItem(`myco-issues-${k}`); } catch { return null; } },
  set: (k: string, v: string) => { try { localStorage.setItem(`myco-issues-${k}`, v); } catch { /* (a private window) */ } },
};

function save(id: string, patch: Partial<Issue>) {
  const cur = issues.get(id);
  if (!cur) return;
  const next: Issue & Record<string, unknown> = { ...cur.data, ...patch };
  if (patch.status !== undefined) next.done_at = isClosed(String(patch.status)) ? (cur.data.done_at || nowIso()) : null;
  void kit.track(issues.put(next, id));
}

// ---- a small popover with a list to pick from (type to narrow, ↑↓ ⏎) ----
type Anchor = { x: number; y: number; w: number; h: number };
const rectOf = (el: Element): Anchor => { const r = el.getBoundingClientRect(); return { x: r.left, y: r.top, w: r.width, h: r.height }; };

function Pop({ at, onClose, children, width = 240 }: { at: Anchor; onClose: () => void; children: ReactNode; width?: number }) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: -9999, top: -9999 });
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    let left = at.x, top = at.y + at.h + 4;
    if (left + r.width > window.innerWidth - 8) left = window.innerWidth - 8 - r.width;
    if (top + r.height > window.innerHeight - 8) top = Math.max(8, at.y - r.height - 4);
    setPos({ left: Math.max(8, left), top });
  }, [at.x, at.y, at.w, at.h]);
  useEffect(() => {
    const down = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) onClose(); };
    const t = setTimeout(() => document.addEventListener("mousedown", down), 0);
    return () => { clearTimeout(t); document.removeEventListener("mousedown", down); };
  }, [onClose]);
  return createPortal(<div ref={ref} className="pop is-pick" style={{ ...pos, width }}>{children}</div>, document.body);
}

type Opt = { id: string; label: string; icon?: ReactNode; hint?: string; on?: boolean };
function Pick({ at, opts, placeholder, multi, create, onPick, onClose }: {
  at: Anchor; opts: Opt[]; placeholder: string; multi?: boolean; create?: (q: string) => Opt | null; onPick: (id: string) => void; onClose: () => void;
}) {
  const [q, setQ] = useState("");
  const [i, setI] = useState(0);
  const list = opts.filter((o) => !q || o.label.toLowerCase().includes(q.toLowerCase()));
  const extra = q && create && !opts.some((o) => o.label === q.trim()) ? create(q.trim()) : null;
  const all = extra ? [...list, extra] : list;
  const pick = (o: Opt | undefined) => { if (!o) return; onPick(o.id); if (!multi) onClose(); else setQ(""); };
  return (
    <Pop at={at} onClose={onClose}>
      <input className="pick-q" autoFocus value={q} placeholder={placeholder} onChange={(e) => { setQ(e.target.value); setI(0); }}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown") { e.preventDefault(); setI((x) => Math.min(all.length - 1, x + 1)); }
          else if (e.key === "ArrowUp") { e.preventDefault(); setI((x) => Math.max(0, x - 1)); }
          else if (e.key === "Enter") { e.preventDefault(); pick(all[i]); }
          else if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); onClose(); }
        }} />
      <div className="pick-list" role="listbox">
        {all.map((o, k) => (
          <button key={o.id + k} type="button" role="option" aria-selected={k === i} className={`pick-item${k === i ? " is-cur" : ""}`}
            onMouseEnter={() => setI(k)} onClick={() => pick(o)}>
            <span className="pick-ico">{o.icon}</span><span className="pick-label">{o.label}</span>
            {o.hint && <kbd>{o.hint}</kbd>}{o.on && <Check className="ico pick-on" />}
          </button>
        ))}
        {!all.length && <div className="pick-empty">{uiText("没有")}</div>}
      </div>
    </Pop>
  );
}

// the menus of an issue's properties
type MenuKind = "status" | "priority" | "assignee" | "labels" | "project" | "due";
function PropMenu({ kind, ids, at, onClose, me }: { kind: MenuKind; ids: string[]; at: Anchor; onClose: () => void; me: string }) {
  const first = issues.get(ids[0])?.data || {};
  const setAll = (p: Partial<Issue>) => ids.forEach((id) => save(id, p));
  if (kind === "status") return <Pick at={at} placeholder={uiText("改状态…")} onClose={onClose} onPick={(s) => setAll({ status: s })}
    opts={STATUSES.map((s, k) => ({ id: s.id, label: s.name, icon: <StatusIcon s={s.id} />, hint: String(k + 1), on: first.status === s.id }))} />;
  if (kind === "priority") return <Pick at={at} placeholder={uiText("改优先级…")} onClose={onClose} onPick={(p) => setAll({ priority: Number(p) })}
    opts={PRIORITIES.map((p) => ({ id: String(p.id), label: p.name, icon: <PriorityIcon p={p.id} />, on: num(first.priority, 0) === p.id }))} />;
  if (kind === "assignee") return <Pick at={at} placeholder={uiText("指派给…（可以写 agent 的名字）")} onClose={onClose}
    onPick={(a) => setAll({ assignee: a === "\u0000" ? null : a })}
    create={(q) => ({ id: q, label: uiText("指派给「{0}」", q), icon: <Avatar name={q} /> })}
    opts={[{ id: "\u0000", label: uiText("不指派"), icon: <Avatar name={null} />, on: !first.assignee },
      ...people(me).map((p) => ({ id: p, label: p === me ? `${p}（我）` : p, icon: <Avatar name={p} />, on: first.assignee === p }))]} />;
  if (kind === "labels") {
    const has = new Set(first.labels || []);
    return <Pick at={at} multi placeholder={uiText("加标签…")} onClose={onClose}
      create={(q) => ({ id: q, label: uiText("新标签「{0}」", q), icon: <Tag className="ico" /> })}
      onPick={(l) => ids.forEach((id) => {
        const cur = new Set(issues.get(id)?.data.labels || []);
        if (has.has(l)) cur.delete(l); else cur.add(l);
        save(id, { labels: [...cur] });
      })}
      opts={allLabels().map((l) => ({ id: l, label: l, icon: <LabelDot l={l} />, on: has.has(l) }))} />;
  }
  if (kind === "project") return <Pick at={at} placeholder={uiText("放进项目…")} onClose={onClose} onPick={(p) => setAll({ project: p === "\u0000" ? null : p })}
    opts={[{ id: "\u0000", label: uiText("不放进项目"), icon: <FolderKanban className="ico" />, on: !first.project },
      ...projects.rows.map((p) => ({ id: p.id, label: String(p.data.name || "项目"), icon: <FolderKanban className="ico" />, on: first.project === p.id }))]} />;
  // due date
  return (
    <Pop at={at} onClose={onClose} width={220}>
      <div className="due-pop">
        <input type="date" className="inp" autoFocus defaultValue={String(first.due || "")} onChange={(e) => setAll({ due: e.target.value || null })} />
        <button type="button" className="btn" onClick={() => { setAll({ due: null }); onClose(); }}>{uiText("清除")}</button>
      </div>
    </Pop>
  );
}

// ---- the app ----
function App() {
  useKit(kit);
  const me = kit.me;
  const [view, setViewState] = useState<View>(() => ({ kind: (store.get("view") as "all") || "all" }));
  const [layout, setLayoutState] = useState<"list" | "board">(() => (store.get("layout") === "board" ? "board" : "list"));
  const [q, setQ] = useState("");
  const [filters, setFilters] = useState<Filters>(NO_FILTERS);
  const [open, setOpen] = useState<string | null>(null);
  const [focus, setFocus] = useState<string | null>(null);
  const [menu, setMenu] = useState<null | { kind: MenuKind; ids: string[]; at: Anchor }>(null);
  const [creating, setCreating] = useState<null | Partial<Issue>>(null);
  const [palette, setPalette] = useState(false);
  const [filterAt, setFilterAt] = useState<Anchor | null>(null);
  const [folded, setFolded] = useState<Record<string, boolean>>({ canceled: true });
  const [teamAt, setTeamAt] = useState<Anchor | null>(null);
  const searchRef = useRef<HTMLInputElement>(null);

  const setView = (v: View) => { setViewState(v); if (v.kind !== "project") store.set("view", v.kind); setFocus(null); };
  const setLayout = (l: "list" | "board") => { setLayoutState(l); store.set("layout", l); };

  // the issues of this view, filtered and searched
  const shown = useMemo(() => {
    const ql = q.trim().toLowerCase();
    return issues.rows.filter((r) => {
      const d = r.data, s = statusOf(r);
      if (view.kind === "mine" && d.assignee !== me) return false;
      if (view.kind === "active" && !["todo", "progress", "review"].includes(s)) return false;
      if (view.kind === "backlog" && s !== "backlog") return false;
      if (view.kind === "project" && d.project !== view.id) return false;
      if (filters.status.length && !filters.status.includes(s)) return false;
      if (filters.priority.length && !filters.priority.includes(num(d.priority, 0))) return false;
      if (filters.assignee.length && !filters.assignee.includes(String(d.assignee || ""))) return false;
      if (filters.labels.length && !(d.labels || []).some((l) => filters.labels.includes(l))) return false;
      if (ql && !(`${keyOf(r)} ${d.title || ""}`.toLowerCase().includes(ql))) return false;
      return true;
    });
  }, [kit.version, view, q, filters, me]); // eslint-disable-line react-hooks/exhaustive-deps

  const groups = STATUSES.map((s) => ({ ...s, rows: shown.filter((r) => statusOf(r) === s.id).sort(layout === "board" ? byOrder : byPriority) }))
    .filter((g) => layout === "board" ? g.id !== "canceled" || g.rows.length : g.rows.length);
  const flat = layout === "list" ? groups.flatMap((g) => (folded[g.id] ? [] : g.rows)) : groups.flatMap((g) => g.rows);

  // keyboard
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      const typing = t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable);
      const mod = e.metaKey || e.ctrlKey;
      if (mod && e.key.toLowerCase() === "k") { e.preventDefault(); setPalette((x) => !x); return; }
      if (typing || menu || creating || palette) return;
      const cur = focus || open;
      const idx = flat.findIndex((r) => r.id === focus);
      const anchorOf = (id: string) => {
        const el = document.querySelector(`[data-issue="${id}"]`);
        return el ? rectOf(el) : { x: window.innerWidth / 2 - 120, y: 120, w: 0, h: 0 };
      };
      if (e.key === "c" || e.key === "C") { e.preventDefault(); setCreating(view.kind === "project" ? { project: view.id } : {}); }
      else if (e.key === "/") { e.preventDefault(); searchRef.current?.focus(); }
      else if (e.key === "j" || e.key === "ArrowDown") { e.preventDefault(); const n = flat[Math.min(flat.length - 1, idx + 1)]; if (n) { setFocus(n.id); if (open) setOpen(n.id); } }
      else if (e.key === "k" || e.key === "ArrowUp") { e.preventDefault(); const n = flat[Math.max(0, idx - 1)]; if (n) { setFocus(n.id); if (open) setOpen(n.id); } }
      else if ((e.key === "Enter" || e.key === "o") && focus) { e.preventDefault(); setOpen(focus); }
      else if (e.key === "Escape") { if (open) setOpen(null); else setFocus(null); }
      else if (cur && /^[1-6]$/.test(e.key)) save(cur, { status: STATUSES[Number(e.key) - 1].id });
      else if (cur && (e.key === "s" || e.key === "p" || e.key === "a" || e.key === "l")) {
        e.preventDefault();
        const kind = ({ s: "status", p: "priority", a: "assignee", l: "labels" } as const)[e.key];
        setMenu({ kind, ids: [cur], at: anchorOf(cur) });
      } else if (cur && e.key === "i") { e.preventDefault(); save(cur, { assignee: me }); kit.flash(uiText("已指派给你")); }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  });

  const nOpen = (pred: (r: IssueRec) => boolean) => issues.rows.filter((r) => !isClosed(statusOf(r)) && pred(r)).length;
  const filterCount = filters.status.length + filters.priority.length + filters.assignee.length + filters.labels.length;
  const title = view.kind === "project" ? String(projects.get(view.id)?.data.name || "项目") : VIEW_NAME[view.kind];

  return (
    <div className="app is">
      <aside className="is-side">
        <div className="is-team">
          <button type="button" className="is-logo" title={uiText("名称和编号前缀")} aria-label={uiText("名称和编号前缀")} onClick={(e) => setTeamAt(rectOf(e.currentTarget))}><Logo /></button>
          <input className="is-team-name" value={teamTitle.get("")} placeholder={uiText("任务追踪")} aria-label={uiText("名称")} onChange={(e) => teamTitle.set(e.target.value)} />
        </div>
        <button type="button" className="is-new" onClick={() => setCreating(view.kind === "project" ? { project: view.id } : {})}>
          <Plus className="ico" />{uiText("新建问题")}<kbd>C</kbd>
        </button>
        <nav className="is-nav">
          <NavItem icon={<Inbox className="ico" />} on={view.kind === "mine"} label={uiText("我的问题")} n={nOpen((r) => r.data.assignee === me)} onClick={() => setView({ kind: "mine" })} />
          <NavItem icon={<ListTodo className="ico" />} on={view.kind === "all"} label={uiText("全部问题")} onClick={() => setView({ kind: "all" })} />
          <NavItem icon={<CircleDot className="ico" />} on={view.kind === "active"} label={uiText("进行中")} n={nOpen((r) => ["progress", "review"].includes(statusOf(r)))} onClick={() => setView({ kind: "active" })} />
          <NavItem icon={<Clock className="ico" />} on={view.kind === "backlog"} label={uiText("待规划")} n={nOpen((r) => statusOf(r) === "backlog")} onClick={() => setView({ kind: "backlog" })} />
        </nav>
        <div className="is-nav-head">
          <button type="button" className={`is-nav-title${view.kind === "projects" ? " is-on" : ""}`} onClick={() => setView({ kind: "projects" })}>{uiText("项目")}</button>
          <button type="button" className="icon-btn" title={uiText("新建项目")} aria-label={uiText("新建项目")} onClick={() => {
            const id = uid("p");
            void kit.track(projects.put({ name: "新项目", status: "planned", lead: me || null, order: projects.rows.length + 1 }, id));
            setView({ kind: "project", id });
          }}><Plus className="ico" /></button>
        </div>
        <nav className="is-nav">
          {projects.rows.map((p) => (
            <NavItem key={p.id} icon={<FolderKanban className="ico" />} on={view.kind === "project" && view.id === p.id} label={String(p.data.name || "项目")}
              n={nOpen((r) => r.data.project === p.id)} onClick={() => setView({ kind: "project", id: p.id })} />
          ))}
          {!projects.rows.length && <div className="is-nav-empty">{uiText("还没有项目")}</div>}
        </nav>
        <div className="is-side-foot">
          <span className="faint">{kit.pending ? uiText("正在保存…") : kit.failed ? uiText("没保存上") : uiText("已保存")}</span>
          {me && <span className="faint">{uiText("你是")}{" "}{me}</span>}
        </div>
      </aside>

      <main className="is-main">
        <header className="is-head">
          {view.kind === "project"
            ? <h1 className="is-crumb"><button type="button" onClick={() => setView({ kind: "projects" })}>{uiText("项目")}</button><ChevronRight className="ico" /><span>{title}</span></h1>
            : <h1>{title}</h1>}
          {view.kind !== "projects" && <span className="is-count">{shown.length}</span>}
          <span className="top-gap" />
          {view.kind !== "projects" && (
            <>
              <label className="is-search">
                <Search className="ico" />
                <input ref={searchRef} value={q} placeholder={uiText("搜索")} onChange={(e) => setQ(e.target.value)}
                  onKeyDown={(e) => { if (e.key === "Escape") { setQ(""); (e.target as HTMLInputElement).blur(); } }} />
                {!q && <kbd>/</kbd>}
              </label>
              <button type="button" className={`tool${filterCount ? " is-on" : ""}`} onClick={(e) => setFilterAt(rectOf(e.currentTarget))}>
                <SlidersHorizontal className="ico" />{uiText("筛选")}{filterCount ? ` ${filterCount}` : ""}
              </button>
              <div className="seg" role="radiogroup" aria-label={uiText("显示")}>
                <button type="button" role="radio" aria-checked={layout === "list"} className={layout === "list" ? "is-on" : ""} title={uiText("列表")} onClick={() => setLayout("list")}><LayoutList className="ico" /></button>
                <button type="button" role="radio" aria-checked={layout === "board"} className={layout === "board" ? "is-on" : ""} title={uiText("看板")} onClick={() => setLayout("board")}><Columns3 className="ico" /></button>
              </div>
            </>
          )}
          <button type="button" className="tool" title={uiText("命令（⌘K）")} onClick={() => setPalette(true)}><Command className="ico" /></button>
        </header>
        {view.kind !== "projects" && <FilterBar filters={filters} setFilters={setFilters} me={me} />}
        {view.kind === "project" && <ProjectHead id={view.id} me={me} onGone={() => setView({ kind: "projects" })} />}
        {view.kind === "projects" ? <Projects onOpen={(id) => setView({ kind: "project", id })} /> : (
          <div className={`is-body${open ? " has-panel" : ""}`}>
            {layout === "list"
              ? <List groups={groups} folded={folded} setFolded={setFolded} focus={focus} open={open} me={me}
                  onOpen={(id) => { setFocus(id); setOpen(id); }} onMenu={(kind, id, at) => setMenu({ kind, ids: [id], at })}
                  onAdd={(s) => setCreating({ status: s, ...(view.kind === "project" ? { project: view.id } : {}) })} empty={!issues.rows.length} />
              : <Board groups={groups} focus={focus} open={open}
                  onOpen={(id) => { setFocus(id); setOpen(id); }} onMenu={(kind, id, at) => setMenu({ kind, ids: [id], at })}
                  onAdd={(s) => setCreating({ status: s, ...(view.kind === "project" ? { project: view.id } : {}) })} />}
            {open && issues.get(open) && <Panel id={open} me={me} onClose={() => setOpen(null)} onMenu={(kind, at) => setMenu({ kind, ids: [open], at })} />}
          </div>
        )}
      </main>

      {menu && <PropMenu kind={menu.kind} ids={menu.ids} at={menu.at} me={me} onClose={() => setMenu(null)} />}
      {teamAt && <TeamPop at={teamAt} onClose={() => setTeamAt(null)} />}
      {filterAt && <FilterPop at={filterAt} filters={filters} setFilters={setFilters} me={me} onClose={() => setFilterAt(null)} />}
      {creating && <Create init={creating} me={me} onClose={() => setCreating(null)} onMade={(id) => { setFocus(id); }} />}
      {palette && <Palette me={me} focus={focus || open} onClose={() => setPalette(false)}
        run={(a) => {
          setPalette(false);
          if (a.startsWith("go:")) { const k = a.slice(3); setView(k.startsWith("p:") ? { kind: "project", id: k.slice(2) } : { kind: k as "all" }); }
          else if (a === "new") setCreating({});
          else if (a === "list" || a === "board") setLayout(a);
          else if (a === "search") setTimeout(() => searchRef.current?.focus());
          else if (a.startsWith("status:") && (focus || open)) save((focus || open)!, { status: a.slice(7) });
          else if (a === "mine" && (focus || open)) save((focus || open)!, { assignee: me });
          else if (a === "copy" && (focus || open)) void copyForAgent((focus || open)!);
        }} />}
      <Toast kit={kit} />
    </div>
  );
}

/** The app's mark: a circle half done, as its icon. */
function Logo() {
  return (
    <svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true">
      <circle cx="7" cy="7" r="5.5" fill="none" stroke="currentColor" strokeWidth="1.5" /><path d="M7 4a3 3 0 0 1 0 6z" fill="currentColor" />
    </svg>
  );
}

/** The tracker's name and the key its issues are numbered with (MYC-12). */
function TeamPop({ at, onClose }: { at: Anchor; onClose: () => void }) {
  const d = team.get("main")?.data || {};
  const [key, setKey] = useState(teamKey());
  const saveKey = () => {
    const k = key.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 6) || "TASK";
    setKey(k);
    if (k !== teamKey()) void kit.track(team.put({ ...d, key: k }, "main"));
  };
  return (
    <Pop at={at} onClose={() => { saveKey(); onClose(); }} width={260}>
      <div className="team-pop">
        <label>{uiText("名称")}<input className="inp" value={teamTitle.get("")} placeholder={uiText("任务追踪")} onChange={(e) => teamTitle.set(e.target.value)} /></label>
        <label>{uiText("编号前缀")}<input className="inp" value={key} maxLength={6} onChange={(e) => setKey(e.target.value.toUpperCase())} onBlur={saveKey}
          onKeyDown={(e) => { if (e.key === "Enter") { saveKey(); onClose(); } }} /></label>
        <span className="faint">{uiText("问题显示成")}{" "}{key || "TASK"}{uiText("-12；改了以后所有问题跟着变。")}</span>
      </div>
    </Pop>
  );
}

function NavItem({ icon, label, on, n, onClick }: { icon: ReactNode; label: string; on: boolean; n?: number; onClick: () => void }) {
  return (
    <button type="button" className={`is-nav-item${on ? " is-on" : ""}`} onClick={onClick}>
      {icon}<span className="is-nav-label">{label}</span>{n ? <span className="is-nav-n">{n}</span> : null}
    </button>
  );
}

// ---- the list ----
type Group = { id: StatusId; name: string; rows: IssueRec[] };
function List({ groups, folded, setFolded, focus, open, me, onOpen, onMenu, onAdd, empty }: {
  groups: Group[]; folded: Record<string, boolean>; setFolded: (f: Record<string, boolean>) => void; focus: string | null; open: string | null; me: string;
  onOpen: (id: string) => void; onMenu: (k: MenuKind, id: string, at: Anchor) => void; onAdd: (s: StatusId) => void; empty: boolean;
}) {
  useEffect(() => { if (focus) document.querySelector(`[data-issue="${focus}"]`)?.scrollIntoView({ block: "nearest" }); }, [focus]);
  if (!groups.length) return (
    <div className="is-empty">
      {empty ? <><b>{uiText("还没有问题")}</b><span>{uiText("按")}{" "}<kbd>C</kbd> {" "}{uiText("新建一个，或者让 agent 帮你把要做的事建成问题。")}</span></> : <span>{uiText("这里没有问题")}</span>}
    </div>
  );
  return (
    <div className="is-list" role="list">
      {groups.map((g) => (
        <section key={g.id}>
          <div className="is-group">
            <button type="button" className="is-group-fold" aria-label={folded[g.id] ? uiText("展开") : uiText("收起")} onClick={() => setFolded({ ...folded, [g.id]: !folded[g.id] })}>
              {folded[g.id] ? <ChevronRight className="ico" /> : <ChevronDown className="ico" />}
            </button>
            <StatusIcon s={g.id} /><b>{g.name}</b><span className="faint">{g.rows.length}</span>
            <span className="top-gap" />
            <button type="button" className="icon-btn" title={uiText("新建「{0}」的问题", g.name)} aria-label={uiText("新建")} onClick={() => onAdd(g.id)}><Plus className="ico" /></button>
          </div>
          {!folded[g.id] && g.rows.map((r) => <Row key={r.id} r={r} focus={focus === r.id} open={open === r.id} me={me} onOpen={onOpen} onMenu={onMenu} />)}
        </section>
      ))}
    </div>
  );
}

function Row({ r, focus, open, me, onOpen, onMenu }: { r: IssueRec; focus: boolean; open: boolean; me: string; onOpen: (id: string) => void; onMenu: (k: MenuKind, id: string, at: Anchor) => void }) {
  const d = r.data, s = statusOf(r);
  const proj = d.project ? projects.get(String(d.project)) : null;
  const late = d.due && !isClosed(s) && String(d.due) < new Date().toISOString().slice(0, 10);
  const m = (k: MenuKind) => (e: React.MouseEvent) => { e.stopPropagation(); onMenu(k, r.id, rectOf(e.currentTarget)); };
  return (
    <div role="listitem" data-issue={r.id} className={`is-row${focus ? " is-focus" : ""}${open ? " is-open" : ""}${isClosed(s) ? " is-closed" : ""}`} onClick={() => onOpen(r.id)}>
      <button type="button" className="is-cell" title={uiText("优先级：{0}", priorityName(num(d.priority, 0)))} onClick={m("priority")}><PriorityIcon p={num(d.priority, 0)} /></button>
      <span className="is-key">{keyOf(r)}</span>
      <button type="button" className="is-cell" title={uiText("状态：{0}", statusName(s))} onClick={m("status")}><StatusIcon s={s} /></button>
      <span className="is-title">{d.title || uiText("（没有标题）")}</span>
      <span className="is-meta">
        {(d.labels || []).slice(0, 3).map((l) => <span key={l} className="chip"><LabelDot l={l} />{l}</span>)}
        {proj && <span className="chip"><FolderKanban className="ico" />{String(proj.data.name || "项目")}</span>}
        {d.due && <span className={`chip${late ? " is-late" : ""}`}><Calendar className="ico" />{String(d.due).slice(5)}</span>}
        <span className="is-date faint">{timeAgo(String(d.created || ""))}</span>
        <button type="button" className="is-cell" title={d.assignee ? uiText("负责人：{0}{1}", d.assignee, d.assignee === me ? "（我）" : "") : uiText("指派")} onClick={m("assignee")}><Avatar name={d.assignee ? String(d.assignee) : null} /></button>
      </span>
    </div>
  );
}

// ---- the board ----
function Board({ groups, focus, open, onOpen, onMenu, onAdd }: {
  groups: Group[]; focus: string | null; open: string | null; onOpen: (id: string) => void; onMenu: (k: MenuKind, id: string, at: Anchor) => void; onAdd: (s: StatusId) => void;
}) {
  const [drag, setDrag] = useState<string | null>(null);
  const [over, setOver] = useState<{ col: string; before: string | null } | null>(null);
  const drop = (col: StatusId, before: string | null) => {
    const r = drag ? issues.get(drag) : null;
    setDrag(null); setOver(null);
    if (!r) return;
    const rows = (groups.find((g) => g.id === col)?.rows || []).filter((x) => x.id !== r.id);
    const i = before ? rows.findIndex((x) => x.id === before) : rows.length;
    const at = i < 0 ? rows.length : i;
    const prev = rows[at - 1], next = rows[at];
    const o = (x: IssueRec | undefined, d: number) => num(x?.data.order, d);
    const order = prev && next ? (o(prev, 0) + o(next, 0)) / 2 : prev ? o(prev, 0) + 1 : next ? o(next, 0) - 1 : 0;
    save(r.id, { status: col, order });
  };
  return (
    <div className="is-board">
      {groups.map((g) => (
        <section key={g.id} className={`is-col${over?.col === g.id ? " is-over" : ""}`}
          onDragOver={(e) => { e.preventDefault(); if (over?.col !== g.id) setOver({ col: g.id, before: null }); }}
          onDrop={(e) => { e.preventDefault(); drop(g.id, over?.col === g.id ? over.before : null); }}>
          <div className="is-col-head">
            <StatusIcon s={g.id} /><b>{g.name}</b><span className="faint">{g.rows.length}</span>
            <span className="top-gap" />
            <button type="button" className="icon-btn" aria-label={uiText("新建")} title={uiText("新建「{0}」的问题", g.name)} onClick={() => onAdd(g.id)}><Plus className="ico" /></button>
          </div>
          <div className="is-col-body">
            {g.rows.map((r) => {
              const d = r.data;
              return (
                <div key={r.id} data-issue={r.id} draggable className={`is-card${focus === r.id ? " is-focus" : ""}${open === r.id ? " is-open" : ""}${drag === r.id ? " is-drag" : ""}${over?.before === r.id ? " is-before" : ""}`}
                  onDragStart={(e) => { setDrag(r.id); e.dataTransfer.effectAllowed = "move"; e.dataTransfer.setData("text/plain", keyOf(r)); }}
                  onDragEnd={() => { setDrag(null); setOver(null); }}
                  onDragOver={(e) => { e.preventDefault(); e.stopPropagation(); if (over?.before !== r.id) setOver({ col: g.id, before: r.id }); }}
                  onDrop={(e) => { e.preventDefault(); e.stopPropagation(); drop(g.id, r.id); }}
                  onClick={() => onOpen(r.id)}>
                  <div className="is-card-top">
                    <span className="is-key">{keyOf(r)}</span>
                    <span className="top-gap" />
                    <button type="button" className="is-cell" onClick={(e) => { e.stopPropagation(); onMenu("assignee", r.id, rectOf(e.currentTarget)); }}><Avatar name={d.assignee ? String(d.assignee) : null} /></button>
                  </div>
                  <div className="is-card-title">{d.title || uiText("（没有标题）")}</div>
                  <div className="is-card-meta">
                    <button type="button" className="is-cell is-boxed" onClick={(e) => { e.stopPropagation(); onMenu("priority", r.id, rectOf(e.currentTarget)); }}><PriorityIcon p={num(d.priority, 0)} /></button>
                    {(d.labels || []).slice(0, 2).map((l) => <span key={l} className="chip"><LabelDot l={l} />{l}</span>)}
                    {d.due && <span className="chip"><Calendar className="ico" />{String(d.due).slice(5)}</span>}
                  </div>
                </div>
              );
            })}
            {!g.rows.length && <div className="is-col-empty">{uiText("拖到这里")}</div>}
          </div>
        </section>
      ))}
    </div>
  );
}

// ---- an issue's page, beside the list ----
async function copyForAgent(id: string) {
  const r = issues.get(id);
  if (!r) return;
  const text = `帮我处理 myco 任务追踪（网页 ${kit.page}）里的 ${keyOf(r)}「${r.data.title || ""}」。先运行 myco template get myco/issues 看它的数据怎么读写，` +
    `再用 myco page data get --page ${kit.page} --collection issues --id ${r.id} 读这个问题、` +
    `myco page data list --page ${kit.page} --collection comments 读评论；开始做时把状态改成 progress，做完改成 review 或 done，并留一条评论说明做了什么。`;
  try { await navigator.clipboard.writeText(text); kit.flash(uiText("已复制，发给负责的 agent 就行")); }
  catch { kit.flash(uiText("没复制上")); }
}

function Panel({ id, me, onClose, onMenu }: { id: string; me: string; onClose: () => void; onMenu: (k: MenuKind, at: Anchor) => void }) {
  const r = issues.get(id)!;
  const d = r.data, s = statusOf(r);
  const [title, setTitle] = useState<string | null>(null);
  const [desc, setDesc] = useState<string | null>(null);
  const [say, setSay] = useState("");
  useEffect(() => { setTitle(null); setDesc(null); setSay(""); }, [id]);
  const proj = d.project ? projects.get(String(d.project)) : null;
  const notes = comments.rows.filter((c) => c.data.issue === id).sort((a, b) => String(a.data.at || "").localeCompare(String(b.data.at || "")));
  const m = (k: MenuKind) => (e: React.MouseEvent) => onMenu(k, rectOf(e.currentTarget));
  const send = () => {
    const body = say.trim();
    if (!body) return;
    void kit.track(comments.put({ issue: id, body, by: me, at: nowIso() }, uid("c")));
    setSay("");
  };
  const remove = () => {
    const keep = { ...r.data }, keepNotes = notes.map((c) => ({ id: c.id, data: { ...c.data } }));
    void kit.track(issues.remove(id));
    for (const c of notes) void kit.track(comments.remove(c.id));
    onClose();
    kit.flash(uiText("已删除 {0}", keyOf(r)), () => {
      void kit.track(issues.put(keep, id));
      for (const c of keepNotes) void kit.track(comments.put(c.data, c.id));
    });
  };
  return (
    <aside className="is-panel" aria-label={keyOf(r)}>
      <div className="is-panel-head">
        <span className="is-key">{keyOf(r)}</span>
        <span className="top-gap" />
        <button type="button" className="tool" onClick={() => void copyForAgent(id)} title={uiText("复制一段话，交给 agent 去做")}><Bot className="ico" />{uiText("交给 agent")}</button>
        <button type="button" className="icon-btn" title={uiText("复制编号")} aria-label={uiText("复制编号")} onClick={() => { void navigator.clipboard.writeText(keyOf(r)).then(() => kit.flash(uiText("已复制 {0}", keyOf(r))), () => undefined); }}><Copy className="ico" /></button>
        <button type="button" className="icon-btn" title={uiText("删除")} aria-label={uiText("删除")} onClick={remove}><Trash2 className="ico" /></button>
        <button type="button" className="icon-btn" title={uiText("关闭（Esc）")} aria-label={uiText("关闭")} onClick={onClose}><X className="ico" /></button>
      </div>
      <div className="is-panel-body">
        <textarea className="is-panel-title" rows={1} value={title ?? String(d.title || "")} placeholder={uiText("问题标题")}
          onChange={(e) => { setTitle(e.target.value); e.target.style.height = "auto"; e.target.style.height = `${e.target.scrollHeight}px`; }}
          onBlur={() => { if (title !== null && title.trim() !== (d.title || "")) save(id, { title: title.trim() }); setTitle(null); }}
          onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); (e.target as HTMLTextAreaElement).blur(); } }} />
        <div className="is-props">
          <button type="button" className="is-prop" onClick={m("status")}><StatusIcon s={s} />{statusName(s)}</button>
          <button type="button" className="is-prop" onClick={m("priority")}><PriorityIcon p={num(d.priority, 0)} />{priorityName(num(d.priority, 0))}</button>
          <button type="button" className="is-prop" onClick={m("assignee")}><Avatar name={d.assignee ? String(d.assignee) : null} />{d.assignee ? String(d.assignee) : uiText("未指派")}</button>
          <button type="button" className="is-prop" onClick={m("labels")}><Tag className="ico" />{(d.labels || []).length ? (d.labels || []).join("、") : uiText("标签")}</button>
          <button type="button" className="is-prop" onClick={m("project")}><FolderKanban className="ico" />{proj ? String(proj.data.name || "项目") : uiText("项目")}</button>
          <button type="button" className="is-prop" onClick={m("due")}><Calendar className="ico" />{d.due ? uiText("截止 {0}", d.due) : uiText("截止日期")}</button>
        </div>
        {desc !== null ? (
          <textarea className="is-desc-edit inp" autoFocus value={desc} placeholder={uiText("写点描述…（支持 Markdown）")} rows={8}
            onChange={(e) => setDesc(e.target.value)}
            onBlur={() => { if (desc !== (d.desc || "")) save(id, { desc }); setDesc(null); }}
            onKeyDown={(e) => { if (e.key === "Escape") { e.stopPropagation(); (e.target as HTMLTextAreaElement).blur(); } }} />
        ) : d.desc ? (
          <div className="is-desc md" onClick={(e) => { if (!(e.target as HTMLElement).closest("a")) setDesc(String(d.desc || "")); }} dangerouslySetInnerHTML={md(String(d.desc))} />
        ) : (
          <button type="button" className="is-desc-empty" onClick={() => setDesc("")}>{uiText("写点描述…")}</button>
        )}
        <div className="is-activity">
          <div className="is-activity-head">{uiText("动态")}</div>
          <div className="is-event"><Avatar name={String(d.by || "")} size={16} /><span><b>{String(d.by || "有人")}</b> {" "}{uiText("创建了这个问题")}</span><span className="faint">{timeAgo(String(d.created || ""))}</span></div>
          {d.done_at && <div className="is-event"><StatusIcon s={s} /><span>{s === "canceled" ? uiText("取消了") : uiText("完成了")}</span><span className="faint">{timeAgo(String(d.done_at))}</span></div>}
          {notes.map((c) => (
            <div key={c.id} className="is-comment">
              <div className="is-comment-head"><Avatar name={String(c.data.by || "")} size={18} /><b>{String(c.data.by || "有人")}</b><span className="faint">{timeAgo(String(c.data.at || ""))}</span>
                {c.data.by === me && <button type="button" className="icon-btn" aria-label={uiText("删除评论")} onClick={() => void kit.track(comments.remove(c.id))}><Trash2 className="ico" /></button>}
              </div>
              <div className="md" dangerouslySetInnerHTML={md(String(c.data.body || ""))} />
            </div>
          ))}
          <div className="is-say">
            <textarea className="inp" rows={3} value={say} placeholder={uiText("写评论…（⌘⏎ 发送；agent 也在这里说做了什么）")} onChange={(e) => setSay(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); send(); } }} />
            <button type="button" className="btn btn-primary" disabled={!say.trim()} onClick={send}>{uiText("评论")}</button>
          </div>
        </div>
      </div>
    </aside>
  );
}

// ---- new issue ----
function Create({ init, me, onClose, onMade }: { init: Partial<Issue>; me: string; onClose: () => void; onMade: (id: string) => void }) {
  const [d, setD] = useState<Issue>({ status: "todo", priority: 0, assignee: null, labels: [], project: null, due: null, ...init });
  const [more, setMore] = useState(false);
  const [menu, setMenu] = useState<null | { kind: MenuKind; at: Anchor }>(null);
  const [busy, setBusy] = useState(false);
  const titleRef = useRef<HTMLInputElement>(null);
  const make = async () => {
    if (!String(d.title || "").trim() || busy) return;
    setBusy(true);
    try {
      const r = await kit.track(createIssue({ ...d, title: String(d.title).trim() }, me));
      if (r) { kit.flash(uiText("已创建 {0}", keyOf(r))); onMade(r.id); }
      if (more) { setD({ ...d, title: "", desc: "" }); titleRef.current?.focus(); } else onClose();
    } finally { setBusy(false); }
  };
  // the property menus here change the draft, not a record
  const pick = (kind: MenuKind, at: Anchor) => setMenu({ kind, at });
  const proj = d.project ? projects.get(String(d.project)) : null;
  return (
    <div className="modal-back" role="dialog" aria-modal="true" aria-label={uiText("新建问题")} onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="modal is-create" onKeyDown={(e) => {
        if (e.key === "Escape" && !menu) { e.preventDefault(); onClose(); }
        if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); void make(); }
      }}>
        <div className="modal-head"><span className="is-key">{teamKey()}</span><span className="faint">{uiText("新建问题")}</span></div>
        <input ref={titleRef} className="is-create-title" autoFocus value={d.title || ""} placeholder={uiText("问题标题")} onChange={(e) => setD({ ...d, title: e.target.value })}
          onKeyDown={(e) => { if (e.key === "Enter" && !e.metaKey && !e.ctrlKey) { e.preventDefault(); void make(); } }} />
        <textarea className="is-create-desc" rows={4} value={d.desc || ""} placeholder={uiText("补充描述…（支持 Markdown）")} onChange={(e) => setD({ ...d, desc: e.target.value })} />
        <div className="is-props">
          <button type="button" className="is-prop" onClick={(e) => pick("status", rectOf(e.currentTarget))}><StatusIcon s={String(d.status)} />{statusName(String(d.status))}</button>
          <button type="button" className="is-prop" onClick={(e) => pick("priority", rectOf(e.currentTarget))}><PriorityIcon p={num(d.priority, 0)} />{priorityName(num(d.priority, 0))}</button>
          <button type="button" className="is-prop" onClick={(e) => pick("assignee", rectOf(e.currentTarget))}><Avatar name={d.assignee || null} />{d.assignee || uiText("负责人")}</button>
          <button type="button" className="is-prop" onClick={(e) => pick("labels", rectOf(e.currentTarget))}><Tag className="ico" />{d.labels?.length ? d.labels.join("、") : uiText("标签")}</button>
          <button type="button" className="is-prop" onClick={(e) => pick("project", rectOf(e.currentTarget))}><FolderKanban className="ico" />{proj ? String(proj.data.name || "项目") : uiText("项目")}</button>
        </div>
        <div className="modal-foot">
          <label className="is-more"><input type="checkbox" checked={more} onChange={(e) => setMore(e.target.checked)} />{uiText("继续新建")}</label>
          <span className="row">
            <button type="button" className="btn" onClick={onClose}>{uiText("取消")}</button>
            <button type="button" className="btn btn-primary" disabled={!String(d.title || "").trim() || busy} onClick={() => void make()}>{uiText("新建问题")}<kbd>⌘⏎</kbd></button>
          </span>
        </div>
        {menu && <DraftMenu kind={menu.kind} at={menu.at} d={d} setD={setD} me={me} onClose={() => { setMenu(null); titleRef.current?.focus(); }} />}
      </div>
    </div>
  );
}
function DraftMenu({ kind, at, d, setD, me, onClose }: { kind: MenuKind; at: Anchor; d: Issue; setD: (d: Issue) => void; me: string; onClose: () => void }) {
  if (kind === "status") return <Pick at={at} placeholder={uiText("状态…")} onClose={onClose} onPick={(s) => setD({ ...d, status: s })}
    opts={STATUSES.map((s) => ({ id: s.id, label: s.name, icon: <StatusIcon s={s.id} />, on: d.status === s.id }))} />;
  if (kind === "priority") return <Pick at={at} placeholder={uiText("优先级…")} onClose={onClose} onPick={(p) => setD({ ...d, priority: Number(p) })}
    opts={PRIORITIES.map((p) => ({ id: String(p.id), label: p.name, icon: <PriorityIcon p={p.id} />, on: num(d.priority, 0) === p.id }))} />;
  if (kind === "assignee") return <Pick at={at} placeholder={uiText("指派给…")} onClose={onClose} onPick={(a) => setD({ ...d, assignee: a === "\u0000" ? null : a })}
    create={(q) => ({ id: q, label: uiText("指派给「{0}」", q), icon: <Avatar name={q} /> })}
    opts={[{ id: "\u0000", label: uiText("不指派"), icon: <Avatar name={null} /> }, ...people(me).map((p) => ({ id: p, label: p === me ? `${p}（我）` : p, icon: <Avatar name={p} />, on: d.assignee === p }))]} />;
  if (kind === "labels") return <Pick at={at} multi placeholder={uiText("标签…")} onClose={onClose}
    create={(q) => ({ id: q, label: uiText("新标签「{0}」", q), icon: <Tag className="ico" /> })}
    onPick={(l) => { const s = new Set(d.labels || []); if (s.has(l)) s.delete(l); else s.add(l); setD({ ...d, labels: [...s] }); }}
    opts={allLabels().map((l) => ({ id: l, label: l, icon: <LabelDot l={l} />, on: (d.labels || []).includes(l) }))} />;
  return <Pick at={at} placeholder={uiText("项目…")} onClose={onClose} onPick={(p) => setD({ ...d, project: p === "\u0000" ? null : p })}
    opts={[{ id: "\u0000", label: uiText("不放进项目"), icon: <FolderKanban className="ico" /> }, ...projects.rows.map((p) => ({ id: p.id, label: String(p.data.name || "项目"), icon: <FolderKanban className="ico" />, on: d.project === p.id }))]} />;
}

// ---- filters ----
type FKind = keyof Filters;
const F_TABS: [FKind, string][] = [["status", "状态"], ["priority", "优先级"], ["assignee", "负责人"], ["labels", "标签"]];
/** What can be picked in a filter, with how many issues each matches. */
function filterOptions(k: FKind, me: string): { v: string | number; label: string; icon: ReactNode; n: number }[] {
  const rows = issues.rows;
  if (k === "status") return STATUSES.map((s) => ({ v: s.id, label: s.name, icon: <StatusIcon s={s.id} />, n: rows.filter((r) => statusOf(r) === s.id).length }));
  if (k === "priority") return PRIORITIES.map((p) => ({ v: p.id, label: p.name, icon: <PriorityIcon p={p.id} />, n: rows.filter((r) => num(r.data.priority, 0) === p.id).length }));
  if (k === "assignee") return [{ v: "", label: uiText("未指派"), icon: <Avatar name={null} />, n: rows.filter((r) => !r.data.assignee).length },
    ...people(me).map((p) => ({ v: p, label: p === me ? `${p}（我）` : p, icon: <Avatar name={p} />, n: rows.filter((r) => r.data.assignee === p).length }))];
  return allLabels().map((l) => ({ v: l, label: l, icon: <LabelDot l={l} />, n: rows.filter((r) => (r.data.labels || []).includes(l)).length }));
}
function toggleIn(filters: Filters, k: FKind, v: string | number): Filters {
  const cur = filters[k] as (string | number)[];
  return { ...filters, [k]: cur.includes(v) ? cur.filter((x) => x !== v) : [...cur, v] };
}

function FilterPop({ at, filters, setFilters, me, onClose }: { at: Anchor; filters: Filters; setFilters: (f: Filters) => void; me: string; onClose: () => void }) {
  const [tab, setTab] = useState<FKind>("status");
  const opts = filterOptions(tab, me);
  const total = F_TABS.reduce((n, [k]) => n + filters[k].length, 0);
  return (
    <Pop at={at} onClose={onClose} width={320}>
      <div className="filter-pop">
        <div className="filter-tabs" role="tablist">
          {F_TABS.map(([k, name]) => (
            <button key={k} type="button" role="tab" aria-selected={tab === k} className={tab === k ? "is-on" : ""} onClick={() => setTab(k)}>
              {name}{filters[k].length ? <span className="filter-n">{filters[k].length}</span> : null}
            </button>
          ))}
        </div>
        <div className="filter-list" role="listbox" aria-multiselectable="true">
          {opts.map((o) => {
            const on = (filters[tab] as (string | number)[]).includes(o.v);
            return (
              <button key={String(o.v)} type="button" role="option" aria-selected={on} className={`filter-row${on ? " is-on" : ""}`} onClick={() => setFilters(toggleIn(filters, tab, o.v))}>
                <span className="filter-box">{on && <Check className="ico" />}</span>
                <span className="filter-ico">{o.icon}</span>
                <span className="filter-label">{o.label}</span>
                <span className="filter-count">{o.n}</span>
              </button>
            );
          })}
          {!opts.length && <div className="pick-empty">{uiText("还没有")}{tab === "labels" ? uiText("标签") : uiText("可选的")}</div>}
        </div>
        <div className="filter-foot">
          <button type="button" className="link-btn" disabled={!total} onClick={() => setFilters(NO_FILTERS)}>{uiText("清除全部")}</button>
          <button type="button" className="btn btn-primary" onClick={onClose}>{uiText("完成")}</button>
        </div>
      </div>
    </Pop>
  );
}

/** The filters in use, under the header: one chip a kind, each removable. */
function FilterBar({ filters, setFilters, me }: { filters: Filters; setFilters: (f: Filters) => void; me: string }) {
  const used = F_TABS.filter(([k]) => filters[k].length);
  if (!used.length) return null;
  const nameOf = (k: FKind, v: string | number) => {
    if (k === "status") return statusName(String(v));
    if (k === "priority") return priorityName(Number(v));
    if (k === "assignee") return v === "" ? "未指派" : v === me ? `${v}（我）` : String(v);
    return String(v);
  };
  return (
    <div className="filter-bar">
      {used.map(([k, name]) => (
        <span key={k} className="filter-chip">
          <span className="faint">{name}</span>
          <span>{(filters[k] as (string | number)[]).map((v) => nameOf(k, v)).join("、")}</span>
          <button type="button" aria-label={uiText("去掉「{0}」筛选", name)} onClick={() => setFilters({ ...filters, [k]: [] })}><X className="ico" /></button>
        </span>
      ))}
      <button type="button" className="link-btn" onClick={() => setFilters(NO_FILTERS)}>{uiText("清除")}</button>
    </div>
  );
}

// ---- projects ----
function progressOf(pid: string) {
  const all = issues.rows.filter((r) => r.data.project === pid && statusOf(r) !== "canceled");
  const done = all.filter((r) => statusOf(r) === "done").length;
  return { all: all.length, done, pct: all.length ? done / all.length : 0 };
}
function Projects({ onOpen }: { onOpen: (id: string) => void }) {
  if (!projects.rows.length) return <div className="is-empty"><b>{uiText("还没有项目")}</b><span>{uiText("点左边「项目」旁的 ＋ 新建一个，把相关的问题放进去，就能看到进度。")}</span></div>;
  return (
    <div className="is-projects">
      <div className="is-proj-row is-head-row"><span>{uiText("项目")}</span><span>{uiText("状态")}</span><span>{uiText("负责人")}</span><span>{uiText("目标日期")}</span><span>{uiText("进度")}</span></div>
      {projects.rows.map((p) => {
        const g = progressOf(p.id);
        return (
          <button key={p.id} type="button" className="is-proj-row" onClick={() => onOpen(p.id)}>
            <span className="is-proj-name"><FolderKanban className="ico" />{String(p.data.name || "项目")}</span>
            <span className="faint">{PROJECT_STATUSES.find((s) => s.id === p.data.status)?.name || uiText("计划中")}</span>
            <span className="is-proj-lead">{p.data.lead ? <><Avatar name={String(p.data.lead)} />{String(p.data.lead)}</> : <span className="faint">—</span>}</span>
            <span className="faint">{p.data.target ? String(p.data.target) : "—"}</span>
            <span className="is-prog"><span className="is-prog-bar"><i style={{ width: `${Math.round(g.pct * 100)}%` }} /></span><span className="faint">{g.done}/{g.all}</span></span>
          </button>
        );
      })}
    </div>
  );
}
function ProjectHead({ id, me, onGone }: { id: string; me: string; onGone: () => void }) {
  const p = projects.get(id);
  const [name, setName] = useState<string | null>(null);
  const [menu, setMenu] = useState<null | { kind: "status" | "lead"; at: Anchor }>(null);
  if (!p) return null;
  const d = p.data;
  const set = (patch: Record<string, unknown>) => void kit.track(projects.put({ ...p.data, ...patch }, id));
  const g = progressOf(id);
  return (
    <div className="is-proj-head">
      <input className="is-proj-title" value={name ?? String(d.name || "")} placeholder={uiText("项目名")} onChange={(e) => setName(e.target.value)}
        onBlur={() => { if (name !== null) set({ name: name.trim() || "项目" }); setName(null); }} onKeyDown={(e) => { if (e.key === "Enter") (e.target as HTMLInputElement).blur(); }} />
      <button type="button" className="is-prop" onClick={(e) => setMenu({ kind: "status", at: rectOf(e.currentTarget) })}>{PROJECT_STATUSES.find((s) => s.id === d.status)?.name || uiText("计划中")}</button>
      <button type="button" className="is-prop" onClick={(e) => setMenu({ kind: "lead", at: rectOf(e.currentTarget) })}><Users className="ico" />{d.lead ? String(d.lead) : uiText("负责人")}</button>
      <label className="is-prop"><Calendar className="ico" /><input type="date" value={String(d.target || "")} onChange={(e) => set({ target: e.target.value || null })} aria-label={uiText("目标日期")} /></label>
      <span className="is-prog"><span className="is-prog-bar"><i style={{ width: `${Math.round(g.pct * 100)}%` }} /></span><span className="faint">{g.done}/{g.all} {" "}{uiText("完成")}</span></span>
      <span className="top-gap" />
      <button type="button" className="icon-btn" title={uiText("删除项目（问题会留下）")} aria-label={uiText("删除项目")} onClick={() => {
        const keep = { ...p.data };
        void kit.track(projects.remove(id));
        onGone();
        kit.flash(uiText("已删除项目，问题都还在"), () => void kit.track(projects.put(keep, id)));
      }}><Trash2 className="ico" /></button>
      {menu?.kind === "status" && <Pick at={menu.at} placeholder={uiText("项目状态…")} onClose={() => setMenu(null)} onPick={(s) => set({ status: s })}
        opts={PROJECT_STATUSES.map((s) => ({ id: s.id, label: s.name, on: d.status === s.id }))} />}
      {menu?.kind === "lead" && <Pick at={menu.at} placeholder={uiText("负责人…")} onClose={() => setMenu(null)} onPick={(a) => set({ lead: a === "\u0000" ? null : a })}
        create={(q) => ({ id: q, label: `「${q}」`, icon: <Avatar name={q} /> })}
        opts={[{ id: "\u0000", label: uiText("没有"), icon: <Avatar name={null} /> }, ...people(me).map((x) => ({ id: x, label: x, icon: <Avatar name={x} />, on: d.lead === x }))]} />}
    </div>
  );
}

// ---- ⌘K ----
function Palette({ me, focus, onClose, run }: { me: string; focus: string | null; onClose: () => void; run: (a: string) => void }) {
  const [q, setQ] = useState("");
  const [i, setI] = useState(0);
  const cur = focus ? issues.get(focus) : null;
  const all: { id: string; label: string; hint?: string; icon: ReactNode }[] = [
    { id: "new", label: uiText("新建问题"), hint: "C", icon: <Plus className="ico" /> },
    { id: "search", label: uiText("搜索问题"), hint: "/", icon: <Search className="ico" /> },
    ...(cur ? [
      ...STATUSES.map((s, k) => ({ id: `status:${s.id}`, label: uiText("{0} 改成「{1}」", keyOf(cur), s.name), hint: String(k + 1), icon: <StatusIcon s={s.id} /> })),
      { id: "mine", label: uiText("{0} 指派给我", keyOf(cur)), hint: "I", icon: <Avatar name={me} /> },
      { id: "copy", label: uiText("{0} 交给 agent（复制给它的话）", keyOf(cur)), icon: <Bot className="ico" /> },
    ] : []),
    { id: "go:mine", label: uiText("去「我的问题」"), icon: <Inbox className="ico" /> },
    { id: "go:all", label: uiText("去「全部问题」"), icon: <ListTodo className="ico" /> },
    { id: "go:active", label: uiText("去「进行中」"), icon: <CircleDot className="ico" /> },
    { id: "go:backlog", label: uiText("去「待规划」"), icon: <Clock className="ico" /> },
    { id: "go:projects", label: uiText("去「项目」"), icon: <FolderKanban className="ico" /> },
    ...projects.rows.map((p) => ({ id: `go:p:${p.id}`, label: uiText("去项目「{0}」", String(p.data.name || "项目")), icon: <FolderKanban className="ico" /> })),
    { id: "list", label: uiText("按列表显示"), icon: <LayoutList className="ico" /> },
    { id: "board", label: uiText("按看板显示"), icon: <Columns3 className="ico" /> },
  ];
  const list = all.filter((a) => !q || a.label.toLowerCase().includes(q.toLowerCase()));
  return (
    <div className="modal-back is-pal-back" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="is-pal" role="dialog" aria-label={uiText("命令")}>
        <input autoFocus value={q} placeholder={uiText("输入命令…")} onChange={(e) => { setQ(e.target.value); setI(0); }}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") { e.preventDefault(); setI((x) => Math.min(list.length - 1, x + 1)); }
            else if (e.key === "ArrowUp") { e.preventDefault(); setI((x) => Math.max(0, x - 1)); }
            else if (e.key === "Enter") { e.preventDefault(); if (list[i]) run(list[i].id); }
            else if (e.key === "Escape") { e.preventDefault(); onClose(); }
          }} />
        <div className="pick-list">
          {list.map((a, k) => (
            <button key={a.id} type="button" className={`pick-item${k === i ? " is-cur" : ""}`} onMouseEnter={() => setI(k)} onClick={() => run(a.id)}>
              <span className="pick-ico">{a.icon}</span><span className="pick-label">{a.label}</span>{a.hint && <kbd>{a.hint}</kbd>}
            </button>
          ))}
          {!list.length && <div className="pick-empty">{uiText("没有这个命令")}</div>}
        </div>
      </div>
    </div>
  );
}

mount(kit, App);
