import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { Check, ChevronLeft, ChevronRight, Download, X } from "lucide-react";
import { L } from "@/lib/i18n";

/** A picture to show full size: where it is, and how to keep it (a sent
 *  file; one not sent yet has nothing to save). */
export type Picture = { name: string; src: string; save?: () => Promise<unknown> };

/** A picture full size over the app: Esc or a click outside closes it;
 *  ← → go through the pictures with it (a message's, or those waiting to be
 *  sent with one). */
export function Lightbox({ items, index, onIndex, onClose }: {
  items: Picture[]; index: number; onIndex: (i: number) => void; onClose: () => void;
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
  if (!a) return null;
  const save = async () => {
    try { await a.save?.(); setSaved(true); } catch { /* the row's way shows why */ }
  };
  return createPortal(
    <div className="app-portal lightbox" role="dialog" aria-label={a.name} onClick={onClose}>
      <div className="lightbox-bar" onClick={(e) => e.stopPropagation()}>
        <span className="lightbox-name">{a.name}{items.length > 1 && <span className="lightbox-count">{index + 1} / {items.length}</span>}</span>
        {a.save && (
          <button type="button" className="lightbox-btn" onClick={() => void save()} title={L("存到「下载」文件夹", "Save to Downloads")}>
            {saved ? <Check /> : <Download />}{saved ? L("已保存", "Saved") : L("保存", "Save")}
          </button>
        )}
        <button type="button" className="lightbox-btn is-icon" onClick={onClose} aria-label={L("关闭", "Close")} title={L("关闭（Esc）", "Close (Esc)")}><X /></button>
      </div>
      {index > 0 && (
        <button type="button" className="lightbox-nav is-prev" onClick={(e) => { e.stopPropagation(); onIndex(index - 1); }} aria-label={L("上一张", "Previous")}><ChevronLeft /></button>
      )}
      <img className="lightbox-img" src={a.src} alt={a.name} onClick={(e) => e.stopPropagation()} draggable={false} />
      {index < items.length - 1 && (
        <button type="button" className="lightbox-nav is-next" onClick={(e) => { e.stopPropagation(); onIndex(index + 1); }} aria-label={L("下一张", "Next")}><ChevronRight /></button>
      )}
    </div>,
    document.body,
  );
}
