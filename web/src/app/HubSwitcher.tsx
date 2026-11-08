import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import {
  Check, ChevronDown, ChevronUp, Layers, Plus, Settings,
} from "lucide-react";
import type { Hub, HubOverview } from "@/appapi";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/** The switcher sits at the sidebar bottom ("sidebar") or, when the sidebar
 *  is collapsed, as a compact pill in the title bar ("pill"). */
type Variant = "sidebar" | "pill";

type Entry =
  | { key: string; kind: "all" }
  | { key: string; kind: "hub"; hub: Hub }
  | { key: string; kind: "add" }
  | { key: string; kind: "settings" };

function initial(name: string): string {
  const ch = Array.from(name.trim())[0] || "?";
  return /[a-z]/.test(ch) ? ch.toUpperCase() : ch;
}

/** Stable, soft colour per hub id. */
function colorIndex(id: string): number {
  let h = 0;
  for (let i = 0; i < id.length; i += 1) h = (h * 31 + id.charCodeAt(i)) >>> 0;
  return h % 8;
}

function Avatar({ name, id, all }: { name?: string; id?: string; all?: boolean }) {
  if (all) {
    return <span className="app-hub-avatar is-all"><Layers /></span>;
  }
  return (
    <span className={cn("app-hub-avatar", `c${colorIndex(id || name || "")}`)}>
      {initial(name || "")}
    </span>
  );
}

function nodeCount(hubsOv: HubOverview[] | undefined, id: string): number | undefined {
  const o = hubsOv?.find((h) => h.id === id)?.overview;
  return o ? o.agents.length : undefined;
}

/** Hub picker: all hubs, one hub, add another, or open Hub settings. */
export function HubSwitcher({ hubs, hubsOv, value, onChange, onAdd, onSettings, variant = "sidebar" }: {
  hubs: Hub[];
  hubsOv?: HubOverview[];
  /** "all" or a hub id. */
  value: string;
  onChange: (value: string) => void;
  onAdd: () => void;
  onSettings: () => void;
  variant?: Variant;
}) {
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);

  const selected = hubs.find((h) => h.id === value);
  const connected = hubs.filter((h) => h.ok).length;

  const entries = useMemo<Entry[]>(() => [
    { key: "all", kind: "all" },
    ...hubs.map((h) => ({ key: h.id, kind: "hub" as const, hub: h })),
    { key: "add", kind: "add" },
    { key: "settings", kind: "settings" },
  ], [hubs]);

  const selectedIndex = (): number => {
    if (value === "all") return 0;
    const i = entries.findIndex((e) => e.kind === "hub" && e.hub.id === value);
    return i >= 0 ? i : 0;
  };

  const close = (focusTrigger = false) => {
    setOpen(false);
    if (focusTrigger) triggerRef.current?.focus();
  };

  const pick = (next: string) => { onChange(next); close(); };

  const run = (i: number) => {
    const e = entries[i];
    if (!e) return;
    if (e.kind === "all") pick("all");
    else if (e.kind === "hub") pick(e.hub.id);
    else if (e.kind === "add") { close(); onAdd(); }
    else if (e.kind === "settings") { close(); onSettings(); }
  };

  const toggle = () => {
    if (open) { close(); return; }
    setActive(selectedIndex());
    setOpen(true);
  };

  useEffect(() => {
    if (!open) return;
    itemRefs.current[active]?.focus();
    // Focus the selected row when the menu opens; arrow keys move it after.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        close(true);
      }
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const onMenuKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const step = e.key === "ArrowDown" ? 1 : -1;
      const next = (active + step + entries.length) % entries.length;
      setActive(next);
      itemRefs.current[next]?.focus();
      return;
    }
    if (e.key === "Enter") {
      e.preventDefault();
      const focused = itemRefs.current.findIndex((el) => el === document.activeElement);
      run(focused >= 0 ? focused : active);
    }
  };

  const count = selected ? nodeCount(hubsOv, selected.id) : undefined;
  const sub = selected
    ? selected.ok
      ? L(
        `已连接${count === undefined ? "" : ` · ${count} 个节点`}`,
        `Connected${count === undefined ? "" : ` · ${count} node${count === 1 ? "" : "s"}`}`,
      )
      : L("连不上", "Unreachable")
    : L(`${hubs.length} 个 Hub · ${connected} 个已连接`, `${hubs.length} Hub${hubs.length === 1 ? "" : "s"} · ${connected} connected`);

  const dotClass = selected
    ? selected.ok ? "working" : "down"
    : connected === hubs.length ? "working" : connected === 0 ? "down" : "pending";

  const label = selected ? selected.name : L("全部 Hub", "All Hubs");

  return (
    <div className={cn("app-hub-switch", variant === "pill" && "is-pill")} ref={rootRef}>
      <button
        ref={triggerRef}
        type="button"
        className={cn(
          variant === "pill" ? "app-hub-pill" : "app-hub-trigger",
          open && "open",
        )}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={L(`切换 Hub：${label}`, `Switch Hub: ${label}`)}
        title={selected ? (selected.ok ? selected.url : L(`${selected.url}（连不上）`, `${selected.url} (unreachable)`)) : L("所有 Hub", "All Hubs")}
        onClick={toggle}
      >
        <Avatar
          name={selected?.name}
          id={selected?.id}
          all={!selected}
        />
        {variant === "pill" ? (
          <span className="app-hub-pill-name">{label}</span>
        ) : (
          <span className="app-hub-info">
            <span className="app-hub-name-row">
              <span className="app-hub-name">{label}</span>
              {open ? <ChevronDown className="app-hub-chevron" /> : <ChevronUp className="app-hub-chevron" />}
            </span>
            <span className="app-hub-sub">
              <span className={cn("dot", dotClass)} />
              <span className="app-hub-sub-text">{sub}</span>
            </span>
          </span>
        )}
      </button>

      {open && (
        <div
          className={cn("app-hub-menu", variant === "pill" ? "down" : "up")}
          role="menu"
          aria-label={L("选择 Hub", "Choose Hub")}
          onKeyDown={onMenuKeyDown}
        >
          {entries.map((e, i) => {
            if (e.kind === "all") {
              return (
                <button
                  key={e.key}
                  ref={(el) => { itemRefs.current[i] = el; }}
                  type="button"
                  role="menuitemradio"
                  aria-checked={value === "all"}
                  className={cn(
                    "app-hub-menu-item",
                    value === "all" && "active",
                    active === i && "is-active",
                  )}
                  onMouseEnter={() => setActive(i)}
                  onClick={() => pick("all")}
                >
                  <Avatar all />
                  <span className="app-hub-menu-name">{L("全部 Hub", "All Hubs")}</span>
                  {value === "all" && <Check className="app-hub-menu-check" />}
                </button>
              );
            }
            if (e.kind === "hub") {
              return (
                <button
                  key={e.key}
                  ref={(el) => { itemRefs.current[i] = el; }}
                  type="button"
                  role="menuitemradio"
                  aria-checked={value === e.hub.id}
                  className={cn(
                    "app-hub-menu-item",
                    value === e.hub.id && "active",
                    active === i && "is-active",
                  )}
                  onMouseEnter={() => setActive(i)}
                  onClick={() => pick(e.hub.id)}
                  title={e.hub.ok ? e.hub.url : L(`${e.hub.url}（连不上）`, `${e.hub.url} (unreachable)`)}
                >
                  <Avatar name={e.hub.name} id={e.hub.id} />
                  <span className="app-hub-menu-name">{e.hub.name}</span>
                  <span className={cn("dot", e.hub.ok ? "working" : "down")} />
                  {value === e.hub.id && <Check className="app-hub-menu-check" />}
                </button>
              );
            }
            const Icon = e.kind === "add" ? Plus : Settings;
            return (
              <Fragment key={e.key}>
                {e.kind === "add" && <div className="app-hub-menu-sep" />}
                <button
                  ref={(el) => { itemRefs.current[i] = el; }}
                  type="button"
                  role="menuitem"
                  className={cn("app-hub-menu-item", active === i && "is-active")}
                  onMouseEnter={() => setActive(i)}
                  onClick={() => run(i)}
                >
                  <span className="app-hub-menu-icon-wrap"><Icon /></span>
                  <span className="app-hub-menu-name">
                    {e.kind === "add" ? L("添加 Hub…", "Add Hub…") : L("Hub 设置…", "Hub settings…")}
                  </span>
                </button>
              </Fragment>
            );
          })}
        </div>
      )}
    </div>
  );
}
