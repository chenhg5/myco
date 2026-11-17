import { useCallback, useEffect, useRef, useState } from "react";
import { FileText, Globe, MessageSquareText, Pin as PinIcon, Plus } from "lucide-react";
import { appApi } from "@/appapi";
import type { Attachment, Pin, PinCreate } from "@/api";
import { ErrorText, Modal } from "@/app/dialogs";
import { useContextMenu, type MenuItem } from "@/app/ContextMenu";
import { FilePreview, isText } from "@/app/preview";
import { isPicture } from "@/app/attachments";
import { Lightbox } from "@/app/lightbox";
import { openPage, pageTargetOf } from "@/app/pages";
import { copyText } from "@/app/copy";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * 置顶: what a conversation keeps at hand, one click away above its
 * messages: a web address (a myco page opens in the page panel, a local
 * server too, the rest in the browser), a file (read in the App, or saved),
 * or one of its messages (scrolled to and lit up). Both parties see the
 * same list, kept on the hub (hubs from v0.20.3): the person pins here (a
 * message's 置顶, the header's pin button for an address); an agent with
 * `myco pin add`. Older hubs: no strip and no way in.
 */

/** Tells every strip of this conversation to read its pins again. */
export function pinsChanged(hubId: string) {
  window.dispatchEvent(new CustomEvent("myco-pins-changed", { detail: { hubId } }));
}

/** The pins of the conversation between `a` (you when absent) and
 *  `withWhom`: null while unknown or on a hub without pins. */
export function usePins(hubId: string, withWhom: string, a?: string) {
  const [pins, setPins] = useState<Pin[] | null>(null);
  const [supported, setSupported] = useState(false);
  const key = `${hubId}\u0000${withWhom}\u0000${a || ""}`;
  const keyRef = useRef(key);
  keyRef.current = key;
  const load = useCallback(async () => {
    const k = key;
    try {
      const l = await appApi.pins(hubId, withWhom, a);
      if (keyRef.current !== k) return;
      setSupported(true);
      setPins(l.pins);
    } catch (e) {
      if (keyRef.current !== k) return;
      const code = (e as { code?: string; status?: number })?.code;
      // An older hub (no /v1/pins) or one that does not keep them.
      if (code === "not_found" || code === "not_supported" || (e as { status?: number })?.status === 404) {
        setSupported(false);
        setPins(null);
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  useEffect(() => {
    setPins(null);
    setSupported(false);
    if (!withWhom) return;
    void load();
    // What the agent pins comes in a while later; what you pin, at once.
    const t = window.setInterval(() => { if (document.visibilityState !== "hidden") void load(); }, 20_000);
    const on = (e: Event) => { if ((e as CustomEvent<{ hubId: string }>).detail?.hubId === hubId) void load(); };
    window.addEventListener("myco-pins-changed", on);
    return () => { window.clearInterval(t); window.removeEventListener("myco-pins-changed", on); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  return { pins, setPins, supported, reload: load };
}

/** A message's first words, as a pin's title. */
export function titleOfMessage(body: string): string {
  const line = body.split("\n")
    .map((l) => l.replace(/\[([^\]]*)\]\([^)]*\)/g, "$1").replace(/https?:\/\/\S+/g, "").replace(/^[#>*\-\s`]+/, "").replace(/[*_`[\]]/g, "").replace(/[\s:：,，、]+$/, "").trim())
    .find(Boolean) || "";
  return line.length > 24 ? line.slice(0, 24) + "…" : line;
}

/** Pins something, then tells the strips. */
export async function pinThing(hubId: string, p: PinCreate): Promise<Pin> {
  const out = await appApi.addPin(hubId, p);
  pinsChanged(hubId);
  return out;
}

const iconOf = (p: Pin) => (p.kind === "link" ? Globe : p.kind === "file" ? FileText : MessageSquareText);

/** The strip above the conversation: one chip a pin, in their order. */
export function PinBar({ hubId, withWhom, a, pins, setPins, onJump, adding, onAdding }: {
  hubId: string;
  withWhom: string;
  a?: string;
  pins: Pin[] | null;
  setPins: (p: Pin[]) => void;
  /** scrolls to a message; false when it is not in the conversation */
  onJump: (id: string) => void;
  /** the header's pin button: an address to pin */
  adding: boolean;
  onAdding: (v: boolean) => void;
}) {
  const menu = useContextMenu();
  const [renaming, setRenaming] = useState<Pin | null>(null);
  const [opened, setOpened] = useState<Pin | null>(null);
  const [error, setError] = useState<unknown>(null);
  const dragging = useRef<string | null>(null);
  const [order, setOrder] = useState<Pin[] | null>(null);
  const shown = order || pins || [];

  const open = (p: Pin) => {
    setError(null);
    if (p.kind === "message" && p.target) { onJump(p.target); return; }
    if (p.kind === "link" && p.target) {
      const t = pageTargetOf(p.target);
      if (t) openPage(t);
      else void appApi.openUrl(p.target).catch((e) => setError(e));
      return;
    }
    if (p.kind === "file" && p.file) {
      if (isText(p.file) || isPicture(p.file)) { setOpened(p); return; }
      void appApi.saveAttachment(hubId, p.file.file_id).catch((e) => setError(e));
    }
  };
  const remove = async (p: Pin) => {
    setError(null);
    setPins((pins || []).filter((x) => x.id !== p.id));
    try { await appApi.removePin(hubId, p.id, withWhom, a); } catch (e) { setError(e); }
    pinsChanged(hubId);
  };
  const itemsFor = (p: Pin): MenuItem[] => [
    { label: p.kind === "message" ? L("跳到这条消息", "Go to the message") : p.kind === "file" && p.file && !isText(p.file) && !isPicture(p.file) ? L("存到「下载」", "Save to Downloads") : L("打开", "Open"), onSelect: () => open(p) },
    { label: L("重命名", "Rename"), onSelect: () => setRenaming(p) },
    ...(p.kind === "link" && p.target ? ["sep" as const, { label: L("复制地址", "Copy the address"), onSelect: () => void copyText(p.target!) }] : []),
    "sep",
    { label: L("取消置顶", "Unpin"), danger: true, onSelect: () => void remove(p) },
  ];
  const drop = async () => {
    const ids = shown.map((p) => p.id);
    dragging.current = null;
    if (!order) return;
    setPins(order);
    setOrder(null);
    try { await appApi.orderPins(hubId, withWhom, ids, a); } catch (e) { setError(e); }
    pinsChanged(hubId);
  };

  return (
    <>
      {shown.length > 0 && (
        <div className="pin-bar" role="toolbar" aria-label={L("置顶", "Pinned")} data-testid="pin-bar">
          <PinIcon className="pin-bar-mark" aria-hidden />
          <div className="pin-bar-list">
            {shown.map((p) => {
              const Icon = iconOf(p);
              const by = p.added_by.startsWith("@") ? L("你", "you") : p.added_by;
              return (
                <button key={p.id} type="button" className={cn("pin-chip", `is-${p.kind}`)} draggable
                  title={`${p.title}${p.kind === "link" && p.target ? `\n${p.target}` : ""}\n${L(`${by} 置顶`, `Pinned by ${by}`)}`}
                  onClick={() => open(p)}
                  onContextMenu={(e) => { e.preventDefault(); menu.open(e, itemsFor(p)); }}
                  onDragStart={(e) => { dragging.current = p.id; e.dataTransfer.effectAllowed = "move"; e.dataTransfer.setData("text/plain", p.title); }}
                  onDragOver={(e) => {
                    const from = dragging.current;
                    if (!from) return;
                    e.preventDefault();
                    if (from === p.id) return;
                    const list = [...shown];
                    const i = list.findIndex((x) => x.id === from);
                    const j = list.findIndex((x) => x.id === p.id);
                    if (i < 0 || j < 0) return;
                    const [m] = list.splice(i, 1);
                    list.splice(j, 0, m);
                    setOrder(list);
                  }}
                  onDrop={(e) => { e.preventDefault(); void drop(); }}
                  onDragEnd={() => { if (dragging.current) { dragging.current = null; setOrder(null); } }}>
                  <Icon />
                  <span className="pin-chip-title">{p.title}</span>
                </button>
              );
            })}
            <button type="button" className="pin-chip is-add" onClick={() => onAdding(true)} title={L("置顶一个网址", "Pin an address")} aria-label={L("置顶一个网址", "Pin an address")}>
              <Plus />
            </button>
          </div>
        </div>
      )}
      {error !== null && <div className="pin-bar-error"><ErrorText error={error} /></div>}
      {menu.element}
      <PinEdit open={adding} onClose={() => onAdding(false)} title={L("置顶一个网址", "Pin an address")}
        onSave={async (title, url) => { await pinThing(hubId, { with: withWhom, ...(a ? { a } : {}), kind: "link", target: url, title }); }} />
      <PinEdit open={!!renaming} onClose={() => setRenaming(null)} title={L("重命名", "Rename")} initial={renaming?.title} titleOnly
        onSave={async (title) => { if (!renaming) return; await appApi.renamePin(hubId, renaming.id, withWhom, title, a); pinsChanged(hubId); }} />
      {opened?.file && isPicture(opened.file) && (
        <Lightbox items={[{ name: opened.file.name, src: appApi.attachmentUrl(hubId, opened.file.file_id), save: () => appApi.saveAttachment(hubId, (opened.file as Attachment).file_id) }]}
          index={0} onIndex={() => undefined} onClose={() => setOpened(null)} />
      )}
      {opened?.file && isText(opened.file) && (
        <FilePreview hubId={hubId} file={opened.file} open onOpenChange={(o) => { if (!o) setOpened(null); }} />
      )}
    </>
  );
}

/** Pinning an address (title and address), or renaming a pin. */
function PinEdit({ open, onClose, title, initial, titleOnly, onSave }: {
  open: boolean;
  onClose: () => void;
  title: string;
  initial?: string;
  titleOnly?: boolean;
  onSave: (title: string, url: string) => Promise<void>;
}) {
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => { if (open) { setName(initial || ""); setUrl(""); setError(null); } }, [open, initial]);
  const valid = titleOnly ? !!name.trim() : /^https?:\/\/\S+$/i.test(url.trim());
  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!valid || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onSave(name.trim(), url.trim());
      onClose();
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open={open} onOpenChange={(o) => !o && !busy && onClose()} title={title}
      description={titleOnly ? undefined : L("对话上方会多一个标签，你和它点一下就能打开。", "A tab above the conversation: you and it open it in one click.")}>
      <form onSubmit={save} className="pin-edit">
        {!titleOnly && (
          <label className="app-field">
            <span className="app-field-label">{L("网址", "Address")}</span>
            <input className="app-input" value={url} onChange={(e) => setUrl(e.target.value)} autoFocus inputMode="url" spellCheck={false}
              placeholder="https://" aria-label={L("网址", "Address")} data-testid="pin-url" />
          </label>
        )}
        <label className="app-field">
          <span className="app-field-label">{L("叫什么", "Called")}</span>
          <input className="app-input" value={name} onChange={(e) => setName(e.target.value)} autoFocus={titleOnly} maxLength={80}
            placeholder={titleOnly ? "" : L("不填就用网址", "The address if left empty")} aria-label={L("叫什么", "Called")} data-testid="pin-title" />
        </label>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={onClose} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="submit" className="app-btn app-btn-primary" disabled={!valid || busy}>{busy ? L("正在保存…", "Saving…") : titleOnly ? L("保存", "Save") : L("置顶", "Pin")}</button>
        </div>
      </form>
    </Modal>
  );
}
