import { themeMedia } from "../_lib/appearance";
import { t as uiText, language } from "../_lib/i18n";
// 文档: a document written together, on BlockNote (a block editor of the
// Notion / Feishu kind). Each top-level block is a record of `blocks`
// (its id is the block's id): two people writing different blocks never
// overwrite each other, and agents read and write blocks as records
// (type + text; the editor's own form in `block`).
import "@blocknote/mantine/style.css";
import "./doc.css";
import { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { BlockNoteSchema, defaultBlockSpecs, type Block, type PartialBlock } from "@blocknote/core";
import { zh, en } from "@blocknote/core/locales";
import { useCreateBlockNote } from "@blocknote/react";
import { BlockNoteView } from "@blocknote/mantine";
import { dark, listen, myco, save, Store, type MycoChange } from "../_lib/myco";
import { assignOrders } from "../_lib/order";

// No pictures, video or files yet: a page cannot reach the internet, and
// uploads come later.
const { audio: _a, video: _v, image: _i, file: _f, ...specs } = defaultBlockSpecs;
const schema = BlockNoteSchema.create({ blockSpecs: specs });
type AnyBlock = Block<any, any, any>;

type Rec = { type?: string; text?: string; checked?: boolean; block?: AnyBlock; order?: number };
type Title = { title?: string };

const blocks = new Store<Rec>("blocks");
const docs = new Store<Title>("doc");

// ---- records <-> blocks ------------------------------------------------------
const SIMPLE_TO: Record<string, (text: string, r: Rec) => PartialBlock<any, any, any>> = {
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
function toBlock(id: string, r: Rec): PartialBlock<any, any, any> {
  // The editor's own form, unless someone (an agent) wrote other text since.
  if (r.block && typeof r.block === "object" && r.block.type && (r.text === undefined || r.text === plain(r.block))) return { ...r.block, id };
  const make = SIMPLE_TO[r.type || "p"] || SIMPLE_TO.p;
  return { ...make(r.text || "", r), id };
}
function inline(content: unknown): string {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  return content.map((c: any) => c?.type === "text" ? c.text : c?.type === "link" ? inline(c.content) : "").join("");
}
function plain(b: AnyBlock): string {
  const c: any = b.content;
  if (c && c.type === "tableContent") {
    return (c.rows || []).map((row: any) => (row.cells || []).map((cell: any) => inline(cell?.content ?? cell)).join(" | ")).join("\n");
  }
  const own = inline(c);
  const kids = (b.children || []).map((k: AnyBlock) => plain(k)).filter(Boolean);
  return [own, ...kids].filter(Boolean).join("\n");
}
function simpleOf(b: AnyBlock): Rec {
  const level = (b.props as any)?.level;
  const type = b.type === "heading" ? `h${level || 1}` : ({ paragraph: "p", bulletListItem: "li", numberedListItem: "ol", checkListItem: "todo",
    toggleListItem: "toggle", quote: "quote", codeBlock: "code", divider: "hr", table: "table" } as Record<string, string>)[b.type] || b.type;
  const out: Rec = { type, text: plain(b) };
  if (b.type === "checkListItem") out.checked = !!(b.props as any)?.checked;
  return out;
}

// ---- the page ------------------------------------------------------------------
function Doc({ initial, me, title0 }: { initial: PartialBlock<any, any, any>[]; me: string; title0: string }) {
  const editor = useCreateBlockNote({ schema, dictionary: language() === "en" ? en : zh, initialContent: initial.length ? initial : undefined });
  const [title, setTitle] = useState(title0);
  const [state, setState] = useState<"saved" | "saving" | "error">("saved");
  const [note, setNote] = useState("");
  const [theme, setTheme] = useState<"light" | "dark">(dark() ? "dark" : "light");
  const [menu, setMenu] = useState(false);
  const titleRef = useRef<HTMLTextAreaElement>(null);
  const sync = useRef<{ saved: Map<string, string>; orders: Map<string, number>; remote: number; timer: number; titleTimer: number }>(
    { saved: new Map(), orders: new Map(), remote: 0, timer: 0, titleTimer: 0 });

  useEffect(() => {
    const m = themeMedia();
    const on = () => setTheme(m.matches ? "dark" : "light");
    m.addEventListener("change", on);
    return () => m.removeEventListener("change", on);
  }, []);

  // What is in the editor now is what was saved (or drawn from records).
  useEffect(() => {
    const s = sync.current;
    for (const b of editor.document) {
      s.saved.set(b.id, JSON.stringify(b));
      const r = blocks.get(b.id);
      if (r && typeof r.data.order === "number") s.orders.set(b.id, r.data.order);
    }
  }, [editor]);

  const flash = (t: string) => { setNote(t); window.setTimeout(() => setNote((x) => (x === t ? "" : x)), 2400); };

  // Local edits: each changed block saved on its own, a moment after
  // typing stops; deleted blocks removed; orders kept increasing.
  const push = () => {
    const s = sync.current;
    const doc = editor.document;
    const ids = new Set(doc.map((b) => b.id));
    const writes: Promise<unknown>[] = [];
    for (const id of [...s.saved.keys()]) {
      if (ids.has(id)) continue;
      s.saved.delete(id);
      s.orders.delete(id);
      writes.push(blocks.remove(id));
    }
    const orders = assignOrders(doc.map((b) => b.id), s.orders);
    doc.forEach((b, i) => {
      const json = JSON.stringify(b), o = orders[i];
      if (s.saved.get(b.id) === json && s.orders.get(b.id) === o) return;
      s.saved.set(b.id, json);
      s.orders.set(b.id, o);
      writes.push(blocks.put({ ...simpleOf(b), block: b, order: o }, b.id) as Promise<unknown>);
    });
    if (!writes.length) { setState("saved"); return; }
    Promise.all(writes).then(() => setState("saved"), (e: { code?: string; message?: string }) => {
      if (e?.code === "conflict") { flash(uiText("别人刚改过同一段，已换成最新的")); setState("saved"); }
      else { setState("error"); flash(e?.message || uiText("没保存上")); }
    });
  };
  useEffect(() => editor.onChange(() => {
    const s = sync.current;
    if (s.remote) return;
    setState("saving");
    window.clearTimeout(s.timer);
    s.timer = window.setTimeout(push, 500);
  }), [editor]);

  // What others (an agent, another person) changed comes in place.
  useEffect(() => {
    const s = sync.current;
    const apply = (c: MycoChange) => {
      if (c.collection === "doc") {
        const t = docs.get("main")?.data.title || "";
        if (document.activeElement !== titleRef.current) setTitle(t);
        return;
      }
      s.remote++;
      try {
        const here = editor.getBlock(c.record);
        if (c.op === "delete") {
          if (here) editor.removeBlocks([c.record]);
          s.saved.delete(c.record);
          s.orders.delete(c.record);
          return;
        }
        const r = blocks.get(c.record);
        if (!r) return;
        const partial = toBlock(c.record, r.data);
        let cursor = "";
        try { cursor = editor.getTextCursorPosition().block.id; } catch { /* (no cursor) */ }
        if (here) {
          if (cursor === c.record && s.timer) return; // (being typed in: theirs waits)
          editor.updateBlock(c.record, partial as any);
        } else {
          const o = typeof r.data.order === "number" ? r.data.order : Infinity;
          const doc = editor.document;
          const before = doc.find((b) => (s.orders.get(b.id) ?? Infinity) > o);
          if (before) editor.insertBlocks([partial as any], before.id, "before");
          else editor.insertBlocks([partial as any], doc[doc.length - 1].id, "after");
        }
        const now = editor.getBlock(c.record);
        if (now) s.saved.set(c.record, JSON.stringify(now));
        if (typeof r.data.order === "number") s.orders.set(c.record, r.data.order);
      } finally {
        s.remote--;
      }
      if (c.by && c.by !== me) flash(uiText("{0} 刚改了", c.by));
    };
    listen([docs, blocks], apply);
  }, [editor, me]);

  const saveTitle = (t: string) => {
    const s = sync.current;
    setTitle(t);
    window.clearTimeout(s.titleTimer);
    s.titleTimer = window.setTimeout(() => {
      docs.put({ title: t.trim() }, "main")?.catch((e: { message?: string }) => flash(e?.message || uiText("标题没保存上")));
    }, 500);
  };
  // (the title's height follows its text, and the width: the page may be
  // made while its panel is narrow or hidden, then widened)
  useEffect(() => {
    const el = titleRef.current;
    if (!el) return;
    const fit = () => { el.style.height = "auto"; el.style.height = `${el.scrollHeight}px`; };
    fit();
    let w = el.clientWidth;
    const ro = new ResizeObserver(() => { if (el.clientWidth !== w) { w = el.clientWidth; fit(); } });
    ro.observe(el);
    return () => ro.disconnect();
  }, [title]);

  const fileName = () => (title.trim() || "文档").replace(/[\\/:*?"<>|]/g, " ");
  const exportAs = async (kind: "md" | "html") => {
    setMenu(false);
    window.clearTimeout(sync.current.timer);
    push();
    try {
      if (kind === "md") {
        const md = `# ${title.trim() || "文档"}\n\n${await editor.blocksToMarkdownLossy(editor.document)}`;
        await save(`${fileName()}.md`, md, "text/markdown");
      } else {
        const body = await editor.blocksToHTMLLossy(editor.document);
        const esc = (x: string) => x.replace(/&/g, "&amp;").replace(/</g, "&lt;");
        const html = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>${esc(title || "文档")}</title><style>body{max-width:760px;margin:40px auto;padding:0 20px;font:16px/1.75 -apple-system,"PingFang SC","Microsoft YaHei",sans-serif;color:#1f2329}pre{background:#f5f6f7;padding:12px 14px;border-radius:8px;overflow:auto}blockquote{border-left:3px solid #dee0e3;margin:0;padding-left:14px;color:#646a73}table{border-collapse:collapse}td,th{border:1px solid #dee0e3;padding:6px 10px}</style></head><body><h1>${esc(title || "文档")}</h1>${body}</body></html>`;
        await save(`${fileName()}.html`, html, "text/html");
      }
      flash(uiText("已存到「下载」"));
    } catch (e) {
      flash((e as Error).message || uiText("没存下来"));
    }
  };

  return (
    <div className="doc">
      <header className="doc-bar">
        <span className={`doc-state is-${state}`}>{state === "saving" ? uiText("正在保存…") : state === "error" ? uiText("没保存上") : uiText("已保存")}</span>
        <span className="doc-spacer" />
        {me && <span className="doc-me">{uiText("你是")}{" "}{me}</span>}
        <div className="doc-export">
          <button type="button" className="doc-btn" onClick={() => setMenu((v) => !v)} aria-expanded={menu}>{uiText("导出")}</button>
          {menu && (
            <div className="doc-menu" role="menu" onMouseLeave={() => setMenu(false)}>
              <button type="button" role="menuitem" onClick={() => void exportAs("md")}>Markdown（.md）</button>
              <button type="button" role="menuitem" onClick={() => void exportAs("html")}>{uiText("网页（.html）")}</button>
            </div>
          )}
        </div>
      </header>
      <main className="doc-page">
        <textarea ref={titleRef} className="doc-title" rows={1} value={title} placeholder={uiText("无标题")} aria-label={uiText("标题")}
          onChange={(e) => saveTitle(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.nativeEvent.isComposing) {
              e.preventDefault();
              const first = editor.document[0];
              if (first) editor.setTextCursorPosition(first.id, "start");
              editor.focus();
            }
          }} />
        <BlockNoteView editor={editor} theme={theme} />
      </main>
      {note && <div className="doc-note" role="status">{note}</div>}
    </div>
  );
}

function Outside() {
  return <div className="doc-outside">{uiText("在 myco App 里打开这个网页，才能读写它的内容。")}</div>;
}

async function start() {
  const root = createRoot(document.getElementById("root")!);
  if (!myco) { root.render(<Outside />); return; }
  let me = "";
  try { me = (await myco.ready).me?.name || ""; } catch { root.render(<Outside />); return; }
  await Promise.all([blocks.load(), docs.load()]);
  if (!docs.get("main")) await docs.put({ title: "" }, "main")?.catch(() => docs.load());
  const initial = blocks.rows.map((r) => toBlock(r.id, r.data));
  root.render(<Doc initial={initial} me={me} title0={docs.get("main")?.data.title || ""} />);
}
void start();
