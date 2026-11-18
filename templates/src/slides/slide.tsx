// One slide, drawn at 1280×720 and scaled to fit: in the editor (its texts
// edited in place), as a thumbnail and when presenting.
import { useEffect, useLayoutEffect, useRef, type CSSProperties, type KeyboardEvent } from "react";
import { dark, themeOf, type Chart, type SlideData } from "./model";

export const W = 1280, H = 720;
type Save = (patch: Partial<SlideData>) => void;

/** Text edited in place; what others change shows unless you are in it. */
export function Editable({ value, onSave, className = "", placeholder, editable, multiline, onEnter, onEmptyBack, focusRef, style }: {
  value: string; onSave: (v: string) => void; className?: string; placeholder?: string; editable: boolean; multiline?: boolean;
  onEnter?: (v: string) => void; onEmptyBack?: () => void; focusRef?: (el: HTMLDivElement | null) => void; style?: CSSProperties;
}) {
  const ref = useRef<HTMLDivElement | null>(null);
  const focused = useRef(false);
  const timer = useRef(0);
  const read = () => (ref.current?.innerText || "").replace(/\n$/, "");
  useLayoutEffect(() => {
    if (ref.current && !focused.current && read() !== value) ref.current.innerText = value;
  }, [value]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => window.clearTimeout(timer.current), []);
  const save = () => { window.clearTimeout(timer.current); const v = read(); if (v !== value) onSave(v); };
  if (!editable) return <div className={className} style={style}>{value}</div>;
  return (
    <div ref={(el) => { ref.current = el; focusRef?.(el); }} className={`${className} ed`} style={style} data-ph={placeholder}
      contentEditable={"plaintext-only" as unknown as boolean} suppressContentEditableWarning spellCheck={false}
      onFocus={() => { focused.current = true; }}
      onBlur={() => { focused.current = false; save(); }}
      onInput={() => { window.clearTimeout(timer.current); timer.current = window.setTimeout(save, 600); }}
      onKeyDown={(e: KeyboardEvent) => {
        if (e.nativeEvent.isComposing) return;
        if (e.key === "Enter" && !(multiline && e.shiftKey)) {
          if (onEnter) { e.preventDefault(); window.clearTimeout(timer.current); onEnter(read()); }
          else if (!multiline) { e.preventDefault(); (e.target as HTMLElement).blur(); }
        } else if (e.key === "Backspace" && onEmptyBack && read() === "") { e.preventDefault(); onEmptyBack(); }
        else if (e.key === "Escape") (e.target as HTMLElement).blur();
      }} />
  );
}

/** Points: Enter makes the next one, Backspace on an empty one removes it. */
function Points({ points, save, editable }: { points: string[]; save: (p: string[]) => void; editable: boolean }) {
  const els = useRef<(HTMLDivElement | null)[]>([]);
  const want = useRef<number | null>(null);
  useLayoutEffect(() => {
    if (want.current === null) return;
    const el = els.current[want.current];
    want.current = null;
    if (!el) return;
    el.focus();
    const r = document.createRange();
    r.selectNodeContents(el);
    r.collapse(false);
    const s = getSelection();
    s?.removeAllRanges();
    s?.addRange(r);
  });
  const list = points.length ? points : editable ? [""] : [];
  return (
    <ul className="pts">
      {list.map((p, i) => (
        <li key={i}>
          <Editable value={p} editable={editable} placeholder="要点" focusRef={(el) => { els.current[i] = el; }}
            onSave={(v) => save(list.map((x, j) => (j === i ? v : x)))}
            onEnter={(v) => { want.current = i + 1; save([...list.slice(0, i), v, "", ...list.slice(i + 1)]); }}
            onEmptyBack={list.length > 1 ? () => { want.current = Math.max(0, i - 1); save(list.filter((_, j) => j !== i)); } : undefined} />
        </li>
      ))}
    </ul>
  );
}

/** A chart, drawn here (bars, lines or a pie) in the theme's colours. */
export function ChartView({ chart, colors, fg, muted, line }: { chart?: Chart; colors: string[]; fg: string; muted: string; line: string }) {
  const labels = chart?.labels || [];
  const series = (chart?.series || []).filter((s) => s && Array.isArray(s.values));
  if (!labels.length || !series.length) return <div className="chart-empty" style={{ color: muted }}>在右边填图表的数据</div>;
  const w = 560, h = 360, padL = 48, padB = 36, padT = 16;
  const num = (v: unknown) => (typeof v === "number" && isFinite(v) ? v : 0);
  if (chart?.type === "pie") {
    const vals = labels.map((_, i) => Math.max(0, num(series[0].values?.[i])));
    const sum = vals.reduce((a, b) => a + b, 0) || 1;
    let a0 = -Math.PI / 2;
    const cx = 180, cy = h / 2, r = 150;
    return (
      <svg viewBox={`0 0 ${w} ${h}`} className="chart">
        {vals.map((v, i) => {
          const a1 = a0 + (v / sum) * Math.PI * 2, large = a1 - a0 > Math.PI ? 1 : 0;
          const d = vals.filter((x) => x > 0).length === 1 && v > 0
            ? `M ${cx - r} ${cy} a ${r} ${r} 0 1 0 ${2 * r} 0 a ${r} ${r} 0 1 0 ${-2 * r} 0`
            : `M ${cx} ${cy} L ${cx + r * Math.cos(a0)} ${cy + r * Math.sin(a0)} A ${r} ${r} 0 ${large} 1 ${cx + r * Math.cos(a1)} ${cy + r * Math.sin(a1)} Z`;
          a0 = a1;
          return <path key={i} d={d} fill={colors[i % colors.length]} stroke="#fff" strokeWidth={v > 0 ? 2 : 0} />;
        })}
        {labels.map((l, i) => (
          <g key={i} transform={`translate(370 ${h / 2 - labels.length * 17 + i * 34})`}>
            <rect width="14" height="14" rx="3" y="-11" fill={colors[i % colors.length]} />
            <text x="22" fill={fg} fontSize="18">{l} · {Math.round((vals[i] / sum) * 100)}%</text>
          </g>
        ))}
      </svg>
    );
  }
  const all = series.flatMap((s) => labels.map((_, i) => num(s.values?.[i])));
  const max = Math.max(1, ...all), min = Math.min(0, ...all);
  const nice = (() => { const p = Math.pow(10, Math.floor(Math.log10(max))); return Math.ceil(max / p) * p; })();
  const y = (v: number) => padT + (h - padT - padB) * (1 - (v - min) / (nice - min || 1));
  const bw = (w - padL - 10) / labels.length;
  const ticks = [0, 0.25, 0.5, 0.75, 1].map((t) => min + (nice - min) * t);
  return (
    <svg viewBox={`0 0 ${w} ${h + (series.length > 1 ? 30 : 0)}`} className="chart">
      {ticks.map((t) => (
        <g key={t}>
          <line x1={padL} x2={w} y1={y(t)} y2={y(t)} stroke={line} strokeWidth="1" />
          <text x={padL - 8} y={y(t) + 5} textAnchor="end" fill={muted} fontSize="14">{Math.round(t * 100) / 100}</text>
        </g>
      ))}
      {chart?.type === "line" ? series.map((s, si) => {
        const pts = labels.map((_, i) => `${padL + bw * i + bw / 2},${y(num(s.values?.[i]))}`);
        return (
          <g key={si}>
            <polyline points={pts.join(" ")} fill="none" stroke={colors[si % colors.length]} strokeWidth="4" strokeLinejoin="round" strokeLinecap="round" />
            {pts.map((p, i) => { const [px, py] = p.split(","); return <circle key={i} cx={px} cy={py} r="6" fill="#fff" stroke={colors[si % colors.length]} strokeWidth="3" />; })}
          </g>
        );
      }) : labels.map((_, i) => {
        const gw = bw * 0.64, one = gw / series.length;
        return series.map((s, si) => {
          const v = num(s.values?.[i]);
          return <rect key={`${i}-${si}`} x={padL + bw * i + (bw - gw) / 2 + one * si} y={Math.min(y(v), y(0))} width={Math.max(2, one - 4)} height={Math.abs(y(0) - y(v))} rx="4" fill={colors[si % colors.length]} />;
        });
      })}
      {labels.map((l, i) => <text key={i} x={padL + bw * i + bw / 2} y={h - padB + 24} textAnchor="middle" fill={muted} fontSize="15">{l}</text>)}
      {series.length > 1 && series.map((s, si) => (
        <g key={si} transform={`translate(${padL + si * 140} ${h + 16})`}>
          <rect width="14" height="14" rx="3" y="-11" fill={colors[si % colors.length]} />
          <text x="20" fill={fg} fontSize="15">{s.name || `系列${si + 1}`}</text>
        </g>
      ))}
    </svg>
  );
}

export function SlideView({ s, deckTitle, theme, n, total, editable, save }: {
  s: SlideData; deckTitle: string; theme?: string; n: number; total: number; editable: boolean; save?: Save;
}) {
  const t = themeOf(theme), layout = s.layout || "points", inverse = dark(layout);
  const put: Save = save || (() => undefined);
  const vars = {
    "--bg": inverse ? t.coverBg : t.bg, "--fg": inverse ? t.coverFg : t.fg, "--muted": inverse ? t.coverMuted : t.muted,
    "--accent": t.accent, "--soft": t.soft, "--card": t.card, "--line": t.line, width: W, height: H,
  } as CSSProperties;
  const title = <Editable className="sl-title" value={s.title || ""} placeholder="标题" editable={editable} onSave={(v) => put({ title: v })} />;
  const sub = (ph = "副标题") => <Editable className="sl-sub" value={s.subtitle || ""} placeholder={ph} editable={editable} onSave={(v) => put({ subtitle: v })} />;
  const chart = <ChartView chart={s.chart} colors={t.chart} fg={inverse ? t.coverFg : t.fg} muted={inverse ? t.coverMuted : t.muted} line={t.line} />;
  const table = (
    <table className="sl-table">
      <thead><tr>{(s.table?.columns || []).map((c, i) => (
        <th key={i}><Editable value={c} editable={editable} placeholder="列名" onSave={(v) => put({ table: { ...s.table, columns: (s.table?.columns || []).map((x, j) => (j === i ? v : x)) } })} /></th>
      ))}</tr></thead>
      <tbody>{(s.table?.rows || []).map((r, ri) => (
        <tr key={ri}>{(s.table?.columns || []).map((_, ci) => (
          <td key={ci}><Editable value={r[ci] || ""} editable={editable} onSave={(v) => put({ table: { ...s.table, rows: (s.table?.rows || []).map((row, j) => (j === ri ? (s.table?.columns || []).map((__, k) => (k === ci ? v : row[k] || "")) : row)) } })} /></td>
        ))}</tr>
      ))}</tbody>
    </table>
  );
  let body;
  switch (layout) {
    case "cover": body = <div className="lay-cover"><span className="sl-bar" />{title}{sub()}</div>; break;
    case "section": body = <div className="lay-section"><span className="sl-num">{String(n).padStart(2, "0")}</span>{title}{sub("一句话说明这一部分（可不填）")}</div>; break;
    case "end": body = <div className="lay-end">{title}{sub("联系方式或一句话（可不填）")}</div>; break;
    case "quote": body = (
      <div className="lay-quote"><span className="sl-q">“</span>
        <Editable className="sl-quote" value={s.body || ""} placeholder="引言" editable={editable} multiline onSave={(v) => put({ body: v })} />
        {sub("—— 出处")}</div>
    ); break;
    case "two": body = <div className="lay-main">{title}{s.subtitle || editable ? sub() : null}<div className="lay-two"><Points points={s.points || []} editable={editable} save={(p) => put({ points: p })} /><div className="sl-side">{chart}</div></div></div>; break;
    case "cards": body = (
      <div className="lay-main">{title}{s.subtitle || editable ? sub() : null}
        <div className="sl-cards" style={{ gridTemplateColumns: `repeat(${Math.max(1, Math.min(4, (s.cards || []).length))}, 1fr)` }}>
          {(s.cards || []).map((c, i) => (
            <div className="sl-card" key={i}>
              <Editable className="sl-card-t" value={c.title || ""} placeholder="数字或标题" editable={editable} onSave={(v) => put({ cards: (s.cards || []).map((x, j) => (j === i ? { ...x, title: v } : x)) })} />
              <Editable className="sl-card-x" value={c.text || ""} placeholder="说明" editable={editable} multiline onSave={(v) => put({ cards: (s.cards || []).map((x, j) => (j === i ? { ...x, text: v } : x)) })} />
            </div>
          ))}
        </div>
      </div>
    ); break;
    case "chart": body = <div className="lay-main">{title}{s.subtitle || editable ? sub() : null}<div className="sl-full">{chart}</div></div>; break;
    case "table": body = <div className="lay-main">{title}{s.subtitle || editable ? sub() : null}<div className="sl-full sl-table-wrap">{table}</div></div>; break;
    default: body = <div className="lay-main">{title}{s.subtitle || editable ? sub() : null}<Points points={s.points || []} editable={editable} save={(p) => put({ points: p })} /></div>;
  }
  return (
    <div className={`sl is-${layout}${inverse ? " is-inverse" : ""}`} style={vars}>
      {body}
      {!inverse && layout !== "quote" && <div className="sl-foot"><span>{deckTitle}</span><span>{n} / {total}</span></div>}
    </div>
  );
}

/** A slide scaled to a width. */
export function Scaled({ width, children }: { width: number; children: React.ReactNode }) {
  const k = width / W;
  return (
    <div className="scaled" style={{ width, height: H * k }}>
      <div style={{ transform: `scale(${k})`, transformOrigin: "0 0", width: W, height: H }}>{children}</div>
    </div>
  );
}
