import { t as uiText } from "./i18n";
// What every app page shares: its stores followed in React, saving state,
// a note at the bottom (with undo), the bar on top, and starting up.
import { useSyncExternalStore, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { Store, listen, myco, type MycoChange } from "./myco";

export class Kit {
  me = "";
  page = "";
  pending = 0;
  failed = false;
  version = 0;
  note: { text: string; undo?: () => void } | null = null;
  private subs = new Set<() => void>();
  private noteTimer = 0;
  constructor(readonly stores: Store<any>[], readonly opts: { quiet?: boolean } = {}) {}
  emit = () => { this.version++; for (const f of this.subs) f(); };
  subscribe = (f: () => void) => { this.subs.add(f); return () => { this.subs.delete(f); }; };
  flash(text: string, undo?: () => void) {
    this.note = { text, undo };
    window.clearTimeout(this.noteTimer);
    this.noteTimer = window.setTimeout(() => { this.note = null; this.emit(); }, undo ? 6000 : 2600);
    this.emit();
  }
  /** Follows a write: "saving" while it is on its way, a note if it fails. */
  track<T>(p: Promise<T> | null | undefined): Promise<T | undefined> {
    if (!p) return Promise.resolve(undefined);
    this.pending++;
    this.emit();
    return p.then((v) => { this.pending--; this.failed = false; this.emit(); return v; }, (e: { code?: string; message?: string }) => {
      this.pending--;
      this.failed = e?.code !== "conflict";
      this.flash(e?.code === "conflict" ? uiText("别人刚改过，已显示最新的") : e?.message || uiText("没保存上"));
      return undefined;
    });
  }
  async start(init?: () => Promise<unknown>, onChange?: (c: MycoChange) => void) {
    const ctx = await myco!.ready;
    this.me = ctx.me?.name || "";
    this.page = ctx.page?.id || "";
    await Promise.all(this.stores.map((s) => s.load()));
    if (init) await init();
    listen(this.stores, (c) => {
      this.emit();
      onChange?.(c);
      if (!this.opts.quiet && c.by && c.by !== this.me) this.flash(uiText("{0} 刚改了", c.by));
    });
  }
}

export function useKit(kit: Kit) {
  return useSyncExternalStore(kit.subscribe, () => kit.version);
}

export const uid = (p = "") => p + Math.random().toString(36).slice(2, 9);

/** The bar on top: the title (edited in place), saving, who you are, buttons. */
export function Bar({ kit, title, placeholder, onTitle, children }: {
  kit: Kit; title: string; placeholder: string; onTitle: (t: string) => void; children?: ReactNode;
}) {
  const state = kit.pending ? "saving" : kit.failed ? "error" : "saved";
  return (
    <header className="top">
      <input className="top-title" value={title} placeholder={placeholder} aria-label={uiText("标题")} maxLength={120}
        onChange={(e) => onTitle(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") (e.target as HTMLInputElement).blur(); }} />
      <span className={`top-state is-${state}`}>{state === "saving" ? uiText("正在保存…") : state === "error" ? uiText("没保存上") : uiText("已保存")}</span>
      <span className="top-gap" />
      {kit.me && <span className="top-me">{uiText("你是")}{" "}{kit.me}</span>}
      {children}
    </header>
  );
}

export function Toast({ kit }: { kit: Kit }) {
  if (!kit.note) return null;
  return <div className="note" role="status">{kit.note.text}{kit.note.undo && <button type="button" onClick={kit.note.undo}>{uiText("撤销")}</button>}</div>;
}

/** A title kept in a single record, saved after a pause. */
export function titleSaver(kit: Kit, store: Store<any>, id = "main") {
  let timer = 0;
  let draft: string | null = null;
  return {
    get: (fallback = "") => draft ?? String(store.get(id)?.data.title ?? fallback),
    set: (t: string) => {
      draft = t;
      kit.emit();
      window.clearTimeout(timer);
      timer = window.setTimeout(() => {
        kit.track(store.put({ ...(store.get(id)?.data || {}), title: t.trim() }, id)).finally(() => { draft = null; });
      }, 500);
    },
  };
}

export async function mount(kit: Kit, App: () => JSX.Element, init?: () => Promise<unknown>, onChange?: (c: MycoChange) => void) {
  const root = createRoot(document.getElementById("root")!);
  const outside = <div className="outside">{uiText("在 myco App 里打开这个网页，才能读写它的内容。")}</div>;
  if (!myco) { root.render(outside); return; }
  try { await kit.start(init, onChange); } catch { root.render(outside); return; }
  root.render(<App />);
}
