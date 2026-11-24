import type { Message, TraceEvent } from "@/api";

/**
 * A conversation as a file: Markdown for people (to read, share, keep) or
 * JSON Lines for programs (one message per line, myco's own fields). The
 * same shapes as `myco thread export` (internal/export), so a file from the
 * App and one from the CLI read alike. Not Claude Code's or Codex's
 * session format on purpose: those are one agent's model and tool calls,
 * this is messages between people and agents.
 */

export type ExportFormat = "md" | "jsonl";

const KIND: Record<string, string> = {
  task: "任务", reply: "回复", progress: "进展", steer: "补充", note: "留言", event: "事件",
};

const STOPPED = "stopped by the sender";

function stamp(iso: string): string {
  const d = new Date(iso);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

function size(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

/** A step of a task's run: one tool call (as `myco thread export` keeps it). */
export interface ExportStep { tool: string; input?: string; failed?: boolean }

function oneLine(t: string, max: number): string {
  const s = t.split(/\s+/).filter(Boolean).join(" ");
  const r = [...s];
  return r.length > max ? r.slice(0, max - 1).join("") + "…" : s;
}

/** The tool calls in a task's trace, in order; a call whose result failed is marked. */
export function stepsOf(events: TraceEvent[]): ExportStep[] {
  const out: ExportStep[] = [];
  const at = new Map<string, number>();
  for (const e of events) {
    const s = e.data;
    if (s.kind === "tool" && s.tool) {
      if (s.id) at.set(s.id, out.length);
      const input = oneLine(s.input || "", 160);
      out.push({ tool: s.tool, ...(input ? { input } : {}) });
    } else if (s.kind === "tool_result" && s.failed && s.id && at.has(s.id)) {
      out[at.get(s.id)!].failed = true;
    }
  }
  return out;
}

function stepLine(s: ExportStep): string {
  return s.tool + (s.input ? " `" + s.input.replace(/`/g, "'") + "`" : "") + (s.failed ? "（失败）" : "");
}

/**
 * Markdown: a heading, when and how many, then each message (who → whom,
 * when, what kind), its body, its attachments by name, and, when asked
 * for, the steps of a task folded under it.
 */
export function toMarkdown(messages: Message[], opts: {
  title: string;
  /** a person's or agent's name as the chat shows it */
  nameOf: (n: string) => string;
  steps?: Map<string, ExportStep[]>;
  now?: Date;
}): string {
  const now = opts.now || new Date();
  const lines: string[] = [`# ${opts.title}`, "", `导出时间：${now.getFullYear()}-${stamp(now.toISOString())} · 共 ${messages.length} 条消息`];
  for (const m of messages) {
    let head = `**${opts.nameOf(m.from)}** → ${opts.nameOf(m.to)} · ${stamp(m.created_at)} · ${KIND[m.type] || m.type}`;
    if (m.status === "failed") head += m.error === STOPPED ? " · 已停止" : m.error ? ` · 失败：${m.error}` : " · 失败";
    lines.push("", "---", "", head, "");
    lines.push(m.unreadable ? `（读不出：${m.unreadable}）` : m.body);
    const files = m.attachments || [];
    if (files.length) lines.push("", `附件：${files.map((f) => `${f.name}（${size(f.size)}）`).join("、")}`);
    const st = opts.steps?.get(m.id);
    if (st && st.length) {
      lines.push("", `<details><summary>过程：${st.length} 步</summary>`, "", ...st.map((x) => `- ${stepLine(x)}`), "", "</details>");
    }
  }
  return lines.join("\n") + "\n";
}

/** JSON Lines: one message per line, myco's own field names. */
export function toJsonl(messages: Message[], steps?: Map<string, ExportStep[]>): string {
  return messages.map((m) => {
    const o: Record<string, unknown> = {
      id: m.id, thread_id: m.thread_id, ...(m.project ? { project: m.project } : {}),
      from: m.from, to: m.to, type: m.type, status: m.status,
      ...(m.error ? { error: m.error } : {}), ...(m.reply_to ? { reply_to: m.reply_to } : {}),
      body: m.unreadable ? "" : m.body, ...(m.unreadable ? { unreadable: m.unreadable } : {}),
      attachments: (m.attachments || []).map((f) => ({ name: f.name, size: f.size, ...(f.content_type ? { content_type: f.content_type } : {}) })),
      created_at: m.created_at,
    };
    const st = steps?.get(m.id);
    if (st && st.length) o.steps = st;
    return JSON.stringify(o);
  }).join("\n") + (messages.length ? "\n" : "");
}

/** A file name for the export: who it was with and the day, safe on every system. */
export function exportName(withWhom: string, format: ExportFormat, now = new Date()): string {
  const p = (n: number) => String(n).padStart(2, "0");
  const day = `${now.getFullYear()}-${p(now.getMonth() + 1)}-${p(now.getDate())}`;
  const who = withWhom.replace(/[\\/:*?"<>|\u0000-\u001f]+/g, " ").trim().slice(0, 60) || "myco";
  return `和 ${who} 的对话 ${day}.${format}`;
}
