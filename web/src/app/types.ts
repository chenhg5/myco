import type { HubOverview } from "@/appapi";
import type { AgentStatus, State } from "@/api";

/** One agent together with the hub it was reported by. */
export interface AgentRow {
  hub: HubOverview;
  st: AgentStatus;
}

export interface Filters {
  query: string;
  hub: string;
  machine: string;
  state: State | "all";
}
