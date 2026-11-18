import type { LucideIcon } from "lucide-react";

/**
 * The head of a top-level page (自动化, 应用): its icon on a soft tile, a
 * title, one line on what it is for, and the page's main actions on the
 * right. Under it, sections with a heading of their own (SectionHead).
 */
export function PageHero({ icon: Icon, title, subtitle, children }: { icon: LucideIcon; title: string; subtitle: string; children?: React.ReactNode }) {
  return (
    <header className="ph">
      <span className="ph-icon" aria-hidden><Icon /></span>
      <div className="ph-text">
        <h1>{title}</h1>
        <p>{subtitle}</p>
      </div>
      {children && <div className="ph-actions">{children}</div>}
    </header>
  );
}

/** A section's heading: its name, how many, and its own small actions. */
export function SectionHead({ title, count, children }: { title: string; count?: number; children?: React.ReactNode }) {
  return (
    <div className="ph-section">
      <h2>{title}{count !== undefined && <span className="ph-count">{count}</span>}</h2>
      {children && <div className="ph-section-tools">{children}</div>}
    </div>
  );
}

/** Cards in place of a grid that is still loading (its shape, not a word). */
export function CardsSkeleton({ count = 6, className = "" }: { count?: number; className?: string }) {
  return (
    <div className={`ph-skel-grid ${className}`} aria-busy="true">
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="ph-skel-card">
          <span className="skel ph-skel-icon" />
          <span className="skel-lines">
            <span className="skel skel-line" style={{ width: `${40 + ((i * 17) % 30)}%` }} />
            <span className="skel skel-line is-thin" style={{ width: "86%" }} />
            <span className="skel skel-line is-thin" style={{ width: `${50 + ((i * 11) % 30)}%` }} />
          </span>
        </div>
      ))}
    </div>
  );
}
