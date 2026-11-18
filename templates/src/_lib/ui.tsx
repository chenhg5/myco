// The pieces around a table: popovers, the bar on top, filters and sorts,
// a field's menu, and an input for one value (in a record or a form).
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import {
  AlignLeft, ArrowDownAZ, ArrowUpAZ, ArrowUpDown, Calendar, CheckSquare, ChevronDown, CircleDot, Download, Filter, Hash, Link2,
  ListChecks, Plus, Search, Trash2, Type, User, X, ArrowLeftToLine, ArrowRightToLine, GripVertical,
} from "lucide-react";
import { FIELD_TYPES, parse, typeName, uid, type Field, type FieldType } from "./fields";
import { TAGS } from "./fields";
import { Tag, TagMenu } from "./tagmenu";
import { opName, opsFor, type Cond, type ViewState } from "./view";
import type { Sheet } from "./data";

export const TYPE_ICON: Record<FieldType, typeof Type> = {
  text: Type, long: AlignLeft, number: Hash, date: Calendar, select: CircleDot, multi: ListChecks, check: CheckSquare, person: User, url: Link2,
};
export function TypeIcon({ type }: { type: FieldType }) {
  const I = TYPE_ICON[type] || Type;
  return <I className="ico" />;
}

/** A floating box by an anchor (a rect on screen), closed by a click outside or Esc. */
export function Popover({ at, onClose, children, width, align = "left", className = "" }: {
  at: { x: number; y: number; w?: number; h?: number }; onClose: () => void; children: ReactNode; width?: number; align?: "left" | "right"; className?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState<{ left: number; top: number }>({ left: -9999, top: -9999 });
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const r = el.getBoundingClientRect(), vw = window.innerWidth, vh = window.innerHeight;
    let left = align === "right" ? at.x + (at.w || 0) - r.width : at.x;
    let top = at.y + (at.h || 0) + 4;
    if (left + r.width > vw - 8) left = vw - 8 - r.width;
    if (left < 8) left = 8;
    if (top + r.height > vh - 8) top = Math.max(8, at.y - r.height - 4);
    setPos({ left, top });
  }, [at.x, at.y, at.w, at.h, align]);
  useEffect(() => {
    const down = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) onClose(); };
    const key = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    const t = window.setTimeout(() => document.addEventListener("mousedown", down), 0);
    document.addEventListener("keydown", key);
    return () => { window.clearTimeout(t); document.removeEventListener("mousedown", down); document.removeEventListener("keydown", key); };
  }, [onClose]);
  return createPortal(
    <div ref={ref} className={`pop ${className}`} style={{ left: pos.left, top: pos.top, width }} role="dialog">{children}</div>,
    document.body,
  );
}
export const rectOf = (el: Element) => { const r = el.getBoundingClientRect(); return { x: r.left, y: r.top, w: r.width, h: r.height }; };

export function MenuItem({ icon, children, onClick, danger }: { icon?: ReactNode; children: ReactNode; onClick: () => void; danger?: boolean }) {
  return <button type="button" className={`menu-item${danger ? " is-danger" : ""}`} onClick={onClick}>{icon}<span>{children}</span></button>;
}

/** The bar on top: the title (editable), saving, who you are, export. */
export function TopBar({ sheet, onExport, children }: { sheet: Sheet; onExport: (kind: "xlsx" | "csv") => void; children?: ReactNode }) {
  const [menu, setMenu] = useState<null | ReturnType<typeof rectOf>>(null);
  const state = sheet.pending ? "saving" : sheet.failed ? "error" : "saved";
  return (
    <header className="top">
      <input className="top-title" value={sheet.title} placeholder={`未命名${sheet.cfg.name}`} aria-label="标题" maxLength={120}
        onChange={(e) => sheet.setTitle(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") (e.target as HTMLInputElement).blur(); }} />
      <span className={`top-state is-${state}`}>{state === "saving" ? "正在保存…" : state === "error" ? "没保存上" : "已保存"}</span>
      <span className="top-gap" />
      {children}
      {sheet.me && <span className="top-me" title={`你在这里改的，别人看到是 ${sheet.me} 改的`}>你是 {sheet.me}</span>}
      <button type="button" className="btn" onClick={(e) => setMenu(rectOf(e.currentTarget))} aria-expanded={!!menu}><Download className="ico" />导出</button>
      {menu && (
        <Popover at={menu} align="right" onClose={() => setMenu(null)}>
          <div className="menu">
            <MenuItem onClick={() => { setMenu(null); onExport("xlsx"); }}>Excel（.xlsx）</MenuItem>
            <MenuItem onClick={() => { setMenu(null); onExport("csv"); }}>CSV（.csv）</MenuItem>
          </div>
        </Popover>
      )}
    </header>
  );
}

export function Note({ sheet }: { sheet: Sheet }) {
  if (!sheet.note) return null;
  return (
    <div className="note" role="status">
      {sheet.note.text}
      {sheet.note.undo && <button type="button" onClick={sheet.note.undo}>撤销</button>}
    </div>
  );
}

// ---- filters and sorts ----------------------------------------------------------
export function FilterButton({ fields, view, setView }: { fields: Field[]; view: ViewState; setView: (v: ViewState) => void }) {
  const [at, setAt] = useState<null | ReturnType<typeof rectOf>>(null);
  const n = view.conds.filter((c) => fields.some((f) => f.id === c.field)).length;
  const set = (i: number, c: Partial<Cond>) => setView({ ...view, conds: view.conds.map((x, j) => (j === i ? { ...x, ...c } : x)) });
  return (
    <>
      <button type="button" className={`tool${n ? " is-on" : ""}`} onClick={(e) => setAt(rectOf(e.currentTarget))}><Filter className="ico" />{n ? `${n} 个筛选` : "筛选"}</button>
      {at && (
        <Popover at={at} onClose={() => setAt(null)} width={520}>
          <div className="panel">
            <div className="panel-head">
              <span>设置筛选条件</span>
              {view.conds.length > 1 && (
                <select className="sel" value={view.any ? "any" : "all"} onChange={(e) => setView({ ...view, any: e.target.value === "any" })} aria-label="符合">
                  <option value="all">符合全部条件</option><option value="any">符合任一条件</option>
                </select>
              )}
            </div>
            {view.conds.map((c, i) => {
              const f = fields.find((x) => x.id === c.field);
              const ops = opsFor(f);
              const needValue = c.op !== "empty" && c.op !== "notempty";
              return (
                <div className="cond" key={i}>
                  <select className="sel" value={c.field} aria-label="字段" onChange={(e) => { const nf = fields.find((x) => x.id === e.target.value); set(i, { field: e.target.value, op: opsFor(nf)[0], value: "" }); }}>
                    {fields.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
                  </select>
                  <select className="sel" value={c.op} aria-label="条件" onChange={(e) => set(i, { op: e.target.value as Cond["op"] })}>
                    {ops.map((o) => <option key={o} value={o}>{opName(o, f)}</option>)}
                  </select>
                  {needValue && f?.type !== "check" ? (
                    f && (f.type === "select" || f.type === "multi") ? (
                      <select className="sel cond-v" value={c.value} aria-label="值" onChange={(e) => set(i, { value: e.target.value })}>
                        <option value="">请选择</option>{(f.options || []).map((o) => <option key={o}>{o}</option>)}
                      </select>
                    ) : <input className="inp cond-v" type={f?.type === "date" ? "date" : "text"} value={c.value} placeholder="输入值" aria-label="值" onChange={(e) => set(i, { value: e.target.value })} />
                  ) : <span className="cond-v" />}
                  <button type="button" className="icon-btn" aria-label="去掉这个条件" onClick={() => setView({ ...view, conds: view.conds.filter((_, j) => j !== i) })}><X className="ico" /></button>
                </div>
              );
            })}
            <button type="button" className="link-btn" onClick={() => fields[0] && setView({ ...view, conds: [...view.conds, { field: fields[0].id, op: opsFor(fields[0])[0], value: "" }] })}>
              <Plus className="ico" />添加条件
            </button>
          </div>
        </Popover>
      )}
    </>
  );
}

export function SortButton({ fields, view, setView }: { fields: Field[]; view: ViewState; setView: (v: ViewState) => void }) {
  const [at, setAt] = useState<null | ReturnType<typeof rectOf>>(null);
  const n = view.sorts.filter((s) => fields.some((f) => f.id === s.field)).length;
  return (
    <>
      <button type="button" className={`tool${n ? " is-on" : ""}`} onClick={(e) => setAt(rectOf(e.currentTarget))}><ArrowUpDown className="ico" />{n ? `${n} 个排序` : "排序"}</button>
      {at && (
        <Popover at={at} onClose={() => setAt(null)} width={400}>
          <div className="panel">
            <div className="panel-head"><span>设置排序条件</span></div>
            {view.sorts.map((s, i) => (
              <div className="cond" key={i}>
                <GripVertical className="ico faint" />
                <select className="sel" value={s.field} aria-label="字段" onChange={(e) => setView({ ...view, sorts: view.sorts.map((x, j) => (j === i ? { ...x, field: e.target.value } : x)) })}>
                  {fields.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
                </select>
                <div className="seg">
                  <button type="button" className={!s.desc ? "is-on" : ""} onClick={() => setView({ ...view, sorts: view.sorts.map((x, j) => (j === i ? { ...x, desc: false } : x)) })}>升序</button>
                  <button type="button" className={s.desc ? "is-on" : ""} onClick={() => setView({ ...view, sorts: view.sorts.map((x, j) => (j === i ? { ...x, desc: true } : x)) })}>降序</button>
                </div>
                <button type="button" className="icon-btn" aria-label="去掉这个排序" onClick={() => setView({ ...view, sorts: view.sorts.filter((_, j) => j !== i) })}><X className="ico" /></button>
              </div>
            ))}
            <button type="button" className="link-btn" onClick={() => {
              const f = fields.find((x) => !view.sorts.some((s) => s.field === x.id)) || fields[0];
              if (f) setView({ ...view, sorts: [...view.sorts, { field: f.id, desc: false }] });
            }}><Plus className="ico" />添加排序</button>
          </div>
        </Popover>
      )}
    </>
  );
}

export function SearchButton({ onClick }: { onClick: () => void }) {
  return <button type="button" className="tool" onClick={onClick} title="搜索（Ctrl+F）" aria-label="搜索"><Search className="ico" /></button>;
}

// ---- a field's menu (from its header) ------------------------------------------------
export function FieldMenu({ sheet, field, at, onClose, onSort, first, dark }: {
  sheet: Sheet; field: Field; at: { x: number; y: number; w?: number; h?: number }; onClose: () => void; onSort?: (desc: boolean) => void; first: boolean; dark: boolean;
}) {
  const [name, setName] = useState(field.name);
  const [newOpt, setNewOpt] = useState("");
  const fields = sheet.fields;
  const put = (next: Partial<Field>) => sheet.saveFields(fields.map((f) => (f.id === field.id ? { ...f, ...next } : f)));
  const setType = (type: FieldType) => {
    const next: Partial<Field> = { type };
    if ((type === "select" || type === "multi") && !(field.options || []).length) {
      // (the values already there become the options)
      const seen = new Set<string>();
      for (const r of sheet.list) {
        const v = r.values[field.id];
        for (const x of Array.isArray(v) ? v : v === undefined || v === null || v === "" ? [] : [v]) if (seen.size < 30) seen.add(String(x));
      }
      next.options = seen.size ? [...seen] : ["选项一", "选项二"];
    }
    put(next);
  };
  const renameOption = (i: number, to: string) => {
    const from = (field.options || [])[i];
    to = to.trim();
    if (!to || to === from || (field.options || []).includes(to)) return;
    put({ options: (field.options || []).map((o, j) => (j === i ? to : o)) });
    const changes = sheet.list.filter((r) => {
      const v = r.values[field.id];
      return Array.isArray(v) ? v.includes(from) : v === from;
    }).map((r) => {
      const v = r.values[field.id];
      return { id: r.id, values: { [field.id]: Array.isArray(v) ? v.map((x) => (x === from ? to : x)) : to } };
    });
    if (changes.length) sheet.edit(changes);
  };
  const insert = (side: 0 | 1) => {
    const i = fields.findIndex((f) => f.id === field.id) + side;
    const nf: Field = { id: uid("f"), name: `字段 ${fields.length + 1}`, type: "text" };
    sheet.saveFields([...fields.slice(0, i), nf, ...fields.slice(i)]);
    onClose();
  };
  const opts = field.options || [];
  return (
    <Popover at={at} onClose={onClose} width={280}>
      <div className="menu field-menu">
        <input className="inp" value={name} autoFocus aria-label="字段名" onChange={(e) => setName(e.target.value)}
          onBlur={() => name.trim() && name.trim() !== field.name && put({ name: name.trim() })}
          onKeyDown={(e) => { if (e.key === "Enter" && !e.nativeEvent.isComposing) { if (name.trim()) put({ name: name.trim() }); onClose(); } }} />
        <label className="field-menu-type">
          <span>类型</span>
          <select className="sel" value={field.type} onChange={(e) => setType(e.target.value as FieldType)} aria-label="类型">
            {FIELD_TYPES.map(([t, n]) => <option key={t} value={t}>{n}</option>)}
          </select>
        </label>
        {(field.type === "select" || field.type === "multi") && (
          <div className="opts">
            {opts.map((o, i) => (
              <div className="opt" key={o + i}>
                <span className="opt-dot" style={{ background: dark ? TAGS[i % TAGS.length].dfg : TAGS[i % TAGS.length].fg }} />
                <input className="inp" defaultValue={o} aria-label="选项" onBlur={(e) => renameOption(i, e.target.value)}
                  onKeyDown={(e) => { if (e.key === "Enter") (e.target as HTMLInputElement).blur(); }} />
                <button type="button" className="icon-btn" aria-label={`删除选项${o}`} onClick={() => put({ options: opts.filter((_, j) => j !== i) })}><X className="ico" /></button>
              </div>
            ))}
            <input className="inp" value={newOpt} placeholder="+ 添加选项" aria-label="添加选项" onChange={(e) => setNewOpt(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter" && !e.nativeEvent.isComposing && newOpt.trim() && !opts.includes(newOpt.trim())) { put({ options: [...opts, newOpt.trim()] }); setNewOpt(""); } }} />
          </div>
        )}
        <div className="menu-sep" />
        {onSort && <MenuItem icon={<ArrowUpAZ className="ico" />} onClick={() => { onSort(false); onClose(); }}>升序排列</MenuItem>}
        {onSort && <MenuItem icon={<ArrowDownAZ className="ico" />} onClick={() => { onSort(true); onClose(); }}>降序排列</MenuItem>}
        <MenuItem icon={<ArrowLeftToLine className="ico" />} onClick={() => insert(0)}>在左边插入字段</MenuItem>
        <MenuItem icon={<ArrowRightToLine className="ico" />} onClick={() => insert(1)}>在右边插入字段</MenuItem>
        {!first && <>
          <div className="menu-sep" />
          <MenuItem danger icon={<Trash2 className="ico" />} onClick={() => {
            if (!window.confirm(`删除字段「${field.name}」？这一列的内容也会看不到。`)) return;
            sheet.saveFields(fields.filter((f) => f.id !== field.id));
            onClose();
          }}>删除字段</MenuItem>
        </>}
      </div>
    </Popover>
  );
}

// ---- one value, in a record or a form -----------------------------------------------
export function FieldInput({ field, value, onChange, people, dark, placeholder = "" }: {
  field: Field; value: unknown; onChange: (v: unknown) => void; people: string[]; dark: boolean; placeholder?: string;
}) {
  const [draft, setDraft] = useState<string>(value === undefined || value === null ? "" : Array.isArray(value) ? value.join(", ") : String(value));
  const [pick, setPick] = useState<null | ReturnType<typeof rectOf>>(null);
  useEffect(() => { setDraft(value === undefined || value === null ? "" : Array.isArray(value) ? value.join(", ") : String(value)); }, [value]);
  const commit = () => { const v = parse(field, draft); if (JSON.stringify(v ?? null) !== JSON.stringify(value ?? null)) onChange(v); };
  const t = field.type;
  if (t === "check") return <input type="checkbox" className="chk" checked={!!value} onChange={(e) => onChange(e.target.checked)} aria-label={field.name} />;
  if (t === "select" || t === "multi") {
    const vs = Array.isArray(value) ? value.map(String) : value === undefined || value === null || value === "" ? [] : [String(value)];
    return (
      <>
        <button type="button" className="fi fi-tags" onClick={(e) => setPick(rectOf(e.currentTarget))} aria-label={field.name}>
          {vs.length ? vs.map((v) => <Tag key={v} value={v} options={field.options || []} dark={dark} />) : <span className="faint">{placeholder || "请选择"}</span>}
          <ChevronDown className="ico faint fi-caret" />
        </button>
        {pick && (
          <Popover at={pick} onClose={() => setPick(null)} width={Math.max(240, pick.w || 0)}>
            <TagMenu options={field.options || []} value={vs} multi={t === "multi"} dark={dark}
              onChange={(next) => { onChange(t === "multi" ? next : next[0] ?? null); if (t !== "multi") setPick(null); }}
              onDone={() => setPick(null)} />
          </Popover>
        )}
      </>
    );
  }
  if (t === "long") return <textarea className="fi" rows={3} value={draft} aria-label={field.name} placeholder={placeholder} onChange={(e) => setDraft(e.target.value)} onBlur={commit} />;
  if (t === "date") return <input className="fi" type="date" value={draft} aria-label={field.name} onChange={(e) => { setDraft(e.target.value); onChange(e.target.value || null); }} />;
  const listId = t === "person" ? `people-${field.id}` : undefined;
  return (
    <>
      <input className="fi" value={draft} aria-label={field.name} list={listId} inputMode={t === "number" ? "decimal" : undefined}
        placeholder={t === "url" ? "https://" : placeholder} onChange={(e) => setDraft(e.target.value)} onBlur={commit}
        onKeyDown={(e) => { if (e.key === "Enter" && !e.nativeEvent.isComposing) (e.target as HTMLInputElement).blur(); }} />
      {listId && <datalist id={listId}>{people.map((p) => <option key={p} value={p} />)}</datalist>}
    </>
  );
}

export function fieldLabel(f: Field) {
  return <span className="flabel"><TypeIcon type={f.type} />{f.name}</span>;
}
export { typeName };

/** Light or dark, following the system (and the App). */
export function useDark() {
  const q = typeof window !== "undefined" && window.matchMedia ? window.matchMedia("(prefers-color-scheme: dark)") : null;
  const [d, setD] = useState(!!q?.matches);
  useEffect(() => {
    if (!q) return;
    const on = () => setD(q.matches);
    q.addEventListener("change", on);
    return () => q.removeEventListener("change", on);
  }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return d;
}
