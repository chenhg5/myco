/** GET console/usage: inclusive UTC dates, token fields do not overlap. */
export interface UsageRow {
  day: string; agent: string; project: string; model: string; source: "runner" | "session";
  calls: number; input_tokens: number; cache_write_tokens: number; cache_read_tokens: number; output_tokens: number; cost_usd: number;
}
export interface UsageAgent { name: string; display_name?: string; owner?: string; harness?: string; machine?: string }
export interface UsageResponse { from: string; to: string; rows: UsageRow[]; agents: UsageAgent[] }
export interface UsageTotals { total: number; input: number; cacheWrite: number; cacheRead: number; output: number; calls: number; cost: number }
export const emptyUsage = (): UsageTotals => ({ total: 0, input: 0, cacheWrite: 0, cacheRead: 0, output: 0, calls: 0, cost: 0 });
export function addUsage(sum: UsageTotals, row: UsageRow): void {
  sum.input += row.input_tokens || 0; sum.cacheWrite += row.cache_write_tokens || 0;
  sum.cacheRead += row.cache_read_tokens || 0; sum.output += row.output_tokens || 0;
  sum.total += (row.input_tokens || 0) + (row.cache_write_tokens || 0) + (row.cache_read_tokens || 0) + (row.output_tokens || 0);
  sum.calls += row.calls || 0; sum.cost += row.cost_usd || 0;
}
export function summarizeUsage(rows: UsageRow[]) {
  const total = emptyUsage();
  const days = new Map<string, UsageTotals>();
  const agents = new Map<string, UsageTotals & { models: Set<string> }>();
  const projects = new Map<string, UsageTotals & { agents: Set<string> }>();
  const models = new Map<string, UsageTotals>();
  for (const row of rows) {
    addUsage(total, row);
    const day = days.get(row.day) || emptyUsage();
    const agent = agents.get(row.agent) || { ...emptyUsage(), models: new Set<string>() };
    const project = projects.get(row.project || "") || { ...emptyUsage(), agents: new Set<string>() };
    const model = models.get(row.model || "") || emptyUsage();
    for (const group of [day, agent, project, model]) addUsage(group, row);
    if (row.model) agent.models.add(row.model);
    project.agents.add(row.agent);
    days.set(row.day, day); agents.set(row.agent, agent); projects.set(row.project || "", project); models.set(row.model || "", model);
  }
  const ranked = <T extends UsageTotals>(map: Map<string, T>) => [...map].sort((a, b) => b[1].total - a[1].total || a[0].localeCompare(b[0]));
  return { total, days, agents: ranked(agents), projects: ranked(projects), models: ranked(models) };
}
export function usageRange(days: number, now = new Date()) {
  const to = now.toISOString().slice(0, 10), start = new Date(`${to}T00:00:00Z`);
  start.setUTCDate(start.getUTCDate() - days + 1);
  return { from: start.toISOString().slice(0, 10), to };
}
export function usageDays(from: string, to: string): string[] {
  const days: string[] = [];
  for (const d = new Date(`${from}T00:00:00Z`); d.toISOString().slice(0, 10) <= to; d.setUTCDate(d.getUTCDate() + 1)) days.push(d.toISOString().slice(0, 10));
  return days;
}
export function compactUsage(n: number): string {
  for (const [size, suffix] of [[1e9, "B"], [1e6, "M"], [1e3, "K"]] as const) if (n >= size) return `${(n / size).toFixed(1).replace(/\.0$/, "")}${suffix}`;
  return String(Math.round(n));
}
