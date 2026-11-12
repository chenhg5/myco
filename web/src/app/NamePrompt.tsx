import { useState } from "react";
import { appApi, type HubOverview } from "@/appapi";
import { ErrorText, Modal } from "@/app/dialogs";
import { L } from "@/lib/i18n";

// Hubs whose name should be confirmed (just joined through an invite link),
// and hubs where the person chose "later". Per device, best effort.
const PENDING = "myco.confirmName";
const LATER = "myco.nameLater";

function readSet(key: string, store: Storage | undefined): Set<string> {
  try { return new Set(JSON.parse(store?.getItem(key) || "[]")); } catch { return new Set(); }
}
function writeSet(key: string, s: Set<string>, store: Storage | undefined) {
  try { store?.setItem(key, JSON.stringify([...s])); } catch { /* storage unavailable */ }
}
const local = () => (typeof localStorage === "undefined" ? undefined : localStorage);
const session = () => (typeof sessionStorage === "undefined" ? undefined : sessionStorage);

/** Ask for the name on this hub after joining it through an invite link. */
export function confirmNameLater(hubId: string) {
  const s = readSet(PENDING, local());
  s.add(hubId);
  writeSet(PENDING, s, local());
}

/** Asks "what are you called on this hub?" once: for the owner a hub made
 *  (still named "owner"), and after joining through an invite link. Names
 *  are per hub: the same person can be called differently on each. */
export function NamePrompt({ hubsOv, onRefresh }: { hubsOv?: HubOverview[]; onRefresh: () => Promise<void> }) {
  const [, force] = useState(0);
  const [form, setForm] = useState<{ hub: string; name: string; display: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const pending = readSet(PENDING, local());
  const later = readSet(LATER, session());
  const hub = (hubsOv || []).find((h) => h.ok && h.me?.member && !later.has(h.id) &&
    (h.me.member.name === "owner" || pending.has(h.id)));
  if (!hub || !hub.me?.member) return null;
  const me = hub.me.member;
  const f = form && form.hub === hub.id ? form : { hub: hub.id, name: me.name === "owner" ? "" : me.name, display: me.display_name || "" };

  const done = () => {
    const p = readSet(PENDING, local());
    p.delete(hub.id);
    writeSet(PENDING, p, local());
    setForm(null);
    force((n) => n + 1);
  };
  const skip = () => {
    const l = readSet(LATER, session());
    l.add(hub.id);
    writeSet(LATER, l, session());
    force((n) => n + 1);
  };
  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      await appApi.updateMember(hub.id, me.name, { name: f.name.trim(), display_name: f.display.trim() });
      done();
      await onRefresh();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open onOpenChange={(o) => !o && !busy && skip()} title={L(`你在「${hub.name}」上叫什么？`, `What should you be called on “${hub.name}”?`)}
      description={L("你发的消息会署这个名字，你的 agent 会显示为「名字 的」。之后也能在设置里改。", "This name is used on this Hub only: what you send from the app is signed with it, and nodes you add show as yours. You can use a different name on each Hub, and change it later under Settings → People.")}>
      <form onSubmit={(e) => { e.preventDefault(); void save(); }}>
        <label className="app-field">
          <span className="app-field-label">{L("用户名", "Handle")}</span>
          <input className="app-input mono" value={f.name} onChange={(e) => setForm({ ...f, name: e.target.value })} placeholder="glenn" disabled={busy} required autoFocus />
          <span className="app-small-note">{L("小写字母、数字和 - _ .，比如 glenn", "Lowercase letters, digits and - _ ., e.g. glenn")}</span>
        </label>
        <label className="app-field">
          <span className="app-field-label">{L("名字（可选）", "Name (optional)")}</span>
          <input className="app-input" value={f.display} onChange={(e) => setForm({ ...f, display: e.target.value })} placeholder="Glenn" disabled={busy} />
        </label>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={skip} disabled={busy}>{L("稍后", "Later")}</button>
          <button type="submit" className="app-btn app-btn-primary" disabled={busy || !f.name.trim()}>{busy ? L("正在保存…", "Saving…") : L("保存", "Save")}</button>
        </div>
      </form>
    </Modal>
  );
}
