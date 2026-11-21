import { t as uiText, locale } from "../_lib/i18n";
// 多维表格's other views (board, calendar, gallery, form) and a record
// opened on the side.
import { useEffect, useMemo, useState, type DragEvent } from "react";
import { ChevronLeft, ChevronRight, ChevronUp, ChevronDown, Plus, Trash2, X, CheckCircle2 } from "lucide-react";
import { display, type Field } from "../_lib/fields";
import { Tag } from "../_lib/tagmenu";
import { personColor } from "../_lib/cells";
import { FieldInput, TypeIcon } from "../_lib/ui";
import type { Row, Sheet } from "../_lib/data";

const titleOf = (fields: Field[], r: Row) => {
  const v = fields[0] ? r.values[fields[0].id] : undefined;
  return v === undefined || v === null || v === "" ? "未命名记录" : display(fields[0], v);
};

/** A value as a card shows it. */
export function Value({ f, v, dark }: { f: Field; v: unknown; dark: boolean }) {
  if (v === undefined || v === null || v === "" || (Array.isArray(v) && !v.length)) return null;
  if (f.type === "select" || f.type === "multi") {
    return <span className="vals">{(Array.isArray(v) ? v : [v]).map((x) => <Tag key={String(x)} value={String(x)} options={f.options || []} dark={dark} />)}</span>;
  }
  if (f.type === "person") return <span className="person"><span className="person-dot" style={{ background: personColor(String(v)) }}>{String(v).slice(0, 1).toUpperCase()}</span>{String(v)}</span>;
  if (f.type === "check") return <span className="check">{v ? uiText("✓ 已勾选") : ""}</span>;
  if (f.type === "date") return <span>{String(v).replace(/-/g, "/")}</span>;
  return <span className="text">{display(f, v)}</span>;
}

function Card({ sheet, r, skip, dark, onOpen, draggable, onDragStart }: {
  sheet: Sheet; r: Row; skip?: string; dark: boolean; onOpen: (id: string) => void; draggable?: boolean; onDragStart?: (e: DragEvent) => void;
}) {
  const fields = sheet.fields;
  const rest = fields.slice(1).filter((f) => f.id !== skip && r.values[f.id] !== undefined && r.values[f.id] !== "" && !(Array.isArray(r.values[f.id]) && !(r.values[f.id] as unknown[]).length)).slice(0, 5);
  return (
    <div className="card" role="button" tabIndex={0} data-id={r.id} draggable={draggable} onDragStart={onDragStart}
      onClick={() => onOpen(r.id)} onKeyDown={(e) => { if (e.key === "Enter") onOpen(r.id); }}>
      <div className="card-title">{titleOf(fields, r)}</div>
      {rest.map((f) => (
        <div className="card-row" key={f.id}>
          <span className="card-k" title={f.name}><TypeIcon type={f.type} /></span>
          <Value f={f} v={r.values[f.id]} dark={dark} />
        </div>
      ))}
    </div>
  );
}

/** Which field a view goes by: the one picked, or a likely one. */
function useBy(sheet: Sheet, key: string, types: string[], likely: RegExp) {
  const store = `myco-base-${key}:${sheet.page}`;
  const [picked, setPicked] = useState<string>(() => { try { return localStorage.getItem(store) || ""; } catch { return ""; } });
  const fs = sheet.fields.filter((f) => types.includes(f.type));
  const by = fs.find((f) => f.id === picked) || fs.find((f) => likely.test(f.name)) || fs[0] || null;
  const pick = (id: string) => { setPicked(id); try { localStorage.setItem(store, id); } catch { /* not kept */ } };
  return { by, fs, pick };
}
function ByPicker({ label, by, fs, pick }: { label: string; by: Field | null; fs: Field[]; pick: (id: string) => void }) {
  if (!fs.length) return null;
  return (
    <label className="by">{label}
      <select className="sel" value={by?.id || ""} onChange={(e) => pick(e.target.value)} aria-label={label}>
        {fs.map((f) => <option key={f.id} value={f.id}>{f.name}</option>)}
      </select>
    </label>
  );
}

// ---- 看板 -------------------------------------------------------------------------
export function Board({ sheet, rows, dark, onOpen, onAdded }: { sheet: Sheet; rows: Row[]; dark: boolean; onOpen: (id: string) => void; onAdded: (id: string) => void }) {
  const { by, fs, pick } = useBy(sheet, "group", ["select"], /状态|阶段|进度|status|stage/i);
  const [drag, setDrag] = useState<string | null>(null);
  const [over, setOver] = useState<{ group: string; before: string | null } | null>(null);
  if (!by) return <div className="empty">{uiText("看板按一个「单选」字段分组：先在表格里加一个单选字段（比如「状态」）。")}</div>;
  const groups = [...(by.options || []), ""];
  const of = (g: string) => rows.filter((r) => {
    const v = r.values[by.id];
    return g === "" ? v === undefined || v === null || v === "" || !(by.options || []).includes(String(v)) : v === g;
  });
  const drop = (g: string) => {
    if (!drag) return;
    const list = of(g).filter((r) => r.id !== drag);
    const before = over?.group === g ? over.before : null;
    const i = before ? list.findIndex((r) => r.id === before) : list.length;
    sheet.place(drag, list[i]?.id ?? null, list[i - 1]?.id ?? null, { [by.id]: g || null });
    setDrag(null);
    setOver(null);
  };
  return (
    <div className="view">
      <div className="view-bar"><ByPicker label={uiText("分组依据")} by={by} fs={fs} pick={pick} /><span className="faint">{uiText("拖动卡片改「")}{by.name}{uiText("」，也能调整顺序")}</span></div>
      <div className="board">
        {groups.map((g) => {
          const mine = of(g);
          if (g === "" && !mine.length) return null;
          return (
            <section key={g || "_none"} className={`col${over?.group === g ? " is-over" : ""}`}
              onDragOver={(e) => { e.preventDefault(); if (over?.group !== g) setOver({ group: g, before: null }); }}
              onDrop={(e) => { e.preventDefault(); drop(g); }}>
              <header className="col-head">
                {g ? <Tag value={g} options={by.options || []} dark={dark} /> : <span className="faint">{uiText("未分组")}</span>}
                <span className="col-n">{mine.length}</span>
              </header>
              <div className="col-cards">
                {mine.map((r) => (
                  <div key={r.id} className={`slot${over?.group === g && over.before === r.id ? " is-before" : ""}${drag === r.id ? " is-drag" : ""}`}
                    onDragOver={(e) => { e.preventDefault(); e.stopPropagation(); if (over?.before !== r.id || over.group !== g) setOver({ group: g, before: r.id }); }}
                    onDrop={(e) => { e.preventDefault(); e.stopPropagation(); drop(g); }}>
                    <Card sheet={sheet} r={r} skip={by.id} dark={dark} onOpen={onOpen} draggable
                      onDragStart={(e) => { e.dataTransfer.setData("text/plain", r.id); e.dataTransfer.effectAllowed = "move"; setDrag(r.id); }} />
                  </div>
                ))}
                <button type="button" className="col-add" onClick={() => { const id = sheet.add(g ? { [by.id]: g } : {}); onAdded(id); onOpen(id); }}>
                  <Plus className="ico" />{uiText("新建记录")}{" "}</button>
              </div>
            </section>
          );
        })}
      </div>
    </div>
  );
}

// ---- 日历 -------------------------------------------------------------------------
const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
export function Calendar({ sheet, rows, dark, onOpen, onAdded }: { sheet: Sheet; rows: Row[]; dark: boolean; onOpen: (id: string) => void; onAdded: (id: string) => void }) {
  const { by, fs, pick } = useBy(sheet, "date", ["date"], /日期|时间|截止|date/i);
  const [month, setMonth] = useState(() => { const d = new Date(); d.setDate(1); return d; });
  const [drag, setDrag] = useState<string | null>(null);
  const color = sheet.fields.find((f) => f.type === "select");
  if (!by) return <div className="empty">{uiText("日历按一个「日期」字段排：先在表格里加一个日期字段。")}</div>;
  const first = new Date(month.getFullYear(), month.getMonth(), 1);
  const start = new Date(first);
  start.setDate(1 - ((first.getDay() + 6) % 7));
  const today = ymd(new Date());
  const days = Array.from({ length: 42 }, (_, i) => { const d = new Date(start); d.setDate(start.getDate() + i); return d; });
  const none = rows.filter((r) => !r.values[by.id]).length;
  const go = (n: number) => setMonth((m) => new Date(m.getFullYear(), m.getMonth() + n, 1));
  return (
    <div className="view">
      <div className="view-bar">
        <ByPicker label={uiText("日期字段")} by={by} fs={fs} pick={pick} />
        <div className="cal-nav">
          <button type="button" className="icon-btn" aria-label={uiText("上个月")} onClick={() => go(-1)}><ChevronLeft className="ico" /></button>
          <b>{month.toLocaleDateString(locale(),{year:"numeric",month:"long"})}</b>
          <button type="button" className="icon-btn" aria-label={uiText("下个月")} onClick={() => go(1)}><ChevronRight className="ico" /></button>
          <button type="button" className="btn" onClick={() => { const d = new Date(); d.setDate(1); setMonth(d); }}>{uiText("今天")}</button>
        </div>
        {none > 0 && <span className="faint">{none} {" "}{uiText("条没有「")}{by.name}{uiText("」，在表格里能看到")}</span>}
      </div>
      <div className="cal">
        {Array.from({length:7},(_,i)=>new Intl.DateTimeFormat(locale(),{weekday:"short"}).format(new Date(2026,10,16+i))).map((d) => <div key={d} className="dow">{d}</div>)}
        {days.map((d) => {
          const key = ymd(d), evs = rows.filter((r) => r.values[by.id] === key);
          return (
            <div key={key} className={`day${d.getMonth() !== month.getMonth() ? " is-out" : ""}${key === today ? " is-today" : ""}`}
              onDragOver={(e) => e.preventDefault()}
              onDrop={(e) => { e.preventDefault(); if (drag) sheet.edit([{ id: drag, values: { [by.id]: key } }]); setDrag(null); }}>
              <div className="day-head">
                <span className="day-n">{d.getDate()}</span>
                <button type="button" className="day-add" aria-label={uiText("{0} 新建", key)} onClick={() => { const id = sheet.add({ [by.id]: key }); onAdded(id); onOpen(id); }}><Plus className="ico" /></button>
              </div>
              {evs.slice(0, 4).map((r) => {
                const c = color ? r.values[color.id] : undefined;
                const t = c ? (color!.options || []).indexOf(String(c)) : -1;
                return (
                  <button key={r.id} type="button" className={`ev tone-${t < 0 ? 7 : t % 8}${dark ? " is-dark" : ""}`} draggable
                    onDragStart={(e) => { e.dataTransfer.setData("text/plain", r.id); setDrag(r.id); }} onClick={() => onOpen(r.id)}>
                    {titleOf(sheet.fields, r)}
                  </button>
                );
              })}
              {evs.length > 4 && <span className="more">{uiText("还有")}{" "}{evs.length - 4} {" "}{uiText("条")}</span>}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ---- 画廊 -------------------------------------------------------------------------
export function Gallery({ sheet, rows, dark, onOpen, onAdded }: { sheet: Sheet; rows: Row[]; dark: boolean; onOpen: (id: string) => void; onAdded: (id: string) => void }) {
  return (
    <div className="view">
      <div className="gallery">
        {rows.map((r) => <Card key={r.id} sheet={sheet} r={r} dark={dark} onOpen={onOpen} />)}
        <button type="button" className="card card-add" onClick={() => { const id = sheet.add({}); onAdded(id); onOpen(id); }}><Plus className="ico" />{uiText("新建记录")}</button>
      </div>
    </div>
  );
}

// ---- 表单 -------------------------------------------------------------------------
export function Form({ sheet, dark }: { sheet: Sheet; dark: boolean }) {
  const [values, setValues] = useState<Record<string, unknown>>({});
  const [done, setDone] = useState(false);
  const fields = sheet.fields;
  const people = useMemo(() => sheet.people(), [sheet, sheet.version]); // eslint-disable-line react-hooks/exhaustive-deps
  if (done) {
    return (
      <div className="view form-wrap">
        <div className="form form-done">
          <CheckCircle2 className="form-done-ico" />
          <h2>{uiText("已提交")}</h2>
          <p className="faint">{uiText("这一条已经加到「")}{sheet.title || sheet.cfg.name}{uiText("」里了。")}</p>
          <button type="button" className="btn btn-primary" onClick={() => { setValues({}); setDone(false); }}>{uiText("再填一份")}</button>
        </div>
      </div>
    );
  }
  return (
    <div className="view form-wrap">
      <form className="form" onSubmit={(e) => {
        e.preventDefault();
        if (!Object.values(values).some((v) => v !== undefined && v !== null && v !== "" && v !== false && !(Array.isArray(v) && !v.length))) { sheet.flash(uiText("还什么都没填")); return; }
        sheet.add(values);
        setDone(true);
      }}>
        <h2>{sheet.title || sheet.cfg.name}</h2>
        <p className="faint">{uiText("填好点「提交」，就是一条新记录。")}</p>
        {fields.map((f) => (
          <div className="form-row" key={f.id}>
            <label className="form-label"><TypeIcon type={f.type} />{f.name}</label>
            <FieldInput field={f} value={values[f.id]} people={people} dark={dark} onChange={(v) => setValues((x) => ({ ...x, [f.id]: v }))} />
          </div>
        ))}
        <button type="submit" className="btn btn-primary form-submit">{uiText("提交")}</button>
      </form>
    </div>
  );
}

function TitleInput({ field, value, onSave }: { field: Field; value: string; onSave: (v: string) => void }) {
  const [draft, setDraft] = useState(value);
  const [focus, setFocus] = useState(false);
  useEffect(() => { if (!focus) setDraft(value); }, [value, focus]);
  useEffect(() => {
    if (draft === value) return;
    const t = window.setTimeout(() => onSave(draft), 500);
    return () => window.clearTimeout(t);
  }, [draft]); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <textarea className="drawer-title" rows={1} value={draft} placeholder={uiText("未命名记录")} aria-label={field.name}
      onFocus={() => setFocus(true)} onBlur={() => { setFocus(false); if (draft !== value) onSave(draft); }}
      onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter" && !e.nativeEvent.isComposing) { e.preventDefault(); (e.target as HTMLTextAreaElement).blur(); } }} />
  );
}

// ---- a record, on the side -------------------------------------------------------
export function RecordPanel({ sheet, id, ids, dark, onClose, onGo }: {
  sheet: Sheet; id: string; ids: string[]; dark: boolean; onClose: () => void; onGo: (id: string) => void;
}) {
  const r = sheet.row(id);
  const fields = sheet.fields;
  const people = useMemo(() => sheet.people(), [sheet, sheet.version]); // eslint-disable-line react-hooks/exhaustive-deps
  const at = ids.indexOf(id);
  useEffect(() => {
    const k = (e: KeyboardEvent) => { if (e.key === "Escape" && !(e.target as HTMLElement)?.closest?.(".pop")) onClose(); };
    document.addEventListener("keydown", k);
    return () => document.removeEventListener("keydown", k);
  }, [onClose]);
  if (!r) return null;
  return (
    <>
      <div className="drawer-bg" onClick={onClose} />
      <aside className="drawer" role="dialog" aria-label={titleOf(fields, r)} data-record={id}>
        <header className="drawer-head">
          <button type="button" className="icon-btn" aria-label={uiText("上一条")} disabled={at <= 0} onClick={() => onGo(ids[at - 1])}><ChevronUp className="ico" /></button>
          <button type="button" className="icon-btn" aria-label={uiText("下一条")} disabled={at < 0 || at >= ids.length - 1} onClick={() => onGo(ids[at + 1])}><ChevronDown className="ico" /></button>
          <span className="faint drawer-pos">{at >= 0 ? `${at + 1} / ${ids.length}` : ""}</span>
          <span className="top-gap" />
          <button type="button" className="icon-btn" aria-label={uiText("删除这条记录")} title={uiText("删除")} onClick={() => { onClose(); sheet.remove([id]); }}><Trash2 className="ico" /></button>
          <button type="button" className="icon-btn" aria-label={uiText("关闭")} onClick={onClose}><X className="ico" /></button>
        </header>
        <div className="drawer-body">
          {fields[0] && <TitleInput key={id} field={fields[0]} value={String(r.values[fields[0].id] ?? "")} onSave={(v) => sheet.edit([{ id, values: { [fields[0].id]: v } }])} />}
          {fields.slice(1).map((f) => (
            <div className="drawer-row" key={f.id}>
              <label className="drawer-label"><TypeIcon type={f.type} /><span>{f.name}</span></label>
              <div className="drawer-value">
                <FieldInput field={f} value={r.values[f.id]} people={people} dark={dark} placeholder={uiText("未填写")} onChange={(v) => sheet.edit([{ id, values: { [f.id]: v } }])} />
              </div>
            </div>
          ))}
        </div>
      </aside>
    </>
  );
}
