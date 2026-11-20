import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import type { AgentRow } from "@/app/types";
import { Avatar } from "@/app/persona";
import { nodeName } from "@/app/nodes";
import { recentChats } from "@/app/recentChats";
import { isMacKeys } from "@/app/Shortcuts";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * Switching agents as ⌘Tab switches apps: hold ⌃ (Ctrl) and press Tab, a
 * row of large faces comes up, the ones you talked to last first, the one
 * before this one picked; Tab again moves on (⇧ back, ← → too), letting go
 * of ⌃ opens its chat; Esc or a click outside puts it away. ⌃Tab is free
 * on the Mac and on Windows (theirs are ⌘Tab and Alt+Tab); a browser keeps
 * Ctrl+Tab for its tabs, so ⌃` (the key above Tab) does the same.
 */

/** The key held to switch: ⌃ (Ctrl) with Tab or `; on a Mac ⌥ too, since
 *  the Mac takes ⌃← and ⌃→ for its desktops and ← → are wanted here. */
function switchKey(e: KeyboardEvent): "Control" | "Alt" | null {
  if (e.metaKey || !(e.key === "Tab" || e.code === "Backquote")) return null;
  if (e.ctrlKey && !e.altKey) return "Control";
  if (e.altKey && !e.ctrlKey && isMacKeys()) return "Alt";
  return null;
}

export function AgentSwitcher({ hubId, rows, current, project, onPick }: {
  hubId: string;
  /** the agents to switch between (this workspace's chats) */
  rows: AgentRow[];
  /** the agent whose chat is open, if any */
  current?: string;
  /** the members of the project shown, if one is */
  project?: string[];
  onPick: (r: AgentRow) => void;
}) {
  const [open, setOpen] = useState(false);
  const [at, setAt] = useState(0);
  const order = useRef<AgentRow[]>([]);
  const state = useRef({ open: false, at: 0 });
  const opener = useRef<"Control" | "Alt">("Control");
  state.current = { open, at };
  const pickRef = useRef(onPick);
  pickRef.current = onPick;
  const rowsRef = useRef({ rows, current, hubId, project });
  rowsRef.current = { rows, current, hubId, project };
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // Last talked to first (this workspace), then the rest as listed; the
    // open chat first of all, so one press goes to the one before it.
    // The chat open now: the one AppMode opened, or else (one opened from a
    // project's members) the last one noted while a chat shows.
    const currentNow = () => rowsRef.current.current
      || (document.querySelector(".conv-detail") ? recentChats().find((c) => c.hubId === rowsRef.current.hubId)?.agent : undefined);
    // Who comes first: the open chat (one press goes past it), then this
    // project's members at work, its other members, those at work
    // elsewhere, the rest; within each, the last talked to first. One that
    // is offline is left out (there is nothing to switch to).
    const build = () => {
      const { rows, hubId, project } = rowsRef.current;
      const current = currentNow();
      const seen = recentChats().filter((c) => c.hubId === hubId).map((c) => c.agent);
      const inProject = new Set(project || []);
      const busy = (r: AgentRow) => r.st.state === "working" || (r.st.asking || 0) > 0;
      const group = (r: AgentRow) => (r.st.agent.name === current ? 0 : inProject.has(r.st.agent.name) ? (busy(r) ? 1 : 2) : busy(r) ? 3 : 4);
      const recent = (r: AgentRow) => (seen.indexOf(r.st.agent.name) < 0 ? 1e6 : seen.indexOf(r.st.agent.name));
      return rows.filter((r) => r.st.state !== "offline" || r.st.agent.name === current)
        .sort((a, b) => group(a) - group(b) || recent(a) - recent(b));
    };
    const step = (by: number) => {
      const n = order.current.length;
      if (!n) return;
      setAt((i) => (i + by + n) % n);
    };
    const down = (e: KeyboardEvent) => {
      if (state.current.open) {
        // Tab (⇧ back) and the arrow keys, whatever else is held.
        if (switchKey(e) || e.key === "Tab") {
          e.preventDefault();
          step(e.shiftKey ? -1 : 1);
        } else if (e.key === "ArrowRight" || e.key === "ArrowDown") {
          e.preventDefault();
          step(1);
        } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
          e.preventDefault();
          step(-1);
        } else if (e.key === "Home" || e.key === "End") {
          e.preventDefault();
          setAt(e.key === "Home" ? 0 : Math.max(0, order.current.length - 1));
        } else if (e.key === "Escape") {
          e.preventDefault();
          e.stopPropagation();
          setOpen(false);
        } else if (e.key === "Enter") {
          e.preventDefault();
          const r = order.current[state.current.at];
          setOpen(false);
          if (r) pickRef.current(r);
        }
        return;
      }
      const held = switchKey(e);
      if (!held || e.isComposing) return;
      if (document.querySelector(".app-modal, .cp")) return;
      const list = build();
      if (list.length < 1) return;
      e.preventDefault();
      opener.current = held;
      order.current = list;
      const cur = currentNow();
      const first = cur && list[0]?.st.agent.name === cur ? 1 : 0;
      setAt(Math.min(list.length - 1, e.shiftKey ? list.length - 1 : first));
      setOpen(true);
    };
    const up = (e: KeyboardEvent) => {
      if (!state.current.open || e.key !== opener.current) return;
      const r = order.current[state.current.at];
      setOpen(false);
      if (r) pickRef.current(r);
    };
    // The window losing focus with ⌃ held: put it away, nothing opened.
    const blur = () => setOpen(false);
    window.addEventListener("keydown", down, true);
    window.addEventListener("keyup", up, true);
    window.addEventListener("blur", blur);
    return () => {
      window.removeEventListener("keydown", down, true);
      window.removeEventListener("keyup", up, true);
      window.removeEventListener("blur", blur);
    };
  }, []);

  // The one picked stays in sight when there are more than fit.
  useEffect(() => {
    if (!open) return;
    listRef.current?.querySelector<HTMLElement>(`[data-i="${at}"]`)?.scrollIntoView({ block: "nearest", inline: "nearest" });
  }, [open, at]);

  if (!open) return null;
  const list = order.current;
  const picked = list[at];
  return createPortal(
    <div className="app-portal asw-layer" onMouseDown={() => setOpen(false)}>
      <div className="asw" role="listbox" aria-label={L("切换 agent", "Switch agent")} data-testid="agent-switcher" onMouseDown={(e) => e.stopPropagation()}>
        <div className="asw-row" ref={listRef}>
          {list.map((r, i) => {
            const st = r.st.asking ? "asking" : r.st.state === "working" ? "working" : r.st.state === "offline" ? "offline" : "";
            return (
              <button key={r.st.agent.name} type="button" role="option" aria-selected={i === at} data-i={i}
                className={cn("asw-item", i === at && "is-on", st === "offline" && "is-away")}
                onMouseEnter={() => setAt(i)} onClick={() => { setOpen(false); onPick(r); }}
                title={nodeName(r.st.agent, r.hub)}>
                <Avatar agent={r.st.agent} size={60} badge={false} />
                {st && st !== "offline" && <i className={cn("asw-dot", st === "asking" && "is-asking")} aria-hidden />}
              </button>
            );
          })}
        </div>
        <div className="asw-name">{picked ? nodeName(picked.st.agent, picked.hub) : ""}</div>
        <div className="asw-hint">{isMacKeys() ? L("按住 ⌃ 或 ⌥ 不放，按 Tab 或 ← → 挑，松开打开", "Keep ⌃ or ⌥ held; Tab or ← → to pick, let go to open")
          : L("按住 Ctrl 不放，按 Tab 或 ← → 挑，松开打开", "Keep Ctrl held; Tab or ← → to pick, let go to open")}</div>
      </div>
    </div>,
    document.body,
  );
}
