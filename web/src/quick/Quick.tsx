import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { ArrowUp, Maximize2, ShieldQuestion } from "lucide-react";
import { appApi, type HubOverview } from "@/appapi";
import { Avatar } from "@/app/persona";
import { SHROOM, useSuperagent } from "@/app/Superagent";
import { useThemeMode } from "@/lib/theme";
import { L, E } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import "@/app/app.css";
import "@/app/space.css";
import "./quick.css";

// The workspace on show in the app (AppMode keeps it here): the panel talks
// to that workspace's superagent, as the app is on it (nothing to pick).
const HUB_KEY = "myco-app-hub";

const post = (path: string, body: unknown = {}) =>
  fetch(path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }).catch(() => undefined);

/** The quick panel (⌘⇧Space, the Mac app's own small window): one line to
 *  hand the workspace's superagent a thing. Sent, the panel goes away and the
 *  app opens your chat with it, where its answer comes. */
export function Quick() {
  useThemeMode();
  const [hubs, setHubs] = useState<HubOverview[]>([]);
  const [hubId, setHubId] = useState(() => { try { return localStorage.getItem(HUB_KEY) || ""; } catch { return ""; } });
  const hub = hubs.find((h) => h.id === hubId) || hubs.find((h) => h.ok);
  const sa = useSuperagent(hub?.id);
  const agent = sa?.state?.agent;
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const input = useRef<HTMLTextAreaElement>(null);
  const box = useRef<HTMLDivElement>(null);

  const loadHubs = () => appApi.overview().then((o) => setHubs(o.hubs)).catch(() => undefined);
  useEffect(() => { void loadHubs(); }, []);

  // Shown again: the app may have moved to another workspace meanwhile.
  useEffect(() => {
    const onShow = () => {
      try { const h = localStorage.getItem(HUB_KEY); if (h) setHubId(h); } catch { /* storage unavailable */ }
      void loadHubs();
      setError("");
      setTimeout(() => input.current?.focus(), 30);
    };
    window.addEventListener("quick-show", onShow);
    return () => window.removeEventListener("quick-show", onShow);
  }, []);

  // The window is as tall as what it shows.
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    let last = 0;
    const report = () => {
      const h = Math.ceil(el.getBoundingClientRect().height);
      if (h !== last) { last = h; void post("app/quick/size", { h }); }
    };
    report();
    const ro = new ResizeObserver(report);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") { e.preventDefault(); void post("app/quick/hide"); } };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // One action per press: the app passes a press on to the page itself
  // (quick_click_darwin.go: a panel that is not key loses its first click),
  // and the web view may deliver it too.
  const lastAct = useRef(0);
  const once = (f: () => void) => {
    if (Date.now() - lastAct.current < 600) return;
    lastAct.current = Date.now();
    f();
  };
  useEffect(() => {
    const on = (e: Event) => {
      const d = (e as CustomEvent<{ x: number; y: number }>).detail;
      const el = d && (document.elementFromPoint(d.x, d.y) as HTMLElement | null);
      if (!el) return;
      if (el.closest("textarea")) { input.current?.focus(); return; }
      const b = el.closest<HTMLButtonElement>("button");
      if (b && !b.disabled) b.click();
    };
    window.addEventListener("quick-click", on);
    return () => window.removeEventListener("quick-click", on);
  }, []);

  const openChat = (setup = false) => void post("app/quick/open", { hub: hub?.id || "", agent: agent?.name || "", setup });
  const send = async () => {
    const body = draft.trim();
    if (!body || !hub || !agent || sending) return;
    setSending(true);
    setError("");
    try {
      await appApi.sendMessage(hub.id, { to: agent.name, body });
      setDraft("");
      // Its answer comes in the chat: go there.
      openChat();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSending(false);
    }
  };

  const name = SHROOM;
  const waiting = agent ? (sa?.approvals || []).filter((a) => a.agent === agent.name && a.status === "pending").length : 0;

  return (
    <div className="qp-root app-shell">
      <div className="qp-card" ref={box} data-testid="quick-panel">
        <div className="qp-head">
          {agent ? <Avatar agent={agent} size={22} badge={false} /> : null}
          <span className="qp-name">{name}</span>
          <span style={{ flex: 1 }} />
          {agent && (
            <button type="button" className="app-btn app-btn-small qp-open" onClick={() => once(() => openChat())} title={L("在 App 里打开和它的对话", "Open your chat with it in the app")}>
              <Maximize2 />{L("打开对话", "Open chat")}
            </button>
          )}
        </div>
        {sa && !sa.off && !agent ? (
          <div className="qp-empty">
            <span>{L("这个工作区还没有 superagent。", "This workspace has no superagent yet.")}</span>
            <button type="button" className="app-btn app-btn-primary app-btn-small" onClick={() => once(() => openChat(true))}>{L("配一个", "Set one up")}</button>
          </div>
        ) : sa?.off ? (
          <div className="qp-empty"><span>{L("这个工作区的 Hub 太旧，还没有 superagent。", "This workspace's hub is too old for a superagent.")}</span></div>
        ) : (<>
          <form className="qp-input" onSubmit={(e) => { e.preventDefault(); once(() => void send()); }}>
            <textarea ref={input} rows={1} value={draft} autoFocus disabled={!agent}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); void send(); } }}
              placeholder={L(E`交给 ${name}…`, E`Hand ${name} something…`)} data-testid="quick-input" />
            <button type="submit" className={cn("qp-send", draft.trim() && "on")} disabled={!draft.trim() || sending} aria-label={L("发送", "Send")}>
              {sending ? <span className="app-spinner" /> : <ArrowUp />}
            </button>
          </form>
          {error && <div className="qp-error">{error}</div>}
          {waiting > 0 && (
            <button type="button" className="qp-waiting" onClick={() => once(() => openChat())} data-testid="quick-waiting">
              <ShieldQuestion />{L(E`${name} 有 ${waiting} 件事等你点头`, E`${name} has ${waiting} thing${waiting > 1 ? "s" : ""} waiting for your OK`)}
              <span className="qp-waiting-go">{L("去看", "See")}</span>
            </button>
          )}
          <div className="qp-hint">{L("↩ 发出去，在对话里看它回 · ⇧↩ 换行 · Esc 收起", "↩ send, see the answer in the chat · ⇧↩ new line · Esc close")}</div>
        </>)}
      </div>
    </div>
  );
}
