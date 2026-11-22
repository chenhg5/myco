// Pure board geometry, connector routing and clipboard graph transforms.
// Records keep stable node IDs; connectors attach to edges, never fixed pixels.
export type Kind = "sticky" | "text" | "rect" | "ellipse" | "arrow";
export type Side = "top" | "right" | "bottom" | "left";
export type Point = { x: number; y: number };
export type Item = {
  type?: Kind;
  x?: number;
  y?: number;
  w?: number;
  h?: number;
  text?: string;
  color?: string;
  from?: string;
  to?: string;
  z?: number;
  from_side?: string;
  to_side?: string;
  route?: string;
  dash?: string;
  width?: number;
  font_size?: number;
  align?: string;
  locked?: boolean;
} & Record<string, unknown>;
export type Entry = { id: string; data: Item };
export type Box = {
  id: string;
  type: Kind;
  x: number;
  y: number;
  w: number;
  h: number;
  text: string;
  color: string;
  z: number;
  font_size: number;
  align: string;
  locked: boolean;
};
export const SIZE: Record<Kind, [number, number]> = {
  sticky: [216, 164],
  text: [280, 48],
  rect: [216, 104],
  ellipse: [184, 112],
  arrow: [0, 0],
};
export const SIDES: Side[] = ["top", "right", "bottom", "left"];
export const number = (v: unknown, fallback = 0) =>
  typeof v === "number" && Number.isFinite(v)
    ? Math.max(-1e6, Math.min(1e6, v))
    : fallback;
export function boxes(rows: Entry[]): Box[] {
  let free = 0;
  return rows
    .filter((r) => r.data.type !== "arrow")
    .map(({ id, data: d }) => {
      const type = (
          d.type &&
          Object.prototype.hasOwnProperty.call(SIZE, d.type) &&
          d.type !== "arrow"
            ? d.type
            : "sticky"
        ) as Kind,
        [w, h] = SIZE[type];
      const i = free++;
      return {
        id,
        type,
        x: number(d.x, 40 + (i % 4) * 248),
        y: number(d.y, 40 + Math.floor(i / 4) * 204),
        w: Math.min(5000, Math.max(60, number(d.w, w))),
        h: Math.min(5000, Math.max(32, number(d.h, h))),
        text: String(d.text || ""),
        color: String(d.color || "gray"),
        z: number(d.z),
        font_size: Math.max(
          12,
          Math.min(64, number(d.font_size, type === "text" ? 24 : 16)),
        ),
        align: String(
          d.align ||
            (type === "rect" || type === "ellipse" ? "center" : "left"),
        ),
        locked: !!d.locked,
      };
    })
    .sort((a, b) => a.z - b.z);
}
export function bounds(bs: Pick<Box, "x" | "y" | "w" | "h">[]) {
  if (!bs.length) return { x: 0, y: 0, w: 0, h: 0 };
  const x = Math.min(...bs.map((b) => b.x)),
    y = Math.min(...bs.map((b) => b.y));
  return {
    x,
    y,
    w: Math.max(...bs.map((b) => b.x + b.w)) - x,
    h: Math.max(...bs.map((b) => b.y + b.h)) - y,
  };
}
export function port(b: Pick<Box, "x" | "y" | "w" | "h">, side: Side): Point {
  return {
    x: b.x + (side === "left" ? 0 : side === "right" ? b.w : b.w / 2),
    y: b.y + (side === "top" ? 0 : side === "bottom" ? b.h : b.h / 2),
  };
}
export function nearPort(
  bs: Box[],
  p: Point,
  exclude = "",
  radius = 28,
): { box: Box; side: Side } | null {
  let best: { box: Box; side: Side } | null = null,
    distance = radius;
  for (const b of [...bs].reverse()) {
    if (b.id === exclude) continue;
    for (const side of SIDES) {
      const a = port(b, side),
        d = Math.hypot(p.x - a.x, p.y - a.y);
      if (d < distance) {
        distance = d;
        best = { box: b, side };
      }
    }
  }
  if (best) return best;
  const box = [...bs]
    .reverse()
    .find(
      (b) =>
        b.id !== exclude &&
        p.x >= b.x &&
        p.x <= b.x + b.w &&
        p.y >= b.y &&
        p.y <= b.y + b.h,
    );
  if (!box) return null;
  return {
    box,
    side: SIDES.reduce((a, s) =>
      Math.hypot(p.x - port(box, s).x, p.y - port(box, s).y) <
      Math.hypot(p.x - port(box, a).x, p.y - port(box, a).y)
        ? s
        : a,
    ),
  };
}
const vector: Record<Side, Point> = {
  top: { x: 0, y: -1 },
  right: { x: 1, y: 0 },
  bottom: { x: 0, y: 1 },
  left: { x: -1, y: 0 },
};
const distance = (a: Point, b: Point) => Math.hypot(a.x - b.x, a.y - b.y);
function intersects(a: Point, b: Point, r: Box) {
  return a.x === b.x
    ? a.x > r.x + 1 &&
        a.x < r.x + r.w - 1 &&
        Math.max(a.y, b.y) > r.y + 1 &&
        Math.min(a.y, b.y) < r.y + r.h - 1
    : a.y === b.y &&
        a.y > r.y + 1 &&
        a.y < r.y + r.h - 1 &&
        Math.max(a.x, b.x) > r.x + 1 &&
        Math.min(a.x, b.x) < r.x + r.w - 1;
}
export function connector(
  from: Box,
  to: Box,
  data: Item,
  obstacles: Box[] = [from, to],
) {
  const dx = to.x + to.w / 2 - from.x - from.w / 2,
    dy = to.y + to.h / 2 - from.y - from.h / 2;
  const horizontal =
    Math.abs(dx) / ((from.w + to.w) / 2 + 24) >=
    Math.abs(dy) / ((from.h + to.h) / 2 + 24);
  const fs = SIDES.includes(data.from_side as Side)
    ? (data.from_side as Side)
    : horizontal
      ? dx >= 0
        ? "right"
        : "left"
      : dy >= 0
        ? "bottom"
        : "top";
  const ts = SIDES.includes(data.to_side as Side)
    ? (data.to_side as Side)
    : horizontal
      ? dx >= 0
        ? "left"
        : "right"
      : dy >= 0
        ? "top"
        : "bottom";
  const a = port(from, fs),
    b = port(to, ts),
    av = vector[fs],
    bv = vector[ts];
  if (data.route === "straight")
    return {
      path: `M${a.x},${a.y} L${b.x},${b.y}`,
      label: { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 },
      a,
      b,
    };
  if (data.route === "curve") {
    const k = Math.max(48, distance(a, b) * 0.45),
      c = { x: a.x + av.x * k, y: a.y + av.y * k },
      d = { x: b.x + bv.x * k, y: b.y + bv.y * k };
    return {
      path: `M${a.x},${a.y} C${c.x},${c.y} ${d.x},${d.y} ${b.x},${b.y}`,
      label: {
        x: (a.x + 3 * c.x + 3 * d.x + b.x) / 8,
        y: (a.y + 3 * c.y + 3 * d.y + b.y) / 8,
      },
      a,
      b,
    };
  }
  const u = { x: a.x + av.x * 24, y: a.y + av.y * 24 },
    v = { x: b.x + bv.x * 24, y: b.y + bv.y * 24 };
  const candidates: Point[][] = [
    [a, u, { x: u.x, y: v.y }, v, b],
    [a, u, { x: v.x, y: u.y }, v, b],
  ];
  const area = bounds(obstacles);
  for (const x of [(u.x + v.x) / 2, area.x - 32, area.x + area.w + 32])
    candidates.push([a, u, { x, y: u.y }, { x, y: v.y }, v, b]);
  for (const y of [(u.y + v.y) / 2, area.y - 32, area.y + area.h + 32])
    candidates.push([a, u, { x: u.x, y }, { x: v.x, y }, v, b]);
  const cost = (ps: Point[]) =>
    ps
      .slice(1)
      .reduce(
        (n, p, i) =>
          n +
          distance(ps[i], p) +
          obstacles.filter((o) => intersects(ps[i], p, o)).length * 1e7,
        0,
      );
  let ps = candidates
    .reduce((best, p) => (cost(p) < cost(best) ? p : best))
    .filter((p, i, all) => !i || distance(p, all[i - 1]) > 0.1);
  ps = ps.filter(
    (p, i, all) =>
      !i ||
      i === all.length - 1 ||
      !(
        (all[i - 1].x === p.x &&
          p.x === all[i + 1].x &&
          (p.y - all[i - 1].y) * (all[i + 1].y - p.y) >= 0) ||
        (all[i - 1].y === p.y &&
          p.y === all[i + 1].y &&
          (p.x - all[i - 1].x) * (all[i + 1].x - p.x) >= 0)
      ),
  );
  const length = ps.slice(1).reduce((n, p, i) => n + distance(ps[i], p), 0);
  let remaining = length / 2,
    label = a;
  for (let i = 1; i < ps.length; i++) {
    const d = distance(ps[i - 1], ps[i]);
    if (remaining <= d) {
      label = {
        x: ps[i - 1].x + ((ps[i].x - ps[i - 1].x) * remaining) / (d || 1),
        y: ps[i - 1].y + ((ps[i].y - ps[i - 1].y) * remaining) / (d || 1),
      };
      break;
    }
    remaining -= d;
  }
  return {
    path: ps.map((p, i) => `${i ? "L" : "M"}${p.x},${p.y}`).join(" "),
    label,
    a,
    b,
  };
}
export function copyGraph(rows: Entry[], ids: string[]) {
  const chosen = new Set(ids);
  for (const r of rows)
    if (chosen.has(r.id) && r.data.type === "arrow") {
      chosen.add(String(r.data.from));
      chosen.add(String(r.data.to));
    }
  return rows
    .filter(
      (r) =>
        chosen.has(r.id) ||
        (r.data.type === "arrow" &&
          chosen.has(String(r.data.from)) &&
          chosen.has(String(r.data.to))),
    )
    .map((r) => ({ id: r.id, data: { ...r.data } }));
}
export function cloneGraph(
  rows: Entry[],
  makeID: () => string,
  offset: Point,
): Entry[] {
  const ids = new Map(rows.map((r) => [r.id, makeID()]));
  return rows
    .filter(
      (r) =>
        r.data.type !== "arrow" ||
        (ids.has(String(r.data.from)) && ids.has(String(r.data.to))),
    )
    .map((r) => ({
      id: ids.get(r.id)!,
      data:
        r.data.type === "arrow"
          ? {
              ...r.data,
              from: ids.get(String(r.data.from)),
              to: ids.get(String(r.data.to)),
              locked: false,
            }
          : {
              ...r.data,
              x: number(r.data.x) + offset.x,
              y: number(r.data.y) + offset.y,
              locked: false,
            },
    }));
}
export const clipboardPrefix = "myco-whiteboard:1\n";
export function parseClipboard(text: string): Entry[] | null {
  if (!text.startsWith(clipboardPrefix) || text.length > 1_000_000) return null;
  try {
    const rows = JSON.parse(text.slice(clipboardPrefix.length));
    if (!Array.isArray(rows) || !rows.length || rows.length > 500) return null;
    const ids = new Set<string>();
    for (const r of rows) {
      if (
        !r ||
        typeof r.id !== "string" ||
        r.id.length > 128 ||
        ids.has(r.id) ||
        !r.data ||
        typeof r.data !== "object" ||
        !Object.prototype.hasOwnProperty.call(SIZE, r.data.type) ||
        JSON.stringify(r.data).length > 50_000
      )
        return null;
      for (const key of ["x", "y", "w", "h", "z", "width", "font_size"])
        if (
          r.data[key] !== undefined &&
          (typeof r.data[key] !== "number" ||
            !Number.isFinite(r.data[key]) ||
            Math.abs(r.data[key]) > 1e6)
        )
          return null;
      for (const key of [
        "text",
        "color",
        "from",
        "to",
        "from_side",
        "to_side",
        "route",
        "dash",
        "align",
      ])
        if (
          r.data[key] !== undefined &&
          (typeof r.data[key] !== "string" ||
            r.data[key].length > (key === "text" ? 12000 : 128))
        )
          return null;
      if (r.data.locked !== undefined && typeof r.data.locked !== "boolean")
        return null;
      ids.add(r.id);
    }
    // Copy only the documented fields, never arbitrary prototypes or HTML.
    const fields = [
      "type",
      "x",
      "y",
      "w",
      "h",
      "text",
      "color",
      "from",
      "to",
      "z",
      "from_side",
      "to_side",
      "route",
      "dash",
      "width",
      "font_size",
      "align",
      "locked",
    ];
    return rows.map((r) => ({
      id: r.id,
      data: Object.fromEntries(
        fields
          .filter((k) => Object.prototype.hasOwnProperty.call(r.data, k))
          .map((k) => [k, r.data[k]]),
      ),
    }));
  } catch {
    return null;
  }
}

export const labelWidth = (text: string) =>
  Math.min(
    380,
    Array.from(text).reduce((n, c) => n + (c.charCodeAt(0) > 255 ? 13 : 7), 20),
  );
