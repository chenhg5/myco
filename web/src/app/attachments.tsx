import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { Check, ChevronLeft, ChevronRight, Download, Eye, FileText, Loader2, X } from "lucide-react";
import { appApi } from "@/appapi";
import type { Attachment } from "@/api";
import { bytes } from "@/app/attach";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { FilePreview, isText, useVersion, type Version } from "@/app/preview";

/*
 * The files a message carries, as chat apps show them: pictures as
 * thumbnails (one larger, several in a grid) that open full size; a text
 * file (Markdown, a log, code) as a row that opens to read in the App
 * (preview.tsx); other files as a row you click to save to Downloads
 * (shown in Finder / Explorer). A picture that cannot be read (an older workspace, an expired
 * file) falls back to the row.
 */

const PICTURE = /^image\/(png|jpeg|gif|webp|avif|bmp)$/;
const PICTURE_EXT = /\.(png|jpe?g|gif|webp|avif|bmp)$/i;

export function isPicture(a: Attachment): boolean {
  return a.content_type ? PICTURE.test(a.content_type.split(";")[0].trim()) : PICTURE_EXT.test(a.name);
}

export function MessageAttachments({ hubId, items }: { hubId: string; items: Attachment[] }) {
  // Pictures that would not load: shown as rows instead.
  const [broken, setBroken] = useState<Set<string>>(new Set());
  const [open, setOpen] = useState<number | null>(null);
  const pictures = items.filter((a) => isPicture(a) && !broken.has(a.file_id));
  const files = items.filter((a) => !isPicture(a) || broken.has(a.file_id));
  const fail = (id: string) => setBroken((b) => new Set(b).add(id));
  return (
    <>
      {pictures.length > 0 && (
        <div className={cn("conv-shots", pictures.length === 1 && "is-one")}>
          {pictures.map((a, i) => (
            <button key={a.file_id} type="button" className="conv-attach is-image" onClick={() => setOpen(i)}
              title={L(`${a.name} · 点一下看大图`, `${a.name} · click to enlarge`)}>
              <img src={appApi.attachmentUrl(hubId, a.file_id)} alt={a.name} loading="lazy" draggable={false} onError={() => fail(a.file_id)} />
            </button>
          ))}
        </div>
      )}
      {files.length > 0 && (
        <div className="conv-attach-list">
          {files.map((a) => <FileRow key={a.file_id} hubId={hubId} a={a} />)}
        </div>
      )}
      {open !== null && pictures[open] && (
        <Lightbox hubId={hubId} items={pictures} index={open} onIndex={setOpen} onClose={() => setOpen(null)} />
      )}
    </>
  );
}

/** A file that is not a picture: a text file (Markdown, a log, code)
 *  opens to read in the App; anything else saves to Downloads. A file sent
 *  again under the same name says which version it is. */
function FileRow({ hubId, a }: { hubId: string; a: Attachment }) {
  const version = useVersion(a.file_id);
  const [reading, setReading] = useState(false);
  if (isText(a)) {
    return (
      <>
        <button type="button" className="conv-attach" onClick={() => setReading(true)} title={L("打开看看", "Open to read")}>
          <FileText />
          <span className="conv-attach-name">{a.name}</span>
          {version && version.n > 1 && <span className="conv-attach-version">{L(`第 ${version.n} 版`, `v${version.n}`)}</span>}
          <span className="conv-attach-size">{bytes(a.size)}</span>
          <Eye className="conv-attach-go" />
        </button>
        <FilePreview hubId={hubId} file={a} version={version} open={reading} onOpenChange={setReading} />
      </>
    );
  }
  return <SaveRow hubId={hubId} a={a} version={version} />;
}

/** A file that is not text: click to save it to Downloads. */
function SaveRow({ hubId, a, version }: { hubId: string; a: Attachment; version?: Version }) {
  const [state, setState] = useState<"idle" | "saving" | "saved" | "error">("idle");
  const [error, setError] = useState("");
  const save = async () => {
    if (state === "saving") return;
    setState("saving");
    try {
      await appApi.saveAttachment(hubId, a.file_id);
      setState("saved");
      window.setTimeout(() => setState("idle"), 2400);
    } catch (e) {
      const code = (e as { code?: string })?.code;
      setError(code === "not_found"
        ? L("拿不到这个文件了：可能已过期，或者工作区的 myco 版本太旧", "This file is not available: it may have expired, or the workspace's myco is too old")
        : (e as Error)?.message || L("没存下来", "Could not save it"));
      setState("error");
    }
  };
  return (
    <button type="button" className={cn("conv-attach", state === "error" && "is-error")} onClick={() => void save()}
      title={state === "error" ? error : L("存到「下载」文件夹", "Save to Downloads")}>
      <FileText />
      <span className="conv-attach-name">{a.name}</span>
      {version && version.n > 1 && <span className="conv-attach-version">{L(`第 ${version.n} 版`, `v${version.n}`)}</span>}
      <span className="conv-attach-size">
        {state === "saving" ? <Loader2 className="att-spin" />
          : state === "saved" ? <><Check />{L("已存到「下载」", "Saved to Downloads")}</>
          : state === "error" ? L("没存下来", "Not saved")
          : bytes(a.size)}
      </span>
      {state === "idle" && <Download className="conv-attach-go" />}
    </button>
  );
}

/** A picture full size over the app: Esc or a click outside closes it;
 *  ← → go through the message's pictures. */
function Lightbox({ hubId, items, index, onIndex, onClose }: {
  hubId: string; items: Attachment[]; index: number; onIndex: (i: number) => void; onClose: () => void;
}) {
  const a = items[index];
  const [saved, setSaved] = useState(false);
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      else if (e.key === "ArrowRight" && index < items.length - 1) onIndex(index + 1);
      else if (e.key === "ArrowLeft" && index > 0) onIndex(index - 1);
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [index, items.length, onClose, onIndex]);
  useEffect(() => setSaved(false), [index]);
  const save = async () => {
    try { await appApi.saveAttachment(hubId, a.file_id); setSaved(true); } catch { /* the row's way shows why */ }
  };
  return createPortal(
    <div className="app-portal lightbox" role="dialog" aria-label={a.name} onClick={onClose}>
      <div className="lightbox-bar" onClick={(e) => e.stopPropagation()}>
        <span className="lightbox-name">{a.name}{items.length > 1 && <span className="lightbox-count">{index + 1} / {items.length}</span>}</span>
        <button type="button" className="lightbox-btn" onClick={() => void save()} title={L("存到「下载」文件夹", "Save to Downloads")}>
          {saved ? <Check /> : <Download />}{saved ? L("已保存", "Saved") : L("保存", "Save")}
        </button>
        <button type="button" className="lightbox-btn is-icon" onClick={onClose} aria-label={L("关闭", "Close")} title={L("关闭（Esc）", "Close (Esc)")}><X /></button>
      </div>
      {index > 0 && (
        <button type="button" className="lightbox-nav is-prev" onClick={(e) => { e.stopPropagation(); onIndex(index - 1); }} aria-label={L("上一张", "Previous")}><ChevronLeft /></button>
      )}
      <img className="lightbox-img" src={appApi.attachmentUrl(hubId, a.file_id)} alt={a.name} onClick={(e) => e.stopPropagation()} draggable={false} />
      {index < items.length - 1 && (
        <button type="button" className="lightbox-nav is-next" onClick={(e) => { e.stopPropagation(); onIndex(index + 1); }} aria-label={L("下一张", "Next")}><ChevronRight /></button>
      )}
    </div>,
    document.body,
  );
}
