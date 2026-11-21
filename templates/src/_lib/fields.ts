import { t as uiText, locale } from "./i18n";
// Fields (a table's columns, a base's fields): their types, as people and
// agents name them, and how values are told and compared.
export type FieldType = "text" | "long" | "number" | "date" | "select" | "multi" | "check" | "person" | "url";
export type Field = { id: string; name: string; type: FieldType; options?: string[]; width?: number };

export const FIELD_TYPES: [FieldType, string][] = [
  ["text", uiText("文本")], ["long", uiText("多行文本")], ["number", uiText("数字")], ["select", uiText("单选")], ["multi", uiText("多选")],
  ["date", uiText("日期")], ["check", uiText("勾选")], ["person", uiText("人员")], ["url", uiText("链接")],
];
export const typeName = (t: string) => FIELD_TYPES.find((x) => x[0] === t)?.[1] || t;

/** Option colours, as Feishu's tags: a soft background, a strong text. */
export const TAGS: { bg: string; fg: string; dbg: string; dfg: string }[] = [
  { bg: "#e1eaff", fg: "#245bdb", dbg: "#1c2e55", dfg: "#8fb0f7" },
  { bg: "#d9f5d6", fg: "#237b19", dbg: "#1d3b1a", dfg: "#8fd486" },
  { bg: "#fdf0d5", fg: "#8f5b00", dbg: "#3e3017", dfg: "#f0c069" },
  { bg: "#ece2fe", fg: "#6425d0", dbg: "#33244f", dfg: "#c0a2f7" },
  { bg: "#fee3e2", fg: "#c02a26", dbg: "#4a2322", dfg: "#f6a29e" },
  { bg: "#d5f6f2", fg: "#067b6c", dbg: "#173b37", dfg: "#7fd6ca" },
  { bg: "#fdeed6", fg: "#a1460b", dbg: "#45301c", dfg: "#f3b27a" },
  { bg: "#eff0f1", fg: "#4c5562", dbg: "#2f3133", dfg: "#b8bcc2" },
];
export const tagOf = (options: string[] | undefined, v: string) => {
  const i = (options || []).indexOf(v);
  return TAGS[(i < 0 ? 7 : i) % TAGS.length];
};

export function display(f: Field, v: unknown): string {
  if (v === undefined || v === null || v === "") return "";
  if (f.type === "multi") return Array.isArray(v) ? v.join("、") : String(v);
  if (f.type === "check") return v ? "✓" : "";
  return String(v);
}
export function compare(f: Field, a: unknown, b: unknown): number {
  const empty = (x: unknown) => x === undefined || x === null || x === "" || (Array.isArray(x) && !x.length);
  if (empty(a) && empty(b)) return 0;
  if (empty(a)) return 1;
  if (empty(b)) return -1;
  if (f.type === "number") return Number(a) - Number(b);
  if (f.type === "check") return (a ? 0 : 1) - (b ? 0 : 1);
  if (f.type === "select") return (f.options || []).indexOf(String(a)) - (f.options || []).indexOf(String(b));
  return display(f, a).localeCompare(display(f, b), locale());
}
/** A value pasted or typed in, as the field keeps it. */
export function parse(f: Field, s: string): unknown {
  const t = s.trim();
  if (!t) return null;
  switch (f.type) {
    case "number": { const n = Number(t.replace(/,/g, "")); return isFinite(n) ? n : null; }
    case "check": return /^(1|true|yes|y|是|✓|√|x)$/i.test(t);
    case "multi": return t.split(/[,，、;；\n]/).map((x) => x.trim()).filter(Boolean);
    case "date": {
      const m = /^(\d{4})[-/.年](\d{1,2})[-/.月](\d{1,2})/.exec(t);
      return m ? `${m[1]}-${m[2].padStart(2, "0")}-${m[3].padStart(2, "0")}` : null;
    }
    default: return t;
  }
}
export const uid = (p: string) => p + Math.random().toString(36).slice(2, 8);
