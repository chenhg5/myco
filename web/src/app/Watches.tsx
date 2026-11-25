import { useCallback, useEffect, useRef, useState } from "react";
import { ChevronDown, Eye, Hourglass, X } from "lucide-react";
import { appApi, errorMessage } from "@/appapi";
import type { Watch } from "@/api";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { ConfirmDialog } from "@/app/dialogs";
import { useContextMenu } from "@/app/ContextMenu";

/*
 * 盯着 (myco watch): an agent waiting in the background for something (a
 * reply, a task, a workflow run) has a record the hub keeps. Its chat shows
 * it at the top, "正在盯：mac2 的回复 · 已 12 分钟，剩 18 分钟 · 30 秒前看过",
 * the clock moving each second, with 延长 and 停止; when it ends (met, the
 * time up, stopped) a grey line stays in the chat where it ended.
 */

const POLL_MS = 5000;

/** The watches of one agent (active and ended), read every few seconds
 *  while its chat is open. A hub without watches answers 404: nothing. */
export function useWatches(hubId: string, agent: string) {
  const [list, setList] = useState<Watch[]>([]);
  const [none, setNone] = useState(false);
  const live = useRef(true);
  const load = useCallback(async () => {
    try {
      const r = await appApi.watches(hubId, { agent, state: "all" });
      if (live.current) setList(r.watches || []);
    } catch (e) {
      if ((e as { code?: string; status?: number })?.code === "not_found" || (e as { status?: number })?.status === 404) setNone(true);
    }
  }, [hubId, agent]);
  useEffect(() => {
    live.current = true;
    setList([]);
    setNone(false);
    void load();
    return () => { live.current = false; };
  }, [load]);
  useEffect(() => {
    if (none) return;
    const t = window.setInterval(() => void load(), POLL_MS);
    return () => window.clearInterval(t);
  }, [load, none]);
  /** one changed here (stopped, extended): shown at once */
  const put = (w: Watch) => setList((xs) => (xs.some((x) => x.id === w.id) ? xs.map((x) => (x.id === w.id ? w : x)) : [...xs, w]));
  return { list, active: list.filter((w) => w.state === "watching"), ended: list.filter((w) => w.state !== "watching" && w.ended_at), reload: load, put };
}

function useNow(on: boolean) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!on) return;
    const t = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(t);
  }, [on]);
  return now;
}

/** 12 秒 / 12 分钟 / 1 小时 5 分 */
function span(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 60) return L(`${s} 秒`, `${s}s`);
  const m = Math.floor(s / 60);
  if (m < 60) return L(`${m} 分钟`, `${m} min`);
  const h = Math.floor(m / 60);
  return m % 60 ? L(`${h} 小时 ${m % 60} 分`, `${h} h ${m % 60} min`) : L(`${h} 小时`, `${h} h`);
}

function clock(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString(undefined, { month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit", hour12: false });
}

/** "已 12 分钟，剩 18 分钟 · 30 秒前看过" */
function timing(w: Watch, now: number): string {
  const started = Date.parse(w.started_at);
  const deadline = Date.parse(w.deadline);
  const parts = [L(`已 ${span(now - started)}，剩 ${span(deadline - now)}`, `${span(now - started)} so far, ${span(deadline - now)} left`)];
  if (w.last_check_at) parts.push(L(`${span(now - Date.parse(w.last_check_at))}前看过`, `checked ${span(now - Date.parse(w.last_check_at))} ago`));
  return parts.join(" · ");
}

const EXTEND: [number, string, string][] = [[30 * 60, "30 分钟", "30 minutes"], [60 * 60, "1 小时", "1 hour"], [2 * 60 * 60, "2 小时", "2 hours"]];

/** The watches at work, at the top of the chat: one line each (or one line
 *  for several, opened to the list), with 延长 and 停止. */
export function WatchBar({ hubId, watches, onChanged }: {
  hubId: string;
  watches: Watch[];
  onChanged: (w: Watch) => void;
}) {
  const now = useNow(watches.length > 0);
  const [open, setOpen] = useState(false);
  const [stopping, setStopping] = useState<Watch | null>(null);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const menu = useContextMenu();
  if (!watches.length) return null;
  const extend = async (w: Watch, sec: number) => {
    setBusy(w.id); setError("");
    try { onChanged(await appApi.extendWatch(hubId, w.id, sec)); } catch (e) { setError(errorMessage(e)); } finally { setBusy(""); }
  };
  const stop = async (w: Watch) => {
    setBusy(w.id); setError("");
    try { onChanged(await appApi.stopWatch(hubId, w.id)); setStopping(null); } catch (e) { setError(errorMessage(e)); } finally { setBusy(""); }
  };
  const actions = (w: Watch) => (
    <span className="watch-actions">
      <button type="button" className="watch-btn" disabled={busy === w.id} title={L("延长", "Extend")} aria-label={L("延长", "Extend")}
        onClick={(e) => menu.openUnder(e.currentTarget, [{ header: L("再盯多久（从现在起）", "Watch for (from now)") },
          ...EXTEND.map(([sec, zh, en]) => ({ label: L(zh, en), onSelect: () => void extend(w, sec) }))])}>
        <Hourglass /><span className="watch-btn-label">{L("延长", "Extend")}</span>
      </button>
      <button type="button" className="watch-btn" disabled={busy === w.id} onClick={() => setStopping(w)} title={L("停止", "Stop")} aria-label={L("停止", "Stop")}>
        <X /><span className="watch-btn-label">{L("停止", "Stop")}</span>
      </button>
    </span>
  );
  const detail = (w: Watch) => (
    <div className="watch-detail">
      {w.last_result && <div className="watch-result"><span>{L("最近一次", "Last check")}</span><code>{w.last_result}</code></div>}
      <div className="watch-times">
        {L(`开始 ${clock(w.started_at)} · 截止 ${clock(w.deadline)}`, `Started ${clock(w.started_at)} · until ${clock(w.deadline)}`)}
        {w.checks > 0 && L(` · 看了 ${w.checks} 次`, ` · ${w.checks} checks`)}
      </div>
    </div>
  );
  const one = watches.length === 1 ? watches[0] : null;
  return (
    <div className={cn("watch-bar", open && "is-open")} data-testid="watch-bar">
      <div className="watch-line">
        <button type="button" className="watch-toggle" onClick={() => setOpen((o) => !o)} aria-expanded={open}
          title={open ? L("收起", "Fold") : L("看详细", "Details")}>
          <Eye className="watch-eye" />
          {one ? (
            <>
              <span className="watch-what">{L(`正在盯：${one.what}`, `Watching: ${one.what}`)}</span>
              <span className="watch-time">{timing(one, now)}</span>
            </>
          ) : (
            <span className="watch-what">{L(`正在盯 ${watches.length} 件事`, `Watching ${watches.length} things`)}</span>
          )}
          <ChevronDown className="watch-chev" />
        </button>
        {one && actions(one)}
      </div>
      {open && (one ? detail(one) : (
        <ul className="watch-list">
          {watches.map((w) => (
            <li key={w.id} className="watch-item">
              <div className="watch-item-line">
                <span className="watch-what">{w.what}</span>
                <span className="watch-time">{timing(w, now)}</span>
                {actions(w)}
              </div>
              {detail(w)}
            </li>
          ))}
        </ul>
      ))}
      {error && <div className="app-error-text watch-error">{error}</div>}
      {menu.element}
      <ConfirmDialog open={!!stopping} onOpenChange={(o) => { if (!o) { setStopping(null); setError(""); } }}
        title={L("停止盯这件事？", "Stop watching this?")}
        description={stopping ? L(`「${stopping.what}」。停了以后不再替它等，它会收到一条消息，知道是你停的。`, `“${stopping.what}”. It stops waiting for it, and gets a message saying you stopped it.`) : undefined}
        confirmLabel={L("停止", "Stop")} busy={!!stopping && busy === stopping.id} error={stopping ? error : undefined}
        onConfirm={() => { if (stopping) void stop(stopping); }} />
    </div>
  );
}

/** How a watch ended, one grey line in the chat where it ended. */
export function WatchEnded({ w, nameOf }: { w: Watch; nameOf: (n: string) => string }) {
  const took = w.ended_at ? span(Date.parse(w.ended_at) - Date.parse(w.started_at)) : "";
  const text = w.state === "met"
    ? L(`盯到了：${w.what}${took ? `（${took}）` : ""}`, `Got it: ${w.what}${took ? ` (after ${took})` : ""}`)
    : w.state === "expired"
      ? L(`盯了 ${took} 没等到：${w.what}`, `Watched ${took}, it did not come: ${w.what}`)
      : L(`${w.ended_by ? nameOf(w.ended_by) : "有人"}停止了盯：${w.what}`, `${w.ended_by ? nameOf(w.ended_by) : "Someone"} stopped watching: ${w.what}`);
  return (
    <div className={cn("watch-ended", `is-${w.state}`)} data-watch={w.id} title={w.outcome || undefined}>
      <Eye />
      <span>{text}</span>
      <span className="watch-ended-time">{clock(w.ended_at)}</span>
    </div>
  );
}
