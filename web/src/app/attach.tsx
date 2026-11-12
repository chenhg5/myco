import { useEffect, useRef, useState } from "react";
import { AlertCircle, FileText, Loader2, Paperclip, X } from "lucide-react";
import { appApi } from "@/appapi";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * Files to send with a message: picked with the paperclip, pasted into the
 * box, or dropped on it. Each is uploaded as soon as it is added (to the
 * workspace, as yours), so sending is only the message; it can go with
 * text or alone. Shared by the chat's box and the group's.
 */

export type Pending = {
  key: string;
  name: string;
  size: number;
  /** a picture's thumbnail (object URL) */
  preview?: string;
  state: "uploading" | "done" | "error";
  id?: string;
  error?: string;
};

// A screenshot pasted from the clipboard arrives as "image.png": give it a
// name that says when.
function nameOf(f: File): string {
  if (/^image\.(png|jpe?g|gif|webp)$/i.test(f.name)) {
    const d = new Date();
    const p = (n: number) => String(n).padStart(2, "0");
    return `${L("截图", "screenshot")}-${p(d.getHours())}${p(d.getMinutes())}${p(d.getSeconds())}.${f.name.split(".").pop()}`;
  }
  return f.name || "file";
}

export function bytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(n < 10 * 1024 ? 1 : 0)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export function useAttachments(hubId: string) {
  const [items, setItems] = useState<Pending[]>([]);
  const live = useRef(true);
  useEffect(() => {
    live.current = true;
    return () => { live.current = false; };
  }, []);
  const patch = (key: string, p: Partial<Pending>) => {
    if (live.current) setItems((all) => all.map((x) => (x.key === key ? { ...x, ...p } : x)));
  };
  const add = (files: File[]) => {
    for (const f of files) {
      const key = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      const name = nameOf(f);
      const preview = f.type.startsWith("image/") ? URL.createObjectURL(f) : undefined;
      setItems((all) => [...all, { key, name, size: f.size, preview, state: "uploading" }]);
      appApi.uploadAttachment(hubId, f, name)
        .then((r) => patch(key, { state: "done", id: r.id }))
        .catch((e: { code?: string; message?: string }) => patch(key, {
          state: "error",
          error: e?.code === "not_found"
            ? L("这个工作区还不能从 App 发文件，升级它的 myco 就可以", "This workspace can't take files from the app yet; update its myco")
            : e?.message || L("没传上去", "Upload failed"),
        }));
    }
  };
  const remove = (key: string) => setItems((all) => {
    const gone = all.find((x) => x.key === key);
    if (gone?.preview) URL.revokeObjectURL(gone.preview);
    return all.filter((x) => x.key !== key);
  });
  const clear = () => setItems((all) => {
    all.forEach((x) => x.preview && URL.revokeObjectURL(x.preview));
    return [];
  });
  const ready = items.filter((x) => x.state === "done" && x.id);
  return {
    items,
    add,
    remove,
    clear,
    ids: ready.map((x) => x.id!),
    /** still uploading: wait before sending */
    busy: items.some((x) => x.state === "uploading"),
    /** something to send even with no text */
    any: ready.length > 0,
    /** the text to send: what was typed, or (files alone) what they are */
    body: (text: string) => text.trim() || (ready.length === 1
      ? L(`发来一个文件：${ready[0].name}`, `Sent a file: ${ready[0].name}`)
      : L(`发来 ${ready.length} 个文件：${ready.map((x) => x.name).join("、")}`, `Sent ${ready.length} files: ${ready.map((x) => x.name).join(", ")}`)),
  };
}

/** Files on the clipboard (a screenshot, files copied in Finder). */
export function pastedFiles(e: React.ClipboardEvent): File[] {
  return Array.from(e.clipboardData?.files || []);
}

/** Lets a box take files dropped on it. */
export function dropFiles(onFiles: (f: File[]) => void) {
  return {
    onDragOver: (e: React.DragEvent) => {
      if (Array.from(e.dataTransfer.types).includes("Files")) e.preventDefault();
    },
    onDrop: (e: React.DragEvent) => {
      const files = Array.from(e.dataTransfer.files || []);
      if (!files.length) return;
      e.preventDefault();
      onFiles(files);
    },
  };
}

/** The paperclip: choose files to send. */
export function AttachButton({ onFiles, disabled }: { onFiles: (f: File[]) => void; disabled?: boolean }) {
  const input = useRef<HTMLInputElement>(null);
  return (
    <>
      <button type="button" className="att-btn" disabled={disabled} onClick={(e) => { e.stopPropagation(); input.current?.click(); }}
        title={L("加文件（也可以直接粘贴或拖进来）", "Add files (or paste or drop them here)")} aria-label={L("加文件", "Add files")}>
        <Paperclip />
      </button>
      <input ref={input} type="file" multiple hidden onChange={(e) => {
        const files = Array.from(e.target.files || []);
        e.target.value = "";
        if (files.length) onFiles(files);
      }} />
    </>
  );
}

/** A file name short enough for a tile (two short lines), cut in the
 *  middle so the kind (".md") stays: "2026 年第…说明.md". Wide (CJK)
 *  letters count double. */
export function shortName(name: string, room = 16): string {
  const w = (c: string) => (/[\u2e80-\uffff]/.test(c) ? 2 : 1);
  const width = (t: string) => [...t].reduce((n, c) => n + w(c), 0);
  if (width(name) <= room) return name;
  const dot = name.lastIndexOf(".");
  const ext = dot > 0 && name.length - dot <= 6 ? name.slice(dot) : "";
  const base = [...name.slice(0, name.length - ext.length)];
  // About two thirds of what is left for the start, the rest for the end.
  const left = room - 1 - width(ext);
  let head = "", tail = "";
  for (const c of base) {
    if (width(head + c) > Math.ceil(left * 0.62)) break;
    head += c;
  }
  for (let i = base.length - 1; i >= head.length; i--) {
    if (width(head) + width(base[i] + tail) > left) break;
    tail = base[i] + tail;
  }
  return `${head.trimEnd()}…${tail.trimStart()}${ext}`;
}

/** The files about to go, above the text: small tiles of one size. A
 *  picture is its thumbnail alone (it says what it is); another file is an
 *  icon and its name, cut in the middle. The full name and size on hover. */
export function AttachList({ items, onRemove }: { items: Pending[]; onRemove: (key: string) => void }) {
  if (!items.length) return null;
  return (
    <div className="att-list" onClick={(e) => e.stopPropagation()}>
      {items.map((x) => (
        <div key={x.key} className={cn("att-chip", x.preview && "is-picture", x.state === "error" && "is-error", x.state === "uploading" && "is-busy")}
          title={x.error ? `${x.name}\n${x.error}` : `${x.name} · ${bytes(x.size)}`}>
          {x.preview ? <img className="att-thumb" src={x.preview} alt={x.name} /> : (
            <span className="att-file">
              <FileText />
              <span className="att-name">{shortName(x.name)}</span>
            </span>
          )}
          {x.state === "uploading" && <span className="att-cover" aria-label={L("正在上传…", "Uploading…")}><Loader2 className="att-spin" /></span>}
          {x.state === "error" && <span className="att-cover is-error"><AlertCircle /></span>}
          <button type="button" className="att-x" onClick={() => onRemove(x.key)} aria-label={L("不发这个", "Remove")} title={L("不发这个", "Remove")}><X /></button>
        </div>
      ))}
    </div>
  );
}
