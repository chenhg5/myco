import { useMemo } from "react";
import { Bot, Check, Monitor, UserCheck } from "lucide-react";
import type { RunStep, WfEdgeDef, WfStepDef, WorkflowDef } from "@/api";
import type { AgentRow } from "@/app/types";
import { nodeName } from "@/app/nodes";
import { Avatar } from "@/app/persona";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/**
 * A workflow drawn top to bottom: each step a card (who does it), arrows
 * for the edges. Steps sit in rows by how far they are from the start;
 * an edge back to an earlier step (a loop) runs round the right side,
 * dashed, with how many times it may be taken. With a run, each card shows
 * how its step went (green doing, amber waiting for you, red failed).
 */

const W = 184;
const H = 56;
const GX = 22;
const GY = 46;
const PAD = 8;

type Kind = "agent" | "computer" | "human" | "end" | "fail";
type GNode = { id: string; step?: WfStepDef; kind: Kind; layer: number; x: number; y: number };
type GEdge = { from: string; to: string; back: boolean; skip?: boolean; label: string; onError?: boolean; outK?: number; outN?: number; inK?: number; inN?: number };

export function edgesOf(def: WorkflowDef): WfEdgeDef[] {
  if (def.edges && def.edges.length) return def.edges;
  return def.steps.slice(0, -1).map((s, i) => ({ from: s.id, to: def.steps[i + 1].id }));
}

export function stepKind(s: WfStepDef): "agent" | "computer" | "human" {
  return s.computer ? "computer" : s.human ? "human" : "agent";
}

/** A condition as a person reads it: 选「合并」, 没通过, else the expression. */
function whenWords(when: string): string {
  const w = when.replace(/\s+/g, " ").trim();
  let m = w.match(/^output\.choice ?== ?['"](.+)['"]$/);
  if (m) return L(`选「${m[1]}」`, `chose "${m[1]}"`);
  m = w.match(/^output\.choice ?!= ?['"](.+)['"]$/);
  if (m) return L(`没选「${m[1]}」`, `not "${m[1]}"`);
  if (w === "output.approved") return L("通过", "approved");
  if (w === "!output.approved") return L("没通过", "not approved");
  const short = w.replace(/\boutput\./g, "");
  return short.length > 22 ? `${short.slice(0, 21)}…` : short;
}

function edgeLabel(e: WfEdgeDef): string {
  const parts: string[] = [];
  if (e.onError) parts.push(L("出错时", "on error"));
  if (e.when) parts.push(whenWords(e.when));
  if (e.maxIterations) parts.push(L(`最多 ${e.maxIterations} 次`, `≤${e.maxIterations}×`));
  if (e.map) parts.push(`{${Object.keys(e.map).join(", ")}}`);
  return parts.join(" · ");
}

function layout(def: WorkflowDef) {
  const idx = new Map(def.steps.map((s, i) => [s.id, i]));
  const edges = [...edgesOf(def)];
  // A step with no way on when it succeeds ends its branch: draw that too.
  for (const s of def.steps) if (!edges.some((e) => e.from === s.id && !e.onError)) edges.push({ from: s.id, to: "$end" });
  const layer = new Map<string, number>();
  for (const s of def.steps) {
    let l = layer.get(s.id) ?? 0;
    for (const e of edges) {
      if (e.to === s.id && (idx.get(e.from) ?? 0) < (idx.get(s.id) ?? 0)) l = Math.max(l, (layer.get(e.from) ?? 0) + 1);
    }
    layer.set(s.id, l);
  }
  const ends: Kind[] = [];
  for (const e of edges) {
    for (const t of [e.to, e.else]) {
      if (t === "$end" && !ends.includes("end")) ends.push("end");
      if (t === "$fail" && !ends.includes("fail")) ends.push("fail");
    }
  }
  const last = Math.max(0, ...layer.values());
  const nodes: GNode[] = def.steps.map((s) => ({ id: s.id, step: s, kind: stepKind(s), layer: layer.get(s.id) ?? 0, x: 0, y: 0 }));
  for (const k of ends) nodes.push({ id: k === "end" ? "$end" : "$fail", kind: k, layer: last + 1, x: 0, y: 0 });
  const layerOf = (id: string) => (id === "$end" || id === "$fail" ? last + 1 : layer.get(id) ?? 0);
  const gedges: GEdge[] = [];
  for (const e of edges) {
    const back = idx.has(e.to) && (idx.get(e.to) ?? 0) <= (idx.get(e.from) ?? 0);
    gedges.push({ from: e.from, to: e.to, back, label: edgeLabel(e), onError: e.onError });
    if (e.else) gedges.push({ from: e.from, to: e.else, back: false, label: L("到上限后", "after the limit") });
  }
  // Several edges out of (or into) one card leave (arrive) side by side;
  // one that skips a row goes round the left (so it does not count here).
  let skips = false;
  for (const e of gedges) {
    if (e.back) continue;
    e.skip = layerOf(e.to) - layerOf(e.from) > 1;
    skips ||= e.skip;
  }
  const rows = new Map<number, GNode[]>();
  for (const n of nodes) rows.set(n.layer, [...(rows.get(n.layer) || []), n]);
  const widest = Math.max(...[...rows.values()].map((r) => r.length));
  const backs = gedges.filter((e) => e.back).length;
  const left = skips ? 76 : 0;
  const inner = widest * (W + GX) - GX;
  const width = left + inner + PAD * 2 + (backs ? 40 + backs * 14 : 0);
  for (const [l, row] of rows) {
    const span = row.length * (W + GX) - GX;
    row.forEach((n, i) => {
      n.x = PAD + left + (inner - span) / 2 + i * (W + GX);
      n.y = PAD + l * (H + GY);
    });
  }
  const height = PAD * 2 + (Math.max(...nodes.map((n) => n.layer)) + 1) * (H + GY) - GY;
  const right = PAD + left + inner;
  // Ends in the order of what is at the other end, so they do not cross.
  const xOf = new Map(nodes.map((n) => [n.id, n.x]));
  const fwd = gedges.filter((e) => !e.back);
  const outs = fwd.filter((e) => !e.skip).sort((p, q) => (xOf.get(p.to) ?? 0) - (xOf.get(q.to) ?? 0));
  const ins = [...fwd].sort((p, q) => (xOf.get(p.from) ?? 0) - (xOf.get(q.from) ?? 0) || (p.skip ? -1 : 0));
  const count = new Map<string, number>();
  for (const e of outs) { e.outK = count.get(`o${e.from}`) || 0; count.set(`o${e.from}`, e.outK + 1); }
  for (const e of ins) { e.inK = count.get(`i${e.to}`) || 0; count.set(`i${e.to}`, e.inK + 1); }
  for (const e of fwd) { e.outN = count.get(`o${e.from}`) || 1; e.inN = count.get(`i${e.to}`) || 1; }
  return { nodes, edges: gedges, width, height, right };
}

export type StepState = { status: RunStep["status"]; attempts: number };

/** The latest state of each step in a run. */
export function stepStates(steps: RunStep[] = []): Map<string, StepState> {
  const out = new Map<string, StepState>();
  for (const s of steps) out.set(s.step, { status: s.status, attempts: (out.get(s.step)?.attempts || 0) + 1 });
  return out;
}

export function WorkflowGraph({ def, hubId, rows, states, selected, onSelect }: {
  def: WorkflowDef; hubId: string; rows: AgentRow[];
  /** a run's step states (none: just the plan) */
  states?: Map<string, StepState>;
  selected?: string;
  onSelect?: (step: string) => void;
}) {
  const g = useMemo(() => layout(def), [def]);
  const at = new Map(g.nodes.map((n) => [n.id, n]));
  const rowOf = (name?: string) => (name ? rows.find((r) => r.hub.id === hubId && r.st.agent.name === name) : undefined);
  let backN = 0;
  return (
    <div className="wf-graph-scroll" data-testid="workflow-graph">
      <div className="wf-graph" style={{ width: g.width, height: g.height }}>
        <svg className="wf-graph-lines" width={g.width} height={g.height} aria-hidden>
          <defs>
            <marker id="wf-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
              <path d="M0 0 L10 5 L0 10 z" />
            </marker>
          </defs>
          {g.edges.map((e, i) => {
            const a = at.get(e.from);
            const b = at.get(e.to);
            if (!a || !b) return null;
            let d: string;
            let lx: number;
            let ly: number;
            if (e.back) {
              backN += 1;
              const sx = a.x + W;
              const sy = a.y + H / 2;
              const tx = b.x + W;
              const ty = b.y + H / 2;
              const out = g.right + 24 + backN * 14;
              d = `M${sx} ${sy} C${out} ${sy}, ${out} ${ty}, ${tx + 2} ${ty}`;
              lx = out - 4;
              ly = (sy + ty) / 2;
            } else {
              const span = (k = 0, n = 1, w = W) => (n <= 1 ? w / 2 : w * 0.2 + (w * 0.6 * k) / (n - 1));
              const tw = b.kind === "end" || b.kind === "fail" ? 80 : W;
              const sx = a.x + span(e.outK, e.outN);
              const sy = a.y + H;
              const tx = b.x + (tw === W ? 0 : W / 2 - 40) + span(e.inK, e.inN, tw);
              const ty = b.y - 2;
              if (e.skip) {
                // It skips a row: round the left side, not through the cards between.
                const left = Math.min(a.x, b.x) - 18;
                const y0 = a.y + H / 2;
                d = `M${a.x} ${y0} C${left} ${y0}, ${left} ${ty - 30}, ${tx} ${ty}`;
                lx = left + 4;
                ly = (y0 + ty) / 2;
              } else {
                const mid = (sy + ty) / 2;
                d = `M${sx} ${sy} C${sx} ${mid}, ${tx} ${mid}, ${tx} ${ty}`;
                // nearer the target, so labels of edges leaving one card part
                lx = sx + (tx - sx) * 0.72;
                ly = sy + (ty - sy) * 0.6;
              }
            }
            return (
              <g key={i} className={cn("wf-edge", e.back && "is-back", e.onError && "is-error", e.to === "$fail" && "is-fail")}>
                <path d={d} markerEnd="url(#wf-arrow)" />
                {e.label && <text x={lx} y={ly} textAnchor={e.back || e.skip ? "end" : "middle"} dominantBaseline="middle">{e.label}</text>}
              </g>
            );
          })}
        </svg>
        {g.nodes.map((n) => {
          if (n.kind === "end" || n.kind === "fail") {
            return (
              <div key={n.id} className={cn("wf-term", n.kind === "fail" && "is-fail")} style={{ left: n.x + W / 2 - 40, top: n.y + H / 2 - 13 }}>
                {n.kind === "end" ? L("完成", "Done") : L("失败", "Failed")}
              </div>
            );
          }
          const s = n.step!;
          const st = states?.get(n.id);
          const target = n.kind === "agent" ? s.agent : n.kind === "computer" ? s.computer : undefined;
          const r = rowOf(target);
          const who = n.kind === "human"
            ? (s.human === "$starter" ? L("你", "You") : s.human || "")
            : s.role ? L(`分工：${s.role}`, `Role: ${s.role}`)
              : r ? nodeName(r.st.agent, r.hub) : target || "";
          const tone = !states ? "" : !st ? "is-todo" : `is-${st.status}`;
          return (
            <button key={n.id} type="button" className={cn("wf-node", tone, selected === n.id && "is-selected")}
              style={{ left: n.x, top: n.y, width: W, height: H }} onClick={() => onSelect?.(n.id)} disabled={!onSelect}
              title={s.title ? `${s.title} (${s.id})` : s.id} data-step={n.id}>
              <span className="wf-node-icon">
                {n.kind === "agent" && r ? <Avatar agent={r.st.agent} size={22} badge={false} />
                  : n.kind === "agent" ? <Bot /> : n.kind === "computer" ? <Monitor /> : <UserCheck />}
              </span>
              <span className="wf-node-text">
                <span className="wf-node-name">{s.title || s.id}</span>
                <span className="wf-node-who">{who}</span>
              </span>
              {st && (
                <span className="wf-node-state" aria-label={stateWords(st.status)}>
                  {st.status === "done" ? <Check /> : <i />}
                  {st.attempts > 1 && <small>×{st.attempts}</small>}
                </span>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}

export function stateWords(s: RunStep["status"]): string {
  return s === "running" ? L("在做", "Working")
    : s === "waiting" ? L("等你决定", "Waiting for you")
      : s === "done" ? L("做完了", "Done")
        : s === "failed" ? L("失败了", "Failed")
          : L("没做完就停了", "Stopped");
}
