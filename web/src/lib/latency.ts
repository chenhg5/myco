/** Bounded User Timing diagnostics: identifiers and clocks, never message text,
 * credentials, provider settings, or persistent user data. */
type Stage = "app_send" | "app_send_ack" | "app_trace_received" | "app_text_dom" | "app_paint_opportunity";
type Observation = { times: Map<Stage, number>; published: Set<Stage> };
const tasks = new Map<string, Observation>();
const MAX_TASKS = 64;
let marks = 0;
const keyOf = (hub: string, message: string) => `${hub}\u0000${message}`;

export const latencyPending = (hub: string, message: string) => {
  const seen = tasks.get(keyOf(hub, message));
  return !!seen && !seen.times.has("app_paint_opportunity");
};

export function latencyMark(hub: string, message: string, stage: Stage, at = performance.now()) {
  const key = keyOf(hub, message);
  let entry = tasks.get(key);
  if (!entry) {
    if (stage !== "app_send" && stage !== "app_trace_received") return;
    entry = { times: new Map(), published: new Set() };
  }
  if (entry.times.has(stage)) return;
  // A poll can deliver/render text before the original POST response returns.
  // Keep provisional observations, then bind them to the confirmed message ID.
  // Historical observations are never published without a matching fresh send.
  if (stage === "app_send") for (const [name, time] of entry.times) if (time < at) entry.times.delete(name);
  entry.times.set(stage, at);
  tasks.delete(key); tasks.set(key, entry);
  if (tasks.size > MAX_TASKS) tasks.delete(tasks.keys().next().value as string);
  if (!entry.times.has("app_send")) return;
  for (const [name, time] of [...entry.times].sort((a, b) => a[1] - b[1])) {
    if (entry.published.has(name)) continue;
    entry.published.add(name);
    try {
      if (++marks > MAX_TASKS * 5) { performance.clearMarks("myco.latency"); marks = 1; }
      performance.mark("myco.latency", { startTime: time, detail: { hub, message, stage: name, observed_at_ms: performance.timeOrigin + time } });
    } catch { /* Diagnostics must never prevent a send or render. */ }
  }
}

export function latencySend<T extends { id: string }>(hub: string, started: number, reply: Promise<T>): Promise<T> {
  return reply.then((message) => {
    latencyMark(hub, message.id, "app_send", started);
    latencyMark(hub, message.id, "app_send_ack");
    return message;
  });
}
