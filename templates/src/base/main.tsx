import { t as uiText } from "../_lib/i18n";
// 多维表格: one set of records, Feishu Bitable-like: a table (the grid),
// a board grouped by a single-select field, a calendar by a date field, a
// gallery, and a form to collect records; any record opens on the side.
// Data: base (one record, "main": title, fields) and records (values by
// field id, order).
import "../_lib/sheet.css";
import "./base.css";
import { useCallback, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { CalendarDays, ClipboardList, Images, KanbanSquare, Plus, Table2 } from "lucide-react";
import { myco } from "../_lib/myco";
import { Sheet } from "../_lib/data";
import { SheetGrid } from "../_lib/grid";
import { FilterButton, Note, SearchButton, SortButton, TopBar, useDark } from "../_lib/ui";
import { apply, loadView, saveView, type ViewState } from "../_lib/view";
import { exportRows, useSheet } from "../_lib/sheetkit";
import { Board, Calendar, Form, Gallery, RecordPanel } from "./views";

const sheet = new Sheet({
  meta: "base", fieldsKey: "fields", rows: "records", valuesKey: "values", name: "多维表格", rowWord: "记录",
  defaults: [
    { id: "f1", name: "标题", type: "text" },
    { id: "f2", name: "状态", type: "select", options: ["待办", "进行中", "完成"] },
    { id: "f3", name: "负责人", type: "person" },
    { id: "f4", name: "日期", type: "date" },
  ],
});

type ViewKind = "grid" | "board" | "calendar" | "gallery" | "form";
const VIEWS: [ViewKind, string, typeof Table2][] = [
  ["grid", uiText("表格"), Table2], ["board", uiText("看板"), KanbanSquare], ["calendar", uiText("日历"), CalendarDays], ["gallery", uiText("画廊"), Images], ["form", uiText("表单"), ClipboardList],
];

function App() {
  useSheet(sheet);
  const dark = useDark();
  const key = `myco-base-view:${sheet.page}`;
  const [kind, setKindRaw] = useState<ViewKind>(() => { try { return (localStorage.getItem(`${key}:kind`) as ViewKind) || "grid"; } catch { return "grid"; } });
  const setKind = (k: ViewKind) => { setKindRaw(k); try { localStorage.setItem(`${key}:kind`, k); } catch { /* not kept */ } };
  const [view, setV] = useState<ViewState>(() => loadView(key));
  const setView = useCallback((v: ViewState) => { setV(v); saveView(key, v); }, [key]);
  const [search, setSearch] = useState(false);
  const [open, setOpen] = useState<string | null>(null);
  const [keep, setKeep] = useState<Set<string>>(new Set());
  const onAdded = useCallback((id: string) => setKeep((k) => new Set(k).add(id)), []);
  const fields = sheet.fields;
  const all = sheet.list;
  const rows = useMemo(() => apply(all, fields, view, keep), [all, fields, view, keep]);
  const filtered = rows.length !== all.length || view.sorts.length > 0;
  return (
    <div className="app">
      <TopBar sheet={sheet} onExport={(k) => void exportRows(sheet, rows, k)} />
      <div className="toolbar base-bar">
        <nav className="tabs" role="tablist">
          {VIEWS.map(([k, name, Icon]) => (
            <button key={k} type="button" role="tab" aria-selected={kind === k} className={`tab${kind === k ? " is-on" : ""}`} onClick={() => setKind(k)}>
              <Icon className="ico" />{name}
            </button>
          ))}
        </nav>
        {kind !== "form" && <>
          <span className="toolbar-sep" />
          <FilterButton fields={fields} view={view} setView={setView} />
          <SortButton fields={fields} view={view} setView={setView} />
          {kind === "grid" && <SearchButton onClick={() => setSearch(true)} />}
          <span className="toolbar-count">{filtered ? uiText("{0} / {1} 条", rows.length, all.length) : uiText("共 {0} 条", all.length)}</span>
          <button type="button" className="btn btn-primary base-add" onClick={() => { const id = sheet.add({}); onAdded(id); if (kind !== "grid") setOpen(id); }}>
            <Plus className="ico" />{uiText("添加记录")}{" "}</button>
        </>}
      </div>
      {kind === "grid" && (
        <SheetGrid sheet={sheet} rows={rows} dark={dark} onOpen={setOpen} search={search} setSearch={setSearch} onAdded={onAdded}
          canMove={!view.sorts.length} onSort={(field, desc) => setView({ ...view, sorts: [{ field, desc }] })} />
      )}
      {kind === "board" && <Board sheet={sheet} rows={rows} dark={dark} onOpen={setOpen} onAdded={onAdded} />}
      {kind === "calendar" && <Calendar sheet={sheet} rows={rows} dark={dark} onOpen={setOpen} onAdded={onAdded} />}
      {kind === "gallery" && <Gallery sheet={sheet} rows={rows} dark={dark} onOpen={setOpen} onAdded={onAdded} />}
      {kind === "form" && <Form sheet={sheet} dark={dark} />}
      {open && sheet.row(open) && (
        <RecordPanel sheet={sheet} id={open} ids={rows.map((r) => r.id)} dark={dark} onClose={() => setOpen(null)} onGo={setOpen} />
      )}
      <Note sheet={sheet} />
    </div>
  );
}

async function start() {
  const root = createRoot(document.getElementById("root")!);
  const outside = <div className="outside">{uiText("在 myco App 里打开这个网页，才能读写它的内容。")}</div>;
  if (!myco) { root.render(outside); return; }
  try { await sheet.start(); } catch { root.render(outside); return; }
  root.render(<App />);
}
void start();
