// Metadata-only coordinator contracts; message contents are intentionally absent.
export type TaskBoardItem = {
  id: string; from: string; to: string; thread_id: string; project?: string;
  status: 'queued' | 'delivered' | 'acked' | 'held'; created_at: string;
  active_at?: string; last_activity_at: string; age_ms: number;
  queued_for_ms: number; idle_ms: number; stale_after_ms: number; stale: boolean;
};
export type TaskBoard = { observed_at: string; tasks: TaskBoardItem[] };
export type TaskAlertPolicy = { project: string; stale_seconds: number };
export type TaskAlertPolicyRequest = { stale_seconds: number };
