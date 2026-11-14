import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import {
  Check, ChevronDown, ChevronUp, Laptop, Plus, Server, Settings,
} from "lucide-react";
import type { Hub, HubOverview } from "@/appapi";
import { useHubLooks, type HubLook } from "@/app/hubLooks";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/** The switcher sits at the sidebar bottom ("sidebar") or, when the sidebar
 *  is collapsed, as a compact pill in the title bar ("pill"). */
type Variant = "sidebar" | "pill";

type Entry =
  | { key: string; kind: "hub"; hub: Hub }
  | { key: string; kind: "add" }
  | { key: string; kind: "newLocal" }
  | { key: string; kind: "newOnServer" }
  | { key: string; kind: "settings" };

function initial(name: string): string {
  const ch = Array.from(name.trim())[0] || "?";
  return /[a-z]/.test(ch) ? ch.toUpperCase() : ch;
}

/** Stable, soft colour per hub id. */
export function colorIndex(id: string): number {
  let h = 0;
  for (let i = 0; i < id.length; i += 1) h = (h * 31 + id.charCodeAt(i)) >>> 0;
  return h % 8;
}

/** A workspace's face: the picture or colour chosen in this App, else a
 *  colour from its id; the first letter of its name on the colour. */
export function HubFace({ name, id, look, size }: { name?: string; id?: string; look?: HubLook; size?: number }) {
  const looks = useHubLooks();
  const l = look ?? (id ? looks[id] : undefined);
  const box = size ? { width: size, height: size, fontSize: Math.round(size * 0.43), borderRadius: Math.round(size * 0.29) } : undefined;
  if (l?.pic) {
    return <span className="app-hub-avatar has-pic" style={box}><img src={l.pic} alt="" draggable={false} /></span>;
  }
  return (
    <span className={cn("app-hub-avatar", `c${l?.color ?? colorIndex(id || name || "")}`)} style={box}>
      {initial(name || "")}
    </span>
  );
}

const Avatar = HubFace;

function nodeCount(hubsOv: HubOverview[] | undefined, id: string): number | undefined {
  const o = hubsOv?.find((h) => h.id === id)?.overview;
  return o ? o.agents.length : undefined;
}

/** Workspace picker: one at a time (they are kept apart), add another, or
 *  open their settings. */
export function HubSwitcher({ hubs, hubsOv, value, onChange, onAdd, onNewLocal, onNewOnServer, serverName, onSettings, isLocal, variant = "sidebar", down = false }: {
  hubs: Hub[];
  hubsOv?: HubOverview[];
  /** the workspace on show */
  value: string;
  onChange: (value: string) => void;
  onAdd: () => void;
  /** starts another workspace on this computer */
  onNewLocal: () => void;
  /** adds a workspace on the server of the one on show (when this App
   *  may: it owns that server); serverName names the server */
  onNewOnServer?: () => void;
  serverName?: string;
  onSettings: () => void;
  /** whether a hub runs on this computer (it gets a tag) */
  isLocal?: (hub: Hub) => boolean;
  variant?: Variant;
  /** at the top of the sidebar: the menu opens downwards */
  down?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);

  const selected = hubs.find((h) => h.id === value);
  const connected = hubs.filter((h) => h.ok).length;

  const entries = useMemo<Entry[]>(() => [
    ...hubs.map((h) => ({ key: h.id, kind: "hub" as const, hub: h })),
    { key: "add", kind: "add" },
    ...(onNewOnServer ? [{ key: "newOnServer", kind: "newOnServer" as const }] : []),
    { key: "newLocal", kind: "newLocal" },
    { key: "settings", kind: "settings" },
  ], [hubs, onNewOnServer]);

  const selectedIndex = (): number => {
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
    if (e.kind === "hub") pick(e.hub.id);
    else if (e.kind === "add") { close(); onAdd(); }
    else if (e.kind === "newLocal") { close(); onNewLocal(); }
    else if (e.kind === "newOnServer") { close(); onNewOnServer?.(); }
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
        `已连上${count === undefined ? "" : ` · ${count} 个成员`}`,
        `Connected${count === undefined ? "" : ` · ${count} node${count === 1 ? "" : "s"}`}`,
      )
      : L("连不上", "Unreachable")
    : L(`${hubs.length} 个工作区 · ${connected} 个已连上`, `${hubs.length} workspace${hubs.length === 1 ? "" : "s"} · ${connected} connected`);

  const dotClass = selected
    ? selected.ok ? "working" : "down"
    : connected === hubs.length ? "working" : connected === 0 ? "down" : "pending";

  const label = selected?.name || "";

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
        aria-label={L(`切换工作区：${label}`, `Switch workspace: ${label}`)}
        title={selected ? (selected.ok ? selected.url : L(`${selected.url}（连不上）`, `${selected.url} (unreachable)`)) : undefined}
        onClick={toggle}
      >
        <Avatar name={selected?.name} id={selected?.id} />
        {variant === "pill" ? (
          <span className="app-hub-pill-name">{label}</span>
        ) : (
          <span className="app-hub-info">
            <span className="app-hub-name-row">
              <span className="app-hub-name">{label}</span>
              {open !== down ? <ChevronDown className="app-hub-chevron" /> : <ChevronUp className="app-hub-chevron" />}
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
          className={cn("app-hub-menu", variant === "pill" || down ? "down" : "up")}
          role="menu"
          aria-label={L("切换工作区", "Switch workspace")}
          onKeyDown={onMenuKeyDown}
        >
          {entries.map((e, i) => {
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
                  {isLocal?.(e.hub) && <span className="app-hub-menu-tag">{L("这台电脑", "This computer")}</span>}
                  <span className={cn("dot", e.hub.ok ? "working" : "down")} />
                  {value === e.hub.id && <Check className="app-hub-menu-check" />}
                </button>
              );
            }
            const Icon = e.kind === "add" ? Plus : e.kind === "newLocal" ? Laptop : e.kind === "newOnServer" ? Server : Settings;
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
                    {e.kind === "add"
                      ? L("加入或新建工作区", "Join or start a workspace")
                      : e.kind === "newLocal"
                        ? L("在这台电脑上新建工作区", "New workspace on this computer")
                        : e.kind === "newOnServer"
                          ? (serverName ? L(`在 ${serverName} 上新建工作区`, `New workspace on ${serverName}`) : L("在这台服务器上新建工作区", "New workspace on this server"))
                          : L("设置", "Settings")}
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
