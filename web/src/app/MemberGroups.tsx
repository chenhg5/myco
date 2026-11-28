import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { ChevronRight, MoreHorizontal } from "lucide-react";
import { L, E } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import type { AgentRow } from "@/app/types";
import { Avatar } from "@/app/persona";
import { DRAG_TYPE, type DragAgent } from "@/app/Panes";
import { useContextMenu, type MenuItem } from "@/app/ContextMenu";
import { Modal } from "@/app/dialogs";

/*
 * 成员分组: a project's members in groups (前端, 后端, 应用...), kept by the
 * hub per project (Project.groups, ProjectMember.group; hubs from v0.21.34).
 * Three ways to show them, the person's pick (kept on this computer):
 *   fold   — a heading per group that folds; folded, the group's faces and
 *            how many are at work stay on its heading (the default);
 *   faces  — a group is one row: a tile of its faces and what they do,
 *            opened with a click;
 *   chips  — one list, a row of chips on top shows one group at a time.
 * The coordinator stays on top, in no group; those in none are under 其他.
 * A member is dragged onto a group (fold) or moved from its menu.
 */

export type GroupStyle = "fold" | "faces" | "chips";
/** A group dragged to a new place among the groups. */
const GROUP_DRAG = "application/x-myco-group";
const STYLE_KEY = "myco-member-groups-style";
const FOLD_KEY = "myco-member-groups-folded:";
export const OTHER = () => L("其他", "Other");

export function readStyle(): GroupStyle {
  try { const v = localStorage.getItem(STYLE_KEY); return v === "faces" || v === "chips" ? v : "fold"; } catch { return "fold"; }
}
/** The way groups show, and a setter that keeps it (and tells the others). */
export function useGroupStyle(): [GroupStyle, (s: GroupStyle) => void] {
  const [s, setS] = useState<GroupStyle>(readStyle);
  useEffect(() => {
    const on = () => setS(readStyle());
    window.addEventListener("myco-member-groups-style", on);
    return () => window.removeEventListener("myco-member-groups-style", on);
  }, []);
  return [s, (v) => {
    try { localStorage.setItem(STYLE_KEY, v); } catch { /* not kept */ }
    setS(v);
    window.dispatchEvent(new Event("myco-member-groups-style"));
  }];
}

/** A project's groups, as the members column uses them. */
export type GroupsCtl = {
  /** the groups, in order */
  list: string[];
  /** a member's group ("" for none) */
  groupOf: (agent: string) => string;
  /** may change them (can_edit) */
  canEdit: boolean;
  /** why the last change did not save ("" when it did) */
  error?: string;
  /** saves the groups in order and who goes where (only those given) */
  save: (groups: string[], members?: Record<string, string>) => Promise<void>;
};

export type Group = { name: string; other: boolean; rows: AgentRow[] };
/** The members in their groups, in the groups' order, then those in none
 *  (其他); empty groups are kept (a new one shows until someone is in it). */
export function grouped(rows: AgentRow[], ctl: GroupsCtl): Group[] {
  const out: Group[] = ctl.list.map((g) => ({ name: g, other: false, rows: rows.filter((r) => ctl.groupOf(r.st.agent.name) === g) }));
  const rest = rows.filter((r) => !ctl.list.includes(ctl.groupOf(r.st.agent.name)));
  if (rest.length) out.push({ name: OTHER(), other: true, rows: rest });
  return out;
}

/** A member's menu item to move it to another group: one line (with its
 *  group), which opens the short list of groups in the menu's place. */
export function moveItems(r: AgentRow, ctl: GroupsCtl | undefined, onNew: () => void, again: (items: MenuItem[]) => void): MenuItem[] {
  if (!ctl || !ctl.canEdit) return [];
  const mine = ctl.groupOf(r.st.agent.name);
  const to = (g: string) => void ctl.save(ctl.list, { [r.st.agent.name]: g }).catch(() => undefined);
  const list: MenuItem[] = [
    { header: L("移到分组", "Move to a group") },
    ...ctl.list.map((g) => ({ label: g, checked: mine === g, onSelect: () => to(g) })),
    ...(ctl.list.length ? [{ label: OTHER(), checked: !ctl.list.includes(mine), onSelect: () => to("") }] : []),
    "sep",
    { label: L("新建分组", "New group"), onSelect: onNew },
  ];
  return [
    "sep",
    ctl.list.length
      ? { label: L("移到分组", "Move to a group"), hint: ctl.list.includes(mine) ? mine : OTHER(), onSelect: () => again(list) }
      : { label: L("新建分组", "New group"), onSelect: onNew },
  ];
}

/** Asks for a group's name (new, or renamed). */
export function GroupNameDialog({ open, onClose, initial, taken, onDone, title }: {
  open: boolean; onClose: () => void; initial: string; taken: string[]; title: string;
  onDone: (name: string) => Promise<void>;
}) {
  const [v, setV] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  useEffect(() => { if (open) { setV(initial); setErr(""); } }, [open, initial]);
  const name = v.trim();
  const why = !name ? "" : [...name].length > 40 ? L("最多 40 个字", "40 characters at most")
    : name !== initial && taken.includes(name) ? L("已经有这个分组了", "There is a group by that name") : "";
  const go = async () => {
    if (!name || why || busy) return;
    if (name === initial) { onClose(); return; }
    setBusy(true);
    try { await onDone(name); onClose(); } catch (e) { setErr(e instanceof Error ? e.message : String(e)); } finally { setBusy(false); }
  };
  return (
    <Modal open={open} onOpenChange={(o) => !o && onClose()} title={title}
      description={initial ? L("组里的成员会跟着到新名字下。", "Its members move with it.") : L("把做类似事情的成员放在一起，只在这个项目里。", "Members who do similar work, together; only in this project.")}>
      <form onSubmit={(e) => { e.preventDefault(); void go(); }}>
        <label className="app-field">
          <span className="app-field-label">{L("名字", "Name")}</span>
          <input className="app-input" autoFocus value={v} onChange={(e) => setV(e.target.value)} placeholder={L("比如：前端、后端、应用", "e.g. Frontend, Backend, Apps")} />
        </label>
        <div className="app-modal-actions">
          <span className="app-modal-hint">{why || err}</span>
          <button type="button" className="app-btn" onClick={onClose}>{L("取消", "Cancel")}</button>
          <button type="submit" className="app-btn app-btn-primary" disabled={!name || !!why || busy}>{L("好", "OK")}</button>
        </div>
      </form>
    </Modal>
  );
}

/** Folded groups (fold) and opened ones (faces), per project, on this computer. */
function useSet(key: string, initial: string[] = []): [Set<string>, (name: string) => void] {
  const read = () => { try { const v = JSON.parse(localStorage.getItem(key) || "null"); return new Set<string>(Array.isArray(v) ? v : initial); } catch { return new Set(initial); } };
  const [s, setS] = useState<Set<string>>(read);
  useEffect(() => { setS(read()); /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, [key]);
  return [s, (name) => setS((old) => {
    const n = new Set(old);
    if (n.has(name)) n.delete(name); else n.add(name);
    try { localStorage.setItem(key, JSON.stringify([...n])); } catch { /* not kept */ }
    return n;
  })];
}

const working = (rs: AgentRow[]) => rs.filter((r) => r.st.state === "working").length;
const asking = (rs: AgentRow[]) => rs.filter((r) => (r.st.asking || 0) > 0).length;
/** A group's state in a few words: who is at work, who asks you. */
function summary(rs: AgentRow[]): string {
  const w = working(rs), a = asking(rs), off = rs.filter((r) => r.st.state === "offline").length;
  return [a ? L(E`${a} 个在问你`, E`${a} asking you`) : "", w ? L(E`${w} 个在干活`, E`${w} at work`) : "",
    !w && !a ? (off === rs.length && rs.length ? L("都不在线", "all offline") : L("都空闲", "all idle")) : ""].filter(Boolean).join(" · ");
}

/** The members (without the coordinator) in their groups, the way chosen. */
export function GroupedMembers({ rows, ctl, style, row, foldKey, onNewGroup, onRename, onDelete, lead }: {
  rows: AgentRow[];
  /** what comes first, in no group (the coordinator); under the chips when filtering */
  lead?: ReactNode;
  ctl: GroupsCtl;
  style: GroupStyle;
  /** a member's own row, as the column draws it */
  row: (r: AgentRow) => ReactNode;
  /** where this project's folded groups are kept */
  foldKey: string;
  onNewGroup: () => void;
  onRename: (g: string) => void;
  onDelete: (g: string) => void;
}) {
  const groups = grouped(rows, ctl);
  const [folded, toggleFold] = useSet(FOLD_KEY + "fold:" + foldKey);
  const [opened, toggleOpen] = useSet(FOLD_KEY + "faces:" + foldKey);
  const [chip, setChip] = useState("");
  const [over, setOver] = useState("");
  // A group dragged among the others: where it would go (before or after one).
  const [slot, setSlot] = useState<{ name: string; after: boolean } | null>(null);
  const isGroupDrag = (e: React.DragEvent) => Array.from(e.dataTransfer.types).includes(GROUP_DRAG);
  const groupDragProps = (g: Group) => (!ctl.canEdit || g.other ? {} : {
    draggable: true,
    onDragStart: (e: React.DragEvent) => {
      e.stopPropagation();
      e.dataTransfer.setData(GROUP_DRAG, g.name);
      e.dataTransfer.effectAllowed = "move";
    },
    onDragEnd: () => setSlot(null),
  });
  /** Drops a dragged group before or after g (其他 stays last). */
  const placeGroup = (moved: string, g: Group, after: boolean) => {
    if (!moved || moved === g.name || !ctl.list.includes(moved)) return;
    const rest = ctl.list.filter((x) => x !== moved);
    const at = g.other ? rest.length : rest.indexOf(g.name) + (after ? 1 : 0);
    const next = [...rest.slice(0, at), moved, ...rest.slice(at)];
    if (next.join("\u0000") !== ctl.list.join("\u0000")) void ctl.save(next).catch(() => undefined);
  };
  /** Which half of the box the pointer is in (across for chips). */
  const afterHalf = (e: React.DragEvent, across: boolean) => {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    return across ? e.clientX > r.left + r.width / 2 : e.clientY > r.top + r.height / 2;
  };
  const menu = useContextMenu();
  const groupMenu = (el: Element, g: string) => {
    const i = ctl.list.indexOf(g);
    const move = (d: number) => {
      const next = [...ctl.list];
      [next[i], next[i + d]] = [next[i + d], next[i]];
      void ctl.save(next).catch(() => undefined);
    };
    menu.openUnder(el, [
      { label: L("改名", "Rename"), onSelect: () => onRename(g) },
      ...(i > 0 ? [{ label: L("上移", "Move up"), onSelect: () => move(-1) }] : []),
      ...(i < ctl.list.length - 1 ? [{ label: L("下移", "Move down"), onSelect: () => move(1) }] : []),
      "sep",
      { label: L("删除分组（成员回到「其他」）", "Delete the group (its members go to Other)"), danger: true, onSelect: () => onDelete(g) },
    ]);
  };
  // A member dropped on a group: moved there.
  const dropProps = (g: Group, across = false) => (!ctl.canEdit ? {} : {
    onDragOver: (e: React.DragEvent) => {
      if (isGroupDrag(e)) {
        e.preventDefault();
        e.stopPropagation();
        e.dataTransfer.dropEffect = "move";
        const after = g.other ? false : afterHalf(e, across);
        if (slot?.name !== g.name || slot.after !== after) setSlot({ name: g.name, after });
        return;
      }
      if (!Array.from(e.dataTransfer.types).includes(DRAG_TYPE)) return;
      e.preventDefault();
      e.stopPropagation();
      e.dataTransfer.dropEffect = "move";
      if (over !== g.name) setOver(g.name);
    },
    onDragLeave: (e: React.DragEvent) => { if (!e.currentTarget.contains(e.relatedTarget as Node | null)) { setOver(""); setSlot(null); } },
    onDrop: (e: React.DragEvent) => {
      setOver("");
      if (isGroupDrag(e)) {
        e.preventDefault();
        e.stopPropagation();
        const after = g.other ? false : afterHalf(e, across);
        setSlot(null);
        placeGroup(e.dataTransfer.getData(GROUP_DRAG), g, after);
        return;
      }
      let a: DragAgent | null = null;
      try { a = JSON.parse(e.dataTransfer.getData(DRAG_TYPE)) as DragAgent; } catch { return; }
      if (!a || !rows.some((r) => r.st.agent.name === a!.agent)) return;
      e.preventDefault();
      e.stopPropagation();
      const to = g.other ? "" : g.name;
      if (ctl.groupOf(a.agent) === to) return;
      void ctl.save(ctl.list, { [a.agent]: to }).catch(() => undefined);
    },
  });
  const faces = (rs: AgentRow[], n = 4) => (
    <span className="mg-faces" aria-hidden>{rs.slice(0, n).map((r) => <Avatar key={r.st.agent.name} agent={r.st.agent} size={16} badge={false} />)}</span>
  );
  const more = (g: Group) => (!g.other && ctl.canEdit ? (
    <span className="mg-more" role="button" tabIndex={0} aria-label={L(E`分组「${g.name}」`, E`Group ${g.name}`)}
      onClick={(e) => { e.stopPropagation(); groupMenu(e.currentTarget, g.name); }}
      onKeyDown={(e) => { if (e.key === "Enter") { e.stopPropagation(); groupMenu(e.currentTarget, g.name); } }}><MoreHorizontal /></span>
  ) : null);

  if (style === "chips") {
    const on = groups.find((g) => g.name === chip) ? chip : "";
    const shown = on ? groups.find((g) => g.name === on)!.rows : rows;
    return (
      <div className="mg mg-chips-style">
        <div className="mg-chips" role="tablist" aria-label={L("分组", "Groups")}>
          <button type="button" role="tab" aria-selected={on === ""} className={cn("mg-chip", on === "" && "is-on")} onClick={() => setChip("")}>
            {L("全部", "All")}<span>{rows.length}</span>
          </button>
          {groups.map((g) => (
            <button key={g.name} type="button" role="tab" aria-selected={on === g.name} data-group={g.name}
              className={cn("mg-chip", on === g.name && "is-on", slot?.name === g.name && (slot.after ? "is-slot-after" : "is-slot-before"))}
              onClick={() => setChip(g.name)} {...groupDragProps(g)} {...dropProps(g, true)}>{g.name}<span>{g.rows.length}</span></button>
          ))}
        </div>
        {ctl.error && <p className="mg-error" role="alert">{L(E`分组没存上：${ctl.error}`, E`Groups not saved: ${ctl.error}`)}</p>}
        {lead}
        {shown.map(row)}
        {menu.element}
      </div>
    );
  }

  if (style === "faces") {
    return (
      <div className="mg mg-faces-style">
        {ctl.error && <p className="mg-error" role="alert">{L(E`分组没存上：${ctl.error}`, E`Groups not saved: ${ctl.error}`)}</p>}
        {lead}
        {groups.map((g) => {
          const open = opened.has(g.name);
          return (
            <div key={g.name} className={cn("mg-group", open && "is-open", over === g.name && "is-over", slot?.name === g.name && (slot.after ? "is-slot-after" : "is-slot-before"))} data-group={g.name} {...dropProps(g)}>
              <button type="button" className="mg-tile-row" aria-expanded={open} onClick={() => toggleOpen(g.name)} {...groupDragProps(g)}>
                <span className="mg-tile" aria-hidden>{g.rows.slice(0, 4).map((r) => <Avatar key={r.st.agent.name} agent={r.st.agent} size={14} badge={false} />)}</span>
                <span className="mg-tile-text">
                  <span className="mg-name">{g.name}<span className="mg-count">{g.rows.length}</span></span>
                  <span className="mg-sum">{summary(g.rows)}</span>
                </span>
                {more(g)}
                <ChevronRight className="mg-chev" />
              </button>
              {open && <div className="mg-inside">{g.rows.map(row)}</div>}
            </div>
          );
        })}
        {menu.element}
      </div>
    );
  }

  return (
    <div className="mg mg-fold-style">
      {ctl.error && <p className="mg-error" role="alert">{L(E`分组没存上：${ctl.error}`, E`Groups not saved: ${ctl.error}`)}</p>}
      {lead}
      {groups.map((g) => {
        const open = !folded.has(g.name);
        const w = working(g.rows), a = asking(g.rows);
        return (
          <div key={g.name} className={cn("mg-group", open && "is-open", over === g.name && "is-over", slot?.name === g.name && (slot.after ? "is-slot-after" : "is-slot-before"))} data-group={g.name} {...dropProps(g)}>
            <button type="button" className="mg-head" aria-expanded={open} onClick={() => toggleFold(g.name)} {...groupDragProps(g)}>
              <ChevronRight className="mg-chev" />
              <span className="mg-name">{g.name}</span>
              <span className="mg-count">{g.rows.length}</span>
              <span className="mg-head-end">
                {!open && g.rows.length > 0 && faces(g.rows)}
                {(a > 0 || (!open && w > 0)) && <span className="mg-busy">{a > 0 ? L(E`${a} 个在问你`, E`${a} asking`) : L(E`${w} 个在干活`, E`${w} at work`)}</span>}
                {more(g)}
              </span>
            </button>
            {open && (g.rows.length ? g.rows.map(row) : <p className="mg-empty">{ctl.canEdit ? L("把成员拖到这里", "Drag members here") : L("还没有人", "Nobody yet")}</p>)}
          </div>
        );
      })}
      {ctl.canEdit && (
        <button type="button" className="mg-new" onClick={onNewGroup}>{L("+ 新建分组", "+ New group")}</button>
      )}
      {menu.element}
    </div>
  );
}

/** The faces in their groups' order (folded column, the narrow strip). */
export function inGroupOrder(rows: AgentRow[], ctl?: GroupsCtl): { name: string; rows: AgentRow[] }[] {
  if (!ctl || !ctl.list.length) return [{ name: "", rows }];
  return grouped(rows, ctl).filter((g) => g.rows.length);
}
