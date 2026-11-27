// Read-only Hub contracts; see docs/coordination-statistics.md for the formulas.
export type TimingSummary = { samples: number; median_seconds: number | null; p90_seconds: number | null }
export type AgentTaskStats = {
  name: string; tasks: number; done: number; failed: number; stopped: number
  success_rate: number | null; redone: number; redone_share: number | null
  receipt_to_reply: TimingSummary; queue_wait: TimingSummary; common_task_types: string[]
}
export type AgentStatsList = {
  version: 1; since: string; until: string; scope: 'all' | 'participant_messages'; agents: AgentTaskStats[]
}
export type AgentPick = {
  name: string; score: number; available: boolean; online: boolean; queued: number; in_progress: number
  runner_slots_free: number | null; quota: 'unknown' | 'available' | 'blocked'; reason: string; stats: AgentTaskStats
}
export type AgentPickList = {
  version: 1; generated_at: string; scope: 'all' | 'participant_messages'; candidates: AgentPick[]
}
export type CoordinatorStats = {
  version: 1; name: string; since: string; until: string; scope: 'all' | 'participant_messages'
  person_tasks: number; receipt_to_reply: TimingSummary; delegated_tasks: number; queue_samples: number
  delegated_queued_share: number | null; reassigned: number; withdrawn: number; person_followups: number
  reassignment_method: string
}
