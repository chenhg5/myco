/** Durable Hub-side waits. Older Hubs do not expose /v1/watches. */
export interface Watch {
  id: string;
  agent: string;
  project?: string;
  task?: string;
  what: string;
  kind: "reply" | "task" | "workflow" | "command";
  target?: string;
  command?: string;
  match?: string;
  every_sec?: number;
  started_at: string;
  deadline: string;
  last_check_at?: string;
  last_result?: string;
  checks: number;
  state: "watching" | "met" | "expired" | "stopped";
  ended_at?: string;
  ended_by?: string;
  outcome?: string;
}
export interface WatchStartRequest {
  what: string;
  kind: "reply" | "task" | "workflow";
  target: string;
  project?: string;
  task?: string;
  for_sec?: number;
}
export interface WatchList { watches: Watch[] }
