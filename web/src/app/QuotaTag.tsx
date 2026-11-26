import { Gauge, TriangleAlert } from "lucide-react";
import type { AgentRow } from "@/app/types";
import { machinesOfHub } from "@/app/nodes";
import { quotaSummary } from "@/app/quota";
import { cn } from "@/lib/utils";

/** A member's quota in a list: "剩 38%" (the tightest window), amber when
 *  nearly used, red when spent or a login needs fixing; the whole say
 *  (window, provider · plan · computer, reset, data age) on hover. Only the
 *  account's owner gets a quota from the hub, so others see nothing. */
export function QuotaTag({ row, now, className }: { row: AgentRow; now: number; className?: string }) {
  const quota = row.st.agent.runtime?.quota;
  if (!quota) return null;
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
