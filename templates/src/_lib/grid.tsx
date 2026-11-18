// A table on Glide Data Grid, as 表格 shows it and 多维表格's table view:
// columns are fields (resize, move, a menu each), rows are records (drag by
// the number, a menu on right click), cells edit in place, select a range
// to copy, paste a block from Excel, drag the fill handle, Ctrl+F searches.
import "@glideapps/glide-data-grid/dist/index.css";
import { useCallback, useMemo, useRef, useState } from "react";
import {
  CompactSelection, DataEditor, GridCellKind, GridColumnIcon,
  type DataEditorRef, type EditableGridCell, type GridCell, type GridColumn, type GridSelection, type Item, type Theme,
} from "@glideapps/glide-data-grid";
import { ArrowDownToLine, ArrowUpToLine, Maximize2, Plus, Trash2 } from "lucide-react";
import { parse, uid, type Field, type FieldType } from "./fields";
import { renderers, type DateCell, type PersonCell, type TagsCell } from "./cells";
import { FieldMenu, MenuItem, Popover, rectOf } from "./ui";
import type { Row, Sheet } from "./data";

const ICON: Record<FieldType, GridColumnIcon> = {
  text: GridColumnIcon.HeaderString, long: GridColumnIcon.HeaderTextTemplate, number: GridColumnIcon.HeaderNumber, date: GridColumnIcon.HeaderDate,
  select: GridColumnIcon.HeaderSingleValue, multi: GridColumnIcon.HeaderArray, check: GridColumnIcon.HeaderBoolean, person: GridColumnIcon.HeaderEmoji,
  url: GridColumnIcon.HeaderUri,
};
const WIDTH: Record<FieldType, number> = { text: 180, long: 240, number: 110, date: 130, select: 140, multi: 200, check: 90, person: 130, url: 200 };
const FONT = `-apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", "Segoe UI", sans-serif`;

export function gridTheme(dark: boolean): Partial<Theme> {
  return dark ? {
    accentColor: "#4c88ff", accentFg: "#ffffff", accentLight: "rgba(76, 136, 255, 0.18)",
    textDark: "#ebebeb", textMedium: "#a6a6a6", textLight: "#757575", textBubble: "#ebebeb", textHeader: "#a6a6a6", textHeaderSelected: "#ffffff",
    bgIconHeader: "#a6a6a6", fgIconHeader: "#262626",
    bgCell: "#1f1f1f", bgCellMedium: "#262626", bgHeader: "#262626", bgHeaderHasFocus: "#2e3a52", bgHeaderHovered: "#2c2c2c",
    bgBubble: "#2c2c2c", bgBubbleSelected: "#1f1f1f", bgSearchResult: "#5c4a12",
    borderColor: "#373737", horizontalBorderColor: "#2e2e2e", drilldownBorder: "#373737", linkColor: "#6b9dff",
    headerFontStyle: "500 13px", baseFontStyle: "14px", fontFamily: FONT, editorFontSize: "14px", cellHorizontalPadding: 10, roundingRadius: 6,
  } : {
    accentColor: "#3370ff", accentFg: "#ffffff", accentLight: "rgba(51, 112, 255, 0.1)",
    textDark: "#1f2329", textMedium: "#646a73", textLight: "#8f959e", textBubble: "#1f2329", textHeader: "#646a73", textHeaderSelected: "#ffffff",
    bgIconHeader: "#8f959e", fgIconHeader: "#ffffff",
    bgCell: "#ffffff", bgCellMedium: "#f8f9fa", bgHeader: "#f5f6f7", bgHeaderHasFocus: "#e1eaff", bgHeaderHovered: "#eff0f1",
    bgBubble: "#eff0f1", bgBubbleSelected: "#ffffff", bgSearchResult: "#fff3c4",
    borderColor: "#dee0e3", horizontalBorderColor: "#ebedf0", drilldownBorder: "#dee0e3", linkColor: "#3370ff",
    headerFontStyle: "500 13px", baseFontStyle: "14px", fontFamily: FONT, editorFontSize: "14px", cellHorizontalPadding: 10, roundingRadius: 6,
  };
}

function cellOf(f: Field, v: unknown, dark: boolean, people: string[]): GridCell {
  const s = v === undefined || v === null ? "" : Array.isArray(v) ? v.join(", ") : String(v);
  switch (f.type) {
    case "number": {
      const n = typeof v === "number" ? v : s === "" ? undefined : Number(s);
      return { kind: GridCellKind.Number, data: n !== undefined && isFinite(n) ? n : undefined, displayData: n !== undefined && isFinite(n) ? n.toLocaleString("zh-CN", { maximumFractionDigits: 6 }) : s, allowOverlay: true, contentAlign: "right" };
    }
    case "check": return { kind: GridCellKind.Boolean, data: !!v, allowOverlay: false };
    case "url": return { kind: GridCellKind.Uri, data: s, allowOverlay: true, hoverEffect: true };
    case "select": case "multi": {
      const values = Array.isArray(v) ? v.map(String) : s ? [s] : [];
      return { kind: GridCellKind.Custom, allowOverlay: true, copyData: values.join(", "), data: { kind: "tags", values, options: f.options || [], multi: f.type === "multi", dark } } as TagsCell;
    }
    case "date": return { kind: GridCellKind.Custom, allowOverlay: true, copyData: s, data: { kind: "date", value: s } } as DateCell;
    case "person": return { kind: GridCellKind.Custom, allowOverlay: true, copyData: s, data: { kind: "person", value: s, people } } as PersonCell;
    default: return { kind: GridCellKind.Text, data: s, displayData: f.type === "long" ? s.replace(/\s*\n\s*/g, " ⏎ ") : s, allowOverlay: true };
  }
}
function valueOf(f: Field, c: EditableGridCell): unknown {
  switch (c.kind) {
    case GridCellKind.Number: return c.data === undefined || c.data === null || !isFinite(c.data) ? null : c.data;
    case GridCellKind.Boolean: return c.data === true;
    case GridCellKind.Uri: case GridCellKind.Text: return f.type === "text" || f.type === "long" || f.type === "url" ? c.data : parse(f, String(c.data ?? ""));
    case GridCellKind.Custom: {
      const d = c.data as TagsCell["data"] | DateCell["data"] | PersonCell["data"];
      if (d.kind === "tags") return f.type === "multi" ? d.values : d.values[0] ?? null;
      return d.value || null;
    }
    default: return null;
  }
}

const NONE: GridSelection = { columns: CompactSelection.empty(), rows: CompactSelection.empty() };

export function SheetGrid({ sheet, rows, dark, onOpen, search, setSearch, onSort, onAdded, canMove }: {
  sheet: Sheet; rows: Row[]; dark: boolean;
  onOpen?: (id: string) => void;
  search: boolean; setSearch: (v: boolean) => void;
  onSort: (field: string, desc: boolean) => void;
  onAdded: (id: string) => void;
  canMove: boolean;
}) {
  const fields = sheet.fields;
  const ref = useRef<DataEditorRef>(null);
  const [widths, setWidths] = useState<Record<string, number>>({});
  const [menu, setMenu] = useState<{ field: Field; at: { x: number; y: number; w?: number; h?: number } } | null>(null);
  const [ctx, setCtx] = useState<{ row: number; at: { x: number; y: number } } | null>(null);
  const [sel, setSel] = useState<GridSelection>(NONE);
  const [hover, setHover] = useState(-1);
  const people = useMemo(() => sheet.people(), [sheet, sheet.version]); // eslint-disable-line react-hooks/exhaustive-deps
  const theme = useMemo(() => gridTheme(dark), [dark]);

  const columns = useMemo<GridColumn[]>(() => fields.map((f, i) => ({
    id: f.id, title: f.name, icon: ICON[f.type] || GridColumnIcon.HeaderString, hasMenu: true,
    width: widths[f.id] ?? f.width ?? (i === 0 ? Math.max(200, WIDTH[f.type]) : WIDTH[f.type] || 160),
  })), [fields, widths]);

  const getCellContent = useCallback(([c, r]: Item): GridCell => {
    const f = fields[c], row = rows[r];
    if (!f || !row) return { kind: GridCellKind.Text, data: "", displayData: "", allowOverlay: false };
    return cellOf(f, row.values[f.id], dark, people);
  }, [fields, rows, dark, people]);

  const onCellsEdited = useCallback((items: readonly { location: Item; value: EditableGridCell }[]) => {
    sheet.edit(items.map(({ location: [c, r], value }) => ({ id: rows[r]?.id, values: { [fields[c].id]: valueOf(fields[c], value) } })).filter((x) => x.id));
    return true;
  }, [sheet, rows, fields]);

  const onPaste = useCallback(([c0, r0]: Item, values: readonly (readonly string[])[]) => {
    const edits: { id: string; values: Record<string, unknown> }[] = [];
    values.forEach((line, i) => {
      const vals: Record<string, unknown> = {};
      line.forEach((s, j) => { const f = fields[c0 + j]; if (f) vals[f.id] = parse(f, s); });
      const row = rows[r0 + i];
      if (row) edits.push({ id: row.id, values: vals });
      else onAdded(sheet.add(vals));
    });
    if (edits.length) sheet.edit(edits);
    return false;
  }, [sheet, rows, fields, onAdded]);

  const selectedIds = () => sel.rows.toArray().map((i) => rows[i]?.id).filter(Boolean) as string[];
  const removeRows = (ids: string[]) => { if (ids.length) { sheet.remove(ids); setSel(NONE); } };
  const addField = (at: ReturnType<typeof rectOf>) => {
    const nf: Field = { id: uid("f"), name: `字段 ${fields.length + 1}`, type: "text" };
    sheet.saveFields([...fields, nf]);
    setMenu({ field: nf, at });
  };
  const word = sheet.cfg.rowWord;

  return (
    <div className="grid" data-testid="grid">
      <DataEditor
        ref={ref}
        columns={columns}
        rows={rows.length}
        getCellContent={getCellContent}
        customRenderers={renderers}
        theme={theme}
        width="100%"
        height="100%"
        rowHeight={36}
        headerHeight={38}
        freezeColumns={1}
        smoothScrollX
        smoothScrollY
        rowMarkers={{ kind: "clickable-number", checkboxStyle: "square" }}
        getCellsForSelection
        fillHandle
        onCellsEdited={onCellsEdited}
        onPaste={onPaste}
        gridSelection={sel}
        onGridSelectionChange={setSel}
        rowSelectionMode="multi"
        onDelete={(s) => {
          if (s.rows.length) { removeRows(s.rows.toArray().map((i) => rows[i]?.id).filter(Boolean) as string[]); return false; }
          return true;
        }}
        trailingRowOptions={{ hint: word === "行" ? "新增一行" : `添加${word}`, sticky: true, tint: true }}
        onRowAppended={() => { onAdded(sheet.add({})); }}
        onHeaderMenuClick={(c, b) => fields[c] && setMenu({ field: fields[c], at: { x: b.x, y: b.y, w: b.width, h: b.height } })}
        onHeaderContextMenu={(c, e) => { e.preventDefault(); if (fields[c]) setMenu({ field: fields[c], at: { x: e.bounds.x + e.localEventX, y: e.bounds.y + e.localEventY } }); }}
        onCellContextMenu={([, r], e) => { e.preventDefault(); if (rows[r]) setCtx({ row: r, at: { x: e.bounds.x + e.localEventX, y: e.bounds.y + e.localEventY } }); }}
        onColumnResize={(col, size) => setWidths((w) => ({ ...w, [String(col.id)]: size }))}
        onColumnResizeEnd={(col, size) => sheet.saveFields(fields.map((f) => (f.id === col.id ? { ...f, width: Math.round(size) } : f)))}
        onColumnMoved={(from, to) => {
          const next = fields.slice();
          const [f] = next.splice(from, 1);
          next.splice(to, 0, f);
          sheet.saveFields(next);
        }}
        onRowMoved={canMove ? (from, to) => {
          const ids = rows.map((r) => r.id);
          const [id] = ids.splice(from, 1);
          ids.splice(to, 0, id);
          sheet.reorder(ids);
        } : undefined}
        showSearch={search}
        onSearchClose={() => setSearch(false)}
        keybindings={{ search: true }}
        onItemHovered={(a) => setHover(a.kind === "cell" ? a.location[1] : -1)}
        getRowThemeOverride={(r) => (r === hover ? { bgCell: dark ? "#262626" : "#f5f6f7" } : undefined)}
        drawCell={onOpen ? (a, draw) => {
          draw();
          if (a.col !== 0 || a.row !== hover) return;
          const { ctx: g, rect } = a, s = 22, x = rect.x + rect.width - s - 6, y = rect.y + (rect.height - s) / 2;
          g.save();
          g.beginPath();
          g.roundRect?.(x, y, s, s, 5);
          g.fillStyle = dark ? "#333" : "#ffffff";
          g.strokeStyle = dark ? "#4a4a4a" : "#dee0e3";
          g.fill();
          g.stroke();
          g.strokeStyle = dark ? "#c9c9c9" : "#646a73";
          g.lineWidth = 1.4;
          g.beginPath();
          g.moveTo(x + 7, y + s - 7); g.lineTo(x + s - 7, y + 7);
          g.moveTo(x + 11, y + 7); g.lineTo(x + s - 7, y + 7); g.lineTo(x + s - 7, y + 11);
          g.moveTo(x + 7, y + s - 11); g.lineTo(x + 7, y + s - 7); g.lineTo(x + 11, y + s - 7);
          g.stroke();
          g.restore();
        } : undefined}
        onCellClicked={([c, r], e) => {
          if (onOpen && c === 0 && rows[r] && e.localEventX > e.bounds.width - 34) { e.preventDefault(); onOpen(rows[r].id); }
        }}
        rightElement={
          <div className="grid-add-col">
            <button type="button" aria-label="添加字段" title="添加字段" onClick={(e) => addField(rectOf(e.currentTarget))}><Plus className="ico" /></button>
          </div>
        }
        rightElementProps={{ sticky: false, fill: false }}
      />
      {menu && (
        <FieldMenu sheet={sheet} field={sheet.fields.find((f) => f.id === menu.field.id) || menu.field} at={menu.at} dark={dark}
          first={fields[0]?.id === menu.field.id} onClose={() => setMenu(null)} onSort={(desc) => onSort(menu.field.id, desc)} />
      )}
      {ctx && rows[ctx.row] && (() => {
        const row = rows[ctx.row], many = selectedIds().length > 1 && selectedIds().includes(row.id) ? selectedIds() : [row.id];
        const close = () => setCtx(null);
        return (
          <Popover at={ctx.at} onClose={close}>
            <div className="menu">
              {onOpen && <MenuItem icon={<Maximize2 className="ico" />} onClick={() => { close(); onOpen(row.id); }}>展开{word}</MenuItem>}
              <MenuItem icon={<ArrowUpToLine className="ico" />} onClick={() => { close(); onAdded(sheet.add({}, { before: row.id })); }}>在上方插入{word === "行" ? "一行" : word}</MenuItem>
              <MenuItem icon={<ArrowDownToLine className="ico" />} onClick={() => { close(); onAdded(sheet.add({}, { after: row.id })); }}>在下方插入{word === "行" ? "一行" : word}</MenuItem>
              <div className="menu-sep" />
              <MenuItem danger icon={<Trash2 className="ico" />} onClick={() => { close(); removeRows(many); }}>
                {many.length > 1 ? `删除选中的 ${many.length} ${word === "行" ? "行" : "条"}` : `删除这${word === "行" ? "一行" : "条" + word}`}
              </MenuItem>
            </div>
          </Popover>
        );
      })()}
    </div>
  );
}
