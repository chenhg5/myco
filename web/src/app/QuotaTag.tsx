import { useState } from "react";
import { Gauge, TriangleAlert } from "lucide-react";
import type { AgentRow } from "@/app/types";
import { machinesOfHub } from "@/app/nodes";
import { quotaHasNumbers, quotaLeft, quotaPillLevel, quotaSourceText, quotaSpent, quotaStaleNote, quotaStatusText, quotaSummary, quotaUnreadable, quotaWindowLabel, quotaWindowShort, quotaWindowText, quotaWorst } from "@/app/quota";
import { fitMenu } from "@/app/fitMenu";
import { L } from "@/lib/i18n";
import { timeAgo } from "@/lib/utils";
import { cn } from "@/lib/utils";

/** A member's quota in a list: "剩 38%" (the tightest window), amber when
 *  nearly used, red when spent or a login needs fixing; the whole say
 *  (window, provider · plan · computer, reset, data age) on hover. Only the
 *  account's owner gets a quota from the hub, so others see nothing. */
export function QuotaTag({ row, now, className }: { row: AgentRow; now: number; className?: string }) {
  const quota = row.st.agent.runtime?.quota;
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
 *  red under 5%. A click shows every window, when each resets, whose usage
 *  it is and when it was read. Nothing without numbers (or for anyone but
 *  the account's owner: the hub then sends no quota). */
export function QuotaPill({ row, now }: { row: AgentRow; now: number }) {
  const [open, setOpen] = useState(false);
  const quota = row.st.agent.runtime?.quota;
  if (!quota || !quotaHasNumbers(quota)) return null;
  const worst = quotaWorst(quota);
  if (!worst) return null;
  const level = quotaPillLevel(worst);
  const short = quotaWindowShort(worst);
  const text = quotaSpent(worst) ? L("已用完", "out") : L(`剩 ${quotaLeft(worst)}%`, `${quotaLeft(worst)}% left`);
  const label = [short, text].filter(Boolean).join(" ");
  const status = quotaStatusText(quota);
  const stale = quotaStaleNote(quota, now);
  const source = quotaSourceText(quota, machinesOfHub(row.hub));
  const read = quota.checked_at || quota.at;
  const title = L(`额度：${quotaWindowLabel(worst)}${text}（点开看全部）`, `Quota: ${quotaWindowLabel(worst)} ${text} (click for all)`);
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
            {quota.windows.map((w) => (
              <div key={`${w.name}/${w.model_scope || ""}`} className={cn("quota-pill-row", `is-${quotaPillLevel(w)}`)}>
                <span>{quotaWindowLabel(w)}</span>
                <span>{quotaWindowText(w, now)}</span>
              </div>
            ))}
            {status && <p className="run-pill-note quota-pill-status">{status}{L("，上面是上次取到的。", "; above is the last read.")}</p>}
            <p className="run-pill-note">
              {source && <>{source}<br /></>}
              {read && L(`${timeAgo(read, now)}读取`, `Read ${timeAgo(read, now)}`)}
              {stale && !status && <>{L("（数据较旧）", " (old data)")}</>}
            </p>
          </div>
        </>
      )}
    </span>
  );
}

