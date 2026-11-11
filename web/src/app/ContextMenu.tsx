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

/** How many pointer units one CSS pixel of `left`/`top` covers. The text-size
 * setting zooms the whole page, and then a position given in pixels lands
 * further out than the pointer coordinate it was copied from; measuring
 * a probe gives the factor whatever the engine does with zoom. */
function styleScale(): number {
  const probe = document.createElement("div");
  probe.style.cssText = "position:fixed;left:100px;top:0;width:0;height:0;visibility:hidden;pointer-events:none";
  document.body.appendChild(probe);
  const k = probe.getBoundingClientRect().left / 100;
  probe.remove();
  return k > 0 ? k : 1;
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
    const k = styleScale();
    const height = Math.min(heightOf(items) * k, window.innerHeight - 16);
    setMenu({
      x: Math.max(4, Math.min(e.clientX, window.innerWidth - WIDTH * k - 8)) / k,
      y: Math.max(4, Math.min(e.clientY, window.innerHeight - height - 8)) / k,
      items,
    });
  }, []);

  const openUnder = useCallback((el: Element, items: MenuItem[]) => {
    const r = el.getBoundingClientRect();
    const k = styleScale();
    const height = Math.min(heightOf(items) * k, window.innerHeight - 16);
    setMenu({
      x: Math.max(4, Math.min(r.right - WIDTH * k, window.innerWidth - WIDTH * k - 8)) / k,
      y: (r.bottom + 4 + height > window.innerHeight ? Math.max(4, r.top - height - 4) : r.bottom + 4) / k,
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
