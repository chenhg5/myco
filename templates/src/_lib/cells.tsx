import { t as uiText } from "./i18n";
// The grid's own cells (Glide Data Grid custom renderers): option tags,
// dates and people, drawn on the canvas, each with its editor.
import { useEffect, useMemo, useRef, useState } from "react";
import { GridCellKind, getMiddleCenterBias, measureTextCached, roundedRect, type CustomCell, type CustomRenderer } from "@glideapps/glide-data-grid";
import { tagOf } from "./fields";
import { TagMenu } from "./tagmenu";

export type TagsData = { kind: "tags"; values: string[]; options: string[]; multi: boolean; dark: boolean };
export type DateData = { kind: "date"; value: string };
export type PersonData = { kind: "person"; value: string; people: string[] };
export type TagsCell = CustomCell<TagsData>;
export type DateCell = CustomCell<DateData>;
export type PersonCell = CustomCell<PersonData>;

const pillFont = (theme: { fontFamily: string }) => `12px ${theme.fontFamily}`;

export const tagsRenderer: CustomRenderer<TagsCell> = {
  kind: GridCellKind.Custom,
  isMatch: (c): c is TagsCell => (c.data as TagsData).kind === "tags",
  needsHover: false,
  draw: (args, cell) => {
    const { ctx, rect, theme } = args;
    const { values, options, dark } = cell.data;
    const h = 22, padX = 8, gap = 6;
    let x = rect.x + theme.cellHorizontalPadding;
    const y = rect.y + (rect.height - h) / 2, right = rect.x + rect.width - theme.cellHorizontalPadding;
    ctx.save();
    ctx.beginPath();
    ctx.rect(rect.x, rect.y, rect.width, rect.height);
    ctx.clip();
    ctx.font = pillFont(theme);
    for (const v of values) {
      if (x >= right) break;
      const t = tagOf(options, v), w = measureTextCached(v, ctx).width + padX * 2;
      ctx.beginPath();
      roundedRect(ctx, x, y, w, h, h / 2);
      ctx.fillStyle = dark ? t.dbg : t.bg;
      ctx.fill();
      ctx.fillStyle = dark ? t.dfg : t.fg;
      ctx.fillText(v, x + padX, y + h / 2 + getMiddleCenterBias(ctx, pillFont(theme)));
      x += w + gap;
    }
    ctx.restore();
    return true;
  },
  measure: (ctx, cell, theme) => cell.data.values.reduce((n, v) => n + measureTextCached(v, ctx, pillFont(theme)).width + 22, theme.cellHorizontalPadding * 2),
  provideEditor: () => ({
    disablePadding: true,
    disableStyling: true,
    editor: ({ value, onChange, onFinishedEditing }) => {
      const d = value.data;
      return (
        <div className="cell-pop">
          <TagMenu options={d.options} value={d.values} multi={d.multi} dark={d.dark}
            onChange={(values) => {
              const next = { ...value, data: { ...d, values }, copyData: values.join(", ") };
              if (d.multi) onChange(next);
              else onFinishedEditing(next);
            }}
            onDone={() => onFinishedEditing()} />
        </div>
      );
    },
  }),
  onPaste: (val, d) => {
    const parts = val.split(/[,，、;；\n]/).map((x) => x.trim()).filter(Boolean);
    return { ...d, values: d.multi ? parts : parts.slice(0, 1) };
  },
  onDelete: (c) => ({ ...c, data: { ...c.data, values: [] }, copyData: "" }),
};

function DateEditor({ value, onChange, onFinishedEditing }: { value: DateCell; onChange: (c: DateCell) => void; onFinishedEditing: (c?: DateCell) => void }) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => { ref.current?.focus(); try { ref.current?.showPicker?.(); } catch { /* (not allowed) */ } }, []);
  const set = (v: string) => ({ ...value, data: { ...value.data, value: v }, copyData: v });
  return (
    <div className="cell-pop cell-date">
      <input ref={ref} type="date" value={value.data.value || ""} onChange={(e) => onChange(set(e.target.value))}
        onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); onFinishedEditing(set((e.target as HTMLInputElement).value)); } }} />
      <button type="button" onClick={() => onFinishedEditing(set(new Date().toISOString().slice(0, 10)))}>{uiText("今天")}</button>
      <button type="button" onClick={() => onFinishedEditing(set(""))}>{uiText("清除")}</button>
    </div>
  );
}

export const dateRenderer: CustomRenderer<DateCell> = {
  kind: GridCellKind.Custom,
  isMatch: (c): c is DateCell => (c.data as DateData).kind === "date",
  draw: (args, cell) => {
    const { ctx, rect, theme } = args;
    if (!cell.data.value) return true;
    ctx.fillStyle = theme.textDark;
    ctx.font = theme.baseFontFull;
    ctx.fillText(cell.data.value.replace(/-/g, "/"), rect.x + theme.cellHorizontalPadding, rect.y + rect.height / 2 + getMiddleCenterBias(ctx, theme));
    return true;
  },
  provideEditor: () => ({ disablePadding: true, disableStyling: true, editor: DateEditor as any }),
  onPaste: (val, d) => {
    const m = /^(\d{4})[-/.年](\d{1,2})[-/.月](\d{1,2})/.exec(val.trim());
    return m ? { ...d, value: `${m[1]}-${m[2].padStart(2, "0")}-${m[3].padStart(2, "0")}` } : d;
  },
  onDelete: (c) => ({ ...c, data: { ...c.data, value: "" }, copyData: "" }),
};

/** A person's colour, the same every time for the same name. */
export function personColor(name: string) {
  let h = 0;
  for (const ch of name) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return ["#3370ff", "#00b386", "#f77234", "#7f3bf5", "#e8478b", "#0fa6c2", "#d4a106", "#5b6b8c"][h % 8];
}

function PersonEditor({ value, onFinishedEditing }: { value: PersonCell; onFinishedEditing: (c?: PersonCell) => void }) {
  const [q, setQ] = useState("");
  const [at, setAt] = useState(() => Math.max(0, value.data.people.indexOf(value.data.value)));
  const list = useMemo(() => {
    const s = q.trim().toLowerCase();
    const all = value.data.people.filter((p) => !s || p.toLowerCase().includes(s));
    return q.trim() && !value.data.people.includes(q.trim()) ? [...all, q.trim()] : all;
  }, [q, value.data.people]);
  const pick = (v: string) => onFinishedEditing({ ...value, data: { ...value.data, value: v }, copyData: v });
  return (
    <div className="cell-pop tag-menu">
      <input className="tag-menu-q" autoFocus value={q} placeholder={value.data.value || uiText("输入名字")} onChange={(e) => { setQ(e.target.value); setAt(0); }}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown") { e.preventDefault(); setAt((i) => Math.min(list.length - 1, i + 1)); }
          else if (e.key === "ArrowUp") { e.preventDefault(); setAt((i) => Math.max(0, i - 1)); }
          else if (e.key === "Enter" && !e.nativeEvent.isComposing) { e.preventDefault(); pick(list[at] ?? q.trim()); }
          else if (e.key === "Escape") onFinishedEditing();
        }} />
      <div className="tag-menu-list">
        {list.map((p, i) => (
          <button key={p} type="button" className={`tag-menu-item${i === at ? " is-at" : ""}`} onMouseEnter={() => setAt(i)} onClick={() => pick(p)}>
            <span className="person-dot" style={{ background: personColor(p) }}>{p.slice(0, 1).toUpperCase()}</span>
            <span>{value.data.people.includes(p) ? p : uiText("添加「{0}」", p)}</span>
          </button>
        ))}
        {value.data.value && <button type="button" className="tag-menu-item is-muted" onClick={() => pick("")}>{uiText("清除")}</button>}
      </div>
    </div>
  );
}

export const personRenderer: CustomRenderer<PersonCell> = {
  kind: GridCellKind.Custom,
  isMatch: (c): c is PersonCell => (c.data as PersonData).kind === "person",
  draw: (args, cell) => {
    const { ctx, rect, theme } = args;
    const v = cell.data.value;
    if (!v) return true;
    const r = 10, cx = rect.x + theme.cellHorizontalPadding + r, cy = rect.y + rect.height / 2;
    ctx.beginPath();
    ctx.arc(cx, cy, r, 0, Math.PI * 2);
    ctx.fillStyle = personColor(v);
    ctx.fill();
    ctx.fillStyle = "#fff";
    ctx.font = `600 11px ${theme.fontFamily}`;
    const ch = v.slice(0, 1).toUpperCase();
    ctx.fillText(ch, cx - measureTextCached(ch, ctx).width / 2, cy + getMiddleCenterBias(ctx, `600 11px ${theme.fontFamily}`));
    ctx.fillStyle = theme.textDark;
    ctx.font = theme.baseFontFull;
    ctx.fillText(v, cx + r + 6, cy + getMiddleCenterBias(ctx, theme));
    return true;
  },
  provideEditor: () => ({ disablePadding: true, disableStyling: true, editor: PersonEditor as any }),
  onPaste: (val, d) => ({ ...d, value: val.trim() }),
  onDelete: (c) => ({ ...c, data: { ...c.data, value: "" }, copyData: "" }),
};

export const renderers = [tagsRenderer, dateRenderer, personRenderer];
