import { useEffect, useRef, useState } from "react";
import { appApi, type AppOverview } from "@/appapi";
import type { Agent } from "@/api";
import { Mushroom } from "@/app/Working";
import { Avatar } from "@/app/persona";
import { agentKind } from "@/app/nodes";
import { L } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";
import "@/app/app.css";
import "@/app/space.css";
import "./island.css";

/*
 * The notch's island (macOS app only): while agents work, the black notch
 * grows a little to each side, a mushroom walking on the left and how many
 * are at work on the right, as a phone's island shows a call. Pointing at
 * it opens it downwards with who is working on what; a name opens the app
 * at that member. The window is exactly as big as what is drawn, so it
 * does not get in the way of the menu bar; the app (Go) places it.
 */

const q = new URLSearchParams(window.location.search);
// The notch in points (width 0: a Mac without one; the pill then sits in
// the middle of the menu bar).
const NW = Number(q.get("nw")) || 0;
const NH = Number(q.get("nh")) || 32;
const WING = 64;
const COMPACT_W = NW ? NW + 2 * WING : 190;
const OPEN_W = Math.max(COMPACT_W, 380);
const ROW_H = 46;
// The shoulders: where the pill's top meets the menu bar it curves into it,
// as the notch's own edges do. The window is that much wider each side.
const EAR = 8;
const MAX_ROWS = 5;
const POLL_MS = 2500;
export const ISLAND_KEY = "myco-island";

type Busy = { hubId: string; agent: Agent; status: string; since?: string };

function enabled(): boolean {
  try { return localStorage.getItem(ISLAND_KEY) !== "off"; } catch { return true; }
}

function busyOf(ov: AppOverview): Busy[] {
  const out: Busy[] = [];
  for (const h of ov.hubs) {
    for (const st of h.overview?.agents || []) {
      const k = agentKind(st.agent);
      if (st.state !== "working" || (k !== "agent" && k !== "shell")) continue;
      const task = (st.current || []).find((m) => m.to === st.agent.name && m.type === "task");
      out.push({
        hubId: h.id,
        agent: st.agent,
        status: st.agent.status_text || task?.body.replace(/\s+/g, " ").trim() || L("在干活", "Working"),
        since: task?.created_at,
      });
    }
  }
  return out;
}

// While shown the window keeps one size, as big as the open island can
// get: resizing it while the pill springs made the page blink. The app
// lets clicks through it except on the pill (pw×ph), and says when the
// pointer is on it ("island-hover").
const WIN_W = OPEN_W + 2 * EAR;
const WIN_H = NH + 10 + MAX_ROWS * ROW_H + 10 + 26;

function frame(visible: boolean, pw = 0, ph = 0) {
  return fetch("app/island/frame", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ w: WIN_W, h: WIN_H, pw: pw + 2 * EAR, ph, visible }),
  }).catch(() => undefined);
}

export function Island() {
  const [busy, setBusy] = useState<Busy[]>([]);
  // shown: the pill is out (grown from the notch); open: pointed at.
  const [shown, setShown] = useState(false);
  const [open, setOpen] = useState(false);
  const [now, setNow] = useState(Date.now());
  const closing = useRef<number | null>(null);
  const live = busy.length > 0;

  // Start out of the way (one point, clicks pass through) until someone works.
  useEffect(() => { void frame(false); }, []);

  useEffect(() => {
    let off = false;
    const tick = async () => {
      try {
        const ov = await appApi.overview();
        if (!off) setBusy(enabled() ? busyOf(ov) : []);
      } catch { /* keep what we have */ }
      if (!off) setNow(Date.now());
    };
    void tick();
    const t = window.setInterval(tick, POLL_MS);
    return () => { off = true; window.clearInterval(t); };
  }, []);

  const openRows = Math.min(busy.length, MAX_ROWS);
  const openH = NH + 10 + openRows * ROW_H + 10;

  // Someone starts: show the window (as wide as the pill will be), then let
  // the pill grow out of the notch. Nobody left: shrink back into the
  // notch, then hide the window.
  useEffect(() => {
    if (live) {
      if (closing.current) { window.clearTimeout(closing.current); closing.current = null; }
      void frame(true, COMPACT_W, NH).then(() => requestAnimationFrame(() => setShown(true)));
    } else if (shown) {
      setOpen(false);
      setShown(false);
      closing.current = window.setTimeout(() => { void frame(false); closing.current = null; }, 520);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [live]);

  // Where clicks may land follows the pill: open, or just the bar.
  useEffect(() => {
    if (!live || !shown) return;
    void frame(true, open ? OPEN_W : COMPACT_W, open ? openH : NH);
  }, [open, live, shown, openH]);

  // Pointing at it opens it; leaving closes it (after a moment, so
  // brushing past does not flap it).
  const leaving = useRef<number | null>(null);
  const enter = () => {
    if (leaving.current) { window.clearTimeout(leaving.current); leaving.current = null; }
    if (live) setOpen(true);
  };
  const leave = () => {
    if (leaving.current) window.clearTimeout(leaving.current);
    leaving.current = window.setTimeout(() => { setOpen(false); leaving.current = null; }, 140);
  };
  useEffect(() => {
    const on = (e: Event) => ((e as CustomEvent<boolean>).detail ? enter() : leave());
    window.addEventListener("island-hover", on);
    return () => window.removeEventListener("island-hover", on);
  });

  const openMember = (b: Busy) => {
    setOpen(false);
    void fetch("app/island/open", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ hub: b.hubId, agent: b.agent.name }),
    }).catch(() => undefined);
  };

  const pillW = !shown ? Math.max(NW, 40) : open ? OPEN_W : COMPACT_W;
  const pillH = open ? openH : NH;
  const first = busy[0];

  return (
    <div className="isl-root">
      <div
        className={cn("isl-shape", shown && "is-shown", !NW && "no-notch")}
        style={{ width: pillW, height: pillH, ["--nh" as string]: `${NH}px`, ["--nw" as string]: `${NW}px`, ["--ear" as string]: `${EAR}px` }}
        onMouseEnter={enter}
        onMouseLeave={leave}
      >
      <div className={cn("isl", shown && "is-shown", open && "is-open", !NW && "no-notch")}>
        <div className="isl-bar">
          <span className="isl-wing is-left">
            <Mushroom />
          </span>
          {NW > 0 && <span className="isl-notch" />}
          <span className="isl-wing is-right">
            {NW > 0 ? (
              <b>{busy.length}</b>
            ) : (
              <span className="isl-line">{busy.length === 1 && first ? first.status : L(`${busy.length} 个在干活`, `${busy.length} at work`)}</span>
            )}
          </span>
        </div>
        <div className="isl-list">
          {busy.slice(0, MAX_ROWS).map((b) => (
            <button
              key={`${b.hubId}/${b.agent.name}`}
              type="button"
              className="isl-row"
              // On press (a panel that is not in front may not see a whole
              // click), once; then the island folds back into the notch.
              onMouseDown={(e) => { e.preventDefault(); openMember(b); }}
            >
              <Avatar agent={b.agent} size={28} badge={false} />
              <span className="isl-row-text">
                <span className="isl-row-name">{b.agent.display_name || b.agent.name}</span>
                <span className="isl-row-status">{b.status}</span>
              </span>
              {b.since && <span className="isl-row-time">{timeAgo(b.since, now)}</span>}
            </button>
          ))}
          {busy.length > MAX_ROWS && (
            <div className="isl-more">{L(`还有 ${busy.length - MAX_ROWS} 个在干活`, `${busy.length - MAX_ROWS} more at work`)}</div>
          )}
        </div>
      </div>
      </div>
    </div>
  );
}
