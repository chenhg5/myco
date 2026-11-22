// Transient editor state and gestures: selection, viewport, pointer capture,
// snapping, keyboard and clipboard. Persistent edits go through edit.ts.
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type PointerEvent as RPE,
} from "react";
import { themeMedia } from "../_lib/appearance";
import { t as uiText } from "../_lib/i18n";
import { save } from "../_lib/myco";
import { uid, useKit } from "../_lib/appkit";
import { rectOf } from "../_lib/ui";
import { items, kit, title, currentBoxes, maxZ } from "./model";
import {
  bounds,
  nearPort,
  SIZE,
  number,
  type Box,
  type Entry,
  type Item,
  type Point,
  type Side,
} from "./geometry";
import {
  undo,
  redo,
  change,
  create,
  connect,
  remove,
  duplicate,
  patchMany,
  selectedGraph,
  pasteText,
} from "./edit";
import * as clipboard from "./clipboard";
import { svgDocument } from "./export";
import { tools, type Tool } from "./panels";
type View = { x: number; y: number; k: number };
type Draft = Record<string, Partial<Box>>;
type Link = {
  from: string;
  side?: Side;
  x: number;
  y: number;
  reconnect?: { id: string; end: "from" | "to" };
};
type Operation = {
  kind: "move" | "resize" | "pan" | "select" | "link";
  pointer: number;
  sx: number;
  sy: number;
  view: View;
  start: Box[];
  moved: boolean;
  add?: string[];
};
export const editable = (target: EventTarget | null) =>
  target instanceof HTMLElement &&
  (target.isContentEditable || !!target.closest("input,textarea,select"));
export function useBoard() {
  useKit(kit);
  const [tool, setTool] = useState<Tool>("select"),
    [view, setView] = useState<View>({ x: 0, y: 0, k: 1 }),
    [sel, setSel] = useState<string[]>([]),
    [editing, setEditing] = useState<string | null>(null),
    [draft, setDraft] = useState<Draft>({}),
    [link, setLink] = useState<Link | null>(null),
    [marquee, setMarquee] = useState<Box | null>(null),
    [guides, setGuides] = useState<{ axis: "x" | "y"; pos: number }[]>([]),
    [menu, setMenu] = useState<{ x: number; y: number } | null>(null),
    [pastePrompt, setPastePrompt] = useState(false),
    [help, setHelp] = useState<ReturnType<typeof rectOf> | null>(null),
    [dark, setDark] = useState(!!themeMedia()?.matches),
    [space, setSpace] = useState(false);
  const stage = useRef<HTMLDivElement>(null),
    op = useRef<Operation | null>(null),
    draftRef = useRef<Draft>({}),
    linkRef = useRef<Link | null>(null),
    touched = useRef(false),
    pointers = useRef(new Map<number, Point>()),
    pinch = useRef<{ distance: number; view: View; world: Point } | null>(null),
    pointer = useRef<Point | null>(null),
    pasteCount = useRef(0);
  const list = currentBoxes(),
    byId = new Map(list.map((b) => [b.id, b])),
    arrows = items.rows.filter((r) => r.data.type === "arrow"),
    selected = sel.filter((id) => !!items.get(id)),
    selectionData =
      selected.length === 1 ? items.get(selected[0])?.data : undefined;
  const at = (b: Box) => ({ ...b, ...draft[b.id] });
  const updateDraft = (v: Draft) => {
    draftRef.current = v;
    setDraft(v);
  };
  const updateLink = (v: Link | null) => {
    linkRef.current = v;
    setLink(v);
  };
  const world = useCallback(
    (x: number, y: number) => {
      const r = stage.current!.getBoundingClientRect();
      return {
        x: (x - r.left - view.x) / view.k,
        y: (y - r.top - view.y) / view.k,
      };
    },
    [view],
  );
  const center = () => {
    const r = stage.current!.getBoundingClientRect();
    return world(r.left + r.width / 2, r.top + r.height / 2);
  };
  const fit = useCallback(() => {
    const el = stage.current,
      bs = currentBoxes();
    if (!el) return;
    const b = bounds(bs),
      k = bs.length
        ? Math.min(
            1,
            Math.max(
              0.1,
              Math.min(
                (el.clientWidth - 112) / (b.w || 1),
                (el.clientHeight - 180) / (b.h || 1),
              ),
            ),
          )
        : 1;
    setView({
      k,
      x: (el.clientWidth - b.w * k) / 2 - b.x * k,
      y: (el.clientHeight - b.h * k) / 2 - b.y * k,
    });
  }, []);
  useEffect(() => {
    fit();
    const ro = new ResizeObserver(() => {
      if (!touched.current) fit();
    });
    ro.observe(stage.current!);
    const mq = themeMedia(),
      f = () => setDark(!!mq?.matches);
    mq?.addEventListener("change", f);
    return () => {
      ro.disconnect();
      mq?.removeEventListener("change", f);
    };
  }, [fit]);
  const zoom = (factor: number, p?: Point) => {
    touched.current = true;
    const r = stage.current!.getBoundingClientRect(),
      px = p ? p.x - r.left : r.width / 2,
      py = p ? p.y - r.top : r.height / 2;
    setView((v) => {
      const k = Math.max(0.1, Math.min(4, v.k * factor));
      return {
        k,
        x: px - ((px - v.x) * k) / v.k,
        y: py - ((py - v.y) * k) / v.k,
      };
    });
  };
  useEffect(() => {
    const el = stage.current!;
    const wheel = (e: WheelEvent) => {
      if ((e.target as Element).closest(".wb-chrome")) return;
      e.preventDefault();
      touched.current = true;
      if (e.ctrlKey || e.metaKey)
        zoom(Math.exp(-e.deltaY * 0.01), { x: e.clientX, y: e.clientY });
      else setView((v) => ({ ...v, x: v.x - e.deltaX, y: v.y - e.deltaY }));
    };
    el.addEventListener("wheel", wheel, { passive: false });
    return () => el.removeEventListener("wheel", wheel);
  }, []);
  const selectTool = (next: Tool) => {
    setTool(next);
    updateLink(null);
    setMenu(null);
  };
  const startLink = (
    e: RPE,
    id: string,
    side?: Side,
    reconnect?: Link["reconnect"],
  ) => {
    e.stopPropagation();
    const p = world(e.clientX, e.clientY);
    updateLink({ from: id, side, ...p, reconnect });
    op.current = {
      kind: "link",
      pointer: e.pointerId,
      sx: e.clientX,
      sy: e.clientY,
      view,
      start: [],
      moved: false,
    };
    stage.current?.setPointerCapture(e.pointerId);
  };
  const finishLink = (p: Point) => {
    const l = linkRef.current;
    if (!l) return;
    const from = byId.get(l.from);
    if (!from) {
      updateLink(null);
      return;
    }
    const hit = nearPort(list, p, from.id, 28 / view.k);
    if (hit) {
      if (l.reconnect)
        patchMany(
          [l.reconnect.id],
          l.reconnect.end === "from"
            ? { from: hit.box.id, from_side: hit.side }
            : { to: hit.box.id, to_side: hit.side },
        );
      else
        setSel([
          connect(from.id, hit.box.id, {
            from_side: l.side || "auto",
            to_side: hit.side,
          }),
        ]);
    } else if (
      !l.reconnect &&
      Math.hypot(p.x - from.x - from.w / 2, p.y - from.y - from.h / 2) >
        Math.max(from.w, from.h) / 2 + 48
    ) {
      const type = from.type === "text" ? "rect" : from.type,
        [w, h] = SIZE[type],
        id = uid("i"),
        arrow = uid("a");
      change([
        [
          id,
          {
            type,
            x: p.x - w / 2,
            y: p.y - h / 2,
            w,
            h,
            text: "",
            color: from.color,
            z: maxZ() + 1,
          },
        ],
        [
          arrow,
          {
            type: "arrow",
            from: from.id,
            to: id,
            from_side: l.side || "auto",
            route: "elbow",
            color: "gray",
          },
        ],
      ]);
      setSel([id]);
      setEditing(id);
    }
    updateLink(null);
    setTool("select");
  };
  const down = (e: RPE, id?: string, resize = false) => {
    if (e.button !== 0 && e.button !== 1) return;
    if (e.button === 1) e.preventDefault();
    if (editing) {
      if (id === editing) return;
      (document.activeElement as HTMLElement)?.blur();
      setEditing(null);
    }
    e.stopPropagation();
    setMenu(null);
    setHelp(null);
    pointer.current = { x: e.clientX, y: e.clientY };
    if (e.pointerType === "touch") {
      pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY });
      if (pointers.current.size === 2) {
        const [a, b] = [...pointers.current.values()];
        pinch.current = {
          distance: Math.hypot(a.x - b.x, a.y - b.y),
          view,
          world: world((a.x + b.x) / 2, (a.y + b.y) / 2),
        };
        op.current = null;
        updateDraft({});
        updateLink(null);
        setMarquee(null);
        stage.current?.setPointerCapture(e.pointerId);
        return;
      }
    }
    const p = world(e.clientX, e.clientY);
    if (linkRef.current && !op.current) {
      finishLink(p);
      return;
    }
    if (
      e.button === 1 ||
      space ||
      tool === "hand" ||
      (!id && e.pointerType === "touch" && tool === "select")
    ) {
      op.current = {
        kind: "pan",
        pointer: e.pointerId,
        sx: e.clientX,
        sy: e.clientY,
        view,
        start: [],
        moved: false,
      };
    } else if (!id && ["sticky", "text", "rect", "ellipse"].includes(tool)) {
      const nid = create(tool as "sticky", p);
      setSel([nid]);
      setTool("select");
      setEditing(nid);
      return;
    } else if (id && tool === "arrow") {
      startLink(e, id);
      return;
    } else if (id) {
      const b = byId.get(id);
      if (!b) return;
      const now = resize
        ? [id]
        : e.shiftKey
          ? sel.includes(id)
            ? sel.filter((x) => x !== id)
            : [...sel, id]
          : sel.includes(id)
            ? sel
            : [id];
      setSel(now);
      if (b.locked) return;
      op.current = {
        kind: resize ? "resize" : "move",
        pointer: e.pointerId,
        sx: e.clientX,
        sy: e.clientY,
        view,
        start: (resize ? [b] : list.filter((b) => now.includes(b.id))).filter(
          (b) => !b.locked,
        ),
        moved: false,
      };
    } else {
      if (!e.shiftKey) setSel([]);
      op.current = {
        kind: "select",
        pointer: e.pointerId,
        sx: e.clientX,
        sy: e.clientY,
        view,
        start: [],
        moved: false,
        add: e.shiftKey ? sel : [],
      };
    }
    stage.current?.setPointerCapture(e.pointerId);
  };
  const move = (e: RPE) => {
    pointer.current = { x: e.clientX, y: e.clientY };
    if (pointers.current.has(e.pointerId))
      pointers.current.set(e.pointerId, pointer.current);
    if (pinch.current && pointers.current.size === 2) {
      const [a, b] = [...pointers.current.values()],
        r = stage.current!.getBoundingClientRect(),
        k = Math.max(
          0.1,
          Math.min(
            4,
            (pinch.current.view.k * Math.hypot(a.x - b.x, a.y - b.y)) /
              (pinch.current.distance || 1),
          ),
        );
      touched.current = true;
      setView({
        k,
        x: (a.x + b.x) / 2 - r.left - pinch.current.world.x * k,
        y: (a.y + b.y) / 2 - r.top - pinch.current.world.y * k,
      });
      return;
    }
    const o = op.current;
    if (!o) {
      if (linkRef.current)
        updateLink({ ...linkRef.current, ...world(e.clientX, e.clientY) });
      return;
    }
    if (o.pointer !== e.pointerId) return;
    let dx = (e.clientX - o.sx) / (o.kind === "pan" ? 1 : view.k),
      dy = (e.clientY - o.sy) / (o.kind === "pan" ? 1 : view.k);
    if (Math.abs(dx) + Math.abs(dy) > 3 / view.k) o.moved = true;
    if (!o.moved) return;
    if (o.kind === "pan") {
      touched.current = true;
      setView({ ...o.view, x: o.view.x + dx, y: o.view.y + dy });
    } else if (o.kind === "link" && linkRef.current)
      updateLink({ ...linkRef.current, ...world(e.clientX, e.clientY) });
    else if (o.kind === "select") {
      const a = world(o.sx, o.sy),
        b = world(e.clientX, e.clientY),
        r = {
          x: Math.min(a.x, b.x),
          y: Math.min(a.y, b.y),
          w: Math.abs(a.x - b.x),
          h: Math.abs(a.y - b.y),
        };
      setMarquee({
        ...r,
        id: "",
        type: "rect",
        text: "",
        color: "gray",
        z: 0,
        font_size: 16,
        align: "left",
        locked: false,
      });
      setSel([
        ...new Set([
          ...(o.add || []),
          ...list
            .filter(
              (b) =>
                b.x >= r.x &&
                b.y >= r.y &&
                b.x + b.w <= r.x + r.w &&
                b.y + b.h <= r.y + r.h,
            )
            .map((b) => b.id),
        ]),
      ]);
    } else if (o.kind === "move") {
      const b = bounds(o.start),
        rest = list.filter((b) => !o.start.some((s) => s.id === b.id)),
        g: typeof guides = [];
      if (!e.altKey)
        for (const axis of ["x", "y"] as const) {
          const size = axis === "x" ? "w" : "h",
            delta = axis === "x" ? dx : dy,
            points = [b[axis], b[axis] + b[size] / 2, b[axis] + b[size]];
          let correction = 6 / view.k,
            found: number | undefined;
          for (const other of rest)
            for (const target of [
              other[axis],
              other[axis] + other[size] / 2,
              other[axis] + other[size],
            ])
              for (const p of points) {
                const d = target - p - delta;
                if (Math.abs(d) < Math.abs(correction)) {
                  correction = d;
                  found = target;
                }
              }
          if (found !== undefined) {
            if (axis === "x") dx += correction;
            else dy += correction;
            g.push({ axis, pos: found });
          }
        }
      setGuides(g);
      updateDraft(
        Object.fromEntries(
          o.start.map((b) => [
            b.id,
            { x: Math.round(b.x + dx), y: Math.round(b.y + dy) },
          ]),
        ),
      );
    } else if (o.kind === "resize") {
      const b = o.start[0];
      let w = Math.max(60, Math.round(b.w + dx)),
        h = Math.max(32, Math.round(b.h + dy));
      if (e.shiftKey) h = (w * b.h) / b.w;
      updateDraft({ [b.id]: { w, h } });
    }
  };
  const up = (e: RPE) => {
    pointers.current.delete(e.pointerId);
    if (pinch.current) {
      if (pointers.current.size < 2) pinch.current = null;
      op.current = null;
      return;
    }
    const o = op.current;
    if (!o || o.pointer !== e.pointerId) return;
    op.current = null;
    if (o.kind === "link") {
      if (o.moved) finishLink(world(e.clientX, e.clientY));
      else if (linkRef.current?.reconnect) updateLink(null);
    } else if (o.moved && (o.kind === "move" || o.kind === "resize"))
      change(
        Object.entries(draftRef.current).flatMap(([id, p]) => {
          const r = items.get(id);
          return r ? [[id, { ...r.data, ...p }] as [string, Item]] : [];
        }),
      );
    setMarquee(null);
    setGuides([]);
    updateDraft({});
    if (stage.current?.hasPointerCapture(e.pointerId))
      stage.current.releasePointerCapture(e.pointerId);
  };
  const cancel = () => {
    op.current = null;
    pinch.current = null;
    pointers.current.clear();
    updateDraft({});
    updateLink(null);
    setMarquee(null);
    setGuides([]);
  };
  const paste = (
    value: { rows: Entry[] | null; text: string },
    position?: Point,
  ) => {
    const p = position || center();
    pasteCount.current++;
    if (value.rows) {
      const area = bounds(
        value.rows
          .filter((r) => r.data.type !== "arrow")
          .map((r) => ({
            x: number(r.data.x),
            y: number(r.data.y),
            w: number(r.data.w, 216),
            h: number(r.data.h, 164),
          })),
      );
      setSel(
        duplicate(value.rows, {
          x: p.x - area.x - area.w / 2 + pasteCount.current * 16,
          y: p.y - area.y - area.h / 2 + pasteCount.current * 16,
        }),
      );
    } else if (value.text) setSel(pasteText(value.text, p));
  };
  const menuPaste = async () => {
    try {
      paste(await clipboard.read(), menu ? world(menu.x, menu.y) : undefined);
    } catch {
      setPastePrompt(true);
    }
    setMenu(null);
  };
  const copy = async (cut = false) => {
    if (!selected.length) return;
    try {
      if (await clipboard.write(selectedGraph(selected))) {
        if (cut) remove(selected);
        kit.flash(cut ? uiText("已剪切") : uiText("已复制"));
      } else kit.flash(uiText("请使用 ⌘/Ctrl+C 复制"));
    } catch {
      kit.flash(uiText("请使用 ⌘/Ctrl+C 复制"));
    }
    setMenu(null);
  };
  useEffect(() => {
    const copied = (e: ClipboardEvent) => {
      if (editing || editable(e.target) || !sel.length || !e.clipboardData)
        return;
      e.preventDefault();
      clipboard.fillClipboard(e.clipboardData, selectedGraph(sel));
      pasteCount.current = 0;
      if (e.type === "cut") remove(sel);
    };
    const pasted = (e: ClipboardEvent) => {
      if (editing || editable(e.target) || !e.clipboardData) return;
      e.preventDefault();
      paste(clipboard.readClipboard(e.clipboardData));
    };
    const key = (e: KeyboardEvent) => {
      if (editable(e.target) || editing) return;
      if (e.key === " ") {
        e.preventDefault();
        setSpace(true);
        return;
      }
      const mod = e.metaKey || e.ctrlKey,
        k = e.key.toLowerCase();
      if (mod) {
        if (k === "z") {
          e.preventDefault();
          e.shiftKey ? redo() : undo();
        } else if (k === "y") {
          e.preventDefault();
          redo();
        } else if (k === "a") {
          e.preventDefault();
          setSel(items.rows.map((r) => r.id));
        } else if (k === "d") {
          e.preventDefault();
          setSel(duplicate(selectedGraph(sel)));
        }
        return;
      }
      if (e.key === "Delete" || e.key === "Backspace") {
        e.preventDefault();
        remove(sel);
        setSel([]);
        return;
      }
      if (e.key === "Escape") {
        cancel();
        setSel([]);
        setTool("select");
        setMenu(null);
        setHelp(null);
        return;
      }
      if (e.key.startsWith("Arrow") && sel.length) {
        e.preventDefault();
        const step = e.shiftKey ? 10 : 1;
        change(
          list
            .filter((b) => sel.includes(b.id) && !b.locked)
            .map((b) => [
              b.id,
              {
                ...items.get(b.id)!.data,
                x:
                  b.x +
                  (e.key === "ArrowRight"
                    ? step
                    : e.key === "ArrowLeft"
                      ? -step
                      : 0),
                y:
                  b.y +
                  (e.key === "ArrowDown"
                    ? step
                    : e.key === "ArrowUp"
                      ? -step
                      : 0),
              },
            ]),
        );
        return;
      }
      if (
        e.key === "Enter" &&
        sel.length === 1 &&
        !items.get(sel[0])?.data.locked
      ) {
        e.preventDefault();
        setEditing(sel[0]);
        return;
      }
      const t = tools.find((t) => t[3].toLowerCase() === k);
      if (t) selectTool(t[0]);
    };
    const released = (e: KeyboardEvent) => {
        if (e.key === " ") setSpace(false);
      },
      blur = () => {
        setSpace(false);
        cancel();
      };
    document.addEventListener("copy", copied);
    document.addEventListener("cut", copied);
    document.addEventListener("paste", pasted);
    document.addEventListener("keydown", key);
    document.addEventListener("keyup", released);
    window.addEventListener("blur", blur);
    return () => {
      document.removeEventListener("copy", copied);
      document.removeEventListener("cut", copied);
      document.removeEventListener("paste", pasted);
      document.removeEventListener("keydown", key);
      document.removeEventListener("keyup", released);
      window.removeEventListener("blur", blur);
    };
  });
  const exportSvg = async () => {
    if (!list.length) {
      kit.flash(uiText("白板上还什么都没有"));
      return;
    }
    try {
      await save(
        `${(title.get() || uiText("白板")).replace(/[\\/:*?"<>|]/g, " ")}.svg`,
        svgDocument(items.rows),
        "image/svg+xml",
      );
      kit.flash(uiText("已存到「下载」"));
    } catch (e) {
      kit.flash((e as Error).message || uiText("没存下来"));
    }
  };
  const hit = link
    ? nearPort(list, { x: link.x, y: link.y }, link.from, 28 / view.k)
    : null;
  const projected = list.map(at),
    projectedMap = new Map(projected.map((b) => [b.id, b]));
  return {
    arrows,
    tool,
    view,
    sel,
    editing,
    link,
    marquee,
    guides,
    menu,
    help,
    dark,
    space,
    stage,
    touched,
    list,
    selected,
    selectionData,
    world,
    fit,
    zoom,
    selectTool,
    startLink,
    down,
    move,
    up,
    cancel,
    menuPaste,
    paste,
    pastePrompt,
    setPastePrompt,
    copy,
    exportSvg,
    hit,
    projected,
    projectedMap,
    setSel,
    setEditing,
    setMenu,
    setHelp,
  };
}
