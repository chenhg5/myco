import { useEffect, useMemo, useState } from "react";
import { MessageSquare, Plus, Search } from "lucide-react";
import { appApi, errorMessage, type Conversation, type Hub } from "@/appapi";
import type { Message } from "@/api";
import type { AgentRow } from "@/app/types";
import { cn, timeAgo } from "@/lib/utils";

export interface ConversationRow {
  hubId: string;
  hubName: string;
  conv: Conversation;
}

const PAGE = 30;

type Tag = { label: string; cls: "" | "green" | "amber" | "red" };

/** A conversation's state, from the last message and the task's status. */
function convTag(c: Conversation): Tag {
  const m = c.last;
  if (m.type === "event" || m.status === "failed") return { label: "失败", cls: "red" };
  if (m.type === "reply") return { label: "已完成", cls: "green" };
  if (m.type === "note") return { label: "", cls: "" };
  if (m.type === "task" && m.status === "done") return { label: "已完成", cls: "green" };
  return { label: "处理中", cls: "amber" };
}

function summarize(m: Message): string {
  const body = (m.body || "").trim();
  if (!body) return "（空消息）";
  if (body[0] === "{") {
    try {
      const v: unknown = JSON.parse(body);
      if (v && typeof v === "object" && "exit_code" in v) {
        const code = (v as { exit_code?: unknown }).exit_code;
        return typeof code === "number"
          ? `命令执行${code === 0 ? "完成" : "失败"}（退出码 ${code}）`
          : "命令执行结果";
      }
    } catch { /* ordinary text */ }
  }
  return body
    .replace(/```[\s\S]*?```/g, "［代码］")
    .replace(/^\s*\|?[\s:|-]*-{3,}[\s:|-]*\|?\s*$/gm, "") // table separator rows
    .replace(/\s*\|\s*/g, " ") // table cell borders
    .replace(/[#>*`_~]/g, "")
    .replace(/\s+/g, " ")
    .trim();
}

function oldestOf(list: ConversationRow[]): number | undefined {
  const t = list.map((r) => new Date(r.conv.last.updated_at).getTime()).filter((n) => !Number.isNaN(n));
  return t.length ? Math.min(...t) : undefined;
}

export function ConversationsPage({ hubs, hub, rows, now, onOpen, onNew }: {
  hubs: Hub[];
  /** The Hub selected in the switcher: "all" or a hub id. */
  hub: string;
  rows: AgentRow[];
  now: number;
  onOpen: (row: ConversationRow) => void;
  onNew: () => void;
}) {
  const [q, setQ] = useState("");
  const [query, setQuery] = useState("");
  const [items, setItems] = useState<ConversationRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [more, setMore] = useState<Record<string, boolean>>({});
  const [moreBusy, setMoreBusy] = useState(false);

  const targets = useMemo(
    () => hubs.filter((h) => (hub === "all" || h.id === hub) && h.ok),
    [hubs, hub],
  );

  useEffect(() => {
    const t = window.setTimeout(() => setQuery(q.trim()), 300);
    return () => window.clearTimeout(t);
  }, [q]);

  // The hub list refreshes every 5s; reloading with it keeps the list current.
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    const run = async () => {
      const results = await Promise.all(targets.map(async (h) => {
        try {
          const r = await appApi.conversations(h.id, { q: query || undefined });
          return { h, list: r.conversations, failed: null as unknown };
        } catch (e) {
          return { h, list: [] as Conversation[], failed: e };
        }
      }));
      if (cancelled) return;
      const merged: ConversationRow[] = results.flatMap((r) =>
        r.list.map((conv) => ({ hubId: r.h.id, hubName: r.h.name, conv })),
      );
      merged.sort((a, b) => new Date(b.conv.last.updated_at).getTime() - new Date(a.conv.last.updated_at).getTime());
      setItems(merged);
      setMore(Object.fromEntries(results.map((r) => [r.h.id, r.list.length >= PAGE])));
      setError(results.find((r) => r.failed)?.failed ?? null);
      setLoading(false);
    };
    void run();
    return () => { cancelled = true; };
  }, [targets, query]);

  const loadMore = async () => {
    const wanted = targets.filter((h) => more[h.id]);
    if (wanted.length === 0) return;
    setMoreBusy(true);
    try {
      const results = await Promise.all(wanted.map(async (h) => {
        const before = oldestOf(items.filter((i) => i.hubId === h.id));
        const r = await appApi.conversations(h.id, { q: query || undefined, before });
        return { h, list: r.conversations };
      }));
      const fresh = results.flatMap((r) =>
        r.list.map((conv) => ({ hubId: r.h.id, hubName: r.h.name, conv })),
      );
      setItems((prev) => {
        const seen = new Set(prev.map((p) => `${p.hubId}\u0000${p.conv.thread_id}`));
        const add = fresh.filter((f) => !seen.has(`${f.hubId}\u0000${f.conv.thread_id}`));
        return [...prev, ...add].sort(
          (a, b) => new Date(b.conv.last.updated_at).getTime() - new Date(a.conv.last.updated_at).getTime(),
        );
      });
      setMore((prev) => ({
        ...prev,
        ...Object.fromEntries(results.map((r) => [r.h.id, r.list.length >= PAGE])),
      }));
    } catch (e) {
      setError(e);
    } finally {
      setMoreBusy(false);
    }
  };

  const nameOf = (name?: string): string => {
    if (!name) return "未知节点";
    const row = rows.find((r) => r.st.agent.name === name);
    return row?.st.agent.display_name || name;
  };

  const firstLoad = loading && items.length === 0;
  const canLoadMore = Object.values(more).some(Boolean);

  return (
    <div className="app-page">
      <div className="app-page-head">
        <p className="app-summary">
          {loading && items.length === 0 ? "正在加载…" : `${items.length} 个对话`}
        </p>
        <button type="button" className="app-btn app-btn-primary" onClick={onNew}>
          <Plus />新对话
        </button>
      </div>

      <div className="app-toolbar">
        <div className="app-search">
          <Search />
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="搜索对话内容或节点"
            aria-label="搜索对话"
          />
        </div>
      </div>

      {error !== null && (
        <div className="app-error-text" style={{ marginBottom: 12 }}>{errorMessage(error)}</div>
      )}

      {firstLoad ? (
        <div className="app-empty-note">正在加载…</div>
      ) : items.length === 0 ? (
        <div className="app-empty" style={{ margin: "8vh auto 0" }}>
          <div className="app-empty-icon"><MessageSquare /></div>
          <h2>{query ? "没有匹配的对话" : "还没有对话"}</h2>
          <p>
            {query
              ? "换一个搜索词试试。"
              : "发一条任务给节点，处理过程和结果都会出现在这里。"}
          </p>
          {!query && (
            <div className="app-empty-actions">
              <button type="button" className="app-btn app-btn-primary" onClick={onNew}>
                <Plus />新对话
              </button>
            </div>
          )}
        </div>
      ) : (
        <div className="app-list">
          {items.map((row) => {
            const tag = convTag(row.conv);
            return (
              <button
                key={`${row.hubId}\u0000${row.conv.thread_id}`}
                type="button"
                className="conv-row"
                onClick={() => onOpen(row)}
              >
                <span className="conv-row-main">
                  <span className="conv-row-top">
                    <span className="conv-row-title">{nameOf(row.conv.agent)}</span>
                    {hub === "all" && <span className="conv-row-hub">{row.hubName}</span>}
                    <span className="conv-row-time">{timeAgo(row.conv.last.updated_at, now)}</span>
                  </span>
                  <span className="conv-row-summary">{summarize(row.conv.last)}</span>
                </span>
                {tag.label && <span className={cn("app-pill", tag.cls)}>{tag.label}</span>}
              </button>
            );
          })}
        </div>
      )}

      {canLoadMore && (
        <div className="app-actions" style={{ justifyContent: "center" }}>
          <button type="button" className="app-btn" onClick={() => void loadMore()} disabled={moreBusy}>
            {moreBusy ? "正在加载…" : "加载更多"}
          </button>
        </div>
      )}
    </div>
  );
}
