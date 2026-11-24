// Portable rendering/export helpers. Typed numeric strings are never converted
// through JavaScript Number, so large identifiers and decimals remain exact.
import type { Cell, Column, Engine, Result, Row, Table } from "./client";
export const cellText = (cell: Cell) => cell.kind === "null" ? "NULL" : cell.value || "";
export const tableKey = (table: Table) => table.schema + "\u0000" + table.name;
export function quoteIdentifier(name: string, engine: Engine) { const quote = engine === "mysql" ? "`" : '"'; return quote + name.split(quote).join(quote + quote) + quote; }
export function selectSQL(table: Table, engine: Engine) { return `SELECT *\nFROM ${table.schema && engine !== "sqlite" ? quoteIdentifier(table.schema, engine) + "." : ""}${quoteIdentifier(table.name, engine)}\nLIMIT 100;`; }
export function initialCell(column: Column): Cell { return { kind: /INT|NUMERIC|DECIMAL|REAL|FLOAT|DOUBLE/i.test(column.type) ? "number" : /BOOL/i.test(column.type) ? "bool" : /BLOB|BINARY|BYTEA/i.test(column.type) ? "binary" : /JSON/i.test(column.type) ? "json" : "text", value: /BOOL/i.test(column.type) ? "false" : "" }; }
export function changedCells(columns: Column[], before: Row | null, cells: Record<string, Cell>, enabled: Set<string>) { const out: Record<string, Cell> = {}; columns.forEach((c, i) => { if (!enabled.has(c.name) || c.generated) return; const next = cells[c.name]; if (next && (!before || next.kind !== before.values[i]?.kind || (next.value || "") !== (before.values[i]?.value || ""))) out[c.name] = next; }); return out; }
export function resultCSV(result: Result): string {
  // A spreadsheet must not interpret database text as a formula on open.
  const escape = (s: string) => '"' + (/^[\t\r\n ]*[=+@-]/.test(s) ? "'" + s : s).split('"').join('""') + '"';
  return "\uFEFF" + [result.columns.map(c => escape(c.name)).join(","), ...result.rows.map(r => r.values.map(v => escape(v.kind === "null" ? "" : v.value || "")).join(","))].join("\r\n");
}
export function resultJSON(result: Result): string { return JSON.stringify({ columns: result.columns, rows: result.rows.map(r => ({ values: r.values, ...(r.document ? { document: r.document } : {}) })), truncated: result.truncated }, null, 2); }
