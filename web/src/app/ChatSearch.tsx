import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, ChevronUp, Search, X } from "lucide-react";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * Finding a message in a chat: ⌘F (Ctrl+F) or the magnifier in its head
 * opens a bar at the top of the chat. What you type is looked for in the
 * messages on screen (yours, its answers, supplements); every place it
 * occurs is marked, the current one more strongly, and the chat goes to
 * it. It starts at the newest; Enter goes to the one before (up), ⇧Enter
 * to the one after, Esc closes. A long chat whose older part is not loaded
 * yet offers to load it and look there too.
 */

export type SearchItem = { id: string; text: string };

const norm = (s: string) => s.toLocaleLowerCase();

/** The pane ⌘F belongs to: the chat last clicked in (two side by side). */
let activePane: HTMLElement | null = null;
export function markActivePane(el: HTMLElement | null) { activePane = el; }
export function ownsFind(el: HTMLElement | null): boolean {
  if (!el) return false;
  if (activePane && activePane.isConnected) return activePane === el;
  return document.querySelector(".conv-detail") === el;
}

/** Marks every place the text occurs in the chat, and the current one. */
function mark(root: HTMLElement | null, q: string, currentId: string) {
  const hl = (globalThis as { CSS?: { highlights?: Map<string, unknown> } }).CSS?.highlights;
  const Highlight = (globalThis as { Highlight?: new (...r: Range[]) => unknown }).Highlight;
  if (!hl || !Highlight) return;
  hl.delete("conv-search");
  hl.delete("conv-search-current");
  if (!root || !q) return;
  const all: Range[] = [];
  const cur: Range[] = [];
  const needle = norm(q);
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  for (let n = walker.nextNode(); n && all.length < 500; n = walker.nextNode()) {
    const text = norm(n.nodeValue || "");
    if (!text.includes(needle)) continue;
    const owner = (n.parentElement?.closest("[data-msg]") as HTMLElement | null)?.dataset.msg;
    for (let i = text.indexOf(needle); i >= 0; i = text.indexOf(needle, i + needle.length)) {
      const r = document.createRange();
      r.setStart(n, i);
      r.setEnd(n, i + needle.length);
      (owner === currentId ? cur : all).push(r);
    }
  }
  hl.set("conv-search", new Highlight(...all));
  hl.set("conv-search-current", new Highlight(...cur));
}

export function ChatSearch({ items, root, onJump, onClose, onLoadMore, loading }: {
  /** what can be found, in the order shown (id: the message's data-msg) */
  items: SearchItem[];
  /** the chat's timeline, where the places are marked */
  root: React.RefObject<HTMLElement | null>;
  onJump: (id: string) => void;
  onClose: () => void;
  /** there are older messages not loaded yet: load them */
  onLoadMore?: () => void;
  loading?: boolean;
}) {
  const [q, setQ] = useState("");
  const [at, setAt] = useState(-1);
  const input = useRef<HTMLInputElement>(null);
  const hits = useMemo(() => {
    const needle = norm(q.trim());
    return needle ? items.filter((x) => norm(x.text).includes(needle)) : [];
  }, [items, q]);
  // A new query starts at the newest.
  const last = useRef("");
  useEffect(() => {
    if (q.trim() === last.current) return;
    last.current = q.trim();
    setAt(hits.length - 1);
    if (hits.length) onJump(hits[hits.length - 1].id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q, hits.length]);
  const current = at >= 0 && at < hits.length ? hits[at] : undefined;
  useEffect(() => {
    // After the chat drew (a jump, new messages): mark again.
    const t = window.setTimeout(() => mark(root.current, q.trim(), current?.id || ""), 30);
    return () => window.clearTimeout(t);
  }, [q, current?.id, items, root]);
  useEffect(() => () => mark(null, "", ""), []);
  useEffect(() => { input.current?.focus(); }, []);

  const go = (d: -1 | 1) => {
    if (!hits.length) return;
    const n = (at + d + hits.length) % hits.length;
    setAt(n);
    onJump(hits[n].id);
  };
  return (
    <div className="conv-search" role="search">
      <Search className="conv-search-icon" aria-hidden />
      <input ref={input} value={q} onChange={(e) => setQ(e.target.value)} placeholder={L("在这段对话里找", "Find in this chat")}
        aria-label={L("在这段对话里找", "Find in this chat")}
        onKeyDown={(e) => {
          if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); onClose(); }
          else if (e.key === "Enter" && !e.nativeEvent.isComposing) { e.preventDefault(); go(e.shiftKey ? 1 : -1); }
        }} />
      <span className={cn("conv-search-count", q.trim() && !hits.length && "is-none")}>
        {!q.trim() ? "" : hits.length ? `${at + 1}/${hits.length}` : L("没找到", "No match")}
      </span>
      {onLoadMore && q.trim() && (
        <button type="button" className="conv-search-more" disabled={loading} onClick={onLoadMore}>
          {loading ? L("正在加载…", "Loading…") : L("再往前找", "Look further back")}
        </button>
      )}
      <button type="button" className="app-btn app-btn-plain" disabled={!hits.length} onClick={() => go(-1)} title={L("上一个（Enter）", "Previous (Enter)")} aria-label={L("上一个", "Previous")}><ChevronUp /></button>
      <button type="button" className="app-btn app-btn-plain" disabled={!hits.length} onClick={() => go(1)} title={L("下一个（⇧Enter）", "Next (⇧Enter)")} aria-label={L("下一个", "Next")}><ChevronDown /></button>
      <button type="button" className="app-btn app-btn-plain" onClick={onClose} title={L("关闭（Esc）", "Close (Esc)")} aria-label={L("关闭搜索", "Close search")}><X /></button>
    </div>
  );
}
