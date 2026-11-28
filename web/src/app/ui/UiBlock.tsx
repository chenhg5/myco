import { useMemo, useState } from "react";
import type { ReactNode } from "react";
import { Check, CircleDashed, Loader2, Minus, X } from "lucide-react";
import { L, E } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { appApi } from "@/appapi";
import { copyText } from "@/app/copy";
import { Lightbox, type Picture } from "@/app/lightbox";
import { Markdown, type InlineFiles } from "@/app/Markdown";
import { parseUi, tone, type UiElement, type UiSpec } from "@/app/ui/spec";

/*
 * A ```myco-ui block drawn (spec.ts parses it): the catalog's components,
 * in the App's own look. Each reads its props defensively (the App never
 * trusts a block to be well formed: `myco ui check` is the agent's job);
 * an unknown component is a small note, not an error.
 */

/** Said on the block's element when a button puts words in the input box
 *  (ConversationDetail listens on its pane). */
export const UI_REPLY = "myco-ui-reply";

type Ctx = { spec: UiSpec; files?: InlineFiles; zoom: (src: string, name: string) => void };

const str = (v: unknown): string => (typeof v === "string" ? v : typeof v === "number" ? String(v) : "");
const num = (v: unknown): number | null => (typeof v === "number" && Number.isFinite(v) ? v : null);
const list = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);
const obj = (v: unknown): Record<string, unknown> => (v && typeof v === "object" && !Array.isArray(v) ? v as Record<string, unknown> : {});

/** A picture's address: one of the message's own files, or https. */
function srcOf(v: unknown, files?: InlineFiles): string | null {
  const s = str(v);
  if (s.startsWith("attachment:")) {
    let ref = s.slice("attachment:".length);
    try { ref = decodeURIComponent(ref); } catch { /* as written */ }
    const a = files?.items.find((x) => x.file_id === ref) ?? files?.items.find((x) => x.name === ref);
    return a && files ? appApi.attachmentUrl(files.hubId, a.file_id) : null;
  }
  return /^https:\/\//i.test(s) ? s : null;
}
const linkOf = (v: unknown): string | null => (/^https?:\/\//i.test(str(v)) ? str(v) : null);

/** Markdown in a block (no blocks inside it). */
function Md({ text }: { text: string }) {
  return <Markdown text={text} zoom={false} />;
}
/** A code fence around text that may hold backticks itself. */
function fence(code: string, lang = ""): string {
  const run = Math.max(3, ...[...code.matchAll(/`+/g)].map((m) => m[0].length + 1));
  const f = "`".repeat(run);
  return `${f}${lang}\n${code}\n${f}`;
}

function Pic({ src, alt, ctx, className }: { src: string | null; alt: string; ctx: Ctx; className?: string }) {
  if (!src) return <span className={cn("ui-pic-missing", className)}>{L("图片不可用", "Picture unavailable")}</span>;
  return <img className={cn("ui-pic", className)} src={src} alt={alt} loading="lazy" onClick={() => ctx.zoom(src, alt)} />;
}

function Node({ id, ctx, depth = 0 }: { id: string; ctx: Ctx; depth?: number }) {
  const el = ctx.spec.elements[id];
  if (!el || depth > 24) return null;
  return <Element el={el} ctx={ctx} depth={depth} />;
}
function Kids({ el, ctx, depth }: { el: UiElement; ctx: Ctx; depth: number }) {
  return <>{el.children.map((c) => <Node key={c} id={c} ctx={ctx} depth={depth + 1} />)}</>;
}

const STEP_ICON: Record<string, ReactNode> = {
  done: <Check />, doing: <Loader2 className="ui-spin" />, failed: <X />, skipped: <Minus />, todo: <CircleDashed />,
};

function Element({ el, ctx, depth }: { el: UiElement; ctx: Ctx; depth: number }): ReactNode {
  const p = el.props;
  switch (el.type) {
    case "Card":
      return (
        <section className={cn("ui-card", `tone-${tone(p.tone)}`)}>
          {(str(p.title) || str(p.subtitle)) && (
            <header className="ui-card-head">
              {str(p.title) && <h4>{str(p.title)}</h4>}
              {str(p.subtitle) && <p>{str(p.subtitle)}</p>}
            </header>
          )}
          <div className="ui-card-body"><Kids el={el} ctx={ctx} depth={depth} /></div>
        </section>
      );
    case "Stack":
      return <div className={cn("ui-stack", p.direction === "horizontal" && "is-row", `gap-${str(p.gap) || "medium"}`)}><Kids el={el} ctx={ctx} depth={depth} /></div>;
    case "Grid": {
      const cols = Math.min(4, Math.max(1, Math.round(num(p.columns) ?? 2)));
      return <div className="ui-grid" style={{ ["--ui-cols" as string]: cols }}><Kids el={el} ctx={ctx} depth={depth} /></div>;
    }
    case "Tabs":
      return <UiTabs el={el} ctx={ctx} depth={depth} />;
    case "Heading": {
      const level = Math.min(3, Math.max(1, Math.round(num(p.level) ?? 2)));
      return <div className={`ui-heading is-${level}`} role="heading" aria-level={level + 2}>{str(p.text)}</div>;
    }
    case "Text":
      return <div className={cn("ui-text", p.tone === "muted" && "is-muted")}><Md text={str(p.text)} /></div>;
    case "Callout":
      return (
        <div className={cn("ui-callout", `tone-${tone(p.tone ?? "info")}`)}>
          {str(p.title) && <b>{str(p.title)}</b>}
          <Md text={str(p.text)} />
        </div>
      );
    case "Badge":
      return <span className={cn("ui-badge", `tone-${tone(p.tone)}`)}>{str(p.text)}</span>;
    case "KeyValue":
      return (
        <dl className="ui-kv">
          {list(p.items).map(obj).map((it, i) => (
            <div key={i}><dt>{str(it.label)}</dt><dd className={`tone-${tone(it.tone)}`}>{str(it.value)}</dd></div>
          ))}
        </dl>
      );
    case "Table": {
      const cols = list(p.columns).map(str);
      const align = list(p.align).map(str);
      const al = (i: number) => (["left", "center", "right"].includes(align[i]) ? align[i] as "left" | "center" | "right" : undefined);
      return (
        <div className="md ui-table">
          <table>
            <thead><tr>{cols.map((c, i) => <th key={i} align={al(i)}>{c}</th>)}</tr></thead>
            <tbody>{list(p.rows).map((r, i) => <tr key={i}>{list(r).map((c, j) => <td key={j} align={al(j) ?? (typeof c === "number" ? "right" : undefined)}>{str(c)}</td>)}</tr>)}</tbody>
          </table>
        </div>
      );
    }
    case "Metric":
      return (
        <div className="ui-metric">
          <span className="ui-metric-label">{str(p.label)}</span>
          <span className="ui-metric-value">{str(p.value)}</span>
          {str(p.delta) && <span className={cn("ui-metric-delta", `tone-${tone(p.tone)}`)}>{str(p.delta)}</span>}
        </div>
      );
    case "Progress": {
      const v = Math.min(100, Math.max(0, num(p.value) ?? 0));
      return (
        <div className="ui-progress">
          {(str(p.label) || true) && <div className="ui-progress-top"><span>{str(p.label)}</span><span>{Math.round(v)}%</span></div>}
          <div className="ui-progress-track" role="progressbar" aria-valuenow={v} aria-valuemin={0} aria-valuemax={100}>
            <div className={cn("ui-progress-bar", `tone-${tone(p.tone ?? "info")}`)} style={{ width: `${v}%` }} />
          </div>
        </div>
      );
    }
    case "Steps":
      return (
        <ol className="ui-steps">
          {list(p.items).map(obj).map((it, i) => {
            const st = ["done", "doing", "failed", "skipped"].includes(str(it.status)) ? str(it.status) : "todo";
            return (
              <li key={i} className={`is-${st}`}>
                <span className="ui-step-icon" aria-hidden>{STEP_ICON[st]}</span>
                <span className="ui-step-text"><span>{str(it.title)}</span>{str(it.note) && <small>{str(it.note)}</small>}</span>
              </li>
            );
          })}
        </ol>
      );
    case "Image":
      return (
        <figure className="ui-image">
          <Pic src={srcOf(p.src, ctx.files)} alt={str(p.alt) || str(p.caption)} ctx={ctx} />
          {str(p.caption) && <figcaption>{str(p.caption)}</figcaption>}
        </figure>
      );
    case "Gallery": {
      const cols = Math.min(4, Math.max(2, Math.round(num(p.columns) ?? Math.min(3, list(p.items).length || 2))));
      return (
        <div className="ui-gallery" style={{ ["--ui-cols" as string]: cols }}>
          {list(p.items).map(obj).map((it, i) => (
            <figure key={i}>
              <Pic src={srcOf(it.src, ctx.files)} alt={str(it.title)} ctx={ctx} />
              {(str(it.title) || str(it.badge) || str(it.text)) && (
                <figcaption>
                  {str(it.title) && <b>{str(it.title)}</b>}
                  {str(it.badge) && <span className={cn("ui-badge", `tone-${tone(it.tone)}`)}>{str(it.badge)}</span>}
                  {str(it.text) && <span className="ui-muted">{str(it.text)}</span>}
                </figcaption>
              )}
            </figure>
          ))}
        </div>
      );
    }
    case "MediaList":
      return (
        <ul className="ui-media">
          {list(p.items).map(obj).map((it, i) => {
            const href = linkOf(it.href);
            return (
              <li key={i}>
                {it.image !== undefined && <Pic src={srcOf(it.image, ctx.files)} alt={str(it.title)} ctx={ctx} className="ui-media-pic" />}
                <div className="ui-media-body">
                  <div className="ui-media-title">
                    {href ? <a href={href} target="_blank" rel="noopener noreferrer">{str(it.title)}</a> : <b>{str(it.title)}</b>}
                    {list(it.badges).map(str).filter(Boolean).map((b, j) => <span key={j} className="ui-badge">{b}</span>)}
                  </div>
                  {str(it.text) && <div className="ui-media-text"><Md text={str(it.text)} /></div>}
                  {str(it.meta) && <div className="ui-muted">{str(it.meta)}</div>}
                </div>
              </li>
            );
          })}
        </ul>
      );
    case "Diagram":
      return (
        <figure className="ui-diagram">
          <Markdown text={fence(str(p.source), "mermaid")} />
          {str(p.caption) && <figcaption>{str(p.caption)}</figcaption>}
        </figure>
      );
    case "Chart":
      return <UiChart p={p} />;
    case "Code":
      return (
        <div className="ui-code">
          {str(p.title) && <div className="ui-code-title">{str(p.title)}</div>}
          <Md text={fence(str(p.code), str(p.lang).replace(/[^\w+#.-]/g, ""))} />
        </div>
      );
    case "FileChange": {
      const st = ["added", "modified", "deleted", "renamed"].includes(str(p.status)) ? str(p.status) : "modified";
      const add = num(p.additions);
      const del = num(p.deletions);
      const head = (
        <>
          <span className={`ui-file-status is-${st}`}>{{ added: L("新增", "Added"), modified: L("修改", "Modified"), deleted: L("删除", "Deleted"), renamed: L("改名", "Renamed") }[st]}</span>
          <code className="ui-file-path">{str(p.path)}</code>
          <span className="ui-file-count">{add !== null && <span className="is-add">+{add}</span>}{del !== null && <span className="is-del">−{del}</span>}</span>
        </>
      );
      return str(p.diff)
        ? <details className="ui-file"><summary>{head}</summary><Md text={fence(str(p.diff), "diff")} /></details>
        : <div className="ui-file"><div className="ui-file-line">{head}</div></div>;
    }
    case "TestResults": {
      const items = list(p.items).map(obj);
      const order: Record<string, number> = { failed: 0, skipped: 1, passed: 2 };
      items.sort((a, b) => (order[str(a.status)] ?? 3) - (order[str(b.status)] ?? 3));
      const failed = num(p.failed) ?? 0;
      return (
        <div className={cn("ui-tests", failed > 0 ? "tone-danger" : "tone-success")}>
          <div className="ui-tests-sum">
            <span className="is-pass"><Check />{L(E`${num(p.passed) ?? 0} 通过`, E`${num(p.passed) ?? 0} passed`)}</span>
            <span className={cn(failed > 0 && "is-fail")}><X />{L(E`${failed} 失败`, E`${failed} failed`)}</span>
            {num(p.skipped) !== null && <span><Minus />{L(E`${num(p.skipped)} 跳过`, E`${num(p.skipped)} skipped`)}</span>}
            {str(p.duration) && <span className="ui-muted">{str(p.duration)}</span>}
          </div>
          {items.length > 0 && (
            <ul>
              {items.map((it, i) => (
                <li key={i} className={`is-${str(it.status) || "passed"}`}>
                  <span className="ui-test-name">{str(it.name)}</span>
                  {str(it.message) && <pre>{str(it.message)}</pre>}
                </li>
              ))}
            </ul>
          )}
        </div>
      );
    }
    case "Button":
      return <UiButton p={p} />;
    default:
      return <span className="ui-unknown">{L(E`（这里有一个 App 不认识的部分：${el.type}）`, E`(a part this App does not know: ${el.type})`)}</span>;
  }
}

function UiTabs({ el, ctx, depth }: { el: UiElement; ctx: Ctx; depth: number }) {
  const [at, setAt] = useState(0);
  const kids = el.children.filter((c) => ctx.spec.elements[c]);
  if (!kids.length) return null;
  const i = Math.min(at, kids.length - 1);
  return (
    <div className="ui-tabs">
      <div className="seg seg-sm" role="tablist">
        {kids.map((c, j) => (
          <button key={c} type="button" role="tab" aria-selected={j === i} className={cn("seg-btn", j === i && "active")} onClick={() => setAt(j)}>
            {str(ctx.spec.elements[c].props.label) || L(E`第 ${j + 1} 页`, E`Tab ${j + 1}`)}
          </button>
        ))}
      </div>
      <div className="ui-tabs-body" role="tabpanel"><Node id={kids[i]} ctx={ctx} depth={depth + 1} /></div>
    </div>
  );
}

function UiButton({ p }: { p: Record<string, unknown> }) {
  const [done, setDone] = useState(false);
  const label = str(p.label);
  const value = str(p.value);
  const cls = cn("app-btn", p.primary === true && "app-btn-primary", "ui-btn");
  if (p.action === "open") {
    const href = linkOf(value);
    return href
      ? <a className={cls} href={href} target="_blank" rel="noopener noreferrer" title={href}>{label}</a>
      : <button type="button" className={cls} disabled title={L("不是可以打开的网址", "Not an address that can be opened")}>{label}</button>;
  }
  if (p.action === "copy") {
    return (
      <button type="button" className={cls} onClick={() => void copyText(value).then(() => { setDone(true); window.setTimeout(() => setDone(false), 1500); })}>
        {done ? L("已复制", "Copied") : label}
      </button>
    );
  }
  // reply: the words go to your input box, for you to send (or change).
  return (
    <button type="button" className={cls} title={L(E`放进输入框：${value}`, E`Put in the input box: ${value}`)}
      onClick={(e) => e.currentTarget.dispatchEvent(new CustomEvent(UI_REPLY, { bubbles: true, detail: { text: value } }))}>
      {label}
    </button>
  );
}

const PALETTE = ["#4c7bd9", "#e0884a", "#3fa37a", "#c45bb0", "#d4b23c", "#6aa5c8"];

function UiChart({ p }: { p: Record<string, unknown> }) {
  const labels = list(p.labels).map(str);
  const series = list(p.series).map(obj).map((s) => ({ name: str(s.name), values: list(s.values).map((v) => num(v) ?? 0) }));
  const unit = str(p.unit);
  const kind = p.kind === "line" || p.kind === "pie" ? p.kind : "bar";
  const fmt = (v: number) => `${Number.isInteger(v) ? v : v.toFixed(1)}${unit}`;
  const legend = (
    <div className="ui-chart-legend">
      {(kind === "pie" ? labels : series.map((s) => s.name)).map((n, i) => (
        <span key={i}><i style={{ background: PALETTE[i % PALETTE.length] }} />{n}</span>
      ))}
    </div>
  );
  let body: ReactNode;
  if (kind === "pie") {
    const vals = (series[0]?.values || []).map((v) => Math.max(0, v));
    const total = vals.reduce((a, b) => a + b, 0) || 1;
    let a0 = -Math.PI / 2;
    body = (
      <svg viewBox="0 0 200 200" className="ui-chart-pie" role="img">
        {vals.map((v, i) => {
          const a1 = a0 + (v / total) * Math.PI * 2;
          const large = a1 - a0 > Math.PI ? 1 : 0;
          const d = vals.length === 1 || v === total
            ? "M100 10 A90 90 0 1 1 99.99 10 Z"
            : `M100 100 L${100 + 90 * Math.cos(a0)} ${100 + 90 * Math.sin(a0)} A90 90 0 ${large} 1 ${100 + 90 * Math.cos(a1)} ${100 + 90 * Math.sin(a1)} Z`;
          a0 = a1;
          return <path key={i} d={d} fill={PALETTE[i % PALETTE.length]}><title>{`${labels[i] ?? ""}: ${fmt(v)} (${Math.round((v / total) * 100)}%)`}</title></path>;
        })}
      </svg>
    );
  } else {
    const W = 560, H = 220, padL = 40, padB = 26, padT = 10;
    const all = series.flatMap((s) => s.values);
    const max = Math.max(0, ...all);
    const min = Math.min(0, ...all);
    const span = max - min || 1;
    const y = (v: number) => padT + (H - padT - padB) * (1 - (v - min) / span);
    const n = Math.max(1, labels.length);
    const bw = (W - padL) / n;
    const ticks = [min, min + span / 2, max];
    body = (
      <svg viewBox={`0 0 ${W} ${H}`} className="ui-chart-xy" role="img" preserveAspectRatio="none">
        {ticks.map((t, i) => (
          <g key={i}>
            <line x1={padL} x2={W} y1={y(t)} y2={y(t)} className="ui-chart-grid" />
            <text x={padL - 6} y={y(t) + 4} textAnchor="end" className="ui-chart-tick">{fmt(Math.round(t * 10) / 10)}</text>
          </g>
        ))}
        {labels.map((l, i) => <text key={i} x={padL + bw * (i + 0.5)} y={H - 8} textAnchor="middle" className="ui-chart-tick">{l}</text>)}
        {kind === "bar"
          ? series.map((s, si) => s.values.slice(0, n).map((v, i) => {
            const w = (bw * 0.7) / Math.max(1, series.length);
            const x = padL + bw * i + bw * 0.15 + w * si;
            return <rect key={`${si}-${i}`} x={x} width={Math.max(1, w - 2)} y={Math.min(y(v), y(0))} height={Math.abs(y(v) - y(0))} rx={2} fill={PALETTE[si % PALETTE.length]}><title>{`${s.name} · ${labels[i] ?? ""}: ${fmt(v)}`}</title></rect>;
          }))
          : series.map((s, si) => (
            <g key={si}>
              <polyline fill="none" stroke={PALETTE[si % PALETTE.length]} strokeWidth={2} points={s.values.slice(0, n).map((v, i) => `${padL + bw * (i + 0.5)},${y(v)}`).join(" ")} />
              {s.values.slice(0, n).map((v, i) => <circle key={i} cx={padL + bw * (i + 0.5)} cy={y(v)} r={3} fill={PALETTE[si % PALETTE.length]}><title>{`${s.name} · ${labels[i] ?? ""}: ${fmt(v)}`}</title></circle>)}
            </g>
          ))}
      </svg>
    );
  }
  return (
    <figure className={cn("ui-chart", `is-${kind}`)}>
      {str(p.title) && <figcaption>{str(p.title)}</figcaption>}
      {body}
      {(kind === "pie" || series.length > 1) && legend}
    </figure>
  );
}

/** A ```myco-ui block: drawn, or (not JSON, too large) its source and why. */
export function UiBlock({ src, files }: { src: string; files?: InlineFiles }) {
  const parsed = useMemo(() => parseUi(src), [src]);
  const [big, setBig] = useState<{ items: Picture[]; index: number } | null>(null);
  if ("error" in parsed) {
    return (
      <div className="ui-broken">
        <div className="ui-muted">{L(E`这块界面画不出来（${parsed.error}），下面是它的原文：`, E`This part could not be drawn (${parsed.error}); here is its source:`)}</div>
        <Markdown text={fence(src, "json")} />
      </div>
    );
  }
  const ctx: Ctx = {
    spec: parsed.spec,
    files,
    zoom: (s, name) => setBig({ items: [{ name: name || L("图片", "Picture"), src: s }], index: 0 }),
  };
  return (
    <div className="ui-root">
      <Node id={parsed.spec.root} ctx={ctx} />
      {big && <Lightbox items={big.items} index={big.index} onIndex={(i) => setBig({ ...big, index: i })} onClose={() => setBig(null)} />}
    </div>
  );
}
