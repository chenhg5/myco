// Whiteboard composition and accessible canvas rendering. Editor gestures,
// record mutations, clipboard, geometry and export each live in their module.
import "../_lib/sheet.css";
import "./wb.css";
import { useEffect, useRef } from "react";
import {
  ArrowDownToLine,
  ArrowUpToLine,
  Copy,
  Download,
  HelpCircle,
  Maximize,
  Minus,
  Plus,
  Redo2,
  Scissors,
  Trash2,
  Undo2,
  ClipboardPaste,
  LockKeyhole,
} from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { Bar, Toast, mount } from "../_lib/appkit";
import { Popover, MenuItem, rectOf } from "../_lib/ui";
import { board, items, kit, title } from "./model";
import {
  connector,
  labelWidth,
  port,
  SIDES,
  number,
  type Side,
} from "./geometry";
import {
  canUndo,
  canRedo,
  undo,
  redo,
  create,
  remove,
  patchMany,
  duplicate,
  selectedGraph,
  align,
  distribute,
  layer,
} from "./edit";
import { colors } from "./export";
import { Tools, Properties, Empty, PastePrompt } from "./panels";
import { useBoard, editable } from "./useBoard";
import { myco } from "../_lib/myco";
import { startSharedExport } from "./shared-export";
function App() {
  useEffect(() => startSharedExport(myco!.data, () => {
    if (kit.pending || kit.failed) return null;
    return JSON.parse(JSON.stringify({ board: board.rows, items: items.rows }));
  }, kit.subscribe), []);
  const {
    arrows,
    tool,
    view,
    sel,
    editing,
    link,
    marquee,
    guides,
    menu,
    help,
    dark,
    space,
    stage,
    touched,
    list,
    selected,
    selectionData,
    world,
    fit,
    zoom,
    selectTool,
    startLink,
    down,
    move,
    up,
    cancel,
    menuPaste,
    paste,
    pastePrompt,
    setPastePrompt,
    copy,
    exportSvg,
    hit,
    projected,
    projectedMap,
    setSel,
    setEditing,
    setMenu,
    setHelp,
  } = useBoard();
  return (
    <div className="app wb">
      <Bar
        kit={kit}
        title={title.get()}
        placeholder={uiText("未命名白板")}
        onTitle={title.set}
      >
        <div className="wb-history">
          <button
            className="icon-btn"
            type="button"
            aria-label={uiText("撤销")}
            title={uiText("撤销（⌘/Ctrl+Z）")}
            disabled={!canUndo()}
            onClick={undo}
          >
            <Undo2 className="ico" />
          </button>
          <button
            className="icon-btn"
            type="button"
            aria-label={uiText("重做")}
            title={uiText("重做（⌘/Ctrl+Shift+Z）")}
            disabled={!canRedo()}
            onClick={redo}
          >
            <Redo2 className="ico" />
          </button>
        </div>
        <button
          className="icon-btn"
          type="button"
          aria-label={uiText("粘贴")}
          title={uiText("粘贴")}
          onClick={() => void menuPaste()}
        >
          <ClipboardPaste className="ico" />
        </button>
        <button
          className="icon-btn"
          type="button"
          aria-label={uiText("操作帮助")}
          aria-expanded={!!help}
          onClick={(e) => setHelp(help ? null : rectOf(e.currentTarget))}
        >
          <HelpCircle className="ico" />
        </button>
        <button
          className="btn"
          type="button"
          aria-label={uiText("导出 SVG")}
          onClick={() => void exportSvg()}
        >
          <Download className="ico" />
          <span>{uiText("导出 SVG")}</span>
        </button>
      </Bar>
      <div
        ref={stage}
        className={`wb-stage tool-${space ? "hand" : tool}`}
        tabIndex={0}
        aria-label={uiText("白板画布")}
        onPointerDown={(e) => down(e)}
        onPointerMove={move}
        onPointerUp={up}
        onPointerCancel={cancel}
        onContextMenu={(e) => {
          if (editable(e.target)) return;
          e.preventDefault();
          const id = (e.target as Element)
            .closest("[data-id]")
            ?.getAttribute("data-id");
          if (id && !sel.includes(id)) setSel([id]);
          setMenu({ x: e.clientX, y: e.clientY });
        }}
        onDoubleClick={(e) => {
          if ((e.target as Element).closest(".wb-chrome,.wb-item,.wb-links"))
            return;
          const id = create("sticky", world(e.clientX, e.clientY));
          setSel([id]);
          setEditing(id);
        }}
        style={{
          backgroundPosition: `${view.x}px ${view.y}px`,
          backgroundSize: `${24 * view.k}px ${24 * view.k}px`,
        }}
      >
        {pastePrompt && (
          <PastePrompt
            onPaste={(value) => {
              paste(value);
              setPastePrompt(false);
              stage.current?.focus({ preventScroll: true });
            }}
            onClose={() => setPastePrompt(false)}
          />
        )}
        <div
          className="wb-world"
          style={{
            transform: `translate(${view.x}px,${view.y}px) scale(${view.k})`,
          }}
        >
          <svg className="wb-links" width="1" height="1">
            <defs>
              <marker
                id="wb-head"
                viewBox="0 0 10 10"
                refX="9"
                refY="5"
                markerWidth="7"
                markerHeight="7"
                orient="auto-start-reverse"
              >
                <path
                  d="M1 1L9 5L1 9"
                  fill="none"
                  stroke="context-stroke"
                  strokeWidth="1.5"
                />
              </marker>
            </defs>
            {arrows.map((a) => {
              const f = projectedMap.get(String(a.data.from)),
                t = projectedMap.get(String(a.data.to));
              if (!f || !t) return null;
              const c = connector(f, t, a.data, projected),
                on = selected.includes(a.id),
                palette = colors[String(a.data.color)] || colors.gray;
              return (
                <g
                  key={a.id}
                  data-id={a.id}
                  className={`wb-connector${on ? " is-selected" : ""}`}
                  onPointerDown={(e) => {
                    e.stopPropagation();
                    setSel(e.shiftKey ? [...new Set([...sel, a.id])] : [a.id]);
                  }}
                  onDoubleClick={(e) => {
                    e.stopPropagation();
                    if (!a.data.locked) {
                      setSel([a.id]);
                      setEditing(a.id);
                    }
                  }}
                >
                  <path className="wb-link-hit" d={c.path} />
                  <path
                    className="wb-line"
                    data-route={a.data.route || "elbow"}
                    d={c.path}
                    style={{
                      stroke: palette[dark ? 3 : 2],
                      strokeWidth: Math.max(
                        1,
                        Math.min(4, number(a.data.width, 2)),
                      ),
                    }}
                    strokeDasharray={
                      a.data.dash === "dashed" ? "8 5" : undefined
                    }
                    markerEnd="url(#wb-head)"
                  />
                  {String(a.data.text || "") && (
                    <g
                      className="wb-link-caption"
                      transform={`translate(${c.label.x},${c.label.y})`}
                    >
                      <rect
                        x={-labelWidth(String(a.data.text)) / 2}
                        y="-13"
                        width={labelWidth(String(a.data.text))}
                        height="26"
                        rx="5"
                      />
                      <text textAnchor="middle" dominantBaseline="central">
                        {String(a.data.text).slice(0, 60)}
                      </text>
                    </g>
                  )}
                  {on && !a.data.locked && (
                    <>
                      {(["from", "to"] as const).map((end, i) => (
                        <circle
                          key={end}
                          className="wb-link-end"
                          aria-label={
                            i ? uiText("拖动终点") : uiText("拖动起点")
                          }
                          cx={i ? c.b.x : c.a.x}
                          cy={i ? c.b.y : c.a.y}
                          r={6 / view.k}
                          onPointerDown={(e) =>
                            startLink(
                              e,
                              i ? f.id : t.id,
                              (i ? a.data.from_side : a.data.to_side) as Side,
                              { id: a.id, end },
                            )
                          }
                        />
                      ))}
                    </>
                  )}
                </g>
              );
            })}
            {link &&
              (() => {
                const f = projectedMap.get(link.from);
                if (!f) return null;
                const p = hit ? port(hit.box, hit.side) : link,
                  target = hit?.box || {
                    ...f,
                    id: "cursor",
                    x: p.x,
                    y: p.y,
                    w: 0,
                    h: 0,
                  },
                  c = connector(f, target, {
                    from_side: link.side,
                    to_side: hit?.side,
                    route: "elbow",
                  });
                return (
                  <path
                    className="wb-line wb-preview"
                    d={c.path}
                    markerEnd="url(#wb-head)"
                  />
                );
              })()}
          </svg>
          {projected.map((b) => {
            const on = selected.includes(b.id),
              palette = colors[b.color] || colors.gray;
            return (
              <div
                key={b.id}
                data-id={b.id}
                className={`wb-item is-${b.type}${on ? " is-on" : ""}${b.locked ? " is-locked" : ""}${hit?.box.id === b.id ? " is-target" : ""}`}
                style={{
                  left: b.x,
                  top: b.y,
                  width: b.w,
                  height: b.h,
                  zIndex: b.z,
                  background:
                    b.type === "sticky" ? palette[dark ? 1 : 0] : undefined,
                  borderColor:
                    b.type === "rect" || b.type === "ellipse"
                      ? palette[dark ? 3 : 2]
                      : undefined,
                }}
                onPointerDown={(e) => down(e, b.id)}
                onDoubleClick={(e) => {
                  e.stopPropagation();
                  if (!b.locked) {
                    setSel([b.id]);
                    setEditing(b.id);
                  }
                }}
              >
                <Text
                  value={b.text}
                  editing={editing === b.id}
                  fontSize={b.font_size}
                  align={b.align}
                  onDone={(text) => {
                    setEditing(null);
                    if (text !== b.text) patchMany([b.id], { text });
                  }}
                />
                {b.locked && <LockKeyhole className="wb-lock" />}
                {on &&
                  selected.length === 1 &&
                  !b.locked &&
                  editing !== b.id && (
                    <span
                      className="wb-handle"
                      role="button"
                      aria-label={uiText("调整大小")}
                      onPointerDown={(e) => down(e, b.id, true)}
                    />
                  )}
                {!editing &&
                  !b.locked &&
                  (on || tool === "arrow" || !!link) &&
                  SIDES.map((side) => (
                    <button
                      type="button"
                      key={side}
                      tabIndex={-1}
                      className={`wb-port is-${side}`}
                      aria-label={uiText("从{0}侧连接", sideLabel(side))}
                      onPointerDown={(e) => startLink(e, b.id, side)}
                    />
                  ))}
              </div>
            );
          })}
          {marquee && (
            <div
              className="wb-marquee"
              style={{
                left: marquee.x,
                top: marquee.y,
                width: marquee.w,
                height: marquee.h,
              }}
            />
          )}
        </div>
        {guides.map((g) => (
          <div
            key={g.axis}
            className={`wb-guide is-${g.axis}`}
            style={
              g.axis === "x"
                ? { left: view.x + g.pos * view.k }
                : { top: view.y + g.pos * view.k }
            }
          />
        ))}
        {selected.length > 0 && !editing && (
          <Properties
            count={selected.length}
            data={selectionData || { color: "gray" }}
            dark={dark}
            onPatch={(p) => patchMany(selected, p)}
            onAlign={(mode) => align(selected, mode)}
            onDistribute={(axis) => distribute(selected, axis)}
            onDuplicate={() => setSel(duplicate(selectedGraph(selected)))}
            onCopy={() => void copy()}
            onDelete={() => {
              remove(selected);
              setSel([]);
            }}
          />
        )}
        {editing && items.get(editing)?.data.type === "arrow" && (
          <div
            className="wb-label-editor wb-chrome"
            onPointerDown={(e) => e.stopPropagation()}
          >
            <input
              className="inp"
              autoFocus
              aria-label={uiText("连线文字")}
              maxLength={120}
              defaultValue={String(items.get(editing)?.data.text || "")}
              onBlur={(e) => {
                patchMany([editing], { text: e.target.value });
                setEditing(null);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === "Escape")
                  e.currentTarget.blur();
              }}
            />
          </div>
        )}
        <Tools tool={tool} setTool={selectTool} />
        <div
          className="wb-zoom wb-chrome"
          onPointerDown={(e) => e.stopPropagation()}
        >
          <button
            type="button"
            aria-label={uiText("缩小")}
            onClick={() => zoom(1 / 1.2)}
          >
            <Minus className="ico" />
          </button>
          <button
            type="button"
            className="wb-k"
            title={uiText("恢复 100%")}
            onClick={() => zoom(1 / view.k)}
          >
            {Math.round(view.k * 100)}%
          </button>
          <button
            type="button"
            aria-label={uiText("放大")}
            onClick={() => zoom(1.2)}
          >
            <Plus className="ico" />
          </button>
          <span className="wb-sep" />
          <button
            type="button"
            aria-label={uiText("适应画面")}
            onClick={() => {
              touched.current = false;
              fit();
            }}
          >
            <Maximize className="ico" />
          </button>
        </div>
        <div className="wb-hint">
          {link
            ? uiText("选择目标，或在空白处创建节点")
            : uiText("拖动框选 · 空格平移 · 双击编辑")}
        </div>
        {!list.length && <Empty />}
      </div>
      {menu && (
        <Popover at={menu} onClose={() => setMenu(null)} width={220}>
          <div className="menu wb-menu">
            {selected.length > 0 && (
              <>
                <MenuItem
                  icon={<Copy className="ico" />}
                  onClick={() => void copy()}
                >
                  {uiText("复制")}
                  <kbd>⌘/Ctrl C</kbd>
                </MenuItem>
                <MenuItem
                  icon={<Scissors className="ico" />}
                  onClick={() => void copy(true)}
                >
                  {uiText("剪切")}
                  <kbd>⌘/Ctrl X</kbd>
                </MenuItem>
                <MenuItem
                  icon={<Copy className="ico" />}
                  onClick={() => {
                    setSel(duplicate(selectedGraph(selected)));
                    setMenu(null);
                  }}
                >
                  {uiText("创建副本")}
                  <kbd>⌘/Ctrl D</kbd>
                </MenuItem>
              </>
            )}
            <MenuItem
              icon={<ClipboardPaste className="ico" />}
              onClick={() => void menuPaste()}
            >
              {uiText("粘贴")}
              <kbd>⌘/Ctrl V</kbd>
            </MenuItem>
            {selected.length > 0 && (
              <>
                <div className="menu-sep" />
                <MenuItem
                  icon={<ArrowUpToLine className="ico" />}
                  onClick={() => {
                    layer(selected, true);
                    setMenu(null);
                  }}
                >
                  {uiText("置于顶层")}
                </MenuItem>
                <MenuItem
                  icon={<ArrowDownToLine className="ico" />}
                  onClick={() => {
                    layer(selected, false);
                    setMenu(null);
                  }}
                >
                  {uiText("置于底层")}
                </MenuItem>
                <MenuItem
                  icon={<Trash2 className="ico" />}
                  onClick={() => {
                    remove(selected);
                    setSel([]);
                    setMenu(null);
                  }}
                  danger
                >
                  {uiText("删除")}
                </MenuItem>
              </>
            )}
            <div className="menu-sep" />
            <MenuItem
              onClick={() => {
                setSel(items.rows.map((r) => r.id));
                setMenu(null);
              }}
            >
              {uiText("全选")}
              <kbd>⌘/Ctrl A</kbd>
            </MenuItem>
          </div>
        </Popover>
      )}
      {help && (
        <Popover
          at={help}
          align="right"
          width={300}
          onClose={() => setHelp(null)}
        >
          <div className="wb-help">
            <h3>{uiText("让想法自然成形")}</h3>
            <p>
              {uiText("拖动框选，Shift 加选；拖出元素边缘的连接点即可连线。")}
            </p>
            <p>
              {uiText("⌘/Ctrl+C / X / V 复制、剪切、粘贴；⌘/Ctrl+D 创建副本。")}
            </p>
            <p>
              {uiText(
                "⌘/Ctrl+Z 撤销；Shift+⌘/Ctrl+Z 重做；方向键微调，Shift 移动 10 像素。",
              )}
            </p>
            <p>
              {uiText("空格或中键平移，⌘/Ctrl+滚轮缩放；手机双指缩放和平移。")}
            </p>
            <p>
              {uiText(
                "拖动时自动吸附对齐，按住 Alt 暂时关闭；Shift 调整大小保持比例。",
              )}
            </p>
          </div>
        </Popover>
      )}
      <Toast kit={kit} />
    </div>
  );
}
function sideLabel(side: Side) {
  return side === "left"
    ? uiText("左")
    : side === "right"
      ? uiText("右")
      : side === "top"
        ? uiText("上")
        : uiText("下");
}
function Text({
  value,
  editing,
  onDone,
  fontSize,
  align,
}: {
  value: string;
  editing: boolean;
  onDone: (v: string) => void;
  fontSize: number;
  align: string;
}) {
  const ref = useRef<HTMLDivElement>(null),
    before = useRef(value),
    cancelled = useRef(false);
  useEffect(() => {
    if (!editing || !ref.current) return;
    before.current = value;
    cancelled.current = false;
    const el = ref.current;
    el.innerText = value;
    el.focus();
    const range = document.createRange();
    range.selectNodeContents(el);
    range.collapse(false);
    const s = getSelection();
    s?.removeAllRanges();
    s?.addRange(range);
  }, [editing]);
  const style = {
    fontSize,
    textAlign: (["left", "center", "right"].includes(align)
      ? align
      : "left") as "left",
  };
  if (!editing)
    return (
      <div className="wb-text" style={style}>
        {value}
      </div>
    );
  return (
    <div
      ref={ref}
      className="wb-text is-editing"
      style={style}
      contentEditable={"plaintext-only" as unknown as boolean}
      suppressContentEditableWarning
      onPointerDown={(e) => e.stopPropagation()}
      onBlur={() =>
        onDone(
          cancelled.current
            ? before.current
            : (ref.current?.innerText || "").replace(/\n$/, ""),
        )
      }
      onKeyDown={(e) => {
        e.stopPropagation();
        if (
          e.key === "Escape" ||
          (e.key === "Enter" && (e.metaKey || e.ctrlKey))
        ) {
          e.preventDefault();
          cancelled.current = e.key === "Escape";
          e.currentTarget.blur();
        }
      }}
    />
  );
}
void mount(kit, App, async () => {
  if (!board.get("main"))
    await board.put({ title: "" }, "main")?.catch(() => board.load());
});
