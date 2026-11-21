import { t as uiText } from "../_lib/i18n";
// 表格: a table people and agents fill together, Feishu-like: columns with
// types, edit in place, copy and paste blocks, fill down, filter, sort,
// search, export to Excel and CSV. Data: sheet (one record, "main": title,
// columns) and rows (cells by column id, order).
import "../_lib/sheet.css";
import { useCallback, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { Plus } from "lucide-react";
import { myco } from "../_lib/myco";
import { Sheet } from "../_lib/data";
import { SheetGrid } from "../_lib/grid";
import { FilterButton, Note, SearchButton, SortButton, TopBar, useDark } from "../_lib/ui";
import { apply, loadView, saveView, type ViewState } from "../_lib/view";
import { exportRows, useSheet } from "../_lib/sheetkit";

const sheet = new Sheet({
  meta: "sheet", fieldsKey: "columns", rows: "rows", valuesKey: "cells", name: "表格", rowWord: "行",
  defaults: [
    { id: "c1", name: "名称", type: "text" },
    { id: "c2", name: "负责人", type: "person" },
    { id: "c3", name: "状态", type: "select", options: ["待办", "进行中", "完成"] },
    { id: "c4", name: "日期", type: "date" },
  ],
});

function App() {
  useSheet(sheet);
  const dark = useDark();
  const key = `myco-table-view:${sheet.page}`;
  const [view, setV] = useState<ViewState>(() => loadView(key));
  const setView = useCallback((v: ViewState) => { setV(v); saveView(key, v); }, [key]);
  const [search, setSearch] = useState(false);
  const [keep, setKeep] = useState<Set<string>>(new Set());
  const onAdded = useCallback((id: string) => setKeep((k) => new Set(k).add(id)), []);
  const fields = sheet.fields;
  const all = sheet.list;
  const rows = useMemo(() => apply(all, fields, view, keep), [all, fields, view, keep]);
  const filtered = rows.length !== all.length || view.sorts.length > 0;
  return (
    <div className="app">
      <TopBar sheet={sheet} onExport={(k) => void exportRows(sheet, rows, k)} />
      <div className="toolbar">
        <button type="button" className="btn btn-primary" onClick={() => onAdded(sheet.add({}))}><Plus className="ico" />{uiText("新增一行")}</button>
        <FilterButton fields={fields} view={view} setView={setView} />
        <SortButton fields={fields} view={view} setView={setView} />
        <SearchButton onClick={() => setSearch(true)} />
        <span className="toolbar-count">{filtered ? uiText("{0} / {1} 行", rows.length, all.length) : uiText("共 {0} 行", all.length)}</span>
      </div>
      <SheetGrid sheet={sheet} rows={rows} dark={dark} search={search} setSearch={setSearch} onAdded={onAdded}
        canMove={!view.sorts.length}
        onSort={(field, desc) => setView({ ...view, sorts: [{ field, desc }] })} />
      <Note sheet={sheet} />
    </div>
  );
}

async function start() {
  const root = createRoot(document.getElementById("root")!);
  if (!myco) { root.render(<div className="outside">{uiText("在 myco App 里打开这个网页，才能读写它的内容。")}</div>); return; }
  try { await sheet.start(); } catch { root.render(<div className="outside">{uiText("在 myco App 里打开这个网页，才能读写它的内容。")}</div>); return; }
  root.render(<App />);
}
void start();
