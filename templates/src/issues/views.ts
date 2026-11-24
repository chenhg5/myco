// Personal shortcuts contain only view settings, never issue records.
export type View = { kind: "mine" | "all" | "active" | "backlog" | "projects" } | { kind: "project"; id: string };
export type Filters = { status: string[]; priority: number[]; assignee: string[]; labels: string[] };
export type Snapshot = { view: View; filters: Filters; query: string; layout: "list" | "board" };
export type SavedView = Snapshot & { id: string; name: string };
export const NO_FILTERS: Filters = { status: [], priority: [], assignee: [], labels: [] };
export const shortcutKey = (page: string, me: string) => `myco-issues-shortcuts:${JSON.stringify([page, me])}`;
export function readViews(raw: string | null): SavedView[] {
  try {
    const rows = JSON.parse(raw || "[]");
    if (!Array.isArray(rows)) return [];
    const ids = new Set<string>();
    return rows.slice(0, 20).filter((r): r is SavedView => {
      const strings = (a: unknown) => Array.isArray(a) && a.length <= 100 && a.every(x => typeof x === "string" && x.length <= 200);
      const f = r?.filters, v = r?.view;
      const valid = r && typeof r.id === "string" && r.id.length > 0 && r.id.length <= 100 && !ids.has(r.id)
        && typeof r.name === "string" && r.name.trim().length > 0 && r.name.length <= 40
        && typeof r.query === "string" && r.query.length <= 500
        && ["list", "board"].includes(r.layout)
        && v && (["mine", "all", "active", "backlog"].includes(v.kind) || (v.kind === "project" && typeof v.id === "string" && v.id.length <= 200))
        && f && strings(f.status) && f.status.every((s: string) => ["todo", "backlog", "progress", "review", "done", "canceled"].includes(s))
        && Array.isArray(f.priority) && f.priority.length <= 5 && f.priority.every((n: unknown) => Number.isInteger(n) && Number(n) >= 0 && Number(n) <= 4)
        && strings(f.assignee) && strings(f.labels);
      if (valid) ids.add(r.id);
      return !!valid;
    });
  } catch { return []; }
}
export function sameView(a: Snapshot, b: Snapshot): boolean {
  return a.view.kind === b.view.kind && (a.view.kind !== "project" || (b.view.kind === "project" && a.view.id === b.view.id))
    && a.query === b.query && a.layout === b.layout
    && (Object.keys(NO_FILTERS) as (keyof Filters)[]).every(k => JSON.stringify([...a.filters[k]].sort()) === JSON.stringify([...b.filters[k]].sort()));
}
