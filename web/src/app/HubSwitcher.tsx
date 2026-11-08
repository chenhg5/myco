import { useEffect, useRef, useState } from "react";
import { Check, ChevronDown, Plus } from "lucide-react";
import type { Hub } from "@/appapi";
import { cn } from "@/lib/utils";

/** Workspace-style dropdown: all hubs, one hub, or add another. */
export function HubSwitcher({ hubs, value, onChange, onAdd }: {
  hubs: Hub[];
  /** "all" or a hub id. */
  value: string;
  onChange: (value: string) => void;
  onAdd: () => void;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const selected = hubs.find((h) => h.id === value);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setOpen(false); };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const pick = (next: string) => { onChange(next); setOpen(false); };

  const allOnline = hubs.length > 0 && hubs.every((h) => h.ok);
  const dot = selected ? (selected.ok ? "working" : "offline") : allOnline ? "working" : "offline";

  return (
    <div className="app-hub-switch" ref={ref}>
      <button
        type="button"
        className={cn("app-hub-btn", open && "open")}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        <span className={cn("dot", dot)} />
        <span className="app-hub-label">{selected ? selected.name : "全部 Hub"}</span>
        <ChevronDown className="app-hub-chev" />
      </button>

      {open && (
        <div className="app-hub-menu" role="menu" aria-label="选择 Hub">
          <button
            type="button"
            role="menuitemradio"
            aria-checked={value === "all"}
            className={cn("app-hub-menu-item", value === "all" && "active")}
            onClick={() => pick("all")}
          >
            <span className="app-hub-menu-name">全部 Hub</span>
            <span className="app-hub-menu-count">{hubs.length}</span>
            {value === "all" && <Check className="app-hub-menu-check" />}
          </button>
          {hubs.map((h) => (
            <button
              key={h.id}
              type="button"
              role="menuitemradio"
              aria-checked={value === h.id}
              className={cn("app-hub-menu-item", value === h.id && "active")}
              onClick={() => pick(h.id)}
              title={h.ok ? h.url : `${h.url}（连不上）`}
            >
              <span className={cn("dot", h.ok ? "working" : "offline")} />
              <span className="app-hub-menu-name">{h.name}</span>
              {value === h.id && <Check className="app-hub-menu-check" />}
            </button>
          ))}
          <div className="app-hub-menu-sep" />
          <button
            type="button"
            role="menuitem"
            className="app-hub-menu-item"
            onClick={() => { setOpen(false); onAdd(); }}
          >
            <Plus className="app-hub-menu-icon" />
            <span className="app-hub-menu-name">添加 Hub…</span>
          </button>
        </div>
      )}
    </div>
  );
}
