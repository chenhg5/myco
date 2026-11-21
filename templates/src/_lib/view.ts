import { t as uiText } from "./i18n";
// How one person looks at the rows: filters and sorts, kept in this browser
// for this page (not shared, as a search is not).
import { compare, display, type Field } from "./fields";
import type { Row } from "./data";

export type Op = "has" | "nothas" | "is" | "isnot" | "empty" | "notempty" | "gt" | "lt";
export type Cond = { field: string; op: Op; value: string };
export type ViewState = { conds: Cond[]; any: boolean; sorts: { field: string; desc: boolean }[] };
export const NO_VIEW: ViewState = { conds: [], any: false, sorts: [] };

export const OPS: [Op, string][] = [["has", uiText("包含")], ["nothas", uiText("不包含")], ["is", uiText("等于")], ["isnot", uiText("不等于")], ["gt", uiText("大于")], ["lt", uiText("小于")], ["empty", uiText("为空")], ["notempty", uiText("不为空")]];
export const opsFor = (f?: Field): Op[] => !f ? ["has"] :
  f.type === "number" || f.type === "date" ? ["is", "isnot", "gt", "lt", "empty", "notempty"] :
  f.type === "check" ? ["notempty", "empty"] :
  f.type === "select" || f.type === "multi" ? ["is", "isnot", "has", "empty", "notempty"] :
  ["has", "nothas", "is", "isnot", "empty", "notempty"];
export const opName = (op: Op, f?: Field) => f?.type === "check" ? (op === "notempty" ? "已勾选" : "未勾选") : OPS.find((x) => x[0] === op)?.[1] || op;

function test(f: Field, v: unknown, c: Cond): boolean {
  const s = display(f, v), isEmpty = v === undefined || v === null || v === "" || v === false || (Array.isArray(v) && !v.length);
  const want = c.value.trim();
  switch (c.op) {
    case "empty": return isEmpty;
    case "notempty": return !isEmpty;
    case "has": return !want || (Array.isArray(v) ? v.some((x) => String(x).includes(want)) : s.toLowerCase().includes(want.toLowerCase()));
    case "nothas": return !want || !s.toLowerCase().includes(want.toLowerCase());
    case "is": return !want || (Array.isArray(v) ? v.includes(want) : f.type === "number" ? Number(v) === Number(want) : s === want);
    case "isnot": return !want || (Array.isArray(v) ? !v.includes(want) : f.type === "number" ? Number(v) !== Number(want) : s !== want);
    case "gt": return !want || (!isEmpty && (f.type === "number" ? Number(v) > Number(want) : s > want));
    case "lt": return !want || (!isEmpty && (f.type === "number" ? Number(v) < Number(want) : s < want));
  }
}

/** The rows a view shows, in its order; `keep` are shown whatever the filter (just added). */
export function apply(rows: Row[], fields: Field[], v: ViewState, keep?: Set<string>): Row[] {
  const conds = v.conds.filter((c) => fields.some((f) => f.id === c.field));
  let out = conds.length ? rows.filter((r) => {
    if (keep?.has(r.id)) return true;
    const ok = conds.map((c) => test(fields.find((f) => f.id === c.field)!, r.values[c.field], c));
    return v.any ? ok.some(Boolean) : ok.every(Boolean);
  }) : rows.slice();
  const sorts = v.sorts.filter((s) => fields.some((f) => f.id === s.field));
  if (sorts.length) {
    out = out.map((r, i) => ({ r, i })).sort((a, b) => {
      if (keep?.has(a.r.id) !== keep?.has(b.r.id)) return keep?.has(a.r.id) ? 1 : -1;
      for (const s of sorts) {
        const f = fields.find((x) => x.id === s.field)!;
        const d = compare(f, a.r.values[f.id], b.r.values[f.id]);
        if (d) return s.desc ? -d : d;
      }
      return a.i - b.i;
    }).map((x) => x.r);
  }
  return out;
}

export function loadView(key: string): ViewState {
  try { return { ...NO_VIEW, ...JSON.parse(localStorage.getItem(key) || "{}") }; } catch { return NO_VIEW; }
}
export function saveView(key: string, v: ViewState) {
  try { localStorage.setItem(key, JSON.stringify(v)); } catch { /* not kept */ }
}
