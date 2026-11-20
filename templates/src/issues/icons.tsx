// Linear-style marks, in black, white and gray: a status is a circle that
// fills as the work goes on; a priority is bars, or a filled square for
// urgent; a person is their initial in a gray circle.

export function StatusIcon({ s, size = 14 }: { s: string; size?: number }) {
  const r = 6, c = 7;
  const common = { width: size, height: size, viewBox: "0 0 14 14", className: `st st-${s}`, "aria-hidden": true } as const;
  if (s === "backlog") return <svg {...common}><circle cx={c} cy={c} r={r} fill="none" stroke="currentColor" strokeWidth="1.5" strokeDasharray="1.6 1.9" /></svg>;
  if (s === "todo") return <svg {...common}><circle cx={c} cy={c} r={r} fill="none" stroke="currentColor" strokeWidth="1.5" /></svg>;
  if (s === "done") return (
    <svg {...common}><circle cx={c} cy={c} r={r + 0.75} fill="currentColor" /><path d="M4.4 7.2 6.2 9 9.7 5.3" fill="none" stroke="var(--bg)" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" /></svg>
  );
  if (s === "canceled") return (
    <svg {...common}><circle cx={c} cy={c} r={r + 0.75} fill="currentColor" /><path d="M5 5l4 4M9 5l-4 4" stroke="var(--bg)" strokeWidth="1.5" strokeLinecap="round" /></svg>
  );
  // in progress: half; in review: three quarters
  const part = s === "review" ? 0.75 : 0.5;
  const a = 2 * Math.PI * part, x = c + 3.2 * Math.sin(a), y = c - 3.2 * Math.cos(a);
  return (
    <svg {...common}>
      <circle cx={c} cy={c} r={r} fill="none" stroke="currentColor" strokeWidth="1.5" />
      <path d={`M${c} ${c} L${c} ${c - 3.2} A3.2 3.2 0 ${part > 0.5 ? 1 : 0} 1 ${x} ${y} Z`} fill="currentColor" />
    </svg>
  );
}

export function PriorityIcon({ p, size = 14 }: { p: number; size?: number }) {
  const common = { width: size, height: size, viewBox: "0 0 14 14", className: `pr pr-${p}`, "aria-hidden": true } as const;
  if (p === 1) return <svg {...common}><rect x="1" y="1" width="12" height="12" rx="3" fill="currentColor" /><path d="M7 3.6v4.2M7 9.9v.3" stroke="var(--bg)" strokeWidth="1.7" strokeLinecap="round" /></svg>;
  if (!p) return <svg {...common}><path d="M2.5 7h2M6 7h2M9.5 7h2" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" opacity="0.6" /></svg>;
  const on = p === 2 ? 3 : p === 3 ? 2 : 1;
  return (
    <svg {...common}>
      {[0, 1, 2].map((i) => <rect key={i} x={1.5 + i * 4} y={9 - i * 3} width="3" height={3 + i * 3} rx="1" fill="currentColor" opacity={i < on ? 1 : 0.25} />)}
    </svg>
  );
}

export function Avatar({ name, size = 18 }: { name?: string | null; size?: number }) {
  if (!name) return (
    <svg width={size} height={size} viewBox="0 0 18 18" className="av av-none" aria-hidden="true">
      <circle cx="9" cy="9" r="8" fill="none" stroke="currentColor" strokeWidth="1.2" strokeDasharray="2 2" />
      <circle cx="9" cy="7.2" r="2.4" fill="none" stroke="currentColor" strokeWidth="1.2" /><path d="M4.8 14c.8-2 2.3-3 4.2-3s3.4 1 4.2 3" fill="none" stroke="currentColor" strokeWidth="1.2" />
    </svg>
  );
  const ch = Array.from(name.trim())[0]?.toUpperCase() || "?";
  return <span className="av" style={{ width: size, height: size, fontSize: Math.round(size * 0.55) }} title={name}>{ch}</span>;
}
