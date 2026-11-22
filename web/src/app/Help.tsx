import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronLeft, ChevronRight, Search } from "lucide-react";
import { Markdown } from "@/app/Markdown";
import { useNavSub } from "@/app/nav";
import { noteDone } from "@/app/Onboard";
import { Lightbox, type Picture } from "@/app/lightbox";
import { HELP_GROUPS, HELP_ORDER, helpText, helpTitle, isHelpId, type HelpId } from "@/app/help/docs";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * 使用文档: the user guide, as a documentation site reads: the pages in
 * groups down the left (with a search box), the page on the right as a
 * document, the previous and next at its foot. A page links another as
 * [text](#help/<id>). The page last read opens next time (this computer).
 */

const LAST = "myco-help-last";
const lastRead = (): HelpId => {
  try { const v = localStorage.getItem(LAST); if (isHelpId(v)) return v; } catch { /* none */ }
  return "start";
};

/** Opens the guide (at a page) from anywhere: AppMode listens. */
export function openHelp(doc?: HelpId) {
  window.dispatchEvent(new CustomEvent("myco-open-help", { detail: { doc } }));
}

export function HelpPage({ start }: { start?: HelpId }) {
  const [doc, setDocState] = useState<HelpId>(() => start || lastRead());
  const [q, setQ] = useState("");
  const main = useRef<HTMLDivElement>(null);
  const setDoc = (d: HelpId) => {
    setDocState(d);
    try { localStorage.setItem(LAST, d); } catch { /* not kept */ }
    main.current?.scrollTo({ top: 0 });
  };
  useEffect(() => { if (start) setDoc(start); }, [start]); // eslint-disable-line react-hooks/exhaustive-deps
  // Reading the guide is the last step of the 上手清单.
  useEffect(() => { noteDone("guide"); }, []);
  // ← → at the top: the page read is part of where you are.
  useNavSub("help", { doc }, { doc: "start" as HelpId }, (v) => { if (isHelpId(v.doc)) setDocState(v.doc); });

  // Search: the pages whose title or text has every word typed, each with
  // the line it was found in.
  const found = useMemo(() => {
    const words = q.trim().toLowerCase().split(/\s+/).filter(Boolean);
    if (!words.length) return null;
    return HELP_ORDER.flatMap((id) => {
      const text = helpText(id);
      const low = text.toLowerCase();
      if (!words.every((w) => low.includes(w))) return [];
      const line = text.split("\n").find((l) => !l.startsWith("#") && words.some((w) => l.toLowerCase().includes(w))) || "";
      return [{ id, line: line.replace(/[*`#>|[\]()]/g, "").replace(/\s+/g, " ").trim().slice(0, 80) }];
    });
  }, [q]);

  const at = HELP_ORDER.indexOf(doc);
  const prev = at > 0 ? HELP_ORDER[at - 1] : null;
  const next = at < HELP_ORDER.length - 1 ? HELP_ORDER[at + 1] : null;
  // A picture opens full size (← → through the page's pictures).
  const [shots, setShots] = useState<{ items: Picture[]; index: number } | null>(null);
  // A link to another page of the guide opens it here.
  const onClick = (e: React.MouseEvent) => {
    const img = (e.target as Element).closest(".hp-doc .md img");
    if (img instanceof HTMLImageElement && main.current) {
      const all = [...main.current.querySelectorAll<HTMLImageElement>(".hp-doc .md img")].filter((x) => !x.closest("a"));
      const i = all.indexOf(img);
      if (i >= 0) {
        e.preventDefault();
        setShots({ items: all.map((x) => ({ name: x.alt || helpTitle(doc), src: x.currentSrc || x.src })), index: i });
        return;
      }
    }
    const a = (e.target as Element).closest("a");
    const href = a?.getAttribute("href") || "";
    const m = /^#help\/([\w-]+)$/.exec(href);
    if (!m) return;
    e.preventDefault();
    if (isHelpId(m[1])) setDoc(m[1]);
  };

  return (
    <div className="hp" data-testid="help">
      <nav className="hp-nav" aria-label={L("使用文档目录", "Guide contents")}>
        <label className="hp-search">
          <Search />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={L("搜索文档", "Search the guide")} aria-label={L("搜索文档", "Search the guide")}
            onKeyDown={(e) => { if (e.key === "Escape") setQ(""); if (e.key === "Enter" && found?.[0]) { setDoc(found[0].id); setQ(""); } }} />
        </label>
        {found ? (
          <div className="hp-group">
            {found.length === 0 && <p className="hp-none">{L("没有找到", "Nothing found")}</p>}
            {found.map((f) => (
              <button key={f.id} type="button" className={cn("hp-item", doc === f.id && "is-on")} onClick={() => { setDoc(f.id); setQ(""); }}>
                <span>{helpTitle(f.id)}</span>
                {f.line && <small>{f.line}</small>}
              </button>
            ))}
          </div>
        ) : HELP_GROUPS.map((g) => (
          <div className="hp-group" key={g.title()}>
            <div className="hp-group-title">{g.title()}</div>
            {g.ids.map((id) => (
              <button key={id} type="button" className={cn("hp-item", doc === id && "is-on")} aria-current={doc === id ? "page" : undefined}
                onClick={() => setDoc(id)} data-help={id}>
                <span>{helpTitle(id)}</span>
              </button>
            ))}
          </div>
        ))}
        {/* (narrow windows: the same, as one list to pick from) */}
        <select className="app-select-field hp-pick" value={doc} onChange={(e) => isHelpId(e.target.value) && setDoc(e.target.value)} aria-label={L("选一篇", "Pick a page")}>
          {HELP_GROUPS.map((g) => (
            <optgroup key={g.title()} label={g.title()}>
              {g.ids.map((id) => <option key={id} value={id}>{helpTitle(id)}</option>)}
            </optgroup>
          ))}
        </select>
      </nav>
      <div className="hp-main" ref={main} onClick={onClick}>
        <article className="fp-doc hp-doc" data-testid="help-doc" data-doc={doc}>
          <Markdown text={helpText(doc)} />
          <div className="hp-pager">
            {prev ? (
              <button type="button" className="hp-pager-btn" onClick={() => setDoc(prev)}>
                <small>{L("上一篇", "Previous")}</small><span><ChevronLeft />{helpTitle(prev)}</span>
              </button>
            ) : <span />}
            {next && (
              <button type="button" className="hp-pager-btn is-next" onClick={() => setDoc(next)}>
                <small>{L("下一篇", "Next")}</small><span>{helpTitle(next)}<ChevronRight /></span>
              </button>
            )}
          </div>
        </article>
      </div>
      {shots && <Lightbox items={shots.items} index={shots.index} onIndex={(index) => setShots({ ...shots, index })} onClose={() => setShots(null)} />}
    </div>
  );
}
