import { useState } from "react";
import { retryImage } from "@/app/Markdown";
import { Check, Download, Eye, FileText, Loader2 } from "lucide-react";
import { appApi } from "@/appapi";
import type { Attachment } from "@/api";
import { bytes } from "@/app/attach";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { FilePreview, isText, useVersion, type Version } from "@/app/preview";
import { Lightbox } from "@/app/lightbox";

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
              <img src={appApi.attachmentUrl(hubId, a.file_id)} alt={a.name} loading="lazy" draggable={false}
                onError={(e) => { if (!retryImage(e.currentTarget)) fail(a.file_id); }} />
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
        <Lightbox items={pictures.map((a) => ({ name: a.name, src: appApi.attachmentUrl(hubId, a.file_id), save: () => appApi.saveAttachment(hubId, a.file_id) }))}
          index={open} onIndex={setOpen} onClose={() => setOpen(null)} />
      )}
    </>
  );
}

/** A file that is not a picture: a text file (Markdown, a log, code)
 *  opens to read in the App; anything else saves to Downloads. A file sent
 *  again under the same name says which version it is. */
export function FileRow({ hubId, a }: { hubId: string; a: Attachment }) {
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

