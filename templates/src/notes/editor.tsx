import { t as uiText, language } from "../_lib/i18n";
// A page's blocks in BlockNote (the Notion-like editor 文档 uses too). Each
// top-level block is a record of `blocks` with the page's id: the edits of
// a moment are written block by block, and what others write comes into
// the editor in place (as 文档 does it).
import "@blocknote/mantine/style.css";
import { useEffect, useRef } from "react";
import { BlockNoteSchema, defaultBlockSpecs, type Block as BNBlock, type PartialBlock } from "@blocknote/core";
import { zh, en } from "@blocknote/core/locales";
import { useCreateBlockNote } from "@blocknote/react";
import { BlockNoteView } from "@blocknote/mantine";
import type { MycoChange } from "../_lib/myco";
import { assignOrders } from "../_lib/order";
import { blocks, kit, num, type Block } from "./model";

// No pictures, video or files: a page cannot reach the internet.
const { audio: _a, video: _v, image: _i, file: _f, ...specs } = defaultBlockSpecs;
const schema = BlockNoteSchema.create({ blockSpecs: specs });
type AnyBlock = BNBlock<any, any, any>;

const SIMPLE_TO: Record<string, (text: string, r: Block) => PartialBlock<any, any, any>> = {
  p: (t) => ({ type: "paragraph", content: t }),
  h1: (t) => ({ type: "heading", props: { level: 1 }, content: t }),
  h2: (t) => ({ type: "heading", props: { level: 2 }, content: t }),
  h3: (t) => ({ type: "heading", props: { level: 3 }, content: t }),
  li: (t) => ({ type: "bulletListItem", content: t }),
  ol: (t) => ({ type: "numberedListItem", content: t }),
  todo: (t, r) => ({ type: "checkListItem", props: { checked: !!r.checked }, content: t }),
  toggle: (t) => ({ type: "toggleListItem", content: t }),
  quote: (t) => ({ type: "quote", content: t }),
  code: (t) => ({ type: "codeBlock", content: t }),
  hr: () => ({ type: "divider" }),
};
function inline(content: unknown): string {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  return content.map((c: any) => (c?.type === "text" ? c.text : c?.type === "link" ? inline(c.content) : "")).join("");
}
export function plain(b: AnyBlock): string {
  const c: any = b.content;
  if (c && c.type === "tableContent") return (c.rows || []).map((row: any) => (row.cells || []).map((cell: any) => inline(cell?.content ?? cell)).join(" | ")).join("\n");
  const own = inline(c);
  const kids = (b.children || []).map((k: AnyBlock) => plain(k)).filter(Boolean);
  return [own, ...kids].filter(Boolean).join("\n");
}
function simpleOf(b: AnyBlock): Block {
  const level = (b.props as any)?.level;
  const type = b.type === "heading" ? `h${level || 1}` : ({ paragraph: "p", bulletListItem: "li", numberedListItem: "ol", checkListItem: "todo",
    toggleListItem: "toggle", quote: "quote", codeBlock: "code", divider: "hr", table: "table" } as Record<string, string>)[b.type] || b.type;
  const out: Block = { type, text: plain(b) };
  if (b.type === "checkListItem") out.checked = !!(b.props as any)?.checked;
  return out;
}
function toBlock(id: string, r: Block): PartialBlock<any, any, any> {
  // the editor's own form, unless someone (an agent) wrote other text since
  const own = r.block as AnyBlock | undefined;
  if (own && typeof own === "object" && own.type && (r.text === undefined || r.text === plain(own))) return { ...own, id };
  const make = SIMPLE_TO[r.type || "p"] || SIMPLE_TO.p;
  return { ...make(r.text || "", r), id };
}
export const pageBlocks = (page: string) => blocks.rows.filter((r) => r.data.page === page).sort((a, b) => num(a.data.order, 0) - num(b.data.order, 0));

// what others change in blocks goes to the page being edited
let current: ((c: MycoChange) => void) | null = null;
blocks.on((c) => current?.(c));

export function Editor({ page, theme, onState, editorRef }: {
  page: string; theme: "light" | "dark"; onState: (s: "saved" | "saving" | "error") => void;
  editorRef?: (e: { focusStart: () => void; markdown: () => Promise<string> } | null) => void;
}) {
  const initial = pageBlocks(page).map((r) => toBlock(r.id, r.data));
  const editor = useCreateBlockNote({ schema, dictionary: { ...(language() === "en" ? en : zh), placeholders: { ...(language() === "en" ? en.placeholders : zh.placeholders), default: uiText("输入文字，或者按「/」插入标题、列表、待办…") } }, initialContent: initial.length ? initial : undefined });
  const sync = useRef({ saved: new Map<string, string>(), orders: new Map<string, number>(), remote: 0, timer: 0 });

  useEffect(() => {
    const s = sync.current;
    for (const b of editor.document) {
      s.saved.set(b.id, JSON.stringify(b));
      const r = blocks.get(b.id);
      if (r && typeof r.data.order === "number") s.orders.set(b.id, r.data.order);
    }
    editorRef?.({
      focusStart: () => { const f = editor.document[0]; if (f) editor.setTextCursorPosition(f.id, "start"); editor.focus(); },
      markdown: async () => editor.blocksToMarkdownLossy(editor.document),
    });
    return () => editorRef?.(null);
  }, [editor]); // eslint-disable-line react-hooks/exhaustive-deps

  // local edits: each changed block saved on its own, a moment after typing
  const push = () => {
    const s = sync.current;
    s.timer = 0;
    const doc = editor.document;
    const ids = new Set(doc.map((b) => b.id));
    const writes: Promise<unknown>[] = [];
    for (const id of [...s.saved.keys()]) {
      if (ids.has(id)) continue;
      s.saved.delete(id); s.orders.delete(id);
      writes.push(blocks.remove(id));
    }
    const orders = assignOrders(doc.map((b) => b.id), s.orders);
    doc.forEach((b, i) => {
      const json = JSON.stringify(b), o = orders[i];
      if (s.saved.get(b.id) === json && s.orders.get(b.id) === o) return;
      // (an empty page's lone empty paragraph is not worth a record)
      if (doc.length === 1 && b.type === "paragraph" && !plain(b) && !blocks.get(b.id)) return;
      s.saved.set(b.id, json); s.orders.set(b.id, o);
      writes.push(blocks.put({ ...simpleOf(b), page, block: b, order: o }, b.id) as Promise<unknown>);
    });
    if (!writes.length) { onState("saved"); return; }
    Promise.all(writes).then(() => onState("saved"), (e: { code?: string; message?: string }) => {
      if (e?.code === "conflict") { kit.flash(uiText("别人刚改过同一段，已换成最新的")); onState("saved"); }
      else { onState("error"); kit.flash(e?.message || uiText("没保存上")); }
    });
  };
  useEffect(() => editor.onChange(() => {
    const s = sync.current;
    if (s.remote) return;
    onState("saving");
    window.clearTimeout(s.timer);
    s.timer = window.setTimeout(push, 500);
  }), [editor]); // eslint-disable-line react-hooks/exhaustive-deps
  // leaving the page: what is still waiting is written now
  useEffect(() => () => { if (sync.current.timer) { window.clearTimeout(sync.current.timer); push(); } }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // what others write comes in place
  useEffect(() => {
    const s = sync.current;
    current = (c: MycoChange) => {
      const r = blocks.get(c.record);
      if (c.op !== "delete" && r?.data.page !== page) return;
      s.remote++;
      try {
        const here = editor.getBlock(c.record);
        if (c.op === "delete") {
          if (here) editor.removeBlocks([c.record]);
          s.saved.delete(c.record); s.orders.delete(c.record);
          return;
        }
        if (!r) return;
        const partial = toBlock(c.record, r.data);
        let cursor = "";
        try { cursor = editor.getTextCursorPosition().block.id; } catch { /* (no cursor) */ }
        if (here) {
          if (cursor === c.record && s.timer) return; // (being typed in: theirs waits)
          editor.updateBlock(c.record, partial as any);
        } else {
          const o = num(r.data.order, Infinity);
          const doc = editor.document;
          const before = doc.find((b) => (s.orders.get(b.id) ?? Infinity) > o);
          const last = doc[doc.length - 1];
          // (an empty page's placeholder paragraph gives way)
          if (doc.length === 1 && !blocks.get(last.id) && !plain(last)) editor.replaceBlocks([last.id], [partial as any]);
          else if (before) editor.insertBlocks([partial as any], before.id, "before");
          else editor.insertBlocks([partial as any], last.id, "after");
        }
        const now = editor.getBlock(c.record);
        if (now) s.saved.set(c.record, JSON.stringify(now));
        if (typeof r.data.order === "number") s.orders.set(c.record, r.data.order);
      } finally {
        s.remote--;
      }
      if (c.by && c.by !== kit.me) kit.flash(uiText("{0} 刚改了这一页", c.by));
    };
    return () => { current = null; };
  }, [editor, page]);

  return <BlockNoteView editor={editor} theme={theme} />;
}
