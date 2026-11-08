import { useEffect, useMemo, useState } from "react";
import { MessageSquare, Plus, Search } from "lucide-react";
import { appApi, errorMessage, type Conversation, type Hub } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { cn, timeAgo } from "@/lib/utils";
import { summarize } from "@/app/summary";
import { L } from "@/lib/i18n";

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
  if (m.type === "event" || m.status === "failed") return { label: L("失败", "Failed"), cls: "red" };
  if (m.type === "reply") return { label: L("已完成", "Done"), cls: "green" };
  if (m.type === "note") return { label: "", cls: "" };
  if (m.type === "task" && m.status === "done") return { label: L("已完成", "Done"), cls: "green" };
  return { label: L("处理中", "Working"), cls: "amber" };
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
    if (!name) return L("未知节点", "Unknown node");
    const row = rows.find((r) => r.st.agent.name === name);
    return row?.st.agent.display_name || name;
  };

  const firstLoad = loading && items.length === 0;
  const canLoadMore = Object.values(more).some(Boolean);

  return (
    <div className="app-page">
      <div className="app-page-head">
        <p className="app-summary">
          {loading && items.length === 0 ? L("正在加载…", "Loading…") : L(`${items.length} 个对话`, `${items.length} conversation${items.length === 1 ? "" : "s"}`)}
        </p>
        <button type="button" className="app-btn app-btn-primary" onClick={onNew}>
          <Plus />{L("新对话", "New conversation")}
        </button>
      </div>

      <div className="app-toolbar">
        <div className="app-search">
          <Search />
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={L("搜索对话内容或节点", "Search conversations or nodes")}
            aria-label={L("搜索对话", "Search conversations")}
          />
        </div>
      </div>

      {error !== null && (
        <div className="app-error-text" style={{ marginBottom: 12 }}>{errorMessage(error)}</div>
      )}

      {firstLoad ? (
        <div className="app-empty-note">{L("正在加载…", "Loading…")}</div>
      ) : items.length === 0 ? (
        <div className="app-empty" style={{ margin: "8vh auto 0" }}>
          <div className="app-empty-icon"><MessageSquare /></div>
          <h2>{query ? L("没有匹配的对话", "No matching conversations") : L("还没有对话", "No conversations yet")}</h2>
          <p>
            {query
              ? L("换一个搜索词试试。", "Try a different search.")
              : L("发一条任务给节点，处理过程和结果都会出现在这里。", "Give a node a task. Its progress and results will show up here.")}
          </p>
          {!query && (
            <div className="app-empty-actions">
              <button type="button" className="app-btn app-btn-primary" onClick={onNew}>
                <Plus />{L("新对话", "New conversation")}
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
            {moreBusy ? L("正在加载…", "Loading…") : L("加载更多", "Load more")}
          </button>
        </div>
      )}
    </div>
  );
}
