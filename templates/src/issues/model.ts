import { t as uiText } from "../_lib/i18n";
// 任务追踪's data: a team (its name and the key its issues are numbered
// with, MYC-12), issues, projects, and comments. People and agents read and
// write the same records; an issue's id is "i" and its number, so two
// people creating one at once cannot take the same number.
import { Store, type MycoRecord } from "../_lib/myco";
import { Kit } from "../_lib/appkit";

export type Team = { title?: string; key?: string };
export type Issue = {
  n?: number; title?: string; desc?: string; status?: string; priority?: number; assignee?: string | null;
  labels?: string[]; project?: string | null; due?: string | null; estimate?: number | null; order?: number;
  by?: string; created?: string; done_at?: string | null;
};
export type Project = { name?: string; desc?: string; status?: string; lead?: string | null; target?: string | null; order?: number };
export type Comment = { issue?: string; body?: string; by?: string; at?: string };

export const team = new Store<Team & Record<string, unknown>>("team");
export const issues = new Store<Issue & Record<string, unknown>>("issues");
export const projects = new Store<Project & Record<string, unknown>>("projects");
export const comments = new Store<Comment & Record<string, unknown>>("comments");
export const kit = new Kit([team, issues, projects, comments], { quiet: true });

export type IssueRec = MycoRecord<Issue & Record<string, unknown>>;

export const STATUSES = [
  { id: "backlog", name: uiText("待规划") },
  { id: "todo", name: uiText("待办") },
  { id: "progress", name: uiText("进行中") },
  { id: "review", name: uiText("评审中") },
  { id: "done", name: uiText("已完成") },
  { id: "canceled", name: uiText("已取消") },
] as const;
export type StatusId = (typeof STATUSES)[number]["id"];
export const statusName = (s: string) => STATUSES.find((x) => x.id === s)?.name || "待办";
export const statusOf = (r: IssueRec): StatusId => (STATUSES.some((x) => x.id === r.data.status) ? r.data.status : "todo") as StatusId;
export const isClosed = (s: string) => s === "done" || s === "canceled";

/** Priorities as Linear numbers them: 1 is the most urgent, 0 is none. */
export const PRIORITIES = [
  { id: 1, name: uiText("紧急") }, { id: 2, name: uiText("高") }, { id: 3, name: uiText("中") }, { id: 4, name: uiText("低") }, { id: 0, name: uiText("无优先级") },
] as const;
export const priorityName = (p: number) => PRIORITIES.find((x) => x.id === p)?.name || "无优先级";
const prioRank = (p: number) => (p >= 1 && p <= 4 ? p : 5);

export const PROJECT_STATUSES = [
  { id: "planned", name: uiText("计划中") }, { id: "started", name: uiText("进行中") }, { id: "paused", name: uiText("暂停") }, { id: "done", name: uiText("已完成") },
] as const;

export const teamKey = () => String(team.get("main")?.data.key || "TASK").toUpperCase();
export const keyOf = (r: IssueRec) => `${teamKey()}-${Number(r.data.n) || 0}`;
export const num = (v: unknown, d: number) => (typeof v === "number" && Number.isFinite(v) ? v : d);

/** Issues in the order a list shows them: by priority, then newest first. */
export function byPriority(a: IssueRec, b: IssueRec) {
  return prioRank(num(a.data.priority, 0)) - prioRank(num(b.data.priority, 0)) || num(b.data.n, 0) - num(a.data.n, 0);
}
/** On the board: as people placed them (order), then by priority. */
export function byOrder(a: IssueRec, b: IssueRec) {
  const x = a.data.order, y = b.data.order;
  if (typeof x === "number" && typeof y === "number" && x !== y) return x - y;
  return byPriority(a, b);
}

/** Everyone who appears in the tracker, and this person first. */
export function people(me: string) {
  const s = new Set<string>();
  if (me) s.add(me);
  for (const r of issues.rows) { if (r.data.assignee) s.add(String(r.data.assignee)); if (r.data.by) s.add(String(r.data.by)); }
  for (const r of comments.rows) if (r.data.by) s.add(String(r.data.by));
  for (const r of projects.rows) if (r.data.lead) s.add(String(r.data.lead));
  return [...s];
}
export function allLabels() {
  const s = new Map<string, number>();
  for (const r of issues.rows) for (const l of r.data.labels || []) s.set(l, (s.get(l) || 0) + 1);
  return [...s.entries()].sort((a, b) => b[1] - a[1]).map(([l]) => l);
}

export const nowIso = () => new Date().toISOString();

/** A new issue with the next number; when someone took that number in the
 *  meantime, the one after it. */
export async function createIssue(d: Issue, me: string): Promise<IssueRec | null> {
  let n = issues.rows.reduce((m, r) => Math.max(m, num(r.data.n, 0)), 0) + 1;
  const same = issues.rows.filter((r) => statusOf(r) === (d.status || "todo"));
  const order = same.reduce((m, r) => Math.min(m, num(r.data.order, 0)), 0) - 1;
  for (let i = 0; i < 8; i++, n++) {
    const data: Issue = { status: "todo", priority: 0, assignee: null, labels: [], project: null, due: null, estimate: null, desc: "", ...d, n, order, by: me, created: nowIso() };
    if (isClosed(String(data.status))) data.done_at = nowIso();
    try {
      return (await issues.put(data as Issue & Record<string, unknown>, `i${n}`)) as IssueRec | null;
    } catch (e) {
      const err = e as { code?: string };
      // (a number someone else just used: drop our copy, try the next)
      const cur = issues.get(`i${n}`);
      if (err.code !== "conflict") { if (cur && cur.version === 0) issues.rows.splice(issues.rows.indexOf(cur), 1); throw e; }
    }
  }
  return null;
}

export function timeAgo(iso: string | undefined) {
  if (!iso) return "";
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return "";
  const s = Math.max(0, (Date.now() - t) / 1000);
  if (s < 60) return "刚刚";
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前`;
  if (s < 86400) return `${Math.floor(s / 3600)} 小时前`;
  if (s < 86400 * 30) return `${Math.floor(s / 86400)} 天前`;
  const d = new Date(t);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}
