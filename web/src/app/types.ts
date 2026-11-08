import type { HubOverview } from "@/appapi";
import type { AgentStatus, State } from "@/api";

/** One agent together with the hub it was reported by. */
export interface AgentRow {
  hub: HubOverview;
  st: AgentStatus;
}

/** Node kinds that are shown in the UI; `machine` nodes never are. */
export type NodeKind = "agent" | "shell" | "human";

export interface Filters {
  query: string;
  hub: string;
  /** "all", "cloud" (cloud sessions), or `m:<hubId>:<machineId>`. */
  machine: string;
  state: State | "all";
  kind: NodeKind | "all";
}
