// What 表格 and 多维表格 share besides the grid: following the data in
// React, and exporting.
import { useSyncExternalStore } from "react";
import { save } from "./myco";
import { csv, xlsx } from "./export";
import type { Row, Sheet } from "./data";

export function useSheet(sheet: Sheet) {
  return useSyncExternalStore(sheet.subscribe, () => sheet.version);
}

export async function exportRows(sheet: Sheet, rows: Row[], kind: "xlsx" | "csv") {
  const fields = sheet.fields;
  const table: unknown[][] = [fields.map((f) => f.name), ...rows.map((r) => fields.map((f) => {
    const v = r.values[f.id];
    if (v === undefined || v === null) return "";
    if (f.type === "number") return typeof v === "number" ? v : Number(v) || String(v);
    if (f.type === "check") return !!v;
    return Array.isArray(v) ? v.join("、") : String(v);
  }))];
  const name = (sheet.title.trim() || sheet.cfg.name).replace(/[\\/:*?"<>|]/g, " ");
  try {
    if (kind === "csv") await save(`${name}.csv`, csv(table), "text/csv");
    else await save(`${name}.xlsx`, xlsx(name, table, fields.map((f) => f.width || 140)), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet");
    sheet.flash("已存到「下载」");
  } catch (e) {
    sheet.flash((e as Error).message || "没存下来");
  }
}
