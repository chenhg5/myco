import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { ArrowUp, Maximize2 } from "lucide-react";
import { appApi, type HubOverview } from "@/appapi";
import type { Message } from "@/api";
import { Avatar } from "@/app/persona";
import { Markdown } from "@/app/Markdown";
import { Mushroom } from "@/app/Working";
import { ApprovalStrip, SHROOM, useSuperagent } from "@/app/Superagent";
import { useThemeMode } from "@/lib/theme";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import "@/app/app.css";
import "@/app/space.css";
import "./quick.css";

// The workspace on show in the app (AppMode keeps it here).
const HUB_KEY = "myco-app-hub";

const post = (path: string, body: unknown = {}) =>
  fetch(path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }).catch(() => undefined);

/** The quick panel (⌘⇧Space, the Mac app's own small window): one line to
 *  hand the workspace's superagent a thing, its latest answer under it, and
 *  the way into the full chat. */
export function Quick() {
  useThemeMode();
  const [hubs, setHubs] = useState<HubOverview[]>([]);
  const [hubId, setHubId] = useState(() => { try { return localStorage.getItem(HUB_KEY) || ""; } catch { return ""; } });
  const hub = hubs.find((h) => h.id === hubId) || hubs.find((h) => h.ok);
  const sa = useSuperagent(hub?.id);
  const agent = sa?.state?.agent;
  const [peer, setPeer] = useState<string | undefined>();
  const [msgs, setMsgs] = useState<Message[]>([]);
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

  // Who you are in your chat with it (the name you talked to it as).
  useEffect(() => {
    setPeer(undefined);
    setMsgs([]);
    if (!hub?.id || !agent) return;
    let gone = false;
    appApi.recent(hub.id, { group: "pair", side: "me" }).then((r) => {
      const it = r.items.find((x) => x.kind === "pair" && ((x.node === agent.name && x.other?.startsWith("@console:")) || (x.other === agent.name && x.node?.startsWith("@console:"))));
      if (!gone) setPeer(it ? (it.node === agent.name ? it.other : it.node) : undefined);
    }).catch(() => undefined);
    return () => { gone = true; };
  }, [hub?.id, agent?.name]);

  // The latest of it, every few seconds.
  useEffect(() => {
    if (!hub?.id || !agent || !peer) return;
    let gone = false;
    const load = () => appApi.exchanges(hub.id, agent.name, peer).then((r) => {
      // (newest first from the hub: oldest first here)
      if (!gone) setMsgs([...r.messages].sort((a, b) => a.created_at.localeCompare(b.created_at)));
    }).catch(() => undefined);
    void load();
    const t = setInterval(load, 3000);
    return () => { gone = true; clearInterval(t); };
  }, [hub?.id, agent?.name, peer]);

  // The window is as tall as what it shows.
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    let last = 0;
    const report = () => {
      const h = Math.ceil(el.getBoundingClientRect().height) + 24;
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

  const send = async () => {
    const body = draft.trim();
    if (!body || !hub || !agent || sending) return;
    setSending(true);
    setError("");
    try {
      const m = await appApi.sendMessage(hub.id, { to: agent.name, body });
      setDraft("");
      setPeer(m.from);
      setMsgs((x) => [...x, m]);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSending(false);
    }
  };
  const openChat = (setup = false) => void post("app/quick/open", { hub: hub?.id || "", agent: agent?.name || "", setup });

  // Your last message and what it said or is doing since.
  const mineAt = msgs.map((m) => m.from === peer && m.to === agent?.name).lastIndexOf(true);
  const mine = mineAt >= 0 ? msgs[mineAt] : undefined;
  const after = msgs.slice(mineAt + 1).filter((m) => m.from === agent?.name);
  const answer = [...after].reverse().find((m) => m.type === "reply") || [...after].reverse().find((m) => m.type === "progress");
  const open = !!mine && mine.type === "task" && !["done", "failed", "expired", "canceled", "cancelled"].includes(mine.status) && !after.some((m) => m.type === "reply");
  const name = agent?.display_name || SHROOM;

  return (
    <div className="qp-root app-shell">
      <div className="qp-card" ref={box} data-testid="quick-panel">
        <div className="qp-head">
          {agent ? <Avatar agent={agent} size={22} badge={false} /> : null}
          <span className="qp-name">{name}</span>
          {hubs.filter((h) => h.ok).length > 1 ? (
            <select className="qp-hub" value={hub?.id || ""} onChange={(e) => setHubId(e.target.value)} aria-label={L("工作区", "Workspace")}>
              {hubs.filter((h) => h.ok).map((h) => <option key={h.id} value={h.id}>{h.name}</option>)}
            </select>
          ) : hub ? <span className="qp-hub-name">{hub.name}</span> : null}
          <span style={{ flex: 1 }} />
          {agent && (
            <button type="button" className="app-btn app-btn-small qp-open" onClick={() => openChat()} title={L("打开完整对话", "Open the full chat")}>
              <Maximize2 />{L("打开对话", "Open chat")}
            </button>
          )}
        </div>
        {sa && !sa.off && !agent ? (
          <div className="qp-empty">
            <span>{L("这个工作区还没有 superagent。", "This workspace has no superagent yet.")}</span>
            <button type="button" className="app-btn app-btn-primary app-btn-small" onClick={() => openChat(true)}>{L("配一个", "Set one up")}</button>
          </div>
        ) : sa?.off ? (
          <div className="qp-empty"><span>{L("这个工作区的 Hub 太旧，还没有 superagent。", "This workspace's hub is too old for a superagent.")}</span></div>
        ) : (<>
          <form className="qp-input" onSubmit={(e) => { e.preventDefault(); void send(); }}>
            <textarea ref={input} rows={1} value={draft} autoFocus disabled={!agent}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); void send(); } }}
              placeholder={L(`交给 ${name}…`, `Hand ${name} something…`)} data-testid="quick-input" />
            <button type="submit" className={cn("qp-send", draft.trim() && "on")} disabled={!draft.trim() || sending} aria-label={L("发送", "Send")}>
              {sending ? <span className="app-spinner" /> : <ArrowUp />}
            </button>
          </form>
          {error && <div className="qp-error">{error}</div>}
          {mine && (
            <div className="qp-last">
              <div className="qp-mine">{mine.body.split("\n")[0]}</div>
              {answer && (
                <div className={cn("qp-answer", answer.type === "progress" && "is-progress")}><Markdown text={answer.body} /></div>
              )}
              {open && (
                <div className="qp-working"><Mushroom /><span>{agent?.status_text || L("在做…", "Working…")}</span></div>
              )}
            </div>
          )}
          {hub && agent && <ApprovalStrip hubId={hub.id} agent={agent.name} />}
          <div className="qp-hint">{L("↩ 交给它 · ⇧↩ 换行 · Esc 收起", "↩ send · ⇧↩ new line · Esc close")}</div>
        </>)}
      </div>
    </div>
  );
}
