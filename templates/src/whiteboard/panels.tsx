// Floating tools and contextual formatting stay out of the drawing surface.
// Commands are supplied by the composition; shared record writes live in edit.
import {
  ArrowUpRight,
  Circle,
  Copy,
  CopyPlus,
  Hand,
  LockKeyhole,
  MousePointer2,
  Square,
  StickyNote,
  Trash2,
  Type,
  UnlockKeyhole,
} from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { colors } from "./export";
import { readClipboard } from "./clipboard";
import type { Item, Kind } from "./geometry";
export type Tool = "select" | "hand" | Kind;
export const tools: [Tool, string, typeof Hand, string][] = [
  ["select", uiText("选择"), MousePointer2, "V"],
  ["hand", uiText("拖动画布"), Hand, "H"],
  ["sticky", uiText("便签"), StickyNote, "N"],
  ["text", uiText("文字"), Type, "T"],
  ["rect", uiText("矩形"), Square, "R"],
  ["ellipse", uiText("椭圆"), Circle, "O"],
  ["arrow", uiText("连线"), ArrowUpRight, "L"],
];
export function Tools({
  tool,
  setTool,
}: {
  tool: Tool;
  setTool: (v: Tool) => void;
}) {
  return (
    <div
      className="wb-tools wb-chrome"
      role="toolbar"
      aria-label={uiText("绘图工具")}
      onPointerDown={(e) => e.stopPropagation()}
    >
      {tools.map(([key, name, Icon, shortcut]) => (
        <button
          key={key}
          type="button"
          className={tool === key ? "is-on" : ""}
          title={`${name} (${shortcut})`}
          aria-label={name}
          aria-pressed={tool === key}
          onClick={() => setTool(key)}
        >
          <Icon className="ico" />
        </button>
      ))}
    </div>
  );
}
const colorLabels: Record<string, string> = {
  gray: uiText("中性灰"),
  yellow: uiText("奶油黄"),
  pink: uiText("雾玫瑰"),
  blue: uiText("雾蓝"),
  green: uiText("鼠尾草"),
  purple: uiText("浅紫"),
  orange: uiText("杏色"),
};
export function Properties({
  count,
  data,
  dark,
  onPatch,
  onAlign,
  onDistribute,
  onDuplicate,
  onCopy,
  onDelete,
}: {
  count: number;
  data: Item;
  dark: boolean;
  onPatch: (v: Item) => void;
  onAlign: (v: string) => void;
  onDistribute: (v: "x" | "y") => void;
  onDuplicate: () => void;
  onCopy: () => void;
  onDelete: () => void;
}) {
  const arrow = data.type === "arrow",
    locked = !!data.locked;
  return (
    <div
      className="wb-props wb-chrome"
      role="toolbar"
      aria-label={uiText("选择样式")}
      onPointerDown={(e) => e.stopPropagation()}
    >
      {count > 1 && (
        <span className="wb-count">{uiText("已选 {0} 项", count)}</span>
      )}
      {!locked && (
        <>
          <div className="wb-palette" role="group" aria-label={uiText("颜色")}>
            {Object.keys(colors).map((c) => (
              <button
                type="button"
                key={c}
                className={`wb-swatch${(data.color || "gray") === c ? " is-on" : ""}`}
                aria-label={colorLabels[c]}
                aria-pressed={(data.color || "gray") === c}
                style={{ background: colors[c][dark ? 1 : 0] }}
                onClick={() => onPatch({ color: c })}
              />
            ))}
          </div>
          <span className="wb-sep" />
          {arrow ? (
            <>
              <select
                className="sel wb-route"
                aria-label={uiText("连线类型")}
                value={data.route || "elbow"}
                onChange={(e) => onPatch({ route: e.target.value })}
              >
                <option value="elbow">{uiText("折线连接")}</option>
                <option value="curve">{uiText("曲线")}</option>
                <option value="straight">{uiText("直线")}</option>
              </select>
              <select
                className="sel"
                aria-label={uiText("线条样式")}
                value={data.dash || "solid"}
                onChange={(e) => onPatch({ dash: e.target.value })}
              >
                <option value="solid">{uiText("实线")}</option>
                <option value="dashed">{uiText("虚线")}</option>
              </select>
              <input
                className="inp wb-link-label"
                aria-label={uiText("连线文字")}
                placeholder={uiText("添加说明")}
                defaultValue={String(data.text || "")}
                key={String(data.text)}
                maxLength={120}
                onBlur={(e) => {
                  if (e.target.value !== (data.text || ""))
                    onPatch({ text: e.target.value });
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter") e.currentTarget.blur();
                }}
              />
            </>
          ) : (
            <>
              <select
                className="sel wb-font"
                aria-label={uiText("字号")}
                value={data.font_size || (data.type === "text" ? 24 : 16)}
                onChange={(e) => onPatch({ font_size: Number(e.target.value) })}
              >
                {[12, 14, 16, 20, 24, 32, 48, 64].map((n) => (
                  <option key={n}>{n}</option>
                ))}
              </select>
              <select
                className="sel wb-text-align"
                aria-label={uiText("文字对齐")}
                value={
                  data.align ||
                  (data.type === "rect" || data.type === "ellipse"
                    ? "center"
                    : "left")
                }
                onChange={(e) => onPatch({ align: e.target.value })}
              >
                <option value="left">{uiText("左对齐")}</option>
                <option value="center">{uiText("居中")}</option>
                <option value="right">{uiText("右对齐")}</option>
              </select>
            </>
          )}
          {count > 1 && (
            <select
              className="sel"
              aria-label={uiText("排列元素")}
              value=""
              onChange={(e) => {
                if (e.target.value === "x" || e.target.value === "y")
                  onDistribute(e.target.value);
                else onAlign(e.target.value);
              }}
            >
              <option value="">{uiText("排列")}</option>
              <option value="left">{uiText("左对齐")}</option>
              <option value="center">{uiText("水平居中")}</option>
              <option value="right">{uiText("右对齐")}</option>
              <option value="top">{uiText("顶端对齐")}</option>
              <option value="middle">{uiText("垂直居中")}</option>
              <option value="bottom">{uiText("底端对齐")}</option>
              <option value="x">{uiText("水平等距")}</option>
              <option value="y">{uiText("垂直等距")}</option>
            </select>
          )}
          <button
            className="icon-btn"
            type="button"
            aria-label={uiText("复制")}
            title={uiText("复制")}
            onClick={onCopy}
          >
            <Copy className="ico" />
          </button>
          <button
            className="icon-btn"
            type="button"
            title={uiText("创建副本（⌘/Ctrl+D）")}
            aria-label={uiText("创建副本")}
            onClick={onDuplicate}
          >
            <CopyPlus className="ico" />
          </button>
        </>
      )}
      <button
        className="icon-btn"
        type="button"
        title={locked ? uiText("解锁") : uiText("锁定")}
        aria-label={locked ? uiText("解锁") : uiText("锁定")}
        onClick={() => onPatch({ locked: !locked })}
      >
        {locked ? (
          <UnlockKeyhole className="ico" />
        ) : (
          <LockKeyhole className="ico" />
        )}
      </button>
      {!locked && (
        <button
          className="icon-btn"
          type="button"
          title={uiText("删除（Delete）")}
          aria-label={uiText("删除")}
          onClick={onDelete}
        >
          <Trash2 className="ico" />
        </button>
      )}
    </div>
  );
}
export function Empty() {
  return (
    <div className="wb-empty">
      <div className="wb-empty-symbol">
        <StickyNote />
      </div>
      <h2>{uiText("从一个想法开始")}</h2>
      <p>{uiText("双击贴便签，拖出连接点画流程。")}</p>
      <span>{uiText("也可以粘贴文字，或让 agent 帮你整理想法。")}</span>
    </div>
  );
}
export function PastePrompt({
  onPaste,
  onClose,
}: {
  onPaste: (value: ReturnType<typeof readClipboard>) => void;
  onClose: () => void;
}) {
  return (
    <div
      className="wb-paste-prompt wb-chrome"
      role="dialog"
      aria-label={uiText("粘贴到白板")}
      onPointerDown={(e) => e.stopPropagation()}
    >
      <strong>{uiText("粘贴到白板")}</strong>
      <p>{uiText("在下方长按并选择「粘贴」，即可插入复制的元素或文字。")}</p>
      <textarea
        className="inp"
        autoFocus
        aria-label={uiText("在此使用系统粘贴")}
        placeholder={uiText("在此使用系统粘贴")}
        onPaste={(e) => {
          e.preventDefault();
          onPaste(readClipboard(e.clipboardData));
        }}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.preventDefault();
            onClose();
          }
        }}
      />
      <button className="btn" type="button" onClick={onClose}>
        {uiText("关闭")}
      </button>
    </div>
  );
}
