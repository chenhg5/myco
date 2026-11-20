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

const isSwitchKey = (e: KeyboardEvent) => e.ctrlKey && !e.metaKey && !e.altKey && (e.key === "Tab" || e.code === "Backquote");

export function AgentSwitcher({ hubId, rows, current, onPick }: {
  hubId: string;
  /** the agents to switch between (this workspace's chats) */
  rows: AgentRow[];
  /** the agent whose chat is open, if any */
  current?: string;
  onPick: (r: AgentRow) => void;
}) {
  const [open, setOpen] = useState(false);
  const [at, setAt] = useState(0);
  const order = useRef<AgentRow[]>([]);
  const state = useRef({ open: false, at: 0 });
  state.current = { open, at };
  const pickRef = useRef(onPick);
  pickRef.current = onPick;
  const rowsRef = useRef({ rows, current, hubId });
  rowsRef.current = { rows, current, hubId };
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // Last talked to first (this workspace), then the rest as listed; the
    // open chat first of all, so one press goes to the one before it.
    const build = () => {
      const { rows, current, hubId } = rowsRef.current;
      const seen = recentChats().filter((c) => c.hubId === hubId).map((c) => c.agent);
      const rank = (r: AgentRow) => (r.st.agent.name === current ? -1 : seen.indexOf(r.st.agent.name) < 0 ? 1e6 : seen.indexOf(r.st.agent.name));
      return [...rows].sort((a, b) => rank(a) - rank(b));
    };
    const step = (by: number) => {
      const n = order.current.length;
      if (!n) return;
      setAt((i) => (i + by + n) % n);
    };
    const down = (e: KeyboardEvent) => {
      if (state.current.open) {
        if (isSwitchKey(e) || (e.ctrlKey && (e.key === "ArrowRight" || e.key === "ArrowLeft"))) {
          e.preventDefault();
          step(e.shiftKey || e.key === "ArrowLeft" ? -1 : 1);
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
      if (!isSwitchKey(e) || e.isComposing) return;
      if (document.querySelector(".app-modal, .cp")) return;
      const list = build();
      if (list.length < 1) return;
      e.preventDefault();
      order.current = list;
      const first = rowsRef.current.current && list[0]?.st.agent.name === rowsRef.current.current ? 1 : 0;
      setAt(Math.min(list.length - 1, e.shiftKey ? list.length - 1 : first));
      setOpen(true);
    };
    const up = (e: KeyboardEvent) => {
      if (!state.current.open || e.key !== "Control") return;
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
        <div className="asw-hint">{isMacKeys() ? L("按住 ⌃ 再按 Tab 换下一个，松开打开", "Hold ⌃ and press Tab for the next; let go to open")
          : L("按住 Ctrl 再按 Tab 换下一个，松开打开", "Hold Ctrl and press Tab for the next; let go to open")}</div>
      </div>
    </div>,
    document.body,
  );
}
