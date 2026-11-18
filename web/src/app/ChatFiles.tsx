import { useMemo, useState } from "react";
import { ImageOff, Loader2, LocateFixed } from "lucide-react";
import { appApi } from "@/appapi";
import type { Attachment } from "@/api";
import { FileRow, isPicture } from "@/app/attachments";
import { Lightbox } from "@/app/lightbox";
import { DockPanel } from "@/app/preview";
import { L } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";

/** A file sent in this chat: the message it came with (to go back to it),
 *  who sent it and when. */
export type ChatFile = { a: Attachment; msg: string; from: string; at: string };

/*
 * Everything sent in a chat, in one place: the pictures as a grid (newest
 * first, one click to see them full size and go through them), the other
 * files as rows (open to read, or save), each with who sent it, when, and
 * a way back to its message. Only what is loaded is listed: a long chat
 * between agents loads earlier pages from here too.
 */
export function ChatFiles({ open, onOpenChange, hubId, items, nameOf, now, more, loadingMore, onMore, onJump }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  hubId: string;
  items: ChatFile[];
  nameOf: (n: string) => string;
  now: number;
  /** earlier messages can still be loaded */
  more?: boolean;
  loadingMore?: boolean;
  onMore?: () => void;
  onJump: (messageId: string) => void;
}) {
  const [kind, setKind] = useState<"all" | "pictures" | "files">("all");
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
  const pictures = newest.filter((f) => isPicture(f.a));
  const files = newest.filter((f) => !isPicture(f.a));
  const meta = (f: ChatFile) => `${nameOf(f.from)} · ${timeAgo(f.at, now)}`;
  const jump = (f: ChatFile) => { onJump(f.msg); if (window.innerWidth < 900) onOpenChange(false); };
  const count = L(`${pictures.length} 张图片 · ${files.length} 个文件`, `${pictures.length} pictures · ${files.length} files`);
  return (
    <DockPanel open={open} onOpenChange={onOpenChange} title={L("图片和文件", "Pictures and files")} about={count} className="cf-panel" bodyClass="cf-body">
      <div className="cf-tabs" role="tablist">
        {([["all", L("全部", "All")], ["pictures", L(`图片 ${pictures.length}`, `Pictures ${pictures.length}`)], ["files", L(`文件 ${files.length}`, `Files ${files.length}`)]] as const).map(([k, label]) => (
          <button key={k} type="button" role="tab" aria-selected={kind === k} className={cn("tp-chip", kind === k && "is-on")} onClick={() => setKind(k)}>{label}</button>
        ))}
      </div>
      {items.length === 0 && (
        <div className="cf-empty" data-testid="chat-files-empty">
          <b>{L("还没有图片和文件", "No pictures or files yet")}</b>
          <p>{more ? L("加载过的消息里没有。更早的消息里可能有。", "None in the messages loaded so far; earlier ones may have some.")
            : L("在输入框里粘贴、拖进来或点回形针发的，和 agent 发给你的，都会列在这里。", "What you paste, drop or attach, and what agents send you, is listed here.")}</p>
        </div>
      )}
      {kind !== "files" && pictures.length > 0 && (
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
      {kind !== "pictures" && files.length > 0 && (
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
