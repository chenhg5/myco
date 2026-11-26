/** 目标 (myco goal): a goal set on a chat. The hub wakes the agent on a
 *  schedule, while it is idle, to push the goal forward until it is done,
 *  given up, or out of time or rounds. Older hubs have no /v1/goals. */
export interface Goal {
  id: string;
  agent: string;
  project?: string;
  /** where the wake-up tasks go (the message that set the goal) */
  thread?: string;
  /** the goal as written: read-only, the anchor of every round */
  objective: string;
  /** when it counts as achieved */
  done_when: string;
  /** the interval now (backs off when nothing moves) */
  every_sec: number;
  base_every_sec: number;
  max_rounds: number;
  deadline: string;
  /** rounds run so far */
  round: number;
  /** rounds in a row without progress */
  no_progress: number;
  last_summary?: string;
  next_step?: string;
  next_wake_at?: string;
  state: "active" | "paused" | "achieved" | "abandoned" | "expired" | "stopped";
  started_at: string;
  ended_at?: string;
  ended_by?: string;
  /** why it stopped (or paused) */
  outcome?: string;
  created_by: string;
  /** GET /v1/goals/{id}: its rounds, oldest first */
  rounds?: GoalRound[];
}
export interface GoalRound {
  goal: string;
  round: number;
  started_at: string;
  ended_at?: string;
  summary: string;
  next_step?: string;
  progressed: boolean;
  blocked?: boolean;
  /** the wake-up task's id */
  message?: string;
}
export interface GoalList { goals: Goal[] }
export interface GoalStartRequest {
  agent: string;
  objective: string;
  done_when: string;
  every_sec?: number;
  max_rounds?: number;
  for_sec?: number;
  thread?: string;
  project?: string;
}
export interface GoalPatchRequest { every_sec?: number; max_rounds?: number; for_sec?: number }
/** an agent's goal at work, as agent lists and the overview show it */
export interface GoalBrief { id: string; objective: string; round: number; max_rounds?: number; next_wake_at?: string; state?: Goal["state"] }

/** Detail endpoint wraps public goal metadata and persisted turns. */
export interface GoalDetail { goal: Goal; rounds: GoalRound[] }
/** Reports bind to one acknowledged task, preventing delayed retries crossing turns. */
export interface GoalReportRequest {
  message: string; summary: string; progressed: boolean; next_step?: string;
  blocked?: boolean; achieved?: boolean; abandoned?: boolean; reason?: string;
}
