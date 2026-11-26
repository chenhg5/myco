import type { Agent, AgentStatus } from "@/api";
import { L } from "@/lib/i18n";

/** Standby is an additive wire flag; old clients continue to see offline. */
export function withPresence(st: AgentStatus): AgentStatus {
  return st.state === "offline" && st.standby && !st.agent.online ? { ...st, state: "standby" } : st;
}

export function standbyHint(): string {
  return L("待命 · 有消息会自动唤醒", "Standby · wakes automatically when a message arrives");
}

/** Pull has no wake path. Never infer automatic wake from a cloud harness. */
export function sleepingCloudSession(a: Agent): boolean {
  return !a.online && a.binding.harness === "claude-cloud" && a.delivery.mode === "pull"
    && !!a.last_seen_at && Number.isFinite(Date.parse(a.last_seen_at));
}

export function cloudSleepHint(): string {
  return L("云端会话已休眠：容器空闲后被回收，myco 消息唤不醒；在 claude.ai 打开会话即可恢复",
    "Cloud session asleep: its idle container was reclaimed and myco messages cannot wake it. Open the session at claude.ai to resume.");
}
