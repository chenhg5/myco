// Shared by the ordinary and SHOWCASE mock; no private usage is invented.
export function usageFixture(hub, projects, { from, to, project }) {
  const available = hub.agents.filter((a) => (!a.kind || a.kind === "agent") && a.visibility !== "private")
    .sort((a, b) => Number((b.binding?.harness || "").includes("cloud")) - Number((a.binding?.harness || "").includes("cloud")));
  if (available.filter((a) => (a.binding?.harness || "").includes("cloud")).length < 2) {
    available.splice(1, 0, { name: "usage-opus-planner", display_name: "Opus · Planning", owner: "alice", binding: { harness: "claude-cloud" } });
  }
  const agents = available.slice(0, 8).map((a) => ({ name: a.name, display_name: a.display_name, owner: a.owner || "alice", harness: a.binding?.harness, machine: hub.machines?.find((m) => m.id === a.machine_id)?.name || "Cloud" }));
  const names = ["Opus · Research", "Opus · Planning", "Codex · Backend", "Codex · App", "DeepSeek · Docs", "DeepSeek · Review", "Codex · Tests", "Sonnet · Support"];
  for (let i = agents.length; i < 8; i++) agents.push({ name: `usage-demo-${i}`, display_name: names[i], owner: "alice", harness: i < 2 ? "claude-cloud" : "codex", machine: "Cloud" });
  const models = ["claude-opus-4-5", "claude-opus-4-5", "gpt-5.1-codex", "gpt-5.1-codex", "deepseek-v3.2", "deepseek-v3.2", "gpt-5.1-codex", "claude-sonnet-4-5"];
  const projectNames = projects.map((p) => p.name).slice(0, 3);
  for (const name of ["website", "research", "infrastructure"]) if (projectNames.length < 3 && !projectNames.includes(name)) projectNames.push(name);
  const rows = [], today = new Date().toISOString().slice(0, 10), start = new Date(`${today}T00:00:00Z`);
  start.setUTCDate(start.getUTCDate() - 29);
  for (let d = 0; d < 30; d++) {
    const date = new Date(start); date.setUTCDate(date.getUTCDate() + d);
    const day = date.toISOString().slice(0, 10);
    for (let i = 0; i < agents.length; i++) {
      const p = (d + i) % 7 === 0 ? "" : projectNames[(d + i) % 3];
      if (day < from || day > to || (project && (project === "-" ? !!p : p !== project))) continue;
      const scale = (i < 2 ? 12 - i * 2 : 8 / (i + 1)) * (0.65 + ((d * 7 + i * 3) % 11) / 10);
      const input_tokens = Math.round(4200 * scale), cache_write_tokens = Math.round(1900 * scale), cache_read_tokens = Math.round(32000 * scale), output_tokens = Math.round(3500 * scale);
      rows.push({ day, agent: agents[i].name, project: p, model: models[i], source: i < 2 ? "session" : "runner", calls: Math.round(5 * scale), input_tokens, cache_write_tokens, cache_read_tokens, output_tokens, cost_usd: i === 4 || i === 5 ? 0 : Math.round((input_tokens * 5 + cache_write_tokens * 6.25 + cache_read_tokens * .5 + output_tokens * 25) / 10000) / 100 });
    }
  }
  return { from, to, rows, agents };
}
