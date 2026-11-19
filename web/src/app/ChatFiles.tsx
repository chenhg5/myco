import { useMemo, useState } from "react";
import { ImageOff, Loader2, LocateFixed } from "lucide-react";
import { appApi } from "@/appapi";
import type { Attachment } from "@/api";
import { FileRow, isPicture } from "@/app/attachments";
import { Lightbox } from "@/app/lightbox";
import { PageCard } from "@/app/pages";
import { DockPanel } from "@/app/preview";
import { L } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";

/** A file sent in this chat: the message it came with (to go back to it),
 *  who sent it and when. */
export type ChatFile = { a: Attachment; msg: string; from: string; at: string };
/** An app (a page) linked in this chat, with the message it came in. */
export type ChatApp = { hubId: string; id: string; url: string; msg: string; from: string; at: string };

/*
 * 文件和应用: everything sent in a chat, in one place. The apps (pages)
 * linked in it, newest first, as the same cards as in the chat; the pictures as a grid (newest
 * first, one click to see them full size and go through them), the other
 * files as rows (open to read, or save), each with who sent it, when, and
 * a way back to its message. Only what is loaded is listed: a long chat
 * between agents loads earlier pages from here too.
 */
export function ChatFiles({ open, onOpenChange, hubId, items, apps = [], nameOf, now, more, loadingMore, onMore, onJump }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  hubId: string;
  items: ChatFile[];
  apps?: ChatApp[];
  nameOf: (n: string) => string;
  now: number;
  /** earlier messages can still be loaded */
  more?: boolean;
  loadingMore?: boolean;
  onMore?: () => void;
  onJump: (messageId: string) => void;
}) {
  const [kind, setKind] = useState<"all" | "apps" | "pictures" | "files">("all");
  const [broken, setBroken] = useState<Set<string>>(new Set());
  const [shown, setShown] = useState<number | null>(null);
  // Newest message first; a message's own files in the order it sent them.
  const newest = useMemo(() => {
    const out: ChatFile[] = [];
    for (let end = items.length; end > 0;) {
      let start = end - 1;
      while (start > 0 && items[start - 1].msg === items[end - 1].msg && items[start - 1].at === items[end - 1].at) start--;
      out.push(...items.slice(start, end));
      end = start;
    }
    return out;
  }, [items]);
  // Each app once, where it was last linked.
  const appList = useMemo(() => {
    const by = new Map<string, ChatApp>();
    for (const a of apps) { const had = by.get(a.id); if (!had || a.at >= had.at) by.set(a.id, a); }
    return [...by.values()].sort((x, y) => (y.at > x.at ? 1 : -1));
  }, [apps]);
  const pictures = newest.filter((f) => isPicture(f.a));
  const files = newest.filter((f) => !isPicture(f.a));
  const meta = (f: { from: string; at: string }) => `${nameOf(f.from)} · ${timeAgo(f.at, now)}`;
  const jump = (f: { msg: string }) => { onJump(f.msg); if (window.innerWidth < 900) onOpenChange(false); };
  const count = L(`${appList.length} 个应用 · ${pictures.length} 张图片 · ${files.length} 个文件`, `${appList.length} apps · ${pictures.length} pictures · ${files.length} files`);
  return (
    <DockPanel open={open} onOpenChange={onOpenChange} title={L("文件和应用", "Files and apps")} about={count} className="cf-panel" bodyClass="cf-body">
      <div className="cf-tabs" role="tablist">
        {([["all", L("全部", "All")], ["apps", L(`应用 ${appList.length}`, `Apps ${appList.length}`)], ["pictures", L(`图片 ${pictures.length}`, `Pictures ${pictures.length}`)], ["files", L(`文件 ${files.length}`, `Files ${files.length}`)]] as const).map(([k, label]) => (
          <button key={k} type="button" role="tab" aria-selected={kind === k} className={cn("tp-chip", kind === k && "is-on")} onClick={() => setKind(k)}>{label}</button>
        ))}
      </div>
      {items.length === 0 && appList.length === 0 && (
        <div className="cf-empty" data-testid="chat-files-empty">
          <b>{L("还没有文件和应用", "No files or apps yet")}</b>
          <p>{more ? L("加载过的消息里没有。更早的消息里可能有。", "None in the messages loaded so far; earlier ones may have some.")
            : L("你和 agent 发的图片、文件，agent 做好发在对话里的应用（网页），都会列在这里。", "Pictures and files you and the agent send, and the apps (pages) it makes and shares here, are listed here.")}</p>
        </div>
      )}
      {(kind === "all" || kind === "apps") && appList.length > 0 && (
        <section className="cf-section">
          {kind === "all" && <h3>{L("应用", "Apps")}<span>{appList.length}</span></h3>}
          <div className="cf-apps" data-testid="chat-files-apps">
            {appList.map((a) => (
              <div key={a.id} className="cf-app">
                <PageCard hubId={a.hubId} id={a.id} url={a.url} />
                <div className="cf-file-meta is-app">
                  <button type="button" className="cf-locate" onClick={() => jump(a)} title={L("回到对话里这条消息", "Go to its message in the chat")}>
                    <LocateFixed />{L("在对话中查看", "Show in chat")}
                  </button>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}
      {kind === "apps" && appList.length === 0 && items.length > 0 && <div className="cf-empty"><p>{L("这段对话里还没有应用。agent 发布的网页、做好的小工具会出现在这里。", "No apps in this chat yet. Pages agents publish and tools they make show up here.")}</p></div>}
      {(kind === "all" || kind === "pictures") && pictures.length > 0 && (
        <section className="cf-section">
          {kind === "all" && <h3>{L("图片", "Pictures")}<span>{pictures.length}</span></h3>}
          <div className="cf-grid" data-testid="chat-files-pictures">
            {pictures.map((f, i) => (
              <button key={`${f.msg}-${f.a.file_id}`} type="button" className="cf-shot" onClick={() => setShown(i)} title={`${f.a.name}\n${meta(f)}`}>
                {broken.has(f.a.file_id)
                  ? <span className="cf-shot-gone"><ImageOff /><small>{L("打不开", "Not available")}</small></span>
                  : <img src={appApi.attachmentUrl(hubId, f.a.file_id)} alt={f.a.name} loading="lazy" draggable={false}
                    onError={() => setBroken((b) => new Set(b).add(f.a.file_id))} />}
              </button>
            ))}
          </div>
        </section>
      )}
      {(kind === "all" || kind === "files") && files.length > 0 && (
        <section className="cf-section">
          {kind === "all" && <h3>{L("文件", "Files")}<span>{files.length}</span></h3>}
          <div className="cf-files" data-testid="chat-files-files">
            {files.map((f) => (
              <div key={`${f.msg}-${f.a.file_id}`} className="cf-file">
                <FileRow hubId={hubId} a={f.a} />
                <div className="cf-file-meta">
                  <span>{meta(f)}</span>
                  <button type="button" className="cf-locate" onClick={() => jump(f)} title={L("回到对话里这条消息", "Go to its message in the chat")}>
                    <LocateFixed />{L("在对话中查看", "Show in chat")}
                  </button>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}
      {more && onMore && (
        <button type="button" className="app-btn cf-more" onClick={onMore} disabled={loadingMore}>
          {loadingMore && <Loader2 className="att-spin" />}{L("再往前找", "Look further back")}
        </button>
      )}
      {shown !== null && pictures[shown] && (
        <Lightbox items={pictures.map((f) => ({ name: `${f.a.name} · ${meta(f)}`, src: appApi.attachmentUrl(hubId, f.a.file_id), save: () => appApi.saveAttachment(hubId, f.a.file_id) }))}
          index={shown} onIndex={setShown} onClose={() => setShown(null)} />
      )}
    </DockPanel>
  );
}
