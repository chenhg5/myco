// 笔记: notes like Notion: pages in a tree on the left (nested, dragged
// around, favourites, a trash), and each page an icon, a cover, a title and
// blocks written with "/" in a block editor. People and agents share the
// records: an agent makes pages and writes blocks with the CLI, and the
// page follows as it writes.
import "../_lib/sheet.css";
import "./notes.css";
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import {
  ChevronRight, FileText, MoreHorizontal, Plus, Search, Star, Trash2, Undo2, X, ImageIcon, Smile, Download, Bot, Copy,
} from "lucide-react";
import { mount, titleSaver, uid, useKit, Toast } from "../_lib/appkit";
import { save } from "../_lib/myco";
import {
  COVERS, ICONS, blocks, childrenOf, kit, nowIso, num, orderAt, pages, pathOf, space, subtree, titleOf, within, type PageRec,
} from "./model";
import { Editor, pageBlocks } from "./editor";
import { GLYPHS, IconView, TINTS, parseIcon } from "./icons";

const spaceTitle = titleSaver(kit, space);
const store = {
  get: (k: string) => { try { return localStorage.getItem(`myco-notes-${k}`); } catch { return null; } },
  set: (k: string, v: string) => { try { localStorage.setItem(`myco-notes-${k}`, v); } catch { /* (a private window) */ } },
};

type Anchor = { x: number; y: number; w: number; h: number };
const rectOf = (el: Element): Anchor => { const r = el.getBoundingClientRect(); return { x: r.left, y: r.top, w: r.width, h: r.height }; };
function Pop({ at, onClose, children, width = 220, className = "" }: { at: Anchor; onClose: () => void; children: ReactNode; width?: number; className?: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: -9999, top: -9999 });
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    let left = at.x, top = at.y + at.h + 4;
    if (left + r.width > window.innerWidth - 8) left = window.innerWidth - 8 - r.width;
    if (top + r.height > window.innerHeight - 8) top = Math.max(8, at.y - r.height - 4);
    setPos({ left: Math.max(8, left), top });
  }, [at.x, at.y, at.w, at.h]);
  useEffect(() => {
    const down = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) onClose(); };
    const key = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    const t = setTimeout(() => document.addEventListener("mousedown", down), 0);
    document.addEventListener("keydown", key);
    return () => { clearTimeout(t); document.removeEventListener("mousedown", down); document.removeEventListener("keydown", key); };
  }, [onClose]);
  return createPortal(<div ref={ref} className={`pop ${className}`} style={{ ...pos, width }}>{children}</div>, document.body);
}

function useTheme() {
  const q = window.matchMedia?.("(prefers-color-scheme: dark)");
  const [d, setD] = useState<"light" | "dark">(q?.matches ? "dark" : "light");
  useEffect(() => { if (!q) return; const f = () => setD(q.matches ? "dark" : "light"); q.addEventListener("change", f); return () => q.removeEventListener("change", f); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return d;
}

// ---- making and moving pages ----
function newPage(parent: string | null, me: string, title = "") {
  const sib = childrenOf(parent);
  const id = uid("pg");
  void kit.track(pages.put({ title, icon: "", cover: "", parent, order: orderAt(sib, sib.length), favorite: false, trashed: false, by: me, created: nowIso() }, id));
  return id;
}
function patchPage(id: string, p: Record<string, unknown>) {
  const cur = pages.get(id);
  if (cur) void kit.track(pages.put({ ...cur.data, ...p }, id));
}
function trash(id: string) {
  const ids = subtree(id).filter((x) => !pages.get(x)?.data.trashed);
  for (const x of ids) patchPage(x, { trashed: true });
  kit.flash(`已移到回收站：${titleOf(pages.get(id))}`, () => { for (const x of ids) patchPage(x, { trashed: false }); });
}
function destroy(id: string) {
  for (const x of subtree(id)) {
    for (const b of pageBlocks(x)) void kit.track(blocks.remove(b.id));
    void kit.track(pages.remove(x));
  }
}

// ---- the app ----
function App() {
  useKit(kit);
  const me = kit.me;
  const theme = useTheme();
  const [open, setOpenState] = useState<string | null>(() => store.get("open"));
  const [view, setView] = useState<"page" | "trash">("page");
  const [folded, setFolded] = useState<Record<string, boolean>>(() => { try { return JSON.parse(store.get("folded") || "{}"); } catch { return {}; } });
  const [finding, setFinding] = useState(false);
  const [narrowSide, setNarrowSide] = useState(false);
  const setOpen = (id: string | null) => { setOpenState(id); setView("page"); setNarrowSide(false); if (id) store.set("open", id); };
  const fold = (id: string, v: boolean) => { const f = { ...folded, [id]: v }; setFolded(f); store.set("folded", JSON.stringify(f)); };

  // what is open: the remembered page, else the first one
  const live = open && pages.get(open) && !pages.get(open)!.data.trashed ? open : childrenOf(null)[0]?.id || null;
  useEffect(() => {
    // a page opened is shown in the tree: its ancestors unfold
    if (!live) return;
    const closed = pathOf(live).filter((p) => folded[p.id] !== false);
    if (closed.length) { const f = { ...folded }; for (const p of closed) f[p.id] = false; setFolded(f); }
  }, [live]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey;
      if (mod && (e.key.toLowerCase() === "k" || e.key.toLowerCase() === "p")) { e.preventDefault(); setFinding((x) => !x); }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, []);

  const favs = pages.rows.filter((p) => p.data.favorite && !p.data.trashed);
  const trashed = pages.rows.filter((p) => p.data.trashed);

  return (
    <div className={`app nt${narrowSide ? " side-open" : ""}`}>
      <aside className="nt-side">
        <div className="nt-space">
          <span className="nt-logo"><FileText className="ico" /></span>
          <input className="nt-space-name" value={spaceTitle.get("")} placeholder="笔记" aria-label="名称" onChange={(e) => spaceTitle.set(e.target.value)} />
        </div>
        <button type="button" className="nt-side-item" onClick={() => setFinding(true)}><Search className="ico" /><span>搜索</span><kbd>⌘K</kbd></button>
        <button type="button" className="nt-side-item" onClick={() => setOpen(newPage(null, me))}><Plus className="ico" /><span>新建页面</span></button>
        {favs.length > 0 && (
          <>
            <div className="nt-sec">收藏</div>
            {favs.map((p) => (
              <button key={p.id} type="button" className={`nt-side-item is-page${view === "page" && live === p.id ? " is-on" : ""}`} onClick={() => setOpen(p.id)}>
                <PageIcon p={p} /><span>{titleOf(p)}</span>
              </button>
            ))}
          </>
        )}
        <div className="nt-sec">
          <span>页面</span>
          <button type="button" className="icon-btn" title="新建页面" aria-label="新建页面" onClick={() => setOpen(newPage(null, me))}><Plus className="ico" /></button>
        </div>
        <Tree parent={null} depth={0} open={view === "page" ? live : null} folded={folded} fold={fold} me={me} onOpen={setOpen} />
        {!childrenOf(null).length && <div className="nt-side-empty">还没有页面</div>}
        <div className="nt-side-foot">
          <button type="button" className={`nt-side-item${view === "trash" ? " is-on" : ""}`} onClick={() => { setView("trash"); setNarrowSide(false); }}>
            <Trash2 className="ico" /><span>回收站</span>{trashed.length ? <span className="nt-n">{trashed.length}</span> : null}
          </button>
          <div className="nt-state faint">{kit.pending ? "正在保存…" : kit.failed ? "没保存上" : "已保存"}{me ? ` · 你是 ${me}` : ""}</div>
        </div>
      </aside>
      <main className="nt-main">
        {view === "trash" ? <Trash onOpen={setOpen} /> : live ? <PageView key={live} id={live} me={me} theme={theme} onOpen={setOpen} onSide={() => setNarrowSide(true)} />
          : <Empty me={me} onOpen={setOpen} />}
      </main>
      {narrowSide && <div className="nt-scrim" onClick={() => setNarrowSide(false)} />}
      {finding && <Finder onOpen={(id) => { setFinding(false); setOpen(id); }} onClose={() => setFinding(false)} />}
      <Toast kit={kit} />
    </div>
  );
}

function PageIcon({ p, size = 16 }: { p: PageRec; size?: number }) {
  return <IconView v={p.data.icon} size={size} />;
}

/** Choosing a page's icon: a line icon in a quiet colour, or an emoji. */
function IconPicker({ at, value, onPick, onClose }: { at: Anchor; value: string; onPick: (v: string) => void; onClose: () => void }) {
  const cur = parseIcon(value);
  const [tab, setTab] = useState<"glyph" | "emoji">(cur?.kind === "emoji" ? "emoji" : "glyph");
  const [tint, setTint] = useState<string>(cur?.kind === "glyph" ? cur.tint : "gray");
  return (
    <Pop at={at} onClose={onClose} width={316} className="nt-icons">
      <div className="nt-icon-head">
        <div className="seg">
          <button type="button" className={tab === "glyph" ? "is-on" : ""} onClick={() => setTab("glyph")}>图标</button>
          <button type="button" className={tab === "emoji" ? "is-on" : ""} onClick={() => setTab("emoji")}>Emoji</button>
        </div>
        {value && <button type="button" className="link-btn" onClick={() => { onPick(""); onClose(); }}>去掉</button>}
      </div>
      {tab === "glyph" ? (
        <>
          <div className="nt-tints" role="radiogroup" aria-label="颜色">
            {TINTS.map((t) => <button key={t} type="button" role="radio" aria-checked={tint === t} aria-label={t} className={tint === t ? "is-on" : ""} style={{ background: `var(--tint-${t})` }} onClick={() => setTint(t)} />)}
          </div>
          <div className="nt-icon-grid">
            {Object.entries(GLYPHS).map(([name, G]) => (
              <button key={name} type="button" title={name} className={cur?.kind === "glyph" && cur.name === name ? "is-on" : ""} onClick={() => { onPick(`i:${name}:${tint}`); onClose(); }}>
                <G width={18} height={18} strokeWidth={1.8} style={{ color: `var(--tint-${tint})` }} />
              </button>
            ))}
          </div>
        </>
      ) : (
        <div className="nt-icon-grid is-emoji">
          {ICONS.map((ic) => <button key={ic} type="button" onClick={() => { onPick(ic); onClose(); }}>{ic}</button>)}
        </div>
      )}
    </Pop>
  );
}

// ---- the tree, with dragging (onto a page: inside it; at its top or bottom edge: before or after) ----
let dragging: string | null = null;
function Tree({ parent, depth, open, folded, fold, me, onOpen }: {
  parent: string | null; depth: number; open: string | null; folded: Record<string, boolean>; fold: (id: string, v: boolean) => void; me: string; onOpen: (id: string) => void;
}) {
  const [over, setOver] = useState<{ id: string; where: "before" | "inside" | "after" } | null>(null);
  const [menu, setMenu] = useState<null | { id: string; at: Anchor }>(null);
  const list = childrenOf(parent);
  const drop = (target: PageRec, where: "before" | "inside" | "after") => {
    const id = dragging;
    dragging = null; setOver(null);
    if (!id || id === target.id || within(target.id, id)) return;
    if (where === "inside") {
      const kids = childrenOf(target.id).filter((x) => x.id !== id);
      patchPage(id, { parent: target.id, order: orderAt(kids, kids.length) });
      fold(target.id, false);
      return;
    }
    const newParent = (target.data.parent as string | null) || null;
    const sib = childrenOf(newParent).filter((x) => x.id !== id);
    const i = sib.findIndex((x) => x.id === target.id) + (where === "after" ? 1 : 0);
    patchPage(id, { parent: newParent, order: orderAt(sib, i) });
  };
  return (
    <div className="nt-tree" role={depth ? "group" : "tree"}>
      {list.map((p) => {
        const kids = childrenOf(p.id);
        // (closed until opened; the open page's ancestors are opened for it)
        const shut = kids.length === 0 || folded[p.id] !== false;
        return (
          <div key={p.id} role="treeitem" aria-expanded={kids.length ? !shut : undefined}>
            <div className={`nt-row${open === p.id ? " is-on" : ""}${over?.id === p.id ? ` is-${over.where}` : ""}`} style={{ paddingLeft: 6 + depth * 14 }}
              draggable onDragStart={(e) => { dragging = p.id; e.dataTransfer.effectAllowed = "move"; e.dataTransfer.setData("text/plain", titleOf(p)); }}
              onDragEnd={() => { dragging = null; setOver(null); }}
              onDragOver={(e) => {
                if (!dragging || dragging === p.id) return;
                e.preventDefault();
                const r = e.currentTarget.getBoundingClientRect(), y = (e.clientY - r.top) / r.height;
                const where = y < 0.28 ? "before" : y > 0.72 ? "after" : "inside";
                if (over?.id !== p.id || over.where !== where) setOver({ id: p.id, where });
              }}
              onDragLeave={() => setOver((o) => (o?.id === p.id ? null : o))}
              onDrop={(e) => { e.preventDefault(); drop(p, over?.where || "inside"); }}
              onClick={() => onOpen(p.id)}>
              <button type="button" className={`nt-caret${kids.length ? "" : " is-none"}`} aria-label={shut ? "展开" : "收起"}
                onClick={(e) => { e.stopPropagation(); if (kids.length) fold(p.id, !shut); }}>
                <ChevronRight className={`ico${shut ? "" : " is-down"}`} />
              </button>
              <PageIcon p={p} />
              <span className="nt-row-title">{titleOf(p)}</span>
              <span className="nt-row-acts">
                <button type="button" className="icon-btn" title="更多" aria-label="更多" onClick={(e) => { e.stopPropagation(); setMenu({ id: p.id, at: rectOf(e.currentTarget) }); }}><MoreHorizontal className="ico" /></button>
                <button type="button" className="icon-btn" title="在里面新建页面" aria-label="在里面新建页面" onClick={(e) => { e.stopPropagation(); fold(p.id, false); onOpen(newPage(p.id, me)); }}><Plus className="ico" /></button>
              </span>
            </div>
            {!shut && <Tree parent={p.id} depth={depth + 1} open={open} folded={folded} fold={fold} me={me} onOpen={onOpen} />}
          </div>
        );
      })}
      {menu && <PageMenu id={menu.id} at={menu.at} me={me} onClose={() => setMenu(null)} onOpen={onOpen} />}
    </div>
  );
}

function PageMenu({ id, at, me, onClose, onOpen }: { id: string; at: Anchor; me: string; onClose: () => void; onOpen: (id: string) => void }) {
  const p = pages.get(id);
  if (!p) return null;
  const item = (icon: ReactNode, label: string, f: () => void, danger = false) => (
    <button type="button" className={`menu-item${danger ? " is-danger" : ""}`} onClick={() => { onClose(); f(); }}>{icon}<span>{label}</span></button>
  );
  return (
    <Pop at={at} onClose={onClose} width={200}>
      <div className="menu">
        {item(<Star className="ico" />, p.data.favorite ? "取消收藏" : "收藏", () => patchPage(id, { favorite: !p.data.favorite }))}
        {item(<Plus className="ico" />, "在里面新建页面", () => onOpen(newPage(id, me)))}
        {item(<Copy className="ico" />, "复制一份", () => {
          const sib = childrenOf((p.data.parent as string | null) || null);
          const nid = uid("pg");
          void kit.track(pages.put({ ...p.data, title: `${titleOf(p)} 副本`, favorite: false, order: orderAt(sib, sib.findIndex((x) => x.id === id) + 1), by: me, created: nowIso() }, nid));
          for (const b of pageBlocks(id)) {
            const bid = `${nid}-${b.id}`.slice(0, 60);
            const blk = b.data.block && typeof b.data.block === "object" ? { ...(b.data.block as object), id: bid } : undefined;
            void kit.track(blocks.put({ ...b.data, page: nid, block: blk }, bid));
          }
          onOpen(nid);
        })}
        <div className="menu-sep" />
        {item(<Trash2 className="ico" />, "移到回收站", () => trash(id), true)}
      </div>
    </Pop>
  );
}

// ---- a page ----
function PageView({ id, me, theme, onOpen, onSide }: { id: string; me: string; theme: "light" | "dark"; onOpen: (id: string) => void; onSide: () => void }) {
  const p = pages.get(id)!;
  const d = p.data;
  const [title, setTitle] = useState<string | null>(null);
  const [state, setState] = useState<"saved" | "saving" | "error">("saved");
  const [iconAt, setIconAt] = useState<Anchor | null>(null);
  const [coverAt, setCoverAt] = useState<Anchor | null>(null);
  const [menuAt, setMenuAt] = useState<Anchor | null>(null);
  const titleRef = useRef<HTMLTextAreaElement>(null);
  const ed = useRef<{ focusStart: () => void; markdown: () => Promise<string> } | null>(null);
  const timer = useRef(0);
  const path = pathOf(id);
  const kids = childrenOf(id);
  const shown = title ?? String(d.title || "");
  useEffect(() => {
    const el = titleRef.current;
    if (!el) return;
    el.style.height = "auto"; el.style.height = `${el.scrollHeight}px`;
  }, [shown]);
  // a new, untitled page starts in its title
  useEffect(() => { if (!d.title && !pageBlocks(id).length) titleRef.current?.focus(); }, [id]); // eslint-disable-line react-hooks/exhaustive-deps
  const setT = (t: string) => {
    setTitle(t);
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => { patchPage(id, { title: t.trim() }); setTitle(null); }, 400);
  };
  const exportMd = async () => {
    try {
      const md = `# ${titleOf(p)}\n\n${ed.current ? await ed.current.markdown() : ""}`;
      await save(`${titleOf(p).replace(/[\\/:*?"<>|]/g, " ")}.md`, md, "text/markdown");
      kit.flash("已存到「下载」");
    } catch (e) { kit.flash((e as Error).message || "没存下来"); }
  };
  const forAgent = async () => {
    const t = `帮我在 myco 笔记（网页 ${kit.page}）的「${titleOf(p)}」这一页上写东西：先运行 myco template get myco/notes 看它的数据怎么读写，` +
      `再用 myco page data list --page ${kit.page} --collection blocks 读这一页（page 是 ${id}）的段落，按我接下来说的补充或修改。`;
    try { await navigator.clipboard.writeText(t); kit.flash("已复制，发给 agent，再告诉它要写什么"); } catch { kit.flash("没复制上"); }
  };
  return (
    <div className="nt-page-wrap">
      <header className="nt-top">
        <button type="button" className="icon-btn nt-side-toggle" aria-label="页面" onClick={onSide}><FileText className="ico" /></button>
        <nav className="nt-crumbs" aria-label="位置">
          {path.map((x) => (
            <span key={x.id} className="nt-crumb"><button type="button" onClick={() => onOpen(x.id)}><PageIcon p={x} size={14} />{titleOf(x)}</button><span className="faint">/</span></span>
          ))}
          <span className="nt-crumb is-cur"><PageIcon p={p} size={14} />{titleOf(p)}</span>
        </nav>
        <span className="top-gap" />
        <span className={`nt-saving faint${state === "error" ? " is-error" : ""}`}>{state === "saving" ? "正在保存…" : state === "error" ? "没保存上" : ""}</span>
        <button type="button" className={`icon-btn${d.favorite ? " is-fav" : ""}`} title={d.favorite ? "取消收藏" : "收藏"} aria-label="收藏" onClick={() => patchPage(id, { favorite: !d.favorite })}>
          <Star className="ico" />
        </button>
        <button type="button" className="icon-btn" title="更多" aria-label="更多" onClick={(e) => setMenuAt(rectOf(e.currentTarget))}><MoreHorizontal className="ico" /></button>
      </header>
      <div className="nt-scroll">
        {d.cover && COVERS[String(d.cover)] && (
          <div className="nt-cover" style={{ background: COVERS[String(d.cover)] }}>
            <button type="button" className="nt-cover-btn" onClick={(e) => setCoverAt(rectOf(e.currentTarget))}>换封面</button>
          </div>
        )}
        <div className={`nt-page${d.cover ? " has-cover" : ""}`}>
          {parseIcon(d.icon) && <button type="button" className="nt-big-icon" title="换图标" onClick={(e) => setIconAt(rectOf(e.currentTarget))}><IconView v={d.icon} size={60} /></button>}
          <div className="nt-page-acts">
            {!parseIcon(d.icon) && <button type="button" className="nt-ghost" onClick={(e) => setIconAt(rectOf(e.currentTarget))}><Smile className="ico" />添加图标</button>}
            {!d.cover && <button type="button" className="nt-ghost" onClick={(e) => setCoverAt(rectOf(e.currentTarget))}><ImageIcon className="ico" />添加封面</button>}
          </div>
          <textarea ref={titleRef} className="nt-title" rows={1} value={shown} placeholder="无标题" aria-label="标题"
            onChange={(e) => setT(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter" && !e.nativeEvent.isComposing) { e.preventDefault(); ed.current?.focusStart(); } }} />
          <div className="nt-editor">
            <Editor page={id} theme={theme} onState={setState} editorRef={(e) => { ed.current = e; }} />
          </div>
          <div className="nt-kids">
            {kids.map((k) => (
              <button key={k.id} type="button" className="nt-kid" onClick={() => onOpen(k.id)}><PageIcon p={k} /><span>{titleOf(k)}</span></button>
            ))}
            <button type="button" className="nt-kid is-add" onClick={() => onOpen(newPage(id, me))}><Plus className="ico" /><span>添加子页面</span></button>
          </div>
        </div>
      </div>
      {iconAt && <IconPicker at={iconAt} value={String(d.icon || "")} onPick={(v) => patchPage(id, { icon: v })} onClose={() => setIconAt(null)} />}
      {coverAt && (
        <Pop at={coverAt} onClose={() => setCoverAt(null)} width={292} className="nt-covers">
          <div className="nt-cover-grid">
            {Object.entries(COVERS).map(([k, bg]) => <button key={k} type="button" aria-label={k} className={d.cover === k ? "is-on" : ""} style={{ background: bg }} onClick={() => { patchPage(id, { cover: k }); setCoverAt(null); }} />)}
          </div>
          {d.cover && <button type="button" className="link-btn" onClick={() => { patchPage(id, { cover: "" }); setCoverAt(null); }}>去掉封面</button>}
        </Pop>
      )}
      {menuAt && (
        <Pop at={menuAt} onClose={() => setMenuAt(null)} width={210}>
          <div className="menu">
            <button type="button" className="menu-item" onClick={() => { setMenuAt(null); void forAgent(); }}><Bot className="ico" /><span>交给 agent 写</span></button>
            <button type="button" className="menu-item" onClick={() => { setMenuAt(null); void exportMd(); }}><Download className="ico" /><span>导出 Markdown</span></button>
            <div className="menu-sep" />
            <button type="button" className="menu-item is-danger" onClick={() => { setMenuAt(null); trash(id); }}><Trash2 className="ico" /><span>移到回收站</span></button>
          </div>
        </Pop>
      )}
    </div>
  );
}

function Empty({ me, onOpen }: { me: string; onOpen: (id: string) => void }) {
  return (
    <div className="nt-empty">
      <FileText className="ico" />
      <b>还没有页面</b>
      <span>新建一页开始写，「/」插入标题、列表、待办；页面可以一层层套起来。也可以让 agent 帮你整理成页面。</span>
      <button type="button" className="btn btn-primary" onClick={() => onOpen(newPage(null, me))}><Plus className="ico" />新建页面</button>
    </div>
  );
}

// ---- the trash ----
function Trash({ onOpen }: { onOpen: (id: string) => void }) {
  const list = pages.rows.filter((p) => p.data.trashed && !pages.get(String(p.data.parent || ""))?.data.trashed)
    .sort((a, b) => titleOf(a).localeCompare(titleOf(b)));
  return (
    <div className="nt-page-wrap">
      <header className="nt-top"><span className="nt-crumb is-cur"><Trash2 className="ico" />回收站</span></header>
      <div className="nt-scroll">
        <div className="nt-trash">
          {!list.length && <div className="nt-empty-line faint">回收站是空的</div>}
          {list.map((p) => (
            <div key={p.id} className="nt-trash-row">
              <PageIcon p={p} /><span className="nt-row-title">{titleOf(p)}</span>
              <span className="faint">{subtree(p.id).length > 1 ? `含 ${subtree(p.id).length - 1} 个子页面` : ""}</span>
              <span className="top-gap" />
              <button type="button" className="btn" onClick={() => { for (const x of subtree(p.id)) patchPage(x, { trashed: false }); onOpen(p.id); }}><Undo2 className="ico" />恢复</button>
              <button type="button" className="btn is-danger" onClick={() => { if (window.confirm(`彻底删除「${titleOf(p)}」和它的子页面？删了就找不回来了。`)) destroy(p.id); }}><X className="ico" />彻底删除</button>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

// ---- ⌘K: find a page by its title or what is in it ----
function Finder({ onOpen, onClose }: { onOpen: (id: string) => void; onClose: () => void }) {
  const [q, setQ] = useState("");
  const [i, setI] = useState(0);
  const live = pages.rows.filter((p) => !p.data.trashed);
  const ql = q.trim().toLowerCase();
  const hits = (ql ? live.map((p) => {
    const inTitle = titleOf(p).toLowerCase().includes(ql);
    const b = pageBlocks(p.id).find((x) => String(x.data.text || "").toLowerCase().includes(ql));
    return inTitle || b ? { p, snip: b ? String(b.data.text || "") : "", rank: inTitle ? 0 : 1 } : null;
  }).filter((x): x is { p: PageRec; snip: string; rank: number } => !!x).sort((a, b) => a.rank - b.rank)
    : live.slice().sort((a, b) => num(b.version, 0) - num(a.version, 0)).slice(0, 8).map((p) => ({ p, snip: "", rank: 0 }))).slice(0, 30);
  const snip = (s: string) => {
    const k = s.toLowerCase().indexOf(ql);
    return k < 0 ? s.slice(0, 80) : `${k > 20 ? "…" : ""}${s.slice(Math.max(0, k - 20), k + 60)}`;
  };
  return (
    <div className="modal-back" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="nt-find" role="dialog" aria-label="搜索">
        <div className="nt-find-q"><Search className="ico" />
          <input autoFocus value={q} placeholder="搜索页面的标题和内容…" onChange={(e) => { setQ(e.target.value); setI(0); }}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") { e.preventDefault(); setI((x) => Math.min(hits.length - 1, x + 1)); }
              else if (e.key === "ArrowUp") { e.preventDefault(); setI((x) => Math.max(0, x - 1)); }
              else if (e.key === "Enter") { e.preventDefault(); if (hits[i]) onOpen(hits[i].p.id); }
              else if (e.key === "Escape") { e.preventDefault(); onClose(); }
            }} />
        </div>
        <div className="nt-find-list">
          {!ql && <div className="nt-find-sec">最近</div>}
          {hits.map((h, k) => (
            <button key={h.p.id} type="button" className={`nt-find-item${k === i ? " is-cur" : ""}`} onMouseEnter={() => setI(k)} onClick={() => onOpen(h.p.id)}>
              <PageIcon p={h.p} />
              <span className="nt-find-main">
                <span className="nt-find-title"><span className="faint">{pathOf(h.p.id).map((x) => `${titleOf(x)} / `).join("")}</span>{titleOf(h.p)}</span>
                {h.snip && <span className="nt-find-snip">{snip(h.snip)}</span>}
              </span>
            </button>
          ))}
          {ql && !hits.length && <div className="nt-empty-line faint">没有找到「{q}」</div>}
        </div>
      </div>
    </div>
  );
}

mount(kit, App);
