import { Loader2 } from "lucide-react";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/** Placeholder rows (icon, title, line) while a list loads: its shape, not
 *  a word. */
export function RowsSkeleton({ rows = 4, className }: { rows?: number; className?: string }) {
  const widths = [[46, 72], [38, 58], [52, 66], [34, 50], [44, 62]];
  return (
    <div className={cn("skel-rows", className)} aria-busy="true" aria-label={L("正在加载", "Loading")}>
      {Array.from({ length: rows }, (_, i) => {
        const [a, b] = widths[i % widths.length];
        return (
          <div key={i} className="skel-row">
            <span className="skel skel-icon" />
            <span className="skel-lines">
              <span className="skel skel-line" style={{ width: `${a}%` }} />
              <span className="skel skel-line is-thin" style={{ width: `${b}%` }} />
            </span>
          </div>
        );
      })}
    </div>
  );
}

/** A short grey bar in place of a line of text that is still loading. */
export function TextSkeleton({ width = 90 }: { width?: number }) {
  return <span className="skel skel-text" style={{ width }} aria-label={L("正在加载", "Loading")} />;
}

/** A button label while its action runs: a small spinner, same words. */
export function Busy({ busy, children }: { busy: boolean; children: React.ReactNode }) {
  return <>{busy && <Loader2 className="skel-spin" aria-hidden />}{children}</>;
}
