import { useCallback, useEffect, useState } from "react";
import type { MouseEvent, ReactNode } from "react";
import { createPortal } from "react-dom";
import { Check } from "lucide-react";
import type { IconType } from "@/components/icons";
import { cn } from "@/lib/utils";

/** One line of a menu, "sep" for a divider, or a small heading over a group
 * of choices. A choice that is currently on carries `checked`. */
export type MenuItem =
  | { label: string; icon?: IconType; onSelect: () => void; danger?: boolean; disabled?: boolean; checked?: boolean }
  | { header: string }
  | "sep";

const WIDTH = 190;
const ROW = 29;
const heightOf = (items: MenuItem[]) => items.reduce((h, it) => h + (it === "sep" ? 9 : "header" in it ? 24 : ROW), 10);

/** The text-size setting zooms the whole page. Engines then disagree on which
 * numbers are zoomed: a `left` of N pixels, a pointer event's clientX, and a
 * client rect may be in two different units, and which two differs between
 * Chromium and WebKit. So nothing is assumed: one probe is placed at a known
 * `left`, then asked where its rect is, and hit-tested the way a pointer
 * would hit it. That gives how many units of each kind one pixel of
 * `left`/`top` covers. */
let measured: { zoom: number; ev: number; rect: number } | null = null;
function scales(): { zoom: number; ev: number; rect: number } {
  const z = parseFloat(getComputedStyle(document.documentElement).zoom) || 1;
  if (measured && measured.zoom === z) return measured;
  if (Math.abs(z - 1) < 0.001) return (measured = { zoom: z, ev: 1, rect: 1 });
  const at = 200, size = 4;
  const probe = document.createElement("div");
  probe.style.cssText = `position:fixed;left:${at}px;top:${at}px;width:${size}px;height:${size}px;opacity:0;z-index:2147483647`;
  document.body.appendChild(probe);
  const rect = probe.getBoundingClientRect().left / at || 1;
  const hit = (k: number) => document.elementFromPoint((at + size / 2) * k, (at + size / 2) * k) === probe;
  const ev = hit(z) ? z : hit(1) ? 1 : z;
  probe.remove();
  return (measured = { zoom: z, ev, rect });
}

/** A right-click menu for a row: call open(event, items) from onContextMenu
 * and render `element` once anywhere in the page. It closes on a click
 * elsewhere, Escape, scrolling, resizing or the window losing focus. */
export function useContextMenu(): {
  open: (e: MouseEvent, items: MenuItem[]) => void;
  /** The same menu dropped under a button, right edges aligned. */
  openUnder: (el: Element, items: MenuItem[]) => void;
  element: ReactNode;
} {
  const [menu, setMenu] = useState<{ x: number; y: number; items: MenuItem[] } | null>(null);

  useEffect(() => {
    if (!menu) return;
    const close = () => setMenu(null);
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") close(); };
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", onKey);
    window.addEventListener("blur", close);
    window.addEventListener("resize", close);
    document.addEventListener("scroll", close, true);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("blur", close);
      window.removeEventListener("resize", close);
      document.removeEventListener("scroll", close, true);
    };
  }, [menu]);

  const open = useCallback((e: MouseEvent, items: MenuItem[]) => {
    e.preventDefault();
    if (items.length === 0) return;
    // Everything below is in pixels of `left`/`top`.
    const k = scales();
    const vw = window.innerWidth / k.zoom, vh = window.innerHeight / k.zoom;
    const height = Math.min(heightOf(items), vh - 16);
    setMenu({
      x: Math.max(4, Math.min(e.clientX / k.ev, vw - WIDTH - 8)),
      y: Math.max(4, Math.min(e.clientY / k.ev, vh - height - 8)),
      items,
    });
  }, []);

  const openUnder = useCallback((el: Element, items: MenuItem[]) => {
    const k = scales();
    const box = el.getBoundingClientRect();
    const r = { top: box.top / k.rect, right: box.right / k.rect, bottom: box.bottom / k.rect };
    const vw = window.innerWidth / k.zoom, vh = window.innerHeight / k.zoom;
    const height = Math.min(heightOf(items), vh - 16);
    setMenu({
      x: Math.max(4, Math.min(r.right - WIDTH, vw - WIDTH - 8)),
      y: r.bottom + 4 + height > vh ? Math.max(4, r.top - height - 4) : r.bottom + 4,
      items,
    });
  }, []);

  // Up and down move through the items, as in a native menu.
  const onKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
    e.preventDefault();
    const buttons = [...e.currentTarget.querySelectorAll<HTMLButtonElement>("button:not(:disabled)")];
    const at = buttons.indexOf(document.activeElement as HTMLButtonElement);
    const next = e.key === "ArrowDown" ? (at + 1) % buttons.length : (at - 1 + buttons.length) % buttons.length;
    buttons[next]?.focus();
  };

  const element = menu
    ? createPortal(
      <div
        className="app-portal conv-menu"
        role="menu"
        style={{ left: menu.x, top: menu.y, minWidth: WIDTH }}
        onMouseDown={(e) => e.stopPropagation()}
        onContextMenu={(e) => e.preventDefault()}
        onKeyDown={onKeyDown}
      >
        {menu.items.map((it, i) => it === "sep" ? (
          <div key={i} className="conv-menu-sep" role="separator" />
        ) : "header" in it ? (
          <div key={i} className="conv-menu-header">{it.header}</div>
        ) : (
          <button
            key={i}
            type="button"
            role="menuitem"
            className={cn("conv-menu-item", it.danger && "danger")}
            autoFocus={i === menu.items.findIndex((x) => x !== "sep" && !("header" in x))}
            disabled={it.disabled}
            onClick={() => { setMenu(null); it.onSelect(); }}
          >
            {it.checked !== undefined ? (it.checked ? <Check /> : <span className="conv-menu-noicon" />)
              : it.icon ? <it.icon /> : <span className="conv-menu-noicon" />}
            <span className="conv-menu-label">{it.label}</span>
          </button>
        ))}
      </div>,
      document.body,
    )
    : null;

  return { open, openUnder, element };
}
