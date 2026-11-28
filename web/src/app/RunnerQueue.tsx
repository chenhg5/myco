import type { Agent, RunnerSlots } from "@/api";
import { L, E } from "@/lib/i18n";

// A stopped/crashed process must not leave a task labelled queued forever.
export function currentSlots(
  agent?: Agent,
  now = Date.now(),
): RunnerSlots | undefined {
  const s = agent?.runtime?.runner_slots;
  if (
    !s ||
    !agent?.online ||
    agent?.runtime?.away ||
    now - Date.parse(s.at) > 90000 ||
    !Number.isFinite(Date.parse(s.at))
  )
    return undefined;
  return s;
}
/** The limit in force (runners up to v0.23.9 say only max_concurrent). */
const limitOf = (s: RunnerSlots) => s.limit || s.max_concurrent;

/** Whether a task waits only because its computer runs as many as it may. */
export function waitsOnLimit(agent: Agent | undefined, message: string): RunnerSlots | undefined {
  const s = currentSlots(agent);
  return s && s.queue?.some((q) => q.message_id === message) && s.in_use >= limitOf(s) ? s : undefined;
}

export function runnerQueueText(
  agent: Agent | undefined,
  message: string,
): string | undefined {
  const s = currentSlots(agent);
  const q = s?.queue?.find((q) => q.message_id === message);
  if (!s || !q) return undefined;
  const limit = limitOf(s);
  return s.in_use >= limit
    ? L(
        E`在排队：这台电脑同时最多跑 ${limit} 个，现在都在忙。它排第 ${q.position}，轮到就开始。`,
        E`Waiting its turn: this computer runs up to ${limit} at once and all are busy. It is number ${q.position} in line.`,
      )
    : L(
        E`在排队：排第 ${q.position}，马上开始。`,
        E`Waiting its turn: number ${q.position} in line, starting shortly.`,
      );
}
