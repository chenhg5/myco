import type { Message } from "@/api";
import type { RecentItem } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { Avatar } from "@/app/persona";
import { L } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";

/*
 * What two agents said to each other, in a chat that is yours with one of
 * them: never as bubbles (they would read as if it were talking to you),
 * one card per exchange instead. A line says who gave what to whom and how
 * it is going; once something comes back, a second line holds the first
 * line of it (the handoff, not the back and forth). A click opens their
 * whole exchange, with its progress and steps, beside it.
 *
 * Used on a project's home (the coordinator's work handed out, and what the
 * members send it) and in a member's chat (what it sends the others, and
 * what they give it).
 */

export type HandoffState = "queued" | "working" | "waiting" | "stale" | "done" | "failed" | "note";

export type Handoff = {
  /** the first message of the exchange (its key, and where it opens) */
  id: string;
  /** when it started: where the card goes in the chat */
  at: string;
  /** when it last moved */
  updated: string;
  /** out: the agent whose chat this is sent it; in: it was sent to it */
  dir: "out" | "in";
  /** the other agent */
  other: string;
  /** a task (work to do), or only a note */
  task: boolean;
  /** the first line of what was sent */
  text: string;
  state: HandoffState;
  /** the first line of what came back */
  result?: string;
};

const firstLine = (s: string) => (s || "").replace(/\r/g, "").split("\n").map((x) => x.trim()).find(Boolean) || "";

const stateOf = (task?: Message): HandoffState => {
  if (!task) return "note";
  switch (task.status) {
    case "done": return "done";
    case "failed": return "failed";
    // set aside: it waits on an answer (asking) or on others, not working
    case "held": return "waiting";
    case "acked": return task.asking ? "waiting" : "working";
    default: return "queued";
  }
};

/** The cards for what `self` and the others said to each other (the
 *  messages between them, as loaded): one per task, the notes around it
 *  folded into it, a note on its own its own card. */
/** A working task the workspace says shows no life (an agent's
 *  stale_tasks) reads 没动静, not 在做. */
export function withStale(hs: Handoff[], stale: (taskId: string) => boolean): Handoff[] {
  return hs.map((h) => (h.task && h.state === "working" && stale(h.id) ? { ...h, state: "stale" } : h));
}

export function handoffsFrom(messages: Message[], self: string): Handoff[] {
  const byThread = new Map<string, Message[]>();
  for (const m of messages) {
    if (m.type === "progress" || m.type === "steer") continue;
    const list = byThread.get(m.thread_id) || [];
    list.push(m);
    byThread.set(m.thread_id, list);
  }
  const out: Handoff[] = [];
  for (const list of byThread.values()) {
    list.sort((a, b) => a.created_at.localeCompare(b.created_at));
    let card: (Handoff & { from: string; taskId?: string }) | null = null;
    const cards: (Handoff & { from: string; taskId?: string })[] = [];
    for (const m of list) {
      const other = m.from === self ? m.to : m.from;
      if (m.type === "task" || !card) {
        const isTask = m.type === "task";
        card = {
          id: m.id, at: m.created_at, updated: m.updated_at || m.created_at,
          dir: m.from === self ? "out" : "in", other, task: isTask,
          text: firstLine(m.body), state: isTask ? stateOf(m) : "note",
          from: m.from, taskId: isTask ? m.id : undefined,
        };
        cards.push(card);
        continue;
      }
      // What came back: the answer to the task, or a note from the other side.
      const back = m.from !== card.from;
      if (m.type === "event" && (!m.reply_to || m.reply_to === card.taskId)) card.state = "failed";
      else if (back && (m.type === "reply" || m.type === "note")) card.result = firstLine(m.body) || card.result;
      if (m.updated_at && m.updated_at > card.updated) card.updated = m.updated_at;
      else if (m.created_at > card.updated) card.updated = m.created_at;
    }
    for (const c of cards) {
      const { from: _f, taskId: _t, ...h } = c;
      out.push(h);
    }
  }
  return out.sort((a, b) => a.at.localeCompare(b.at));
}

/** The cards on a project's home, from its threads: the work the
 *  coordinator handed out, and what the members sent it. */
export function handoffsOfHome(items: RecentItem[], coordinator: string, isYou: (n: string) => boolean): Handoff[] {
  const out: Handoff[] = [];
  for (const it of items) {
    if (it.kind !== "thread") continue;
    const m = it.task || it.first;
    if (!m || m.from.startsWith("@") || m.to.startsWith("@") || isYou(m.from) || isYou(m.to)) continue;
    const dir = m.from === coordinator ? "out" : m.to === coordinator ? "in" : null;
    if (!dir) continue;
    const isTask = m.type === "task";
    const back = it.answer && it.answer.from !== m.from ? it.answer
      : it.last && it.last.id !== m.id && it.last.from !== m.from && (it.last.type === "reply" || it.last.type === "note") ? it.last
      : undefined;
    out.push({
      id: m.id, at: m.created_at, updated: it.updated_at,
      dir, other: dir === "out" ? m.to : m.from, task: isTask,
      text: firstLine(m.body),
      state: !isTask ? "note" : it.state === "done" ? "done" : it.state === "failed" ? "failed"
        : it.state === "working" ? (stateOf(m) === "waiting" ? "waiting" : "working") : it.state === "queued" ? "queued" : stateOf(m),
      result: back ? firstLine(back.body) : undefined,
    });
  }
  return out;
}

function stateWord(h: Handoff): string {
  switch (h.state) {
    case "note": return L("备注", "Note");
    case "queued": return L("排队中", "Queued");
    case "working": return L("在做", "Working");
    // it set the task aside: asked something, or waits on others
    case "waiting": return L("在等回复", "Waiting");
    // accepted, but no life for a while (the workspace's stale tasks)
    case "stale": return L("没动静", "No activity");
    case "failed": return L("没做成", "Didn't work");
    default: return h.dir === "out" ? L("交回了", "Handed back") : L("做完了", "Done");
  }
}

export function HandoffCard({ h, row, name, now, onOpen }: {
  h: Handoff;
  /** the other agent's row, for its face */
  row?: AgentRow;
  name: string;
  now: number;
  onOpen?: () => void;
}) {
  const who = (
    <span className="handoff-who">
      {row && <Avatar agent={row.st.agent} size={18} badge={false} />}
      {name}
    </span>
  );
  const label = h.dir === "out"
    ? <span className="handoff-label">{h.task ? L("派给", "To") : L("发给", "To")}</span>
    : <span className="handoff-label">{h.task ? L("派来", "From") : L("发来", "From")}</span>;
  return (
    <button type="button" className={cn("handoff", `is-${h.dir}`, (h.state === "done" || h.state === "note") && "is-settled")} data-handoff={h.id} onClick={onOpen}
      title={L("看它们之间的完整往来", "See their whole exchange")}>
      <span className="handoff-line">
        {h.dir === "out" ? <>{label}{who}</> : <>{who}{label}</>}
        <span className="handoff-text">{h.text}</span>
        <span className={cn("handoff-state", `is-${h.state}`)}>{stateWord(h)}</span>
        <span className="handoff-time">{timeAgo(h.updated, now)}</span>
      </span>
      {h.result && (
        <span className="handoff-result">
          <span className="handoff-back" aria-hidden>↩</span>
          <span className="handoff-result-text">{h.result}</span>
        </span>
      )}
    </button>
  );
}
