import { useRef, useState } from "react";
import { Check, ImagePlus } from "lucide-react";
import { Modal } from "@/app/dialogs";
import { PRESETS, defaultBg, keepImage, ownBg, setBg, useImageUrl, type ChatBg } from "@/app/chatBg";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * Picking a chat's background (chatBg.ts): for this conversation, or the
 * default for all of them. What you pick is used at once (the preview
 * shows it with a few lines of chat); 好 closes.
 */
export function ChatBgDialog({ open, onClose, chat, name }: {
  open: boolean;
  onClose: () => void;
  /** the conversation (chatKey); none: the default only (Settings) */
  chat?: string;
  /** its other side, for the preview and the scope's words */
  name?: string;
}) {
  const [scope, setScope] = useState<"chat" | "all">(chat ? "chat" : "all");
  const [, setTick] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const file = useRef<HTMLInputElement>(null);
  const key = scope === "chat" && chat ? chat : "";
  const current: ChatBg | undefined = key ? ownBg(key) : defaultBg();
  const shown: ChatBg | undefined = current ?? (key ? defaultBg() : undefined);
  const pick = (bg: ChatBg | undefined) => { setBg(key, bg); setTick((n) => n + 1); };
  const imgUrl = useImageUrl(shown?.image);
  const presetCss = PRESETS.find((p) => p.id === shown?.preset)?.css;
  const previewBg = shown?.image ? (imgUrl ? `url("${imgUrl}") center / cover no-repeat` : undefined) : presetCss;
  const veil = shown?.veil ?? (shown?.image ? 0.55 : 0);
  const choose = async (f?: File) => {
    if (!f) return;
    setBusy(true);
    setError("");
    try {
      const id = await keepImage(f);
      pick({ image: id, veil: current?.image ? current.veil ?? 0.55 : 0.55 });
    } catch {
      setError(L("这张图片读不了，换一张试试", "This picture could not be read: try another"));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open={open} onOpenChange={(o) => !o && onClose()} title={L("聊天背景", "Chat background")} className="cbg-dialog"
      description={L("只保存在这台电脑上。分栏时每一栏各自铺满、裁边，不会变形。", "Kept on this computer only. Split into panes, each fills its own and crops the edges; nothing is stretched.")}>
      {chat && (
        <div className="seg cbg-scope" role="tablist" aria-label={L("用在哪", "Where")}>
          <button type="button" role="tab" aria-selected={scope === "chat"} className={cn("seg-btn", scope === "chat" && "active")} onClick={() => setScope("chat")}>
            {name ? L(`和 ${name} 的对话`, `This chat with ${name}`) : L("这个对话", "This chat")}
          </button>
          <button type="button" role="tab" aria-selected={scope === "all"} className={cn("seg-btn", scope === "all" && "active")} onClick={() => setScope("all")}>
            {L("所有对话的默认", "Default for all")}
          </button>
        </div>
      )}
      <div className="cbg-preview" data-testid="chat-bg-preview">
        {previewBg && <div className="cbg-layer" style={{ background: previewBg }} />}
        {previewBg && veil > 0 && <div className="cbg-veil" style={{ opacity: veil }} />}
        <div className="cbg-sample">
          <span className="cbg-sample-agent">{L("首页改好了，三种宽度下都看过。", "The home page is done; checked at three widths.")}</span>
          <span className="cbg-sample-me">{L("好，发版吧", "Good, release it")}</span>
        </div>
      </div>
      <div className="cbg-tiles" role="listbox" aria-label={L("背景", "Background")}>
        <button type="button" role="option" aria-selected={!current} className={cn("cbg-tile is-none", !current && "is-on")} onClick={() => pick(undefined)}>
          <span className="cbg-swatch" />
          <span className="cbg-name">{key ? L("跟随默认", "Default") : L("无", "None")}</span>
          {!current && <Check className="cbg-check" />}
        </button>
        {PRESETS.map((p) => {
          const on = current?.preset === p.id && !current.image;
          return (
            <button key={p.id} type="button" role="option" aria-selected={on} className={cn("cbg-tile", on && "is-on")} onClick={() => pick({ preset: p.id })}>
              <span className="cbg-swatch" style={{ background: p.css }} />
              <span className="cbg-name">{p.name()}</span>
              {on && <Check className="cbg-check" />}
            </button>
          );
        })}
        <ImageTile id={current?.image} busy={busy} onPick={() => file.current?.click()} />
        <input ref={file} type="file" accept="image/*" hidden onChange={(e) => { void choose(e.target.files?.[0]); e.target.value = ""; }} />
      </div>
      {error && <div className="app-error">{error}</div>}
      {current?.image && (
        <label className="cbg-veil-line">
          <span>{L("淡化", "Fade")}</span>
          <input type="range" min={0} max={90} step={5} value={Math.round((current.veil ?? 0.55) * 100)}
            onChange={(e) => pick({ ...current, veil: Number(e.target.value) / 100 })} aria-label={L("淡化背景图片", "Fade the picture")} />
          <span className="cbg-veil-value">{Math.round((current.veil ?? 0.55) * 100)}%</span>
        </label>
      )}
      <div className="app-modal-actions">
        <button type="button" className="app-btn app-btn-primary" onClick={onClose}>{L("好", "Done")}</button>
      </div>
    </Modal>
  );
}

function ImageTile({ id, busy, onPick }: { id?: string; busy: boolean; onPick: () => void }) {
  const url = useImageUrl(id);
  return (
    <button type="button" role="option" aria-selected={!!id} className={cn("cbg-tile is-image", !!id && "is-on")} onClick={onPick} disabled={busy}
      title={id ? L("换一张图片", "Choose another picture") : L("从电脑里选一张图片", "Choose a picture on this computer")}>
      <span className="cbg-swatch" style={url ? { background: `url("${url}") center / cover no-repeat` } : undefined}>{!url && <ImagePlus />}</span>
      <span className="cbg-name">{busy ? L("正在读取…", "Reading…") : id ? L("换图片", "Change") : L("选图片", "Picture")}</span>
      {id && <Check className="cbg-check" />}
    </button>
  );
}
