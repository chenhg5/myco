// Every board mutation is one reversible action, including multi-node drags,
// clipboard graph copies, attached connectors, alignment and layer changes.
import { uid } from "../_lib/appkit";
import { t as uiText } from "../_lib/i18n";
import { items, kit, maxZ, currentBoxes } from "./model";
import {
  bounds,
  cloneGraph,
  copyGraph,
  SIZE,
  type Item,
  type Kind,
  type Entry,
  type Point,
} from "./geometry";
type Change = { id: string; before: Item | null; after: Item | null };
const undos: Change[][] = [],
  redos: Change[][] = [];
export const canUndo = () => !!undos.length;
export const canRedo = () => !!redos.length;
function apply(ops: Change[], direction: "before" | "after") {
  for (const op of ops) {
    const value = op[direction];
    void kit.track<unknown>(
      value
        ? items.put({ ...value }, op.id)
        : items.get(op.id)
          ? items.remove(op.id)
          : null,
    );
  }
  kit.emit();
}
export function change(entries: [string, Item | null][]) {
  const ops = entries
    .filter(
      ([id, data]) =>
        JSON.stringify(items.get(id)?.data ?? null) !== JSON.stringify(data),
    )
    .map(([id, after]) => ({
      id,
      before: items.get(id) ? { ...items.get(id)!.data } : null,
      after,
    }));
  if (!ops.length) return;
  undos.push(ops);
  if (undos.length > 100) undos.shift();
  redos.length = 0;
  apply(ops, "after");
}
export function undo() {
  const ops = undos.pop();
  if (ops) {
    if (!matches(ops, "after")) {
      kit.flash(uiText("这些元素已被其他人修改，未覆盖最新内容"));
      kit.emit();
      return;
    }
    redos.push(ops);
    apply([...ops].reverse(), "before");
  }
}
export function redo() {
  const ops = redos.pop();
  if (ops) {
    if (!matches(ops, "before")) {
      kit.flash(uiText("这些元素已被其他人修改，未覆盖最新内容"));
      kit.emit();
      return;
    }
    undos.push(ops);
    apply(ops, "after");
  }
}
function matches(ops: Change[], direction: "before" | "after") {
  return ops.every((op) => {
    const current = items.get(op.id)?.data ?? null,
      expected = op[direction];
    if (!current || !expected) return current === expected;
    return Object.keys({ ...current, ...expected }).every(
      (key) => JSON.stringify(current[key]) === JSON.stringify(expected[key]),
    );
  });
}
export function patchMany(ids: string[], data: Item) {
  change(
    ids.flatMap((id) => {
      const r = items.get(id);
      return r && (!r.data.locked || data.locked !== undefined)
        ? [[id, { ...r.data, ...data }] as [string, Item]]
        : [];
    }),
  );
}
export function create(type: Kind, p: Point, extra: Item = {}) {
  const id = uid("i"),
    [w, h] = SIZE[type];
  change([
    [
      id,
      {
        type,
        x: Math.round(p.x - w / 2),
        y: Math.round(p.y - h / 2),
        w,
        h,
        text: "",
        color: "gray",
        z: maxZ() + 1,
        ...extra,
      },
    ],
  ]);
  return id;
}
export function connect(from: string, to: string, extra: Item = {}) {
  const id = uid("a");
  change([
    [id, { type: "arrow", from, to, color: "gray", route: "elbow", ...extra }],
  ]);
  return id;
}
export function remove(ids: string[]) {
  const gone = new Set(ids.filter((id) => !items.get(id)?.data.locked));
  for (const r of items.rows)
    if (
      r.data.type === "arrow" &&
      (gone.has(String(r.data.from)) || gone.has(String(r.data.to)))
    )
      gone.add(r.id);
  change([...gone].map((id) => [id, null]));
}
export function selectedGraph(ids: string[]): Entry[] {
  const bs = new Map(currentBoxes().map((b) => [b.id, b]));
  return copyGraph(items.rows, ids).map((r) => ({
    id: r.id,
    data: {
      ...r.data,
      ...(bs.has(r.id)
        ? { type: bs.get(r.id)!.type, x: bs.get(r.id)!.x, y: bs.get(r.id)!.y }
        : {}),
    },
  }));
}
export function duplicate(rows: Entry[], offset: Point = { x: 32, y: 32 }) {
  const z = maxZ();
  const next = cloneGraph(rows, () => uid("i"), offset).map((r, i) => ({
    ...r,
    data: { ...r.data, z: z + i + 1 },
  }));
  change(next.map((r) => [r.id, r.data]));
  return next.map((r) => r.id);
}
export function pasteText(text: string, p: Point) {
  const value = text.trim().slice(0, 12000);
  return value
    ? [
        create("sticky", p, {
          text: value,
          h: Math.min(
            480,
            Math.max(164, Math.ceil(value.length / 20) * 24 + 32),
          ),
        }),
      ]
    : [];
}
export function align(ids: string[], mode: string) {
  const bs = currentBoxes().filter((b) => ids.includes(b.id) && !b.locked);
  if (bs.length < 2) return;
  const area = bounds(bs);
  change(
    bs.map((b) => [
      b.id,
      {
        ...items.get(b.id)!.data,
        ...(mode === "left"
          ? { x: area.x }
          : mode === "right"
            ? { x: area.x + area.w - b.w }
            : mode === "center"
              ? { x: area.x + (area.w - b.w) / 2 }
              : mode === "top"
                ? { y: area.y }
                : mode === "bottom"
                  ? { y: area.y + area.h - b.h }
                  : { y: area.y + (area.h - b.h) / 2 }),
      },
    ]),
  );
}
export function distribute(ids: string[], axis: "x" | "y") {
  const bs = currentBoxes()
    .filter((b) => ids.includes(b.id) && !b.locked)
    .sort((a, b) => a[axis] - b[axis]);
  if (bs.length < 3) {
    kit.flash(uiText("至少选择三个元素"));
    return;
  }
  const size = axis === "x" ? "w" : "h",
    first = bs[0][axis],
    end = bs[bs.length - 1][axis] + bs[bs.length - 1][size],
    gap = (end - first - bs.reduce((n, b) => n + b[size], 0)) / (bs.length - 1);
  let cursor = first;
  change(
    bs.map((b) => {
      const pos = cursor;
      cursor += b[size] + gap;
      return [b.id, { ...items.get(b.id)!.data, [axis]: pos }];
    }),
  );
}
export function layer(ids: string[], front: boolean) {
  let z = front
    ? maxZ() + 1
    : Math.min(0, ...items.rows.map((r) => Number(r.data.z) || 0)) - ids.length;
  change(
    ids.flatMap((id) => {
      const r = items.get(id);
      return r ? [[id, { ...r.data, z: z++ }] as [string, Item]] : [];
    }),
  );
}
