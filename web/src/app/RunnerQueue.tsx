import type { Agent, RunnerSlots } from "@/api";
import { L } from "@/lib/i18n";

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
export function runnerQueueText(
  agent: Agent | undefined,
  message: string,
): string | undefined {
  const s = currentSlots(agent);
  const q = s?.queue?.find((q) => q.message_id === message);
  return s && q
    ? L(
        `排队中：这台电脑 ${s.in_use}/${s.max_concurrent} 在忙，排第 ${q.position}`,
        `Queued: this computer has ${s.in_use}/${s.max_concurrent} slots in use, position ${q.position}`,
      )
    : undefined;
}
