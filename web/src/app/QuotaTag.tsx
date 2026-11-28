import { useState } from "react";
import { Gauge, TriangleAlert } from "lucide-react";
import type { AgentRow } from "@/app/types";
import { machinesOfHub } from "@/app/nodes";
import { quotaEffectiveStatus, quotaHasNumbers, quotaLeft, quotaLiveWindows, quotaObservedAt, quotaPillLevel, quotaSourceText, quotaSpent, quotaStaleNote, quotaStatusText, quotaSummary, quotaUnreadable, quotaWindowLabel, quotaWindowShort, quotaWindowText, quotaWorst } from "@/app/quota";
import { fitMenu } from "@/app/fitMenu";
import { L, E } from "@/lib/i18n";
import { timeAgo } from "@/lib/utils";
import { cn } from "@/lib/utils";

/** A member's quota in a list: "剩 38%" (the tightest window), amber when
 *  nearly used, red when spent or a login needs fixing; the whole say
 *  (window, provider · plan · computer, reset, data age) on hover. A read
 *  nobody refreshed for an hour stays, greyed and with its age: it is the
 *  last thing known, never the account's current use. Only the account's
 *  owner gets a quota from the hub, so others see nothing. */
export function QuotaTag({ row, now, className }: { row: AgentRow; now: number; className?: string }) {
  const quota = row.st.agent.runtime?.quota;
  // A connection with nothing readable says nothing in a list (its details
  // explain it); last known numbers still say something.
  if (!quota || quotaUnreadable(quota)) return null;
  const s = quotaSummary(quota, machinesOfHub(row.hub), now);
  const bad = s.level === "action" || s.level === "failed";
  return (
    <span className={cn("app-quota-tag", `is-${s.level}`, s.stale && "is-stale", className)} title={s.line} data-quota={s.level}>
      {bad ? <TriangleAlert aria-hidden /> : <Gauge aria-hidden />}
      <span className="app-quota-tag-text" aria-hidden>{s.short}</span>
      <span className="app-sr-only">{s.line}</span>
    </span>
  );
}

/** The composer's quota pill, beside the model and access: the tightest
 *  window of the agent's account ("5h 剩 38%"), grey, amber under 20% left,
 *  red under 5%. Numbers nobody refreshed for an hour stay, greyed, with
 *  their age ("5h 剩 38% · 3 小时前"), never as today's use; an account with
 *  no window left says quietly "额度未知". A click shows every window, when
 *  each resets, whose usage it is and when it was read. Nothing for a
 *  connection with nothing readable, or for anyone but the account's owner
 *  (the hub then sends no quota). */
export function QuotaPill({ row, now }: { row: AgentRow; now: number }) {
  const [open, setOpen] = useState(false);
  const quota = row.st.agent.runtime?.quota;
  // A connection with nothing readable has no pill (its details say why).
  if (!quota || quotaUnreadable(quota)) return null;
  const has = quotaHasNumbers(quota, now);
  const summary = quotaSummary(quota, machinesOfHub(row.hub), now);
  const worst = has ? quotaWorst(quota, now) : undefined;
  const old = !!worst && quotaEffectiveStatus(quota, now) === "unknown";
  const action = quota.status === "auth_required" || quota.status === "no_credentials";
  const level = worst ? (old ? "old" : quotaPillLevel(worst)) : action ? "low" : "unknown";
  const status = quotaStatusText(quota, now);
  const stale = quotaStaleNote(quota, now);
  const source = quotaSourceText(quota, machinesOfHub(row.hub));
  const read = quota.checked_at || quota.at;
  // (a failed first read has no observation time at all)
  const readAt = read && new Date(read).getTime() > Date.UTC(2000, 0, 1) ? read : "";
  const windows = has ? quotaLiveWindows(quota, now) : [];
  let label: string;
  let title: string;
  if (worst) {
    const text = quotaSpent(worst) ? L("已用完", "out") : L(E`剩 ${quotaLeft(worst)}%`, E`${quotaLeft(worst)}% left`);
    label = [quotaWindowShort(worst), text].filter(Boolean).join(" ");
    if (old) label += ` · ${timeAgo(quotaObservedAt(quota), now)}`;
    title = old
      ? L(E`${summary.line}（点开看全部）`, E`${summary.line} (click for all)`)
      : L(E`额度：${quotaWindowLabel(worst)}${text}（点开看全部）`, E`Quota: ${quotaWindowLabel(worst)} ${text} (click for all)`);
  } else {
    label = summary.short;
    title = L(E`${summary.line}（点开看全部）`, E`${summary.line} (click for all)`);
  }
  return (
    <span className="quota-pill-wrap">
      <button type="button" className={cn("quota-pill", `is-${level}`, (status || stale) && "is-stale")} title={title} aria-label={title}
        aria-expanded={open} onClick={() => setOpen((o) => !o)} onMouseDown={(e) => e.preventDefault()} data-testid="quota-pill">
        <Gauge className="quota-pill-icon" aria-hidden />
        <span className="quota-pill-text">{label}</span>
      </button>
      {open && (
        <>
          <span className="quota-pill-scrim" onClick={() => setOpen(false)} />
          <div ref={fitMenu} className="quota-pill-menu" role="dialog" aria-label={L("额度", "Quota")} onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}>
            <div className="run-pill-group">{L("额度", "Quota")}</div>
            {windows.map((w) => (
              <div key={`${w.name}/${w.model_scope || ""}`} className={cn("quota-pill-row", `is-${quotaPillLevel(w)}`)}>
                <span>{quotaWindowLabel(w)}</span>
                <span>{quotaWindowText(w, now)}</span>
              </div>
            ))}
            {status && <p className={cn("run-pill-note quota-pill-status", !action && "is-quiet")}>{status}{windows.length > 0 && L("，上面是上次取到的。", "; above is the last read.")}</p>}
            <p className="run-pill-note">
              {source && <>{source}<br /></>}
              {readAt && L(E`${timeAgo(readAt, now)}读取`, E`Read ${timeAgo(readAt, now)}`)}
              {stale && !status && <>{L("（数据较旧）", " (old data)")}</>}
            </p>
          </div>
        </>
      )}
    </span>
  );
}

