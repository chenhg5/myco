import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import type { CSSProperties } from "react";
import { Check, Palette, Pipette } from "lucide-react";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { PRESETS } from "@/app/chatBg";

/*
 * 专注模式's background (AppMode): the theme's own, a quiet tone (a light and
 * a dark shade each) or one of the chat backgrounds' gradients and patterns.
 * It covers the whole page, or only what is around the conversation (which
 * then keeps the theme's own, like a sheet on a desk). Kept on this
 * computer, in localStorage.
 */

export type FocusScope = "all" | "outside";
/** custom: the colour you picked yourself (id "custom"), one for each theme:
 *  the picker sets the one for the theme you are in. */
export type FocusBgChoice = { id: string; scope: FocusScope; custom?: { light?: string; dark?: string } };
type Tone = { id: string; name: () => string; light: string; dark: string; solid: boolean };

/** The tones: solid colours first (the words stay readable on them in either
 *  theme), then the chat backgrounds' gradients and patterns. The dark
 *  shades are a little lighter than the dark theme's own (#1b1b1f): a tint,
 *  not a deeper dark, which only felt heavy (all but 高对比). */
export const FOCUS_TONES: Tone[] = [
  { id: "paper", name: () => L("纸张", "Paper"), light: "#f5f0e6", dark: "#2e2a24", solid: true },
  { id: "sand", name: () => L("沙色", "Sand"), light: "#f5eddf", dark: "#322a20", solid: true },
  { id: "rose", name: () => L("樱粉", "Rose"), light: "#f8ecee", dark: "#32262b", solid: true },
  { id: "dusk", name: () => L("暮紫", "Dusk"), light: "#f1ecf4", dark: "#2b2633", solid: true },
  { id: "mist", name: () => L("雾蓝", "Mist"), light: "#eaeff5", dark: "#252c38", solid: true },
  { id: "sky", name: () => L("天青", "Sky"), light: "#e4f0f7", dark: "#22303b", solid: true },
  { id: "celadon", name: () => L("青瓷", "Celadon"), light: "#e4f2ed", dark: "#223029", solid: true },
  { id: "sage", name: () => L("护眼绿", "Sage"), light: "#ebf1e7", dark: "#29302a", solid: true },
  { id: "stone", name: () => L("石墨", "Stone"), light: "#ececef", dark: "#2b2c31", solid: true },
  { id: "contrast", name: () => L("高对比", "High contrast"), light: "#ffffff", dark: "#111113", solid: true },
  ...PRESETS.map((p) => ({ id: `chat-${p.id}`, name: p.name, light: p.css, dark: p.css, solid: false })),
];

const KEY = "myco-focus-bg";
const CUSTOM = "custom";
// The theme's own colours (app.css), for a theme you picked no colour in.
const THEME = { light: "#f8f8fa", dark: "#1b1b1f" };
const HEX = /^#[0-9a-f]{6}$/i;
const isDark = () => document.documentElement.classList.contains("dark");

/** How light a colour is (WCAG relative luminance, 0..1). */
function luminance(hex: string): number {
  const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
  return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
}
/** The theme's words stay readable on it: dark words on a light colour in
 *  the light theme, light words on a dark one in the dark theme. */
export function readable(hex: string, dark: boolean): boolean {
  const l = luminance(hex);
  return dark ? l < 0.2 : l > 0.4;
}

function read(): FocusBgChoice {
  let raw = "";
  try { raw = localStorage.getItem(KEY) || ""; } catch { /* not kept */ }
  let v: Partial<FocusBgChoice> = {};
  try { v = raw.startsWith("{") ? JSON.parse(raw) as Partial<FocusBgChoice> : { id: raw }; } catch { /* as none */ }
  const custom = {
    ...(HEX.test(v.custom?.light || "") ? { light: v.custom!.light } : {}),
    ...(HEX.test(v.custom?.dark || "") ? { dark: v.custom!.dark } : {}),
  };
  return {
    id: v.id === CUSTOM || FOCUS_TONES.some((t) => t.id === v.id) ? v.id! : "",
    scope: v.scope === "outside" ? "outside" : "all",
    ...(custom.light || custom.dark ? { custom } : {}),
  };
}

/** The choice, and a setter that keeps it. */
export function useFocusBg(): [FocusBgChoice, (c: FocusBgChoice) => void] {
  const [c, setC] = useState<FocusBgChoice>(read);
  const set = (next: FocusBgChoice) => {
    setC(next);
    try { localStorage.setItem(KEY, JSON.stringify(next)); } catch { /* not kept */ }
  };
  return [c, set];
}

/** What the app shell carries for a choice (app.css draws it). */
export function focusBgProps(c: FocusBgChoice): { attrs: Record<string, string | undefined>; style: CSSProperties } {
  const t = c.id === CUSTOM
    ? { id: CUSTOM, light: c.custom?.light || THEME.light, dark: c.custom?.dark || THEME.dark, solid: true }
    : FOCUS_TONES.find((x) => x.id === c.id);
  if (!t) return { attrs: {}, style: {} };
  return {
    attrs: {
      "data-focus-bg": t.id,
      "data-focus-kind": t.solid ? "solid" : "pattern",
      "data-focus-scope": c.scope,
    },
    style: { ["--focus-bg-l" as string]: t.light, ["--focus-bg-d" as string]: t.dark },
  };
}

/** The palette button at the top right, and its panel: where the background
 *  goes, and the tones as swatches. Barely there until pointed at. */
export function FocusBgButton({ value, onChange }: { value: FocusBgChoice; onChange: (c: FocusBgChoice) => void }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const popRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const down = (e: MouseEvent) => {
      const t = e.target as Node;
      if (!ref.current?.contains(t) && !popRef.current?.contains(t)) setOpen(false);
    };
    // Esc closes the panel only (not 专注模式: AppMode leaves it alone while
    // .focus-bg-pop is open).
    const key = (e: KeyboardEvent) => { if (e.key === "Escape") { e.preventDefault(); setOpen(false); } };
    window.addEventListener("mousedown", down);
    window.addEventListener("keydown", key);
    return () => { window.removeEventListener("mousedown", down); window.removeEventListener("keydown", key); };
  }, [open]);
  // The colour you picked for the theme you are in, if any.
  const dark = open && isDark();
  const mine = value.custom?.[dark ? "dark" : "light"];
  const pick = (hex: string) => {
    if (!HEX.test(hex)) return;
    onChange({ ...value, id: CUSTOM, custom: { ...value.custom, [isDark() ? "dark" : "light"]: hex.toLowerCase() } });
  };
  const tile = (id: string, name: string, sw?: { light: string; dark: string }) => (
    <button key={id || "theme"} type="button" role="radio" aria-checked={value.id === id}
      className={cn("focus-bg-tile", value.id === id && "is-on", !sw && "is-theme")} title={name}
      onClick={() => onChange({ ...value, id })}>
      <span className="focus-bg-sw" style={sw ? { ["--sw-l" as string]: sw.light, ["--sw-d" as string]: sw.dark } : undefined}>
        {value.id === id && <Check aria-hidden />}
      </span>
      <span className="focus-bg-name">{name}</span>
    </button>
  );
  return (
    <div className="focus-bg" ref={ref}>
      <button type="button" className={cn("focus-tone", open && "is-open")} aria-expanded={open}
        title={L("专注背景", "Focus background")} aria-label={L("换专注模式的背景", "Change the focus background")}
        onClick={() => setOpen((o) => !o)}>
        <Palette />
      </button>
      {/* On the page's own layer (.app-portal), so the panel and its
          swatches keep the theme's colours whatever the background is. */}
      {open && createPortal(
        <div className="app-portal focus-bg-pop" ref={popRef} role="dialog" aria-label={L("专注背景", "Focus background")}
          style={{ ["--wails-draggable" as string]: "no-drag" }}>
          <div className="focus-bg-head">
            <span>{L("专注背景", "Focus background")}</span>
            <div className="seg seg-sm" role="radiogroup" aria-label={L("换哪里", "Where")}>
              {([["all", L("整个页面", "Whole page")], ["outside", L("只改外面", "Outside only")]] as const).map(([s, label]) => (
                <button key={s} type="button" role="radio" aria-checked={value.scope === s}
                  className={cn("seg-btn", value.scope === s && "active")}
                  title={s === "outside" ? L("对话保持原来的底色，只换它外面", "The chat keeps its own colour; only around it changes") : undefined}
                  onClick={() => onChange({ ...value, scope: s })}>{label}</button>
              ))}
            </div>
          </div>
          <div className="focus-bg-grid" role="radiogroup" aria-label={L("背景", "Background")}>
            {tile("", L("跟随主题", "Theme"))}
            {FOCUS_TONES.map((t) => tile(t.id, t.name(), t))}
            {/* Your own colour: the system's colour picker, for the theme you are in. */}
            <label className={cn("focus-bg-tile", value.id === CUSTOM && "is-on", !mine && "is-pick")}
              title={L("自己挑一个颜色", "Pick a colour of your own")}>
              <span className="focus-bg-sw" style={mine ? { ["--sw-l" as string]: mine, ["--sw-d" as string]: mine } : undefined}>
                {value.id === CUSTOM ? <Check aria-hidden /> : <Pipette aria-hidden />}
              </span>
              <span className="focus-bg-name">{L("自定义", "Custom")}</span>
              <input type="color" className="focus-bg-input" aria-label={L("自定义颜色", "Custom colour")}
                value={mine || (dark ? "#2b2633" : "#f1ecf4")}
                onChange={(e) => pick(e.currentTarget.value)}
                onClick={(e) => { if (mine && value.id !== CUSTOM) { e.preventDefault(); onChange({ ...value, id: CUSTOM }); } }} />
            </label>
          </div>
          {value.id === CUSTOM && mine && value.scope === "all" && !readable(mine, dark) && (
            <p className="focus-bg-hint">
              {L("这个颜色上字不太好认。", "Words are hard to read on this colour.")}
              <button type="button" className="app-kv-link" onClick={() => onChange({ ...value, scope: "outside" })}>{L("改成只改外面", "Use it outside only")}</button>
            </p>
          )}
        </div>,
        document.body,
      )}
    </div>
  );
}
